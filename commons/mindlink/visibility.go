package mindlink

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

const OperationOwnerDMVisibility = "owner_dm.visibility.set"

func visibilityDigest(c Claims, hidden bool) string {
	raw, _ := json.Marshal([]any{"owner-dm-visibility-v1", c.Environment, c.TenantID, c.AppKey, c.OwnerID, c.TargetID, c.ExpectedPolicyVersion, c.RequestID, hidden})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func parseOwnerDMVisibility(c Claims, raw []byte) (bool, error) {
	if c.OperationType != OperationOwnerDMVisibility || c.ActorKind != "owner" || validateMessageActor(c) != nil {
		return false, ErrForbidden
	}
	if c.Method != http.MethodPut || !isLowerHex(c.TargetID, 64) || c.Path != "/apigateway/mindlink/conversations/"+c.TargetID+"/visibility" || c.ExpectedPolicyVersion < 1 || c.DesiredPolicyVersion != c.ExpectedPolicyVersion {
		return false, ErrInvalidRequest
	}
	var body struct {
		Hidden *bool `json:"hidden"`
	}
	if DecodeStrictObject(raw, &body, strings.Fields("hidden")) != nil || body.Hidden == nil {
		return false, ErrInvalidRequest
	}
	if visibilityDigest(c, *body.Hidden) != c.RequestDigest {
		return false, ErrRequestConflict
	}
	return *body.Hidden, nil
}

func (v *Verifier) setOwnerDMVisibility(ctx context.Context, db *sql.DB, a *Authorization) (*MutationResult, error) {
	if v == nil || a == nil || a.verifier != v {
		return nil, ErrForbidden
	}
	c := a.claims
	hidden, err := parseOwnerDMVisibility(c, a.body)
	if err != nil {
		return nil, err
	}
	var b ReadBinding
	current := hidden
	result, err := v.Transact(ctx, db, a, func(tx *sql.Tx, c Claims, replay bool) error {
		b, err = readBinding(ctx, tx, c)
		if err != nil {
			return err
		}
		if b.Kind != "owner_avatar_dm" || b.ChannelType != 1 {
			return ErrForbidden
		}
		if replay {
			if err := tx.QueryRowContext(ctx, "SELECT is_deleted FROM conversations WHERE app_key=? AND BINARY user_id=BINARY ? AND BINARY target_id=BINARY ? AND channel_type=1 AND sub_channel='' FOR UPDATE", b.AppKey, b.RequesterID, b.TargetID).Scan(&current); err != nil {
				return ErrUnknownResult
			}
			return nil
		}
		var writable, ready bool
		var version int64
		err := tx.QueryRowContext(ctx, "SELECT b.write_enabled,b.list_projection_ready,m.policy_version FROM mindlink_owner_barriers b JOIN mindlink_conversations m ON m.environment=b.environment AND m.tenant_id=b.tenant_id AND m.appkey=b.appkey AND m.owner_id=b.owner_id WHERE b.environment=? AND b.tenant_id=? AND b.appkey=? AND b.owner_id=? AND m.conversation_id=? FOR UPDATE", c.Environment, c.TenantID, c.AppKey, c.OwnerID, c.TargetID).Scan(&writable, &ready, &version)
		if err != nil {
			return ErrUnknownResult
		}
		if !ready || !writable {
			return ErrNotReady
		}
		if version != c.ExpectedPolicyVersion {
			return ErrStalePolicy
		}
		return nil
	}, func(tx *sql.Tx) (json.RawMessage, error) {
		const where = " WHERE app_key=? AND BINARY user_id=BINARY ? AND BINARY target_id=BINARY ? AND channel_type=1 AND sub_channel=''"
		var prev bool
		if err := tx.QueryRowContext(ctx, "SELECT is_deleted FROM conversations"+where+" FOR UPDATE", b.AppKey, b.RequesterID, b.TargetID).Scan(&prev); err == sql.ErrNoRows {
			return nil, ErrNotReady
		} else if err != nil {
			return nil, ErrUnknownResult
		}
		if _, err := tx.ExecContext(ctx, "UPDATE conversations SET is_deleted=?,sync_time=GREATEST(sync_time+1,CAST(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(3))*1000 AS SIGNED))"+where, hidden, b.AppKey, b.RequesterID, b.TargetID); err != nil {
			return nil, ErrUnknownResult
		}
		return json.Marshal(map[string]any{"operation_id": c.OperationID, "request_id": c.RequestID, "request_digest": c.RequestDigest, "conversation_id": c.TargetID, "hidden": hidden})
	})
	if err != nil {
		return nil, err
	}
	var receipt map[string]json.RawMessage
	if json.Unmarshal(result.Result, &receipt) != nil || len(receipt) != 5 {
		return nil, ErrUnknownResult
	}
	receipt["current_hidden"], _ = json.Marshal(current)
	result.Result, err = json.Marshal(receipt)
	return result, err

}

const OwnerDMHiddenPath = "/apigateway/mindlink/visibility/hidden"
const OperationOwnerDMHidden = "owner_dm.visibility.list"

func (v *Verifier) hiddenOwnerDMs(ctx context.Context, db *sql.DB, a *Authorization) ([]string, error) {
	if v == nil || a == nil || a.verifier != v {
		return nil, ErrForbidden
	}
	c := a.claims
	if c.Method != http.MethodGet || c.Path != OwnerDMHiddenPath || c.OperationType != OperationOwnerDMHidden || c.ActorKind != "owner" || validateMessageActor(c) != nil || c.TargetID != c.OwnerID {
		return nil, ErrForbidden
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, ErrUnknownResult
	}
	defer tx.Rollback()
	var enabled bool
	// Legacy list reads still need native visibility after SDK list projection is disabled.
	if err = tx.QueryRowContext(ctx, "SELECT read_enabled FROM mindlink_owner_barriers WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=?", c.Environment, c.TenantID, c.AppKey, c.OwnerID).Scan(&enabled); err != nil || !enabled {
		return nil, ErrNotReady
	}
	rows, err := tx.QueryContext(ctx, `SELECT m.conversation_id FROM mindlink_conversations m JOIN conversations i ON i.app_key=m.native_appkey AND BINARY i.user_id=BINARY m.owner_im_id AND BINARY i.target_id=BINARY m.native_target_id AND i.channel_type=1 AND i.sub_channel='' WHERE m.environment=? AND m.tenant_id=? AND m.appkey=? AND m.owner_id=? AND m.kind='owner_avatar_dm' AND i.is_deleted<>0 ORDER BY m.conversation_id LIMIT 4097`, c.Environment, c.TenantID, c.AppKey, c.OwnerID)
	if err != nil {
		return nil, ErrUnknownResult
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil || !isLowerHex(id, 64) || len(ids) >= 4096 {
			return nil, ErrUnknownResult
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, ErrUnknownResult
	}
	return ids, nil
}
