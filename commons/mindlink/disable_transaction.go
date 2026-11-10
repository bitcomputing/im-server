package mindlink

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

// The HTTP caller checks the business retirement source before this restrictive
// transaction; the current IM owner/principal authority is locked again below.
func (v *Verifier) transactPrincipalDisable(ctx context.Context, db *sql.DB, auth *Authorization) (*MutationResult, error) {
	body, err := v.ParsePrincipalDisable(auth)
	if err != nil {
		return nil, err
	}
	var generation int64
	return v.Transact(ctx, db, auth, func(tx *sql.Tx, c Claims, replayed bool) error {
		var readable, writable bool
		err := tx.QueryRowContext(ctx, `SELECT read_enabled,write_enabled FROM mindlink_owner_barriers WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=? FOR UPDATE`, c.Environment, c.TenantID, c.AppKey, c.OwnerID).Scan(&readable, &writable)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotReady
		}
		if err != nil {
			return ErrUnknownResult
		}
		if replayed && !readable || !replayed && !writable {
			return ErrForbidden
		}
		principal, err := lockPolicyPrincipal(ctx, tx, c)
		if err != nil {
			return err
		}
		if principal.avatar != body.AvatarID {
			return ErrScopeMismatch
		}
		if replayed {
			var raw []byte
			if err := tx.QueryRowContext(ctx, "SELECT result_json FROM mindlink_operations WHERE issuer=? AND operation_id=?", c.Issuer, c.OperationID).Scan(&raw); err != nil {
				return ErrUnknownResult
			}
			if err := v.validateDisableResult(raw, c); err != nil {
				return err
			}
			var result OperationResult
			if json.Unmarshal(raw, &result) != nil || result.State != "ready" {
				return ErrUnknownResult
			}
			return nil
		}
		if principal.state == "disabled" {
			return ErrPrincipalDisabled
		}
		if principal.policy != body.ExpectedPolicyVersion || body.TargetAvatarGeneration < principal.generation {
			return ErrStalePolicy
		}
		generation = principal.generation
		return nil
	}, func(tx *sql.Tx) (json.RawMessage, error) {
		c := auth.claims
		updated, err := tx.ExecContext(ctx, `UPDATE mindlink_principals SET state='disabled',avatar_generation=?,policy_version=? WHERE appkey=? AND principal_id=? AND environment=? AND tenant_id=? AND owner_id=? AND avatar_id=? AND state='active' AND avatar_generation=? AND policy_version=?`, body.TargetAvatarGeneration, body.DesiredPolicyVersion, c.AppKey, body.PrincipalID, c.Environment, c.TenantID, c.OwnerID, body.AvatarID, generation, body.ExpectedPolicyVersion)
		if err != nil {
			return nil, ErrUnknownResult
		}
		if rows, err := updated.RowsAffected(); err != nil || rows != 1 {
			return nil, ErrUnknownResult
		}
		var created string
		if err := tx.QueryRowContext(ctx, "SELECT DATE_FORMAT(created_time,'%Y-%m-%d %H:%i:%s.%f') FROM mindlink_operations WHERE issuer=? AND operation_id=?", c.Issuer, c.OperationID).Scan(&created); err != nil {
			return nil, ErrUnknownResult
		}
		accepted, err := time.Parse("2006-01-02 15:04:05.999999", created)
		if err != nil {
			return nil, ErrUnknownResult
		}
		var token [32]byte
		if _, err := rand.Read(token[:]); err != nil {
			return nil, ErrUnknownResult
		}
		commitVersion := hex.EncodeToString(token[:])
		committed := time.Now().UTC().Format(ReceiptTimeFormat)
		desired, err := json.Marshal(PrincipalDisabledState{"disabled", body.AvatarID, body.PrincipalID, body.TargetAvatarGeneration})
		if err != nil {
			return nil, ErrUnknownResult
		}
		raw, err := json.Marshal(OperationResult{
			OperationID: c.OperationID, OperationType: c.OperationType, RequestID: c.RequestID, RequestDigest: c.RequestDigest,
			Environment: c.Environment, TenantID: c.TenantID, AppKey: c.AppKey, OwnerID: c.OwnerID, TargetID: c.TargetID,
			ActorKind: c.ActorKind, ActorID: c.ActorID, AvatarGeneration: c.AvatarGeneration,
			ExpectedPolicyVersion: c.ExpectedPolicyVersion, DesiredPolicyVersion: c.DesiredPolicyVersion,
			DesiredState: desired, State: "ready", EffectivePolicyVersion: &body.DesiredPolicyVersion, EffectiveAvatarGeneration: &body.TargetAvatarGeneration,
			CommitVersion: &commitVersion, AcceptedAt: accepted.Format(ReceiptTimeFormat), CommittedAt: &committed,
		})
		if err != nil || v.validateDisableResult(raw, c) != nil {
			return nil, ErrUnknownResult
		}
		return raw, nil
	})
}
