package mindlink

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

func (v *Verifier) QueryMessageResult(ctx context.Context, db *sql.DB, auth *Authorization) (*MutationResult, error) {
	if err := v.ParseMessageResultQuery(auth); err != nil {
		return nil, err
	}
	if err := messageStorageReady(ctx, db); err != nil {
		return nil, err
	}
	var native MessageNativeBinding
	return v.queryOriginal(ctx, db, auth, func(tx *sql.Tx, original Claims, raw []byte) error {
		c := auth.claims
		if original.OperationType != OperationMessageSend || original.ActorKind != c.ActorKind || original.ActorID != c.ActorID || original.AvatarGeneration != c.AvatarGeneration || original.ExpectedPolicyVersion != c.ExpectedPolicyVersion || original.DesiredPolicyVersion != c.DesiredPolicyVersion {
			return ErrRequestConflict
		}
		var body, binding []byte
		if err := tx.QueryRowContext(ctx, `SELECT request_body,native_binding FROM mindlink_message_outbox WHERE outbox_id=? AND issuer=? AND operation_id=?`, messageOutboxID(original), original.Issuer, original.OperationID).Scan(&body, &binding); err != nil {
			return ErrUnknownResult
		}
		if json.Unmarshal(binding, &native) != nil || !validMessageNativeBinding(native) {
			return ErrUnknownResult
		}
		_, err := v.ValidateMessageSendResult(&Authorization{claims: original, body: body, verifier: v}, raw, native)
		return err
	}, func(tx *sql.Tx, query, original Claims) error {
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
		current, err := lockPolicyConversation(ctx, tx, original)
		if err != nil {
			return err
		}
		// A committed result is read evidence, not a new send grant for old members.
		if current.kind == "owner_avatar_dm" {
			sender, target := current.ownerIM, current.native
			if original.ActorKind == "avatar" {
				sender, target = target, sender
			}
			if native.ChannelType != 1 || native.SenderPrincipalID != sender || native.TargetPrincipalID != target || native.ConversationID != min(current.ownerIM, current.native)+":"+max(current.ownerIM, current.native) {
				return ErrRequestConflict
			}
		} else if (current.kind != "mindlink_group" && current.kind != "avatar_pair_group") || native.ChannelType != 2 || native.ConversationID != current.native || native.TargetPrincipalID != current.native {
			return ErrRequestConflict
		}
		return nil
	})
}
