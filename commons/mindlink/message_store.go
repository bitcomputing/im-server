package mindlink

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/commons/tools"

	"google.golang.org/protobuf/proto"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type MessageStore struct{ db *sql.DB }

// NewMessageStore verifies the private MySQL lane's storage prerequisites. It
// does not provision grants, create a projection or enable a public route.
func NewMessageStore(ctx context.Context, db *sql.DB) (*MessageStore, error) {
	if ctx == nil || db == nil || configures.Config.MsgStoreEngine != "" && configures.Config.MsgStoreEngine != configures.MsgStoreEngine_MySQL {
		return nil, ErrNotReady
	}
	var version string
	if db.QueryRowContext(ctx, "SELECT conf_value FROM globalconfs WHERE conf_key='jimdb_version'").Scan(&version) != nil {
		return nil, ErrNotReady
	}
	n, err := strconv.ParseInt(version, 10, 64)
	if err != nil || n < 20261112 {
		return nil, ErrNotReady
	}
	var engines, sequences int
	if db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND ENGINE='InnoDB' AND TABLE_NAME IN ('mindlink_operations','mindlink_tickets','mindlink_owner_barriers','mindlink_principals','mindlink_send_conversations','mindlink_message_outbox','message_sequence','conversation_seen','p_hismsgs','g_hismsgs','users','groupinfos','groupmembers','banusers','p_delhismsgs','g_delhismsgs','convercleantimes','usercleantimes','groupinfoexts','groupmemberexts','apps','appexts','blocks','friendrels')`).Scan(&engines) != nil || engines != 24 {
		return nil, ErrNotReady
	}
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('p_hismsgs','g_hismsgs') AND COLUMN_NAME='msg_seq_no' AND DATA_TYPE='bigint'").Scan(&sequences) != nil || sequences != 2 {
		return nil, ErrNotReady
	}
	return &MessageStore{db}, nil
}

func (s *MessageStore) Send(ctx context.Context, v *Verifier, auth *Authorization) (*MutationResult, error) {
	if s == nil || s.db == nil {
		return nil, ErrNotReady
	}
	body, err := v.ParseMessageSend(auth)
	if err != nil {
		return nil, err
	}
	var projection *messageProjection
	var native MessageNativeBinding
	return v.transact(ctx, s.db, auth, func(tx *sql.Tx, c Claims, replayed bool) error {
		projection, err = lockMessageProjection(ctx, tx, c, !replayed)
		if err != nil {
			return err
		}
		if replayed {
			if err := projection.authorizeReader(ctx, tx, c); err != nil {
				return err
			}
			_, err := s.originalMessage(ctx, tx, v, c, projection)
			return err
		}
		native, err = projection.authorizeSubmission(ctx, tx, c, body)
		return err
	}, func(tx *sql.Tx) (json.RawMessage, error) {
		gdb, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: tx, SkipInitializeWithVersion: true}), &gorm.Config{SkipDefaultTransaction: true, DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			return nil, ErrUnknownResult
		}
		if gdb.Statement.ConnPool != tx {
			return nil, ErrNotReady
		}
		sequence, err := dbcommons.ReserveMessageSequenceWithTx(ctx, gdb, auth.claims.AppKey, native.ConversationID, native.SubChannel, native.ChannelType, dbcommons.MessageSequenceWatermark{}, time.Now().UnixMilli(), func() (dbcommons.MessageSequenceWatermark, error) {
			return dbcommons.MySQLMessageSequenceFloor(ctx, gdb, auth.claims.AppKey, native.ConversationID, native.SubChannel, native.ChannelType)
		})
		if err != nil {
			return nil, ErrUnknownResult
		}
		id := tools.GenerateMsgId(sequence.SendTime, native.ChannelType, native.ConversationID)
		if !validNativeMessageID(id) || tools.ParseTimeFromMsgId(id) != sequence.SendTime {
			return nil, ErrNotReady
		}
		content, _ := json.Marshal(struct {
			Content string `json:"content"`
		}{body.Text})
		payload := &pbobjs.DownMsg{TargetId: native.TargetPrincipalID, ChannelType: pbobjs.ChannelType(native.ChannelType), MsgType: body.MessageType, SenderId: native.SenderPrincipalID, MsgId: id, MsgSeqNo: sequence.Seq, MsgContent: content, MsgTime: sequence.SendTime, Flags: 14, ClientUid: body.OperationID, SubChannel: native.SubChannel}
		if native.ChannelType == 1 {
			// Native private history is the receiver projection; sender sendbox differs.
			payload.TargetId = native.SenderPrincipalID
		} else {
			payload.MemberCount = int32(len(projection.members) - 1)
		}
		if len(body.Mentions) > 0 {
			payload.MentionInfo = &pbobjs.MentionInfo{MentionType: pbobjs.MentionType_Someone}
			for _, avatar := range body.Mentions {
				for _, m := range projection.members {
					if m.ActorKind == "avatar" && m.ActorID == avatar {
						payload.MentionInfo.TargetUsers = append(payload.MentionInfo.TargetUsers, &pbobjs.UserInfo{UserId: m.PrincipalID})
					}
				}
			}
		}
		table := "p_hismsgs"
		if native.ChannelType == 2 {
			table = "g_hismsgs"
		}
		if body.ReplyTo != nil {
			payload.ReferMsg, err = lockMessageQuote(ctx, tx, auth.claims, native, projection, *body.ReplyTo)
			if err != nil {
				return nil, err
			}
		}
		wire, err := proto.Marshal(payload)
		if err != nil {
			return nil, ErrUnknownResult
		}
		columns, placeholders := "app_key,conver_id,sub_channel,sender_id,receiver_id,channel_type,msg_type,msg_id,send_time,msg_seq_no,msg_body", "?,?,?,?,?,?,?,?,?,?,?"
		args := []any{auth.claims.AppKey, native.ConversationID, native.SubChannel, native.SenderPrincipalID, native.TargetPrincipalID, native.ChannelType, body.MessageType, id, sequence.SendTime, sequence.Seq, wire}
		if native.ChannelType == 2 {
			columns, placeholders = columns+",member_count", placeholders+",?"
			args = append(args, len(projection.members)-1)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+table+" ("+columns+") VALUES("+placeholders+")", args...); err != nil {
			return nil, ErrUnknownResult
		}
		binding, _ := json.Marshal(native)
		hash := sha256.Sum256(binding)
		c := auth.claims
		if _, err := tx.ExecContext(ctx, "INSERT INTO mindlink_message_outbox(issuer,operation_id,event_id,appkey,message_id,request_body,native_binding_json,native_binding_hash,members_json,native_payload,created_time) VALUES(?,?,?,?,?,?,?,?,?,?,?)", c.Issuer, c.OperationID, messageOutboxID(c), c.AppKey, id, auth.body, binding, hash[:], projection.rawMembers, wire, time.Now().UTC().Format("2006-01-02 15:04:05.000")); err != nil {
			return nil, ErrUnknownResult
		}
		var created string
		if tx.QueryRowContext(ctx, "SELECT DATE_FORMAT(created_time,'%Y-%m-%d %H:%i:%s.%f') FROM mindlink_operations WHERE issuer=? AND operation_id=?", c.Issuer, c.OperationID).Scan(&created) != nil {
			return nil, ErrUnknownResult
		}
		accepted, err := time.Parse("2006-01-02 15:04:05.999999", created)
		if err != nil {
			return nil, ErrUnknownResult
		}
		var token [32]byte
		if _, err := rand.Read(token[:]); err != nil {
			return nil, ErrUnknownResult
		}
		result := MessageSendResult{Version: MessageWireVersion, OperationID: c.OperationID, OperationType: OperationMessageSend, RequestID: c.RequestID, RequestDigest: c.RequestDigest, BodySHA256: c.BodySHA256, ParentDigest: body.ParentDigest, DeliverySnapshotSHA256: body.DeliverySnapshotSHA256, Environment: c.Environment, TenantID: c.TenantID, AppKey: c.AppKey, OwnerID: c.OwnerID, ActorKind: c.ActorKind, ActorID: c.ActorID, AvatarGeneration: c.AvatarGeneration, ConversationID: body.ConversationID, PolicyVersion: body.PolicyVersion, State: "persisted", MessageID: id, MsgSeqNo: strconv.FormatInt(sequence.Seq, 10), MsgTime: strconv.FormatInt(sequence.SendTime, 10), ChannelType: native.ChannelType, NativeConversationID: native.ConversationID, SenderPrincipalID: native.SenderPrincipalID, TargetPrincipalID: native.TargetPrincipalID, SubChannel: native.SubChannel, OutboxID: messageOutboxID(c), CommitToken: hex.EncodeToString(token[:]), AcceptedAt: accepted.Format(ReceiptTimeFormat), CommittedAt: time.Now().UTC().Format(ReceiptTimeFormat)}
		raw, err := json.Marshal(result)
		if err != nil {
			return nil, ErrUnknownResult
		}
		if _, err := v.ValidateMessageSendResult(auth, raw, native); err != nil {
			return nil, err
		}
		return raw, nil
	}, sql.LevelRepeatableRead)
}

// originalMessage validates the original stored body, independent native binding
// and receipt; it never requires retained history to be rewritten or recreated.
func (s *MessageStore) originalMessage(ctx context.Context, tx *sql.Tx, v *Verifier, original Claims, p *messageProjection) (json.RawMessage, error) {
	var raw, body, binding, hash []byte
	if tx.QueryRowContext(ctx, "SELECT o.result_json,b.request_body,b.native_binding_json,b.native_binding_hash FROM mindlink_operations o JOIN mindlink_message_outbox b ON b.issuer=o.issuer AND b.operation_id=o.operation_id WHERE o.issuer=? AND o.operation_id=?", original.Issuer, original.OperationID).Scan(&raw, &body, &binding, &hash) != nil {
		return nil, ErrUnknownResult
	}
	digest := sha256.Sum256(binding)
	var native MessageNativeBinding
	if !bytes.Equal(hash, digest[:]) || DecodeStrictObject(binding, &native, []string{"channel_type", "native_conversation_id", "sender_principal_id", "target_principal_id", "sub_channel"}) != nil || native.ChannelType != p.channel || native.ConversationID != p.conver || native.SubChannel != p.sub {
		return nil, ErrUnknownResult
	}
	if _, err := v.ValidateMessageSendResult(&Authorization{claims: original, body: body, verifier: v}, raw, native); err != nil {
		return nil, err
	}
	return raw, nil
}
