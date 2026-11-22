package mindlink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
)

// SignalOwnerInbox sends a payload-free synchronization hint. A routing receipt
// is not a client delivery receipt; messages remain in the durable inbox.
func (h *PolicyHTTP) SignalOwnerInbox(ctx context.Context, id string, signal func(context.Context, string, string, int64) error) error {
	if h == nil || h.db == nil || h.verifier == nil || ctx == nil || signal == nil {
		return ErrNotReady
	}
	if !isLowerHex(id, 64) {
		return ErrInvalidRequest
	}
	tx, err := h.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var issuer, operation, state string
	var body, nativeRaw, receiptRaw, binding, hash []byte
	err = tx.QueryRowContext(ctx, `SELECT q.issuer,q.operation_id,q.state,q.request_body,q.native_binding,o.result_json,o.binding_json,o.binding_hash FROM mindlink_message_outbox q JOIN mindlink_operations o ON o.issuer=q.issuer AND o.operation_id=q.operation_id WHERE q.outbox_id=? FOR UPDATE`, id).Scan(&issuer, &operation, &state, &body, &nativeRaw, &receiptRaw, &binding, &hash)
	if err != nil {
		return err
	}
	if state == "signalled" {
		return nil
	}
	if state != "inboxed" && state != "quiet" {
		return ErrNotReady
	}
	var c Claims
	var native MessageNativeBinding
	digest := sha256.Sum256(binding)
	if !bytes.Equal(digest[:], hash) || json.Unmarshal(binding, &c) != nil || json.Unmarshal(nativeRaw, &native) != nil || c.Issuer != issuer || c.OperationID != operation || messageOutboxID(c) != id {
		return ErrUnknownResult
	}
	if !h.verifier.trustsMessageOperation(c) {
		return ErrForbidden
	}
	receipt, err := h.verifier.validateOutboxResult(&Authorization{verifier: h.verifier, claims: c, body: body}, receiptRaw, native)
	if err != nil {
		return err
	}
	var readable bool
	if err = tx.QueryRowContext(ctx, `SELECT read_enabled FROM mindlink_owner_barriers WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=? FOR UPDATE`, c.Environment, c.TenantID, c.AppKey, c.OwnerID).Scan(&readable); err != nil {
		return err
	}
	if !readable {
		return ErrForbidden
	}
	p, err := lockPolicyConversation(ctx, tx, c)
	if err != nil {
		return err
	}
	if c.OperationType == OperationStationStopped {
		if p.kind != "mindlink_group" || native.SenderPrincipalID != p.ownerIM {
			return ErrUnknownResult
		}
		if err := h.verifier.validateStationStopOriginal(ctx, tx, c, receiptRaw); err != nil {
			return err
		}
	}
	box := "inboxmsgs"
	if native.SenderPrincipalID == p.ownerIM {
		box = "sendboxmsgs"
	}
	var position sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT MAX(send_time) FROM `+box+` WHERE app_key=? AND BINARY user_id=? AND BINARY target_id=? AND BINARY msg_id=? AND channel_type=?`, c.AppKey, p.ownerIM, p.native, receipt.MessageID, native.ChannelType).Scan(&position); err != nil {
		return err
	}
	if !position.Valid || position.Int64 <= 0 {
		return ErrNotReady
	}
	if p.kind != "avatar_pair_group" && p.kind != "mindlink_group" && p.kind != "owner_avatar_dm" {
		return ErrNotReady
	}
	if state == "quiet" && p.kind != "avatar_pair_group" {
		return ErrUnknownResult
	}
	// Silent pairs still need SDK synchronization; this hint does not push or count.
	if err = signal(ctx, c.AppKey, p.ownerIM, position.Int64); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE mindlink_message_outbox SET state='signalled' WHERE outbox_id=? AND state IN ('inboxed','quiet')`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (h *PolicyHTTP) DrainOwnerNotifications(ctx context.Context, after string, signal func(context.Context, string, string, int64) error) (string, error) {
	if h == nil || h.db == nil || h.verifier == nil || ctx == nil || signal == nil {
		return after, ErrNotReady
	}
	if after != "" && !isLowerHex(after, 64) {
		return after, ErrInvalidRequest
	}
	rows, err := h.db.QueryContext(ctx, `SELECT outbox_id FROM mindlink_message_outbox WHERE state IN ('inboxed','quiet') AND outbox_id>? ORDER BY outbox_id LIMIT 32`, after)
	if err != nil {
		return after, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return after, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return after, err
	}
	if len(ids) == 0 {
		return "", nil
	}
	var failed error
	for _, id := range ids {
		if err = h.SignalOwnerInbox(ctx, id, signal); err != nil {
			failed = ErrUnknownResult
		}
	}
	return ids[len(ids)-1], failed
}
