package mindlink

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"im-server/commons/configures"
	"im-server/commons/messagesequence"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/commons/tools"
)

func messageStorageReady(ctx context.Context, db *sql.DB) error {
	if configures.Config.MsgStoreEngine != "" && configures.Config.MsgStoreEngine != configures.MsgStoreEngine_MySQL {
		return ErrNotReady
	}
	if _, err := NewNativeFence(ctx, db); err != nil {
		return err
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.TABLES
		WHERE TABLE_SCHEMA=DATABASE() AND ENGINE='InnoDB' AND TABLE_NAME IN (
		'p_hismsgs','g_hismsgs','message_sequence','conversation_seen','mindlink_message_outbox',
		'mindlink_conversations','banusers','blocks','appexts','friendrels',
		'p_delhismsgs','g_delhismsgs','convercleantimes','usercleantimes','groupinfoexts','groupmemberexts')`).Scan(&n); err != nil {
		return ErrUnknownResult
	}
	if n != 16 {
		return ErrNotReady
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('p_hismsgs','g_hismsgs')
		AND COLUMN_NAME='msg_seq_no' AND DATA_TYPE='bigint'`).Scan(&n); err != nil {
		return ErrUnknownResult
	}
	if n != 2 {
		return ErrNotReady
	}
	return nil
}

// SendMessage commits native history, the original receipt and a durable pending
// outbox together. A persisted receipt does not assert recipient delivery.
func (v *Verifier) SendMessage(ctx context.Context, db *sql.DB, auth *Authorization) (*MutationResult, error) {
	b, err := v.ParseMessageSend(auth)
	if err != nil {
		return nil, err
	}
	if err = messageStorageReady(ctx, db); err != nil {
		return nil, err
	}
	var authority *messageAuthority
	// Range locks keep roster and absent native policy keys stable until commit.
	return v.transactWithIsolation(ctx, db, auth, func(tx *sql.Tx, c Claims, replayed bool) error {
		authority, err = authorizeMessage(ctx, tx, c, b, replayed)
		if err != nil {
			return err
		}
		if replayed {
			var raw []byte
			if err := tx.QueryRowContext(ctx, `SELECT result_json FROM mindlink_operations WHERE issuer=? AND operation_id=?`, c.Issuer, c.OperationID).Scan(&raw); err != nil {
				return ErrUnknownResult
			}
			_, err := v.ValidateMessageSendResult(auth, raw, authority.native)
			return err
		}
		return nil
	}, func(tx *sql.Tx) (json.RawMessage, error) {
		c, native := auth.claims, authority.native
		now := time.Now().UnixMilli()
		next, err := messagesequence.Reserve(ctx, tx, c.AppKey, native.ConversationID, native.SubChannel, native.ChannelType, messagesequence.Watermark{}, now, func() (messagesequence.Watermark, error) {
			return messagesequence.SQLFloor(ctx, tx, c.AppKey, native.ConversationID, native.SubChannel, native.ChannelType)
		})
		if err != nil {
			return nil, err
		}
		content, err := json.Marshal(struct {
			Content string         `json:"content"`
			Assets  []MessageAsset `json:"assets,omitempty"`
			ReplyTo *string        `json:"reply_to,omitempty"`
		}{b.Text, b.Assets, b.ReplyTo})
		if err != nil {
			return nil, err
		}
		down := &pbobjs.DownMsg{SenderId: native.SenderPrincipalID, TargetId: native.TargetPrincipalID, ChannelType: pbobjs.ChannelType(native.ChannelType), MsgType: b.MessageType, MsgId: tools.GenerateMsgId(next.SendTime, native.ChannelType, native.ConversationID), MsgSeqNo: next.Seq, MsgTime: next.SendTime, MsgContent: content, Flags: 10, MemberCount: int32(len(authority.recipients))}
		if native.ChannelType == 1 {
			down.TargetId = native.SenderPrincipalID
			down.MemberCount = 0
		}
		if authority.kind == "avatar_pair_group" {
			down.Flags = 8
		}
		if len(authority.mentions) > 0 {
			down.MentionInfo = &pbobjs.MentionInfo{MentionType: pbobjs.MentionType_Someone, TargetUsers: authority.mentions}
		}
		table := "p_hismsgs"
		if native.ChannelType == 2 {
			table = "g_hismsgs"
		}
		if b.ReplyTo != nil {
			down.ReferMsg, err = lockMessageQuote(ctx, tx, c, native, authority.recipients, *b.ReplyTo)
			if err != nil {
				return nil, err
			}
		}
		payload, err := proto.Marshal(down)
		if err != nil {
			return nil, err
		}
		columns, values := "app_key,conver_id,sub_channel,sender_id,receiver_id,channel_type,msg_type,msg_id,send_time,msg_seq_no,msg_body", "?,?,?,?,?,?,?,?,?,?,?"
		args := []any{c.AppKey, native.ConversationID, native.SubChannel, native.SenderPrincipalID, native.TargetPrincipalID, native.ChannelType, b.MessageType, down.MsgId, next.SendTime, next.Seq, payload}
		if native.ChannelType == 2 {
			columns += ",member_count"
			values += ",?"
			args = append(args, down.MemberCount)
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO "+table+" ("+columns+") VALUES("+values+")", args...); err != nil {
			return nil, err
		}
		binding, err := json.Marshal(native)
		if err != nil {
			return nil, err
		}
		recipients, err := json.Marshal(authority.recipients)
		if err != nil {
			return nil, err
		}
		outboxID := messageOutboxID(c)
		if _, err = tx.ExecContext(ctx, `INSERT INTO mindlink_message_outbox(outbox_id,issuer,operation_id,request_body,native_binding,payload,recipients_json) VALUES(?,?,?,?,?,?,?)`, outboxID, c.Issuer, c.OperationID, auth.body, binding, payload, recipients); err != nil {
			return nil, err
		}
		var created string
		if err = tx.QueryRowContext(ctx, "SELECT DATE_FORMAT(created_time,'%Y-%m-%d %H:%i:%s.%f') FROM mindlink_operations WHERE issuer=? AND operation_id=?", c.Issuer, c.OperationID).Scan(&created); err != nil {
			return nil, err
		}
		accepted, err := time.Parse("2006-01-02 15:04:05.999999", created)
		if err != nil {
			return nil, err
		}
		var token [32]byte
		if _, err = rand.Read(token[:]); err != nil {
			return nil, err
		}
		result := MessageSendResult{Version: MessageWireVersion, OperationType: OperationMessageSend, OperationID: c.OperationID, RequestID: c.RequestID, RequestDigest: c.RequestDigest, BodySHA256: c.BodySHA256, ParentDigest: b.ParentDigest, DeliverySnapshotSHA256: b.DeliverySnapshotSHA256, Environment: c.Environment, TenantID: c.TenantID, AppKey: c.AppKey, OwnerID: c.OwnerID, ActorKind: c.ActorKind, ActorID: c.ActorID, AvatarGeneration: c.AvatarGeneration, ConversationID: b.ConversationID, PolicyVersion: b.PolicyVersion, State: "persisted", MessageID: down.MsgId, MsgSeqNo: strconv.FormatInt(next.Seq, 10), MsgTime: strconv.FormatInt(next.SendTime, 10), ChannelType: native.ChannelType, NativeConversationID: native.ConversationID, SenderPrincipalID: native.SenderPrincipalID, TargetPrincipalID: native.TargetPrincipalID, SubChannel: native.SubChannel, OutboxID: outboxID, CommitToken: hex.EncodeToString(token[:]), AcceptedAt: accepted.Format(ReceiptTimeFormat), CommittedAt: time.Now().UTC().Format(ReceiptTimeFormat)}
		raw, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		if _, err = v.ValidateMessageSendResult(auth, raw, native); err != nil {
			return nil, err
		}
		return raw, nil
	}, sql.LevelRepeatableRead)
}
