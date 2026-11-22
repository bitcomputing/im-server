package mindlink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (v *Verifier) parseOperationQuery(auth *Authorization) error {
	if v == nil || auth == nil || auth.verifier != v {
		return ErrForbidden
	}
	c := auth.claims
	if c.OperationType != "operations.query" || c.Method != http.MethodGet || c.ActorKind != "owner" && c.ActorKind != "system" {
		return ErrForbidden
	}
	owner, err := strconv.ParseUint(c.OwnerID, 10, 64)
	if err != nil || owner == 0 || strconv.FormatUint(owner, 10) != c.OwnerID || len(c.TenantID) > 128 || strings.TrimSpace(c.TenantID) == "" || !isASCII(c.TenantID) || c.Path != "/apigateway/mindlink/operations/"+c.OperationID || c.ExpectedPolicyVersion <= 0 || c.ExpectedPolicyVersion != c.DesiredPolicyVersion || len(auth.body) != 0 {
		return ErrInvalidRequest
	}
	return nil
}

// QueryOperation reads a committed original; it never calls a mutation effect.
// authorize must lock/check current read authority and the query policy version
// in this transaction. Its second Claims argument is the immutable mutation.
func (v *Verifier) QueryOperation(ctx context.Context, db *sql.DB, auth *Authorization, authorize func(*sql.Tx, Claims, Claims) error) (*MutationResult, error) {
	if err := v.parseOperationQuery(auth); err != nil {
		return nil, err
	}
	return v.queryOriginal(ctx, db, auth, func(tx *sql.Tx, original Claims, raw []byte) error {
		if original.OperationType == OperationStationStopped {
			return v.validateStationStopOriginal(ctx, tx, original, raw)
		}
		return v.validateOperationResult(raw, original)
	}, authorize)
}

func (v *Verifier) queryOriginal(ctx context.Context, db *sql.DB, auth *Authorization, validate func(*sql.Tx, Claims, []byte) error, authorize func(*sql.Tx, Claims, Claims) error) (*MutationResult, error) {
	if v == nil || len(v.bindings) == 0 || db == nil || auth == nil || auth.verifier != v || validate == nil || authorize == nil {
		return nil, ErrNotReady
	}
	if ctx == nil || len(auth.claims.Issuer) > 1024 {
		return nil, ErrInvalidRequest
	}
	c := auth.claims
	if !validAt(c, time.Now()) {
		return nil, ErrForbidden
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, ErrUnknownResult
	}
	defer tx.Rollback()
	var bindingHash, metadata, raw []byte
	// Preserve mutation's operation -> authority -> ticket locking order.
	if err := tx.QueryRowContext(ctx, `SELECT binding_hash,binding_json,result_json FROM mindlink_operations WHERE issuer=? AND operation_id=? FOR UPDATE`, c.Issuer, c.OperationID).Scan(&bindingHash, &metadata, &raw); err != nil {
		return nil, ErrUnknownResult
	}
	hash := sha256.Sum256(metadata)
	var original Claims
	if !bytes.Equal(hash[:], bindingHash) || DecodeStrictObject(metadata, &original, claimFields) != nil || original.IssuedAt != 0 || original.ExpiresAt != 0 || original.JTI != "" || original.Issuer != c.Issuer || original.OperationID != c.OperationID {
		return nil, ErrUnknownResult
	}
	if original.Audience != c.Audience || original.Environment != c.Environment || original.TenantID != c.TenantID || original.AppKey != c.AppKey || original.OwnerID != c.OwnerID {
		return nil, ErrScopeMismatch
	}
	if original.RequestID != c.RequestID || original.RequestDigest != c.RequestDigest || original.TargetID != c.TargetID {
		return nil, ErrRequestConflict
	}
	if err := validate(tx, original, raw); err != nil {
		return nil, err
	}
	if err := authorize(tx, c, original); err != nil {
		return nil, mutationError(err)
	}
	if !validAt(c, time.Now()) {
		return nil, ErrForbidden
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO mindlink_tickets (issuer,jti,ticket_hash,operation_id) VALUES(?,?,?,?) ON DUPLICATE KEY UPDATE jti=jti`, c.Issuer, c.JTI, auth.ticketHash[:], c.OperationID)
	if err != nil {
		return nil, ErrUnknownResult
	}
	var ticketHash []byte
	var operationID string
	if err := tx.QueryRowContext(ctx, `SELECT ticket_hash,operation_id FROM mindlink_tickets WHERE issuer=? AND jti=? FOR UPDATE`, c.Issuer, c.JTI).Scan(&ticketHash, &operationID); err != nil {
		return nil, ErrUnknownResult
	}
	if !bytes.Equal(ticketHash, auth.ticketHash[:]) || operationID != c.OperationID {
		return nil, ErrRequestConflict
	}
	if !validAt(c, time.Now()) {
		return nil, ErrForbidden
	}
	if err := tx.Commit(); err != nil {
		return nil, ErrUnknownResult
	}
	return &MutationResult{Result: append(json.RawMessage(nil), raw...), Replayed: true}, nil
}

var operationResultFields = strings.Fields("operation_id operation_type request_id request_digest environment tenant_id appkey owner_id target_id actor_kind actor_id avatar_generation expected_policy_version desired_policy_version desired_state state effective_policy_version effective_avatar_generation commit_version accepted_at committed_at failure")

func (v *Verifier) validateDisableResult(raw []byte, original Claims) error {
	if original.OperationType != OperationPrincipalDisable {
		return ErrUnknownResult
	}
	return v.validatePrincipalResult(raw, original)
}

func (v *Verifier) validatePrincipalResult(raw []byte, original Claims) error {
	if original.OperationType != OperationPrincipalEnsure && original.OperationType != OperationPrincipalDisable {
		return ErrUnknownResult
	}
	return v.validateOperationResult(raw, original)
}

func (v *Verifier) validateOperationResult(raw []byte, original Claims) error {
	var result OperationResult
	if DecodeStrictObject(raw, &result, strings.Fields("operation_id operation_type request_id request_digest environment tenant_id appkey owner_id target_id actor_kind actor_id avatar_generation expected_policy_version desired_policy_version desired_state state accepted_at")) != nil {
		return ErrUnknownResult
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != len(operationResultFields) {
		return ErrUnknownResult
	}
	for _, field := range operationResultFields {
		if _, ok := fields[field]; !ok {
			return ErrUnknownResult
		}
	}
	if result.OperationID != original.OperationID || result.OperationType != original.OperationType || result.RequestID != original.RequestID || result.RequestDigest != original.RequestDigest || result.Environment != original.Environment || result.TenantID != original.TenantID || result.AppKey != original.AppKey || result.OwnerID != original.OwnerID || result.TargetID != original.TargetID || result.ActorKind != original.ActorKind || result.ActorID != original.ActorID || result.AvatarGeneration != original.AvatarGeneration || result.ExpectedPolicyVersion != original.ExpectedPolicyVersion || result.DesiredPolicyVersion != original.DesiredPolicyVersion {
		return ErrUnknownResult
	}
	var desired PrincipalDisabledState
	if original.OperationType == OperationPrincipalEnsure {
		var active PrincipalActiveState
		if DecodeStrictObject(result.DesiredState, &active, strings.Fields("state avatar_id principal_id target_avatar_generation display_name")) != nil || active.State != "active" {
			return ErrUnknownResult
		}
		body, err := json.Marshal(PrincipalEnsureRequest{OperationID: original.OperationID, RequestID: original.RequestID, RequestDigest: original.RequestDigest, PrincipalID: active.PrincipalID, AvatarID: active.AvatarID, TargetAvatarGeneration: active.TargetAvatarGeneration, ExpectedPolicyVersion: original.ExpectedPolicyVersion, DesiredPolicyVersion: original.DesiredPolicyVersion, DisplayName: active.DisplayName})
		if err != nil {
			return ErrUnknownResult
		}
		if _, err = v.ParsePrincipalEnsure(&Authorization{claims: original, body: body, verifier: v}); err != nil {
			return ErrUnknownResult
		}
		desired.TargetAvatarGeneration = active.TargetAvatarGeneration
	} else if original.OperationType == OperationPrincipalDisable {
		if DecodeStrictObject(result.DesiredState, &desired, []string{"state", "avatar_id", "principal_id", "target_avatar_generation"}) != nil || desired.State != "disabled" {
			return ErrUnknownResult
		}
		// Recompute N1's semantic digest from the receipt, not just its field count.
		body, err := json.Marshal(PrincipalDisableRequest{original.OperationID, original.RequestID, original.RequestDigest, desired.AvatarID, desired.PrincipalID, desired.TargetAvatarGeneration, original.ExpectedPolicyVersion, original.DesiredPolicyVersion})
		if err != nil {
			return ErrUnknownResult
		}
		if _, err := v.ParsePrincipalDisable(&Authorization{claims: original, body: body, verifier: v}); err != nil {
			return ErrUnknownResult
		}
	} else if original.OperationType == OperationConversationEnsure {
		var active ConversationActiveState
		if DecodeStrictObject(result.DesiredState, &active, strings.Fields("state conversation_id kind native_target_id owner_im_id members")) != nil || active.State != "active" {
			return ErrUnknownResult
		}
		body, err := json.Marshal(ConversationEnsureRequest{OperationID: original.OperationID, RequestID: original.RequestID, RequestDigest: original.RequestDigest, ConversationID: active.ConversationID, Kind: active.Kind, NativeTargetID: active.NativeTargetID, OwnerIMID: active.OwnerIMID, ExpectedPolicyVersion: original.ExpectedPolicyVersion, DesiredPolicyVersion: original.DesiredPolicyVersion, Members: active.Members})
		if err != nil {
			return ErrUnknownResult
		}
		if _, err = v.ParseConversationEnsure(&Authorization{claims: original, body: body, verifier: v}); err != nil {
			return ErrUnknownResult
		}
	} else {
		return ErrUnknownResult
	}
	accepted, err := time.Parse(ReceiptTimeFormat, result.AcceptedAt)
	if err != nil || accepted.Format(ReceiptTimeFormat) != result.AcceptedAt {
		return ErrUnknownResult
	}
	if result.State == "ready" || result.State == "cancelled" && original.OperationType == OperationConversationEnsure {
		if result.State == "ready" && (result.EffectivePolicyVersion == nil || *result.EffectivePolicyVersion != original.DesiredPolicyVersion || result.EffectiveAvatarGeneration == nil || *result.EffectiveAvatarGeneration != desired.TargetAvatarGeneration) || result.State == "cancelled" && (result.EffectivePolicyVersion != nil || result.EffectiveAvatarGeneration != nil) || result.CommitVersion == nil || !isLowerHex(*result.CommitVersion, 64) || result.CommittedAt == nil || result.Failure != nil {
			return ErrUnknownResult
		}
		committed, err := time.Parse(ReceiptTimeFormat, *result.CommittedAt)
		if err != nil || committed.Format(ReceiptTimeFormat) != *result.CommittedAt || committed.Before(accepted) {
			return ErrUnknownResult
		}
		return nil
	}
	if result.EffectivePolicyVersion != nil || result.EffectiveAvatarGeneration != nil || result.CommitVersion != nil || result.CommittedAt != nil {
		return ErrUnknownResult
	}
	if result.State == "accepted" && result.Failure == nil {
		return nil
	}
	var failure OperationFailure
	if DecodeStrictObject(fields["failure"], &failure, []string{"reason", "retryable"}) != nil {
		return ErrUnknownResult
	}
	if result.State == "unknown" && failure.Reason == "UNKNOWN_RESULT" && failure.Retryable {
		return nil
	}
	if result.State == "failed" && !failure.Retryable {
		switch failure.Reason {
		case "INVALID_REQUEST", "SCOPE_MISMATCH", "REQUEST_CONFLICT", "STALE_POLICY", "NOT_READY", "PRINCIPAL_DISABLED", "FORBIDDEN":
			return nil
		}
	}
	return ErrUnknownResult
}
