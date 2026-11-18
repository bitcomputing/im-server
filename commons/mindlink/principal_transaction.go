package mindlink

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Positive projection is reachable only after a fresh, independent BIZ read.
func (v *Verifier) transactPrincipalEnsure(ctx context.Context, db *sql.DB, auth *Authorization, source *DisableAuthoritySource) (*MutationResult, error) {
	if err := source.CheckPrincipal(ctx, v, auth); err != nil {
		return nil, err
	}
	body, err := v.ParsePrincipalEnsure(auth)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, ErrNotReady
	}
	var capacity int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='users' AND COLUMN_NAME='nickname' AND CHARACTER_MAXIMUM_LENGTH>=80 AND CHARACTER_SET_NAME='utf8mb4'`).Scan(&capacity); err != nil {
		return nil, ErrUnknownResult
	}
	if capacity != 1 {
		return nil, ErrNotReady
	}
	return v.Transact(ctx, db, auth, func(tx *sql.Tx, c Claims, replayed bool) error {
		// A current source can bootstrap a missing barrier, never reopen a revoked one.
		if _, err := tx.ExecContext(ctx, `INSERT INTO mindlink_owner_barriers(environment,tenant_id,appkey,owner_id,read_enabled,write_enabled) VALUES(?,?,?,?,TRUE,TRUE) ON DUPLICATE KEY UPDATE owner_id=owner_id`, c.Environment, c.TenantID, c.AppKey, c.OwnerID); err != nil {
			return ErrUnknownResult
		}
		var read, write bool
		if err := tx.QueryRowContext(ctx, `SELECT read_enabled,write_enabled FROM mindlink_owner_barriers WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=? FOR UPDATE`, c.Environment, c.TenantID, c.AppKey, c.OwnerID).Scan(&read, &write); err != nil {
			return ErrUnknownResult
		}
		if replayed && !read || !replayed && !write {
			return ErrForbidden
		}
		principal, err := lockPolicyPrincipal(ctx, tx, c)
		if errors.Is(err, ErrNotReady) && !replayed && body.ExpectedPolicyVersion == 0 {
			// Check absence explicitly: corrupt existing projections must not be replaced.
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM mindlink_principals WHERE appkey=? AND principal_id=?`, c.AppKey, c.TargetID).Scan(&n); err != nil || n != 0 {
				return ErrNotReady
			}
			return nil
		}
		if err != nil {
			return err
		}
		if principal.avatar != body.AvatarID {
			return ErrScopeMismatch
		}
		if replayed {
			var raw []byte
			if err := tx.QueryRowContext(ctx, `SELECT result_json FROM mindlink_operations WHERE issuer=? AND operation_id=?`, c.Issuer, c.OperationID).Scan(&raw); err != nil {
				return ErrUnknownResult
			}
			return v.validatePrincipalResult(raw, c)
		}
		if principal.state == "disabled" {
			return ErrPrincipalDisabled
		}
		if principal.policy != body.ExpectedPolicyVersion || body.TargetAvatarGeneration < principal.generation {
			return ErrStalePolicy
		}
		return nil
	}, func(tx *sql.Tx) (json.RawMessage, error) {
		c := auth.claims
		if body.ExpectedPolicyVersion == 0 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO mindlink_principals(environment,tenant_id,appkey,owner_id,principal_id,avatar_id,avatar_generation,policy_version,state) VALUES(?,?,?,?,?,?,?,?,'active')`, c.Environment, c.TenantID, c.AppKey, c.OwnerID, c.TargetID, body.AvatarID, body.TargetAvatarGeneration, body.DesiredPolicyVersion); err != nil {
				return nil, ErrRequestConflict
			}
			// An existing native user is a collision, not permission to take it over.
			if _, err := tx.ExecContext(ctx, `INSERT INTO users(app_key,user_id,user_type,nickname) VALUES(?,?,1,?)`, c.AppKey, c.TargetID, body.DisplayName); err != nil {
				return nil, ErrRequestConflict
			}
		} else {
			result, err := tx.ExecContext(ctx, `UPDATE mindlink_principals SET avatar_generation=?,policy_version=? WHERE appkey=? AND principal_id=? AND state='active' AND policy_version=?`, body.TargetAvatarGeneration, body.DesiredPolicyVersion, c.AppKey, c.TargetID, body.ExpectedPolicyVersion)
			if err != nil {
				return nil, ErrUnknownResult
			}
			if n, err := result.RowsAffected(); err != nil || n != 1 {
				return nil, ErrStalePolicy
			}
			var kind int
			if err := tx.QueryRowContext(ctx, `SELECT user_type FROM users WHERE app_key=? AND user_id=? FOR UPDATE`, c.AppKey, c.TargetID).Scan(&kind); err != nil || kind != 1 {
				return nil, ErrNotReady
			}
			if _, err := tx.ExecContext(ctx, `UPDATE users SET nickname=? WHERE app_key=? AND user_id=?`, body.DisplayName, c.AppKey, c.TargetID); err != nil {
				return nil, ErrUnknownResult
			}
		}
		desired, err := json.Marshal(PrincipalActiveState{"active", body.AvatarID, body.PrincipalID, body.TargetAvatarGeneration, body.DisplayName})
		if err != nil {
			return nil, ErrUnknownResult
		}
		return v.principalReceipt(ctx, tx, c, body.TargetAvatarGeneration, desired)
	})
}
