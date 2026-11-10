package mindlink

import (
	"context"
	"database/sql"
	"encoding/json"
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
		if original.OperationType != OperationMessageSend || original.ActorKind != c.ActorKind || original.ActorID != c.ActorID || original.AvatarGeneration != c.AvatarGeneration {
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
		current, err := authorizeMessage(ctx, tx, query, nil, true)
		if err != nil {
			return err
		}
		if current.native != native {
			return ErrRequestConflict
		}
		return nil
	})
}
