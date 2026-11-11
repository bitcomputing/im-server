package mindlink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"im-server/commons/pbdefines/pbobjs"
)

// MaterializeOwnerInbox projects a committed message into the owner's native
// inbox. It does not grant SDK read access, deliver a notification or wake agents.
// The outbox state, inbox and unread counter commit together, so retrying cannot
// count twice. Agent execution/inform delivery remains in the business outbox.
func (v *Verifier) MaterializeOwnerInbox(ctx context.Context, db *sql.DB, id string) (bool, error) {
	if v == nil || db == nil || ctx == nil {
		return false, ErrNotReady
	}
	if !isLowerHex(id, 64) {
		return false, ErrInvalidRequest
	}
	if err := messageStorageReady(ctx, db); err != nil {
		return false, err
	}
	var tables int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('conversations','inboxmsgs','sendboxmsgs') AND ENGINE='InnoDB'`).Scan(&tables); err != nil || tables != 3 {
		return false, ErrNotReady
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return false, ErrUnknownResult
	}
	defer tx.Rollback()
	var issuer, operation, state string
	var body, nativeRaw, payload, recipientsRaw []byte
	err = tx.QueryRowContext(ctx, `SELECT issuer,operation_id,state,request_body,native_binding,payload,recipients_json FROM mindlink_message_outbox WHERE outbox_id=? FOR UPDATE`, id).Scan(&issuer, &operation, &state, &body, &nativeRaw, &payload, &recipientsRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotReady
	}
	if err != nil {
		return false, ErrUnknownResult
	}
	if state == "inboxed" || state == "signalled" || state == "quiet" {
		return false, nil
	}
	if state != "pending" {
		return false, ErrNotReady
	}
	var binding, hash, receiptRaw []byte
	if err = tx.QueryRowContext(ctx, `SELECT binding_json,binding_hash,result_json FROM mindlink_operations WHERE issuer=? AND operation_id=?`, issuer, operation).Scan(&binding, &hash, &receiptRaw); err != nil {
		return false, ErrUnknownResult
	}
	digest := sha256.Sum256(binding)
	var c Claims
	var native MessageNativeBinding
	var recipients []string
	if !bytes.Equal(digest[:], hash) || json.Unmarshal(binding, &c) != nil || json.Unmarshal(nativeRaw, &native) != nil || json.Unmarshal(recipientsRaw, &recipients) != nil || c.Issuer != issuer || c.OperationID != operation || messageOutboxID(c) != id {
		return false, ErrUnknownResult
	}
	trusted := false
	for _, b := range v.bindings {
		t := b.trust
		if b.mutations[OperationMessageSend] && t.Issuer == c.Issuer && t.Audience == c.Audience && t.Environment == c.Environment && t.TenantID == c.TenantID && t.AppKey == c.AppKey {
			trusted = true
			break
		}
	}
	if !trusted {
		return false, ErrForbidden
	}
	auth := &Authorization{verifier: v, claims: c, body: body}
	receipt, err := v.ValidateMessageSendResult(auth, receiptRaw, native)
	if err != nil {
		return false, err
	}
	var readable bool
	if err = tx.QueryRowContext(ctx, `SELECT read_enabled FROM mindlink_owner_barriers WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=? FOR UPDATE`, c.Environment, c.TenantID, c.AppKey, c.OwnerID).Scan(&readable); err != nil {
		return false, ErrUnknownResult
	}
	if !readable {
		return false, ErrForbidden
	}
	p, err := lockPolicyConversation(ctx, tx, c)
	if err != nil {
		return false, err
	}
	ownerIncluded := false
	for i, r := range recipients {
		if r == "" || i > 0 && recipients[i-1] >= r {
			return false, ErrUnknownResult
		}
		ownerIncluded = ownerIncluded || r == p.ownerIM
	}
	if !ownerIncluded || p.ownerIM == "" || p.native == "" {
		return false, ErrUnknownResult
	}
	table := "g_hismsgs"
	expectedNative := p.native
	expectedFlags := int32(10)
	switch p.kind {
	case "owner_avatar_dm":
		table = "p_hismsgs"
		expectedNative = min(p.ownerIM, p.native) + ":" + max(p.ownerIM, p.native)
		if native.ChannelType != 1 || !(native.SenderPrincipalID == p.ownerIM && native.TargetPrincipalID == p.native || native.SenderPrincipalID == p.native && native.TargetPrincipalID == p.ownerIM) {
			return false, ErrUnknownResult
		}
	case "avatar_pair_group":
		expectedFlags = 8
	case "mindlink_group":
	default:
		return false, ErrNotReady
	}
	if native.ConversationID != expectedNative || table == "g_hismsgs" && (native.ChannelType != 2 || native.TargetPrincipalID != p.native) {
		return false, ErrUnknownResult
	}
	var stored []byte
	if err = tx.QueryRowContext(ctx, "SELECT msg_body FROM "+table+" WHERE app_key=? AND conver_id=? AND sub_channel='' AND BINARY msg_id=? AND is_delete=0", c.AppKey, expectedNative, receipt.MessageID).Scan(&stored); err != nil {
		return false, ErrNotReady
	}
	var down pbobjs.DownMsg
	if !bytes.Equal(stored, payload) || proto.Unmarshal(payload, &down) != nil || down.MsgId != receipt.MessageID || strconv.FormatInt(down.MsgSeqNo, 10) != receipt.MsgSeqNo || strconv.FormatInt(down.MsgTime, 10) != receipt.MsgTime || down.SenderId != native.SenderPrincipalID || down.Flags != expectedFlags {
		return false, ErrUnknownResult
	}
	// Workers can claim different rows concurrently. The owner barrier serializes
	// this check with materialization, preventing an older pending summary arriving last.
	var earlier bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM mindlink_message_outbox q JOIN mindlink_operations o ON o.issuer=q.issuer AND o.operation_id=q.operation_id
 WHERE q.state='pending' AND JSON_UNQUOTE(JSON_EXTRACT(CONVERT(o.result_json USING utf8mb4),'$.appkey'))=?
 AND JSON_UNQUOTE(JSON_EXTRACT(CONVERT(o.result_json USING utf8mb4),'$.environment'))=? AND JSON_UNQUOTE(JSON_EXTRACT(CONVERT(o.result_json USING utf8mb4),'$.tenant_id'))=?
 AND JSON_UNQUOTE(JSON_EXTRACT(CONVERT(o.result_json USING utf8mb4),'$.owner_id'))=? AND JSON_UNQUOTE(JSON_EXTRACT(CONVERT(o.result_json USING utf8mb4),'$.conversation_id'))=?
 AND CAST(JSON_UNQUOTE(JSON_EXTRACT(CONVERT(o.result_json USING utf8mb4),'$.msg_seq_no')) AS UNSIGNED)<?)`, c.AppKey, c.Environment, c.TenantID, c.OwnerID, c.TargetID, down.MsgSeqNo).Scan(&earlier); err != nil {
		return false, ErrUnknownResult
	}
	if earlier {
		return false, ErrNotReady
	}
	// Only the persisted owner's immutable mapping chooses the recipient/target.
	down.TargetId = p.native
	down.IsSend = down.SenderId == p.ownerIM
	box := "inboxmsgs"
	if down.IsSend {
		box = "sendboxmsgs"
	}
	if down.ReferMsg != nil {
		down.ReferMsg.IsSend = down.ReferMsg.SenderId == p.ownerIM
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO conversations(app_key,user_id,target_id,channel_type,sub_channel) VALUES(?,?,?,?,'') ON DUPLICATE KEY UPDATE id=id`, c.AppKey, p.ownerIM, p.native, native.ChannelType)
	if err != nil {
		return false, err
	}
	var unread, sortTime int64
	if err = tx.QueryRowContext(ctx, `SELECT latest_unread_msg_index,sort_time,undisturb_type FROM conversations WHERE app_key=? AND user_id=? AND target_id=? AND channel_type=? AND sub_channel='' FOR UPDATE`, c.AppKey, p.ownerIM, p.native, native.ChannelType).Scan(&unread, &sortTime, &down.UndisturbType); err != nil {
		return false, err
	}
	if expectedFlags == 10 && !down.IsSend {
		unread++
	}
	down.UnreadIndex = unread
	// Native SDK synchronization uses the delivery timestamp, while native history
	// and the original receipt retain the send timestamp and message sequence.
	var floor int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(send_time),0) FROM `+box+` WHERE app_key=? AND user_id=?`, c.AppKey, p.ownerIM).Scan(&floor); err != nil {
		return false, err
	}
	down.MsgTime = max(time.Now().UnixMilli(), down.MsgTime, floor+1, sortTime+1)
	raw, err := proto.Marshal(&down)
	if err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE conversations SET latest_msg_id=?,latest_msg=?,latest_unread_msg_index=?,sort_time=?,sync_time=?,is_deleted=0 WHERE app_key=? AND user_id=? AND target_id=? AND channel_type=? AND sub_channel=''`, down.MsgId, raw, unread, down.MsgTime, down.MsgTime, c.AppKey, p.ownerIM, p.native, native.ChannelType); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO `+box+`(app_key,user_id,target_id,channel_type,send_time,msg_id,msg_type,msg_body) VALUES(?,?,?,?,?,?,?,?)`, c.AppKey, p.ownerIM, p.native, native.ChannelType, down.MsgTime, down.MsgId, down.MsgType, raw); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE mindlink_message_outbox SET state='inboxed' WHERE outbox_id=?`, id); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, ErrUnknownResult
	}
	return true, nil
}
