package mindlink

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

const ConversationCancelPath = "/apigateway/mindlink/conversations/cancel"
const OperationConversationCancel = "conversation.cancel"

// Only a verified cancellation may use the original ensure's journal identity.
func (v *Verifier) parseConversationCancel(auth *Authorization) (*Authorization, *ConversationEnsureRequest, error) {
	if v == nil || auth == nil || auth.verifier != v || auth.claims.OperationType != OperationConversationCancel || auth.claims.Path != ConversationCancelPath {
		return nil, nil, ErrForbidden
	}
	original := *auth
	original.claims.OperationType, original.claims.Path = OperationConversationEnsure, ConversationEnsurePath
	b, err := v.ParseConversationEnsure(&original)
	return &original, b, err
}

func (v *Verifier) transactConversationCancel(ctx context.Context, db *sql.DB, auth *Authorization, source *DisableAuthoritySource) (*MutationResult, error) {
	original, b, err := v.parseConversationCancel(auth)
	if err != nil {
		return nil, err
	}
	if source == nil || !source.enabled {
		return nil, ErrNotReady
	}
	raw, err := source.invoke(ctx, original.claims.RequestID, "--conversation-cancel")
	if err != nil {
		return nil, err
	}
	if err = checkConversationAuthoritySnapshotFor(raw, original.claims, true); err != nil {
		return nil, err
	}
	return v.Transact(ctx, db, original, func(tx *sql.Tx, c Claims, replayed bool) error {
		var read, write bool
		err := tx.QueryRowContext(ctx, `SELECT read_enabled,write_enabled FROM mindlink_owner_barriers WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=? FOR UPDATE`, c.Environment, c.TenantID, c.AppKey, c.OwnerID).Scan(&read, &write)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotReady
		}
		if err != nil {
			return ErrUnknownResult
		}
		if replayed && !read || !replayed && !write {
			return ErrForbidden
		}
		if replayed {
			var stored []byte
			if err = tx.QueryRowContext(ctx, `SELECT result_json FROM mindlink_operations WHERE issuer=? AND operation_id=?`, c.Issuer, c.OperationID).Scan(&stored); err != nil {
				return ErrUnknownResult
			}
			var result OperationResult
			if v.validateOperationResult(stored, c) != nil || json.Unmarshal(stored, &result) != nil || result.State != "ready" && result.State != "cancelled" {
				return ErrUnknownResult
			}
		}
		return nil
	}, func(tx *sql.Tx) (json.RawMessage, error) {
		desired, err := json.Marshal(ConversationActiveState{"active", b.ConversationID, b.Kind, b.NativeTargetID, b.OwnerIMID, b.Members})
		if err != nil {
			return nil, ErrUnknownResult
		}
		raw, err := v.operationReceipt(ctx, tx, original.claims, 0, desired)
		if err != nil {
			return nil, err
		}
		var result OperationResult
		if json.Unmarshal(raw, &result) != nil {
			return nil, ErrUnknownResult
		}
		result.State, result.EffectivePolicyVersion, result.EffectiveAvatarGeneration = "cancelled", nil, nil
		raw, err = json.Marshal(result)
		if err != nil || v.validateOperationResult(raw, original.claims) != nil {
			return nil, ErrUnknownResult
		}
		return raw, nil
	})
}

// The validated original cancellation has no conversation projection to read.
func authorizeConversationCancellationRead(ctx context.Context, tx *sql.Tx, query, original Claims) (bool, error) {
	if original.OperationType != OperationConversationEnsure {
		return false, nil
	}
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT result_json FROM mindlink_operations WHERE issuer=? AND operation_id=?`, original.Issuer, original.OperationID).Scan(&raw); err != nil {
		return true, ErrUnknownResult
	}
	var result OperationResult
	if json.Unmarshal(raw, &result) != nil {
		return true, ErrUnknownResult
	}
	if result.State != "cancelled" {
		return false, nil
	}
	if query.ActorKind != "system" || query.ActorID != original.ActorID || original.ActorKind != "system" || query.AvatarGeneration != 0 || query.ExpectedPolicyVersion != original.DesiredPolicyVersion || query.DesiredPolicyVersion != original.DesiredPolicyVersion {
		return true, ErrForbidden
	}
	var read bool
	if err := tx.QueryRowContext(ctx, `SELECT read_enabled FROM mindlink_owner_barriers WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=? FOR UPDATE`, original.Environment, original.TenantID, original.AppKey, original.OwnerID).Scan(&read); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, ErrNotReady
		}
		return true, ErrUnknownResult
	}
	if !read || !validAt(query, time.Now()) {
		return true, ErrForbidden
	}
	return true, nil
}
