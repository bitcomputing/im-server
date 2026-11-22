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
	if original.OperationType == OperationConversationEnsure || original.OperationType == OperationStationStopped {
		if original.OperationType == OperationStationStopped && (query.ActorKind != "owner" && (query.ActorKind != "system" || query.ActorID != original.ActorID)) {
			return ErrForbidden
		}
		conversation, err := lockPolicyConversation(ctx, tx, original)
		if err != nil {
			return err
		}
		// A committed station event retains its frozen policy across later group changes.
		if original.OperationType != OperationStationStopped && (query.ExpectedPolicyVersion != conversation.policy || query.DesiredPolicyVersion != conversation.policy) {
			return ErrStalePolicy
		}
		if original.OperationType == OperationStationStopped && conversation.kind != "mindlink_group" {
			return ErrForbidden
		}
		return nil
	}
	principal, err := lockPolicyPrincipal(ctx, tx, original)
	if err != nil {
		return err
	}
	if query.ExpectedPolicyVersion != principal.policy || query.DesiredPolicyVersion != principal.policy {
		return ErrStalePolicy
	}
	return nil
}

type policyPrincipal struct {
	avatar             string
	generation, policy int64
	state              string
}

func lockPolicyPrincipal(ctx context.Context, tx *sql.Tx, c Claims) (*policyPrincipal, error) {
	var environment, tenant, owner, avatar, state string
	var generation, policy int64
	err := tx.QueryRowContext(ctx, `SELECT environment,tenant_id,owner_id,avatar_id,avatar_generation,policy_version,state FROM mindlink_principals WHERE appkey=? AND principal_id=? FOR UPDATE`, c.AppKey, c.TargetID).Scan(&environment, &tenant, &owner, &avatar, &generation, &policy, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotReady
	}
	if err != nil {
		return nil, ErrUnknownResult
	}
	if environment != c.Environment || tenant != c.TenantID || owner != c.OwnerID {
		return nil, ErrScopeMismatch
	}
	if avatar == "" || generation <= 0 || policy <= 0 || state != "active" && state != "disabled" {
		return nil, ErrNotReady
	}
	return &policyPrincipal{avatar, generation, policy, state}, nil
}
