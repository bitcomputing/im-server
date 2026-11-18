package mindlink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

type principalLifecycle struct {
	avatar, state, issuer, operation string
	generation, policy               int64
	absent                           bool
	fresh                            bool
	binding                          []byte
}

// This row serializes projections only; it never claims the native user namespace.
func lockPrincipalLifecycle(ctx context.Context, tx *sql.Tx, c Claims, avatar string, create bool) (*principalLifecycle, error) {
	p := &principalLifecycle{}
	hash := lifecycleBinding(c)
	if create {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mindlink_principal_lifecycle(environment,tenant_id,appkey,owner_id,principal_id,avatar_id,state,avatar_generation,policy_version,projection_absent,operation_issuer,operation_id,binding_hash) VALUES(?,?,?,?,?,?,'open',0,0,FALSE,?,?,?) ON DUPLICATE KEY UPDATE principal_id=principal_id`, c.Environment, c.TenantID, c.AppKey, c.OwnerID, c.TargetID, avatar, c.Issuer, c.OperationID, hash[:]); err != nil {
			return nil, ErrUnknownResult
		}
	}
	var environment, tenant, owner string
	err := tx.QueryRowContext(ctx, `SELECT environment,tenant_id,owner_id,avatar_id,state,avatar_generation,policy_version,projection_absent,operation_issuer,operation_id,binding_hash FROM mindlink_principal_lifecycle WHERE appkey=? AND principal_id=? FOR UPDATE`, c.AppKey, c.TargetID).Scan(&environment, &tenant, &owner, &p.avatar, &p.state, &p.generation, &p.policy, &p.absent, &p.issuer, &p.operation, &p.binding)
	if errors.Is(err, sql.ErrNoRows) && !create {
		return nil, nil
	}
	if err != nil {
		return nil, ErrUnknownResult
	}
	if environment != c.Environment || tenant != c.TenantID || owner != c.OwnerID || avatar != "" && p.avatar != avatar {
		return nil, ErrScopeMismatch
	}
	if p.avatar == "" || p.state != "open" && p.state != "retired" {
		return nil, ErrNotReady
	}
	if p.issuer == "" || !isLowerHex(p.operation, 64) || len(p.binding) != 32 ||
		p.state == "open" && (p.generation != 0 || p.policy != 0 || p.absent) ||
		p.state == "retired" && (p.generation <= 0 || p.policy <= 0) {
		return nil, ErrNotReady
	}
	// The operation lock binds this creation marker; duplicate-key upserts take an exclusive row lock.
	p.fresh = create && p.state == "open" && p.issuer == c.Issuer && p.operation == c.OperationID && bytes.Equal(p.binding, hash[:])
	return p, nil
}

func lifecycleBinding(c Claims) [32]byte {
	c.IssuedAt, c.ExpiresAt, c.JTI = 0, 0, ""
	raw, _ := json.Marshal(c)
	return sha256.Sum256(raw)
}

func retirePrincipalLifecycle(ctx context.Context, tx *sql.Tx, c Claims, body *PrincipalDisableRequest, absent bool) error {
	hash := lifecycleBinding(c)
	r, err := tx.ExecContext(ctx, `UPDATE mindlink_principal_lifecycle SET state='retired',avatar_generation=?,policy_version=?,projection_absent=?,operation_issuer=?,operation_id=?,binding_hash=? WHERE appkey=? AND principal_id=? AND environment=? AND tenant_id=? AND owner_id=? AND avatar_id=? AND state='open'`, body.TargetAvatarGeneration, body.DesiredPolicyVersion, absent, c.Issuer, c.OperationID, hash[:], c.AppKey, c.TargetID, c.Environment, c.TenantID, c.OwnerID, body.AvatarID)
	if err != nil {
		return ErrUnknownResult
	}
	if n, err := r.RowsAffected(); err != nil || n != 1 {
		return ErrUnknownResult
	}
	return nil
}

// Only the fixed-source handler may read this one completed negative projection.
func authorizeRetirementOperationRead(ctx context.Context, tx *sql.Tx, query, original Claims) error {
	if original.OperationType != OperationPrincipalDisable || original.ActorKind != "system" || query.ActorKind != "system" || query.ActorID != original.ActorID {
		return authorizeOperationRead(ctx, tx, query, original)
	}
	p, err := lockPrincipalLifecycle(ctx, tx, original, "", false)
	if err != nil {
		return err
	}
	if p == nil || p.state != "retired" || !p.absent || p.issuer != original.Issuer || p.operation != original.OperationID {
		return authorizeOperationRead(ctx, tx, query, original)
	}
	hash := lifecycleBinding(original)
	if !bytes.Equal(p.binding, hash[:]) || query.ExpectedPolicyVersion != p.policy || query.DesiredPolicyVersion != p.policy || p.policy != original.DesiredPolicyVersion {
		return ErrStalePolicy
	}
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT result_json FROM mindlink_operations WHERE issuer=? AND operation_id=?`, original.Issuer, original.OperationID).Scan(&raw); err != nil {
		return ErrUnknownResult
	}
	var result OperationResult
	var desired PrincipalDisabledState
	if json.Unmarshal(raw, &result) != nil || result.State != "ready" || json.Unmarshal(result.DesiredState, &desired) != nil || desired.AvatarID != p.avatar || desired.TargetAvatarGeneration != p.generation || desired.PrincipalID != original.TargetID {
		return ErrNotReady
	}
	var read bool
	err = tx.QueryRowContext(ctx, `SELECT read_enabled FROM mindlink_owner_barriers WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=? FOR UPDATE`, query.Environment, query.TenantID, query.AppKey, query.OwnerID).Scan(&read)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ErrUnknownResult
	}
	if err == nil && !read {
		return ErrForbidden
	}
	return assertPrincipalProjectionAbsent(ctx, tx, original, p.avatar)
}

func assertPrincipalProjectionAbsent(ctx context.Context, tx *sql.Tx, c Claims, avatar string) error {
	var present int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM mindlink_principals WHERE appkey=? AND (principal_id=? OR (environment=? AND tenant_id=? AND avatar_id=?))`, c.AppKey, c.TargetID, c.Environment, c.TenantID, avatar).Scan(&present); err != nil {
		return ErrUnknownResult
	}
	if present != 0 {
		return ErrNotReady
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE app_key=? AND user_id=? AND NOT(user_type <=> 0)`, c.AppKey, c.TargetID).Scan(&present); err != nil {
		return ErrUnknownResult
	}
	if present != 0 {
		return ErrNotReady
	}
	// shortcut: old journals have no target index; index this rare recovery scan if journal growth makes it slow.
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM mindlink_operations WHERE NOT(issuer=? AND operation_id=?) AND CAST(JSON_UNQUOTE(JSON_EXTRACT(CONVERT(binding_json USING utf8mb4),'$.appkey')) AS BINARY)=? AND CAST(JSON_UNQUOTE(JSON_EXTRACT(CONVERT(binding_json USING utf8mb4),'$.target_id')) AS BINARY)=? AND JSON_UNQUOTE(JSON_EXTRACT(CONVERT(binding_json USING utf8mb4),'$.operation_type')) IN ('principals.ensure','principals.disable'))`, c.Issuer, c.OperationID, c.AppKey, c.TargetID).Scan(&present); err != nil {
		return ErrUnknownResult
	}
	if present != 0 {
		return ErrNotReady
	}
	return nil
}

func verifyPrincipalLifecycleSchema(ctx context.Context, db *sql.DB) error {
	want := map[string]string{"environment": "varbinary(6)", "tenant_id": "varbinary(128)", "appkey": "varbinary(20)", "owner_id": "varbinary(20)", "principal_id": "binary(32)", "avatar_id": "varbinary(400)", "state": "varbinary(8)", "avatar_generation": "bigint", "policy_version": "bigint", "projection_absent": "tinyint(1)", "operation_issuer": "varbinary(1024)", "operation_id": "varbinary(64)", "binding_hash": "varbinary(32)"}
	rows, err := db.QueryContext(ctx, `SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='mindlink_principal_lifecycle'`)
	if err != nil {
		return ErrUnknownResult
	}
	defer rows.Close()
	for rows.Next() {
		var name, kind, nullable string
		if rows.Scan(&name, &kind, &nullable) != nil {
			return ErrUnknownResult
		}
		if expected, ok := want[name]; ok {
			if kind != expected || nullable != "NO" {
				return ErrNotReady
			}
			delete(want, name)
		}
	}
	if rows.Err() != nil {
		return ErrUnknownResult
	}
	if len(want) != 0 {
		return ErrNotReady
	}
	indices, err := db.QueryContext(ctx, `SELECT INDEX_NAME,NON_UNIQUE,COLUMN_NAME,SUB_PART FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='mindlink_principal_lifecycle' AND INDEX_NAME IN ('PRIMARY','uniq_mindlink_lifecycle_avatar') ORDER BY INDEX_NAME,SEQ_IN_INDEX`)
	if err != nil {
		return ErrUnknownResult
	}
	defer indices.Close()
	got := map[string][]string{}
	for indices.Next() {
		var name, column string
		var nonUnique int
		var prefix sql.NullInt64
		if indices.Scan(&name, &nonUnique, &column, &prefix) != nil {
			return ErrUnknownResult
		}
		if nonUnique != 0 || prefix.Valid {
			return ErrNotReady
		}
		got[name] = append(got[name], column)
	}
	if indices.Err() != nil {
		return ErrUnknownResult
	}
	if strings.Join(got["PRIMARY"], ",") != "appkey,principal_id" || strings.Join(got["uniq_mindlink_lifecycle_avatar"], ",") != "environment,tenant_id,appkey,avatar_id" {
		return ErrNotReady
	}
	return nil
}
