package mindlink

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const OperationSeenQuery = "conversation_seen.query"
const OperationSeenAdvance = "conversation_seen.advance"

type ReadRequest struct{ SeenSeq *int64 }
type ReadBinding struct {
	AppKey, RequesterID, TargetID, Kind string
	ChannelType                         int32
	Members                             []string
}
type ReadState struct {
	SeenSeq   string `json:"seen_seq"`
	LatestSeq string `json:"latest_seq"`
	HasNew    bool   `json:"has_new"`
}
type NativeRead func(context.Context, *sql.Tx, ReadBinding, *int64) (ReadState, error)
type ReadResult struct {
	Operation  OperationResult `json:"operation"`
	State      ReadState       `json:"state"`
	ObservedAt string          `json:"state_observed_at"`
}

func (v *Verifier) ParseRead(a *Authorization) (*ReadRequest, error) {
	if v == nil || a == nil || a.verifier != v {
		return nil, ErrForbidden
	}
	c := a.claims
	if c.ActorKind != "owner" || validateMessageActor(c) != nil {
		return nil, ErrForbidden
	}
	if !isLowerHex(c.TargetID, 64) || c.Path != "/apigateway/mindlink/conversations/"+c.TargetID+"/seen" || c.ExpectedPolicyVersion < 1 || c.ExpectedPolicyVersion != c.DesiredPolicyVersion {
		return nil, ErrInvalidRequest
	}
	if c.OperationType == OperationSeenQuery && c.Method == http.MethodGet && len(a.body) == 0 && c.RequestID == c.OperationID {
		return &ReadRequest{}, nil
	}
	if c.OperationType != OperationSeenAdvance || c.Method != http.MethodPut {
		return nil, ErrForbidden
	}
	var body struct {
		OperationID string `json:"operation_id"`
		RequestID   string `json:"request_id"`
		Digest      string `json:"request_digest"`
		Seq         string `json:"seen_seq"`
	}
	if DecodeStrictObject(a.body, &body, strings.Fields("operation_id request_id request_digest seen_seq")) != nil {
		return nil, ErrInvalidRequest
	}
	if body.OperationID != c.OperationID || body.RequestID != c.RequestID || body.Digest != c.RequestDigest {
		return nil, ErrRequestConflict
	}
	n, err := strconv.ParseInt(body.Seq, 10, 64)
	if err != nil || n < 0 || strconv.FormatInt(n, 10) != body.Seq {
		return nil, ErrInvalidRequest
	}
	return &ReadRequest{&n}, nil
}

func ReadDigest(c Claims, b ReadBinding, position *int64) string {
	positions := []string{}
	if position != nil {
		positions = append(positions, strconv.FormatInt(*position, 10))
	}
	raw, _ := json.Marshal([]any{"mindlink-read-r1", c.OperationType, c.Environment, c.TenantID, c.AppKey, c.OwnerID, c.TargetID, b.Kind, c.ActorKind, c.ActorID, c.AvatarGeneration, c.ExpectedPolicyVersion, c.DesiredPolicyVersion, b.RequesterID, b.TargetID, int64(b.ChannelType), "", b.Members, c.Method, c.Path, positions})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func readBinding(ctx context.Context, tx *sql.Tx, c Claims) (ReadBinding, error) {
	original := c
	original.OperationType = OperationConversationEnsure
	if err := authorizeOperationRead(ctx, tx, c, original); err != nil {
		return ReadBinding{}, err
	}
	p, err := lockPolicyConversation(ctx, tx, c)
	if err != nil {
		return ReadBinding{}, err
	}
	b := ReadBinding{AppKey: c.AppKey, RequesterID: p.ownerIM, TargetID: p.native, Kind: p.kind, ChannelType: 2, Members: []string{p.ownerIM}}
	switch p.kind {
	case "owner_avatar_dm":
		b.ChannelType = 1
	case "mindlink_group", "avatar_pair_group":
	default:
		return b, ErrNotReady
	}
	var raw []byte
	if err = tx.QueryRowContext(ctx, "SELECT members_json FROM mindlink_conversations WHERE appkey=? AND conversation_id=?", c.AppKey, c.TargetID).Scan(&raw); err != nil {
		return b, ErrUnknownResult
	}
	var members []ConversationMember
	if json.Unmarshal(raw, &members) != nil || members == nil || b.RequesterID == "" || b.TargetID == "" {
		return b, ErrNotReady
	}
	for _, m := range members {
		if m.PrincipalID == "" {
			return b, ErrNotReady
		}
		b.Members = append(b.Members, m.PrincipalID)
	}
	sort.Strings(b.Members)
	for i := 1; i < len(b.Members); i++ {
		if b.Members[i-1] == b.Members[i] {
			return b, ErrNotReady
		}
	}
	return b, nil
}

func validReadState(s ReadState) bool {
	seen, e1 := strconv.ParseInt(s.SeenSeq, 10, 64)
	latest, e2 := strconv.ParseInt(s.LatestSeq, 10, 64)
	return e1 == nil && e2 == nil && seen >= 0 && latest >= 0 && strconv.FormatInt(seen, 10) == s.SeenSeq && strconv.FormatInt(latest, 10) == s.LatestSeq && s.HasNew == (latest > seen)
}

func (v *Verifier) Read(ctx context.Context, db *sql.DB, a *Authorization, native NativeRead) (*MutationResult, error) {
	request, err := v.ParseRead(a)
	if err != nil {
		return nil, err
	}
	if native == nil {
		return nil, ErrNotReady
	}
	var binding ReadBinding
	var refreshed *ReadState
	result, err := v.transactBound(ctx, db, a, func(tx *sql.Tx, c Claims, replayed bool) error {
		binding, err = readBinding(ctx, tx, c)
		if err != nil {
			return err
		}
		if request.SeenSeq != nil && !replayed {
			var writable bool
			if err = tx.QueryRowContext(ctx, "SELECT write_enabled FROM mindlink_owner_barriers WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=? FOR UPDATE", c.Environment, c.TenantID, c.AppKey, c.OwnerID).Scan(&writable); err != nil {
				return ErrUnknownResult
			}
			if !writable {
				return ErrForbidden
			}
		}
		if ReadDigest(c, binding, request.SeenSeq) != c.RequestDigest {
			return ErrRequestConflict
		}
		if replayed {
			var raw []byte
			if err = tx.QueryRowContext(ctx, "SELECT result_json FROM mindlink_operations WHERE issuer=? AND operation_id=?", c.Issuer, c.OperationID).Scan(&raw); err != nil {
				return ErrUnknownResult
			}
			if err = validateReadResult(raw, c, request); err != nil {
				return err
			}
			if request.SeenSeq != nil {
				state, e := native(ctx, tx, binding, nil)
				if e != nil || !validReadState(state) {
					return ErrUnknownResult
				}
				refreshed = &state
			}
			return nil
		}
		return nil
	}, func(tx *sql.Tx) (json.RawMessage, error) {
		state, err := native(ctx, tx, binding, request.SeenSeq)
		if err != nil {
			return nil, err
		}
		if !validReadState(state) {
			return nil, ErrUnknownResult
		}
		if request.SeenSeq != nil {
			seen, _ := strconv.ParseInt(state.SeenSeq, 10, 64)
			if seen < *request.SeenSeq {
				return nil, ErrUnknownResult
			}
		}
		c := a.claims
		var created string
		if err = tx.QueryRowContext(ctx, "SELECT DATE_FORMAT(created_time,'%Y-%m-%d %H:%i:%s.%f') FROM mindlink_operations WHERE issuer=? AND operation_id=?", c.Issuer, c.OperationID).Scan(&created); err != nil {
			return nil, ErrUnknownResult
		}
		accepted, err := time.Parse("2006-01-02 15:04:05.999999", created)
		if err != nil {
			return nil, ErrUnknownResult
		}
		var token [32]byte
		if _, err = rand.Read(token[:]); err != nil {
			return nil, ErrUnknownResult
		}
		commit := hex.EncodeToString(token[:])
		now := time.Now().UTC().Format(ReceiptTimeFormat)
		desired := json.RawMessage(`{}`)
		if request.SeenSeq != nil {
			desired, _ = json.Marshal(map[string]string{"seen_seq": strconv.FormatInt(*request.SeenSeq, 10)})
		}
		operation := OperationResult{OperationID: c.OperationID, OperationType: c.OperationType, RequestID: c.RequestID, RequestDigest: c.RequestDigest, Environment: c.Environment, TenantID: c.TenantID, AppKey: c.AppKey, OwnerID: c.OwnerID, TargetID: c.TargetID, ActorKind: c.ActorKind, ActorID: c.ActorID, AvatarGeneration: c.AvatarGeneration, ExpectedPolicyVersion: c.ExpectedPolicyVersion, DesiredPolicyVersion: c.DesiredPolicyVersion, DesiredState: desired, State: "ready", EffectivePolicyVersion: &c.DesiredPolicyVersion, CommitVersion: &commit, AcceptedAt: accepted.Format(ReceiptTimeFormat), CommittedAt: &now}
		raw, err := json.Marshal(ReadResult{operation, state, now})
		if err != nil {
			return nil, err
		}
		return raw, validateReadResult(raw, c, request)
	})
	if err != nil {
		return nil, err
	}
	if refreshed != nil {
		var read ReadResult
		if json.Unmarshal(result.Result, &read) != nil {
			return nil, ErrUnknownResult
		}
		read.State = *refreshed
		result.Result, err = json.Marshal(read)
		if err != nil {
			return nil, ErrUnknownResult
		}
	}
	return result, nil
}

func validateReadResult(raw []byte, c Claims, request *ReadRequest) error {
	var r ReadResult
	if DecodeStrictObject(raw, &r, strings.Fields("operation state state_observed_at")) != nil || !validReadState(r.State) {
		return ErrUnknownResult
	}
	var outer map[string]json.RawMessage
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &outer) != nil || json.Unmarshal(outer["operation"], &fields) != nil || len(fields) != len(operationResultFields) {
		return ErrUnknownResult
	}
	if DecodeStrictObject(outer["state"], &r.State, strings.Fields("seen_seq latest_seq has_new")) != nil {
		return ErrUnknownResult
	}
	for _, name := range operationResultFields {
		if _, ok := fields[name]; !ok {
			return ErrUnknownResult
		}
	}
	for _, name := range strings.Fields("operation_id operation_type request_id request_digest environment tenant_id appkey owner_id target_id actor_kind actor_id avatar_generation expected_policy_version desired_policy_version desired_state state effective_policy_version commit_version accepted_at committed_at") {
		if string(fields[name]) == "null" {
			return ErrUnknownResult
		}
	}
	o := r.Operation
	if o.OperationID != c.OperationID || o.OperationType != c.OperationType || o.RequestID != c.RequestID || o.RequestDigest != c.RequestDigest || o.Environment != c.Environment || o.TenantID != c.TenantID || o.AppKey != c.AppKey || o.OwnerID != c.OwnerID || o.TargetID != c.TargetID || o.ActorKind != c.ActorKind || o.ActorID != c.ActorID || o.AvatarGeneration != 0 || o.ExpectedPolicyVersion != c.ExpectedPolicyVersion || o.DesiredPolicyVersion != c.DesiredPolicyVersion || o.State != "ready" || o.EffectivePolicyVersion == nil || *o.EffectivePolicyVersion != c.DesiredPolicyVersion || o.EffectiveAvatarGeneration != nil || o.CommitVersion == nil || !isLowerHex(*o.CommitVersion, 64) || o.CommittedAt == nil || *o.CommittedAt != r.ObservedAt || o.Failure != nil {
		return ErrUnknownResult
	}
	observed, e1 := time.Parse(ReceiptTimeFormat, r.ObservedAt)
	accepted, e2 := time.Parse(ReceiptTimeFormat, o.AcceptedAt)
	if e1 != nil || e2 != nil || observed.Before(accepted) || observed.Format(ReceiptTimeFormat) != r.ObservedAt || accepted.Format(ReceiptTimeFormat) != o.AcceptedAt {
		return ErrUnknownResult
	}
	want := `{}`
	if request.SeenSeq != nil {
		raw, _ := json.Marshal(map[string]string{"seen_seq": strconv.FormatInt(*request.SeenSeq, 10)})
		want = string(raw)
		seen, _ := strconv.ParseInt(r.State.SeenSeq, 10, 64)
		if seen < *request.SeenSeq {
			return ErrUnknownResult
		}
	}
	if string(o.DesiredState) != want {
		return ErrUnknownResult
	}
	return nil
}
