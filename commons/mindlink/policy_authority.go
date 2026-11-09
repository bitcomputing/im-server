package mindlink

import (
	"context"
	"database/sql"
	"errors"
)

// Authority is projected separately. A signed request cannot create a grant,
// owner barrier or principal, including when an original operation is missing.
func authorizeOperationRead(ctx context.Context, tx *sql.Tx, query, original Claims) error {
	var readable bool
	err := tx.QueryRowContext(ctx, `SELECT read_enabled FROM mindlink_owner_barriers WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=? FOR UPDATE`, query.Environment, query.TenantID, query.AppKey, query.OwnerID).Scan(&readable)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotReady
	}
	if err != nil {
		return ErrUnknownResult
	}
	if !readable {
		return ErrForbidden
	}
	var environment, tenant, owner, avatar, state string
	var generation, policy int64
	err = tx.QueryRowContext(ctx, `SELECT environment,tenant_id,owner_id,avatar_id,avatar_generation,policy_version,state FROM mindlink_principals WHERE appkey=? AND principal_id=? FOR UPDATE`, query.AppKey, original.TargetID).Scan(&environment, &tenant, &owner, &avatar, &generation, &policy, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotReady
	}
	if err != nil {
		return ErrUnknownResult
	}
	if environment != query.Environment || tenant != query.TenantID || owner != query.OwnerID {
		return ErrScopeMismatch
	}
	if avatar == "" || generation <= 0 || policy <= 0 || state != "active" && state != "disabled" {
		return ErrNotReady
	}
	if query.ExpectedPolicyVersion != policy || query.DesiredPolicyVersion != policy {
		return ErrStalePolicy
	}
	return nil
}
