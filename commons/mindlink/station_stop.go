package mindlink

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

const (
	OperationStationStopped = "stations.stopped"
	StationStoppedPath      = "/apigateway/mindlink/stations/stopped"
	StationStopVersion      = "mindlink-station-stop-v1"
)

type StationStopRequest struct {
	Version        string `json:"version"`
	OperationID    string `json:"operation_id"`
	RequestID      string `json:"request_id"`
	RequestDigest  string `json:"request_digest"`
	ConversationID string `json:"conversation_id"`
	PolicyVersion  int64  `json:"policy_version"`
	EventID        string `json:"event_id"`
	AccessID       string `json:"access_id"`
	InstanceID     string `json:"instance_id"`
	Epoch          int64  `json:"epoch"`
	Reason         string `json:"reason"`
	Text           string `json:"text"`
}

var stationStopFields = strings.Fields("version operation_id request_id request_digest conversation_id policy_version event_id access_id instance_id epoch reason text")

func (v *Verifier) ParseStationStopped(a *Authorization) (*StationStopRequest, error) {
	if v == nil || a == nil || a.verifier != v {
		return nil, ErrForbidden
	}
	c := a.claims
	if c.OperationType != OperationStationStopped || c.Method != http.MethodPost || c.Path != StationStoppedPath || c.ActorKind != "system" || c.AvatarGeneration != 0 || !v.trustsMessageOperation(c) {
		return nil, ErrForbidden
	}
	owner, err := strconv.ParseUint(c.OwnerID, 10, 64)
	if err != nil || owner == 0 || strconv.FormatUint(owner, 10) != c.OwnerID || len(c.TenantID) > 128 || strings.TrimSpace(c.TenantID) == "" || !isASCII(c.TenantID) {
		return nil, ErrInvalidRequest
	}
	var b StationStopRequest
	if err := DecodeStrictObject(a.body, &b, stationStopFields); err != nil {
		return nil, err
	}
	if b.Version != StationStopVersion || !isLowerHex(b.OperationID, 64) || b.RequestID != b.OperationID || !isLowerHex(b.RequestDigest, 64) || !isLowerHex(b.EventID, 64) || !isLowerHex(b.ConversationID, 64) || b.PolicyVersion <= 0 || b.Epoch <= 0 || len(b.Text) == 0 || len(b.Text) > 65536 {
		return nil, ErrInvalidRequest
	}
	for _, id := range []string{b.AccessID, b.InstanceID} {
		if len(id) == 0 || len(id) > 128 || strings.TrimSpace(id) != id || !isASCII(id) {
			return nil, ErrInvalidRequest
		}
	}
	switch b.Reason {
	case "auth_failed", "account_unavailable", "server_rejected", "tls_failed":
	default:
		return nil, ErrInvalidRequest
	}
	hash := sha256.Sum256(a.body)
	if c.BodySHA256 != hex.EncodeToString(hash[:]) || b.OperationID != c.OperationID || b.RequestID != c.RequestID || b.RequestDigest != c.RequestDigest || b.ConversationID != c.TargetID || b.PolicyVersion != c.ExpectedPolicyVersion || b.PolicyVersion != c.DesiredPolicyVersion || b.OperationID != stationStopOperationID(c, b.EventID) {
		return nil, ErrRequestConflict
	}
	digest, err := StationStopDigest(c, &b)
	if err != nil || digest != b.RequestDigest {
		return nil, ErrRequestConflict
	}
	return &b, nil
}

func StationStopDigest(c Claims, b *StationStopRequest) (string, error) {
	if b == nil {
		return "", ErrInvalidRequest
	}
	raw, err := json.Marshal([]any{b.Version, OperationStationStopped, c.Environment, c.TenantID, c.AppKey, c.OwnerID, c.ActorID, b.ConversationID, b.PolicyVersion, b.EventID, b.AccessID, b.InstanceID, b.Epoch, b.Reason, b.Text})
	if err != nil {
		return "", ErrInvalidRequest
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

func stationStopOperationID(c Claims, event string) string {
	raw, _ := json.Marshal([]string{"station-stop-operation-v1", c.Environment, c.TenantID, c.OwnerID, event})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

// Workers recheck the original operation's configured permission before delivery.
func (v *Verifier) trustsMessageOperation(c Claims) bool {
	if c.OperationType != OperationMessageSend && c.OperationType != OperationStationStopped {
		return false
	}
	for _, b := range v.bindings {
		t := b.trust
		if b.mutations[c.OperationType] && t.Issuer == c.Issuer && t.Audience == c.Audience && t.Environment == c.Environment && t.TenantID == c.TenantID && t.AppKey == c.AppKey && (c.OperationType != OperationStationStopped || t.SystemActorID != "" && t.SystemActorID == c.ActorID && c.ActorKind == "system" && c.AvatarGeneration == 0) {
			return true
		}
	}
	return false
}

type StationStopResult struct {
	Version              string `json:"version"`
	OperationType        string `json:"operation_type"`
	OperationID          string `json:"operation_id"`
	RequestID            string `json:"request_id"`
	RequestDigest        string `json:"request_digest"`
	BodySHA256           string `json:"body_sha256"`
	Environment          string `json:"environment"`
	TenantID             string `json:"tenant_id"`
	AppKey               string `json:"appkey"`
	OwnerID              string `json:"owner_id"`
	ActorKind            string `json:"actor_kind"`
	ActorID              string `json:"actor_id"`
	AvatarGeneration     int64  `json:"avatar_generation"`
	ConversationID       string `json:"conversation_id"`
	PolicyVersion        int64  `json:"policy_version"`
	EventID              string `json:"event_id"`
	State                string `json:"state"`
	MessageID            string `json:"message_id"`
	MsgSeqNo             string `json:"msg_seq_no"`
	MsgTime              string `json:"msg_time"`
	NativeConversationID string `json:"native_conversation_id"`
	SenderPrincipalID    string `json:"sender_principal_id"`
}

var stationStopResultFields = strings.Fields("version operation_type operation_id request_id request_digest body_sha256 environment tenant_id appkey owner_id actor_kind actor_id avatar_generation conversation_id policy_version event_id state message_id msg_seq_no msg_time native_conversation_id sender_principal_id")

func (v *Verifier) validateStationStopResult(a *Authorization, raw []byte, n MessageNativeBinding) (*StationStopResult, error) {
	b, err := v.ParseStationStopped(a)
	if err != nil {
		return nil, err
	}
	var r StationStopResult
	if DecodeStrictObject(raw, &r, stationStopResultFields) != nil {
		return nil, ErrUnknownResult
	}
	c := a.claims
	if r.Version != StationStopVersion || r.OperationType != OperationStationStopped || r.OperationID != c.OperationID || r.RequestID != c.RequestID || r.RequestDigest != c.RequestDigest || r.BodySHA256 != c.BodySHA256 || r.Environment != c.Environment || r.TenantID != c.TenantID || r.AppKey != c.AppKey || r.OwnerID != c.OwnerID || r.ActorKind != c.ActorKind || r.ActorID != c.ActorID || r.AvatarGeneration != 0 || r.ConversationID != b.ConversationID || r.PolicyVersion != b.PolicyVersion || r.EventID != b.EventID || r.State != "persisted" || !validNativeMessageID(r.MessageID) || !positiveDecimal(r.MsgSeqNo) || !positiveDecimal(r.MsgTime) || !validMessageNativeBinding(n) || n.ChannelType != 2 || r.NativeConversationID != n.ConversationID || r.SenderPrincipalID != n.SenderPrincipalID {
		return nil, ErrUnknownResult
	}
	return &r, nil
}

// The existing owner inbox needs only native message identity, not a second receipt format.
func (v *Verifier) validateOutboxResult(a *Authorization, raw []byte, n MessageNativeBinding) (*MessageSendResult, error) {
	if a.claims.OperationType != OperationStationStopped {
		return v.ValidateMessageSendResult(a, raw, n)
	}
	r, err := v.validateStationStopResult(a, raw, n)
	if err != nil {
		return nil, err
	}
	return &MessageSendResult{MessageID: r.MessageID, MsgSeqNo: r.MsgSeqNo, MsgTime: r.MsgTime}, nil
}
