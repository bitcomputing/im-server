package mindlink

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type policyConversation struct {
	kind, native, ownerIM, state string
	policy                       int64
}

func lockPolicyConversation(ctx context.Context, tx *sql.Tx, c Claims) (*policyConversation, error) {
	var environment, tenant, owner string
	var p policyConversation
	err := tx.QueryRowContext(ctx, `SELECT environment,tenant_id,owner_id,kind,native_target_id,owner_im_id,state,policy_version FROM mindlink_conversations WHERE appkey=? AND conversation_id=? FOR UPDATE`, c.AppKey, c.TargetID).Scan(&environment, &tenant, &owner, &p.kind, &p.native, &p.ownerIM, &p.state, &p.policy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotReady
	}
	if err != nil {
		return nil, ErrUnknownResult
	}
	if environment != c.Environment || tenant != c.TenantID || owner != c.OwnerID {
		return nil, ErrScopeMismatch
	}
	if p.policy <= 0 || p.state != "active" && p.state != "disabled" {
		return nil, ErrNotReady
	}
	return &p, nil
}

func (v *Verifier) transactConversationEnsure(ctx context.Context, db *sql.DB, auth *Authorization, source *DisableAuthoritySource) (*MutationResult, error) {
	if err := source.CheckConversation(ctx, v, auth); err != nil {
		return nil, err
	}
	b, err := v.ParseConversationEnsure(auth)
	if err != nil {
		return nil, err
	}
	channel := 2
	if b.Kind == "owner_avatar_dm" {
		channel = 1
	}
	return v.Transact(ctx, db, auth, func(tx *sql.Tx, c Claims, replayed bool) error {
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
		p, err := lockPolicyConversation(ctx, tx, c)
		if err != nil && !(errors.Is(err, ErrNotReady) && !replayed && b.ExpectedPolicyVersion == 0) {
			return err
		}
		if p != nil {
			if p.kind != b.Kind || p.native != b.NativeTargetID || p.ownerIM != b.OwnerIMID {
				return ErrRequestConflict
			}
			if replayed {
				var raw []byte
				if err := tx.QueryRowContext(ctx, `SELECT result_json FROM mindlink_operations WHERE issuer=? AND operation_id=?`, c.Issuer, c.OperationID).Scan(&raw); err != nil {
					return ErrUnknownResult
				}
				return v.validateOperationResult(raw, c)
			}
			if p.state != "active" {
				return ErrForbidden
			}
			if p.policy != b.ExpectedPolicyVersion {
				return ErrStalePolicy
			}
		} else {
			var count int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM mindlink_conversations WHERE appkey=? AND conversation_id=?`, c.AppKey, c.TargetID).Scan(&count); err != nil {
				return ErrUnknownResult
			}
			if count != 0 {
				return ErrNotReady
			}
		}
		var owner string
		if err := tx.QueryRowContext(ctx, `SELECT user_id FROM users WHERE app_key=? AND user_id=? FOR UPDATE`, c.AppKey, b.OwnerIMID).Scan(&owner); err != nil || owner != b.OwnerIMID {
			return ErrNotReady
		}
		var internal int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM mindlink_principals WHERE native_appkey=? AND native_principal_id=?`, c.AppKey, b.OwnerIMID).Scan(&internal); err != nil {
			return ErrUnknownResult
		}
		if internal != 0 {
			return ErrForbidden
		}
		for _, m := range b.Members {
			member := c
			member.TargetID = m.PrincipalID
			p, err := lockPolicyPrincipal(ctx, tx, member)
			if err != nil {
				return err
			}
			if p.state != "active" {
				return ErrPrincipalDisabled
			}
			if p.avatar != m.AvatarID || p.generation != m.Generation || p.policy != m.PolicyVersion {
				return ErrStalePolicy
			}
			var id string
			var kind int
			if err := tx.QueryRowContext(ctx, `SELECT user_id,user_type FROM users WHERE app_key=? AND user_id=? FOR UPDATE`, c.AppKey, m.PrincipalID).Scan(&id, &kind); err != nil || id != m.PrincipalID || kind != 1 {
				return ErrNotReady
			}
		}
		return nil
	}, func(tx *sql.Tx) (json.RawMessage, error) {
		c := auth.claims
		members, err := json.Marshal(b.Members)
		if err != nil {
			return nil, ErrUnknownResult
		}
		if b.ExpectedPolicyVersion == 0 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO mindlink_conversations(environment,tenant_id,appkey,owner_id,conversation_id,kind,channel_type,native_target_id,owner_im_id,policy_version,state,members_json) VALUES(?,?,?,?,?,?,?,?,?,?,'active',?)`, c.Environment, c.TenantID, c.AppKey, c.OwnerID, b.ConversationID, b.Kind, channel, b.NativeTargetID, b.OwnerIMID, b.DesiredPolicyVersion, members); err != nil {
				return nil, ErrRequestConflict
			}
			if channel == 2 {
				// A native collision is never an invitation to adopt an existing group.
				if _, err := tx.ExecContext(ctx, `INSERT INTO groupinfos(app_key,group_id,group_name) VALUES(?,?,'心灵传输')`, c.AppKey, b.NativeTargetID); err != nil {
					return nil, ErrRequestConflict
				}
			}
		} else {
			updated, err := tx.ExecContext(ctx, `UPDATE mindlink_conversations SET policy_version=?,members_json=? WHERE appkey=? AND conversation_id=? AND policy_version=? AND state='active'`, b.DesiredPolicyVersion, members, c.AppKey, b.ConversationID, b.ExpectedPolicyVersion)
			if err != nil {
				return nil, ErrUnknownResult
			}
			if n, err := updated.RowsAffected(); err != nil || n != 1 {
				return nil, ErrStalePolicy
			}
			if channel == 2 {
				var id string
				if err := tx.QueryRowContext(ctx, `SELECT group_id FROM groupinfos WHERE app_key=? AND group_id=? FOR UPDATE`, c.AppKey, b.NativeTargetID).Scan(&id); err != nil || id != b.NativeTargetID {
					return nil, ErrNotReady
				}
			}
		}
		if channel == 2 {
			if _, err := tx.ExecContext(ctx, `DELETE FROM groupmembers WHERE app_key=? AND group_id=?`, c.AppKey, b.NativeTargetID); err != nil {
				return nil, ErrUnknownResult
			}
			mute := 0
			if b.Kind == "avatar_pair_group" {
				mute = 1
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO groupmembers(app_key,group_id,member_id,is_mute) VALUES(?,?,?,?)`, c.AppKey, b.NativeTargetID, b.OwnerIMID, mute); err != nil {
				return nil, ErrUnknownResult
			}
			for _, m := range b.Members {
				if _, err := tx.ExecContext(ctx, `INSERT INTO groupmembers(app_key,group_id,member_id) VALUES(?,?,?)`, c.AppKey, b.NativeTargetID, m.PrincipalID); err != nil {
					return nil, ErrUnknownResult
				}
			}
		}
		desired, err := json.Marshal(ConversationActiveState{"active", b.ConversationID, b.Kind, b.NativeTargetID, b.OwnerIMID, b.Members})
		if err != nil {
			return nil, ErrUnknownResult
		}
		return v.operationReceipt(ctx, tx, c, 0, desired)
	})
}
