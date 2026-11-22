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
	"im-server/commons/messagesequence"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/commons/tools"
)

func stationStopStorageReady(ctx context.Context, db *sql.DB) error {
	if db == nil || ctx == nil {
		return ErrNotReady
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='mindlink_station_stops' AND ENGINE='InnoDB'`).Scan(&n); err != nil {
		return ErrUnknownResult
	}
	if n != 1 {
		return ErrNotReady
	}
	return nil
}

func (v *Verifier) StationStopped(ctx context.Context, db *sql.DB, a *Authorization) (*MutationResult, error) {
	b, err := v.ParseStationStopped(a)
	if err != nil {
		return nil, err
	}
	if err = stationStopStorageReady(ctx, db); err != nil {
		return nil, err
	}
	if err = messageStorageReady(ctx, db); err != nil {
		return nil, err
	}
	var authority *messageAuthority
	return v.transactWithIsolation(ctx, db, a, func(tx *sql.Tx, c Claims, replayed bool) error {
		if replayed {
			if err := authorizeOperationRead(ctx, tx, c, c); err != nil {
				return err
			}
			var raw []byte
			if err := tx.QueryRowContext(ctx, `SELECT result_json FROM mindlink_operations WHERE issuer=? AND operation_id=?`, c.Issuer, c.OperationID).Scan(&raw); err != nil {
				return ErrUnknownResult
			}
			return v.validateStationStopOriginal(ctx, tx, c, raw)
		}
		// Only this already-authenticated system intent may use the owner's native anchor.
		anchor := c
		anchor.ActorKind, anchor.ActorID = "owner", c.OwnerID
		authority, err = authorizeMessage(ctx, tx, anchor, nil, false)
		if err != nil {
			return err
		}
		if authority.kind != "mindlink_group" {
			return ErrForbidden
		}
		var event string
		err = tx.QueryRowContext(ctx, `SELECT event_id FROM mindlink_station_stops WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=? AND access_id=? AND instance_id=? AND epoch=? FOR UPDATE`, c.Environment, c.TenantID, c.AppKey, c.OwnerID, b.AccessID, b.InstanceID, b.Epoch).Scan(&event)
		if err == nil {
			return ErrRequestConflict
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return ErrUnknownResult
		}
		return nil
	}, func(tx *sql.Tx) (json.RawMessage, error) {
		c, n := a.claims, authority.native
		next, err := messagesequence.Reserve(ctx, tx, c.AppKey, n.ConversationID, "", 2, messagesequence.Watermark{}, time.Now().UnixMilli(), func() (messagesequence.Watermark, error) {
			return messagesequence.SQLFloor(ctx, tx, c.AppKey, n.ConversationID, "", 2)
		})
		if err != nil {
			return nil, err
		}
		content, err := json.Marshal(struct {
			Content string `json:"content"`
		}{b.Text})
		if err != nil {
			return nil, err
		}
		down := &pbobjs.DownMsg{SenderId: n.SenderPrincipalID, TargetId: n.TargetPrincipalID, ChannelType: pbobjs.ChannelType_Group, MsgType: "jg:text", MsgId: tools.GenerateMsgId(next.SendTime, 2, n.ConversationID), MsgSeqNo: next.Seq, MsgTime: next.SendTime, MsgContent: content, Flags: 10, MemberCount: int32(len(authority.recipients))}
		payload, err := proto.Marshal(down)
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO g_hismsgs(app_key,conver_id,sub_channel,sender_id,receiver_id,channel_type,msg_type,msg_id,send_time,msg_seq_no,msg_body,member_count) VALUES(?,?,'',?,?,2,'jg:text',?,?,?,?,?)`, c.AppKey, n.ConversationID, n.SenderPrincipalID, n.TargetPrincipalID, down.MsgId, next.SendTime, next.Seq, payload, down.MemberCount); err != nil {
			return nil, err
		}
		hash := sha256.Sum256(payload)
		if _, err = tx.ExecContext(ctx, `INSERT INTO mindlink_station_stops(environment,tenant_id,appkey,owner_id,event_id,access_id,instance_id,epoch,reason,issuer,operation_id,conversation_id,policy_version,system_actor_id,native_conversation_id,sender_principal_id,message_id,msg_seq_no,msg_time,payload_sha256) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, c.Environment, c.TenantID, c.AppKey, c.OwnerID, b.EventID, b.AccessID, b.InstanceID, b.Epoch, b.Reason, c.Issuer, c.OperationID, c.TargetID, b.PolicyVersion, c.ActorID, n.ConversationID, n.SenderPrincipalID, down.MsgId, next.Seq, next.SendTime, hash[:]); err != nil {
			return nil, err
		}
		binding, err := json.Marshal(n)
		if err != nil {
			return nil, err
		}
		recipients, err := json.Marshal(authority.recipients)
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO mindlink_message_outbox(outbox_id,issuer,operation_id,request_body,native_binding,payload,recipients_json) VALUES(?,?,?,?,?,?,?)`, messageOutboxID(c), c.Issuer, c.OperationID, a.body, binding, payload, recipients); err != nil {
			return nil, err
		}
		r := StationStopResult{Version: StationStopVersion, OperationType: OperationStationStopped, OperationID: c.OperationID, RequestID: c.RequestID, RequestDigest: c.RequestDigest, BodySHA256: c.BodySHA256, Environment: c.Environment, TenantID: c.TenantID, AppKey: c.AppKey, OwnerID: c.OwnerID, ActorKind: c.ActorKind, ActorID: c.ActorID, AvatarGeneration: 0, ConversationID: c.TargetID, PolicyVersion: b.PolicyVersion, EventID: b.EventID, State: "persisted", MessageID: down.MsgId, MsgSeqNo: strconv.FormatInt(next.Seq, 10), MsgTime: strconv.FormatInt(next.SendTime, 10), NativeConversationID: n.ConversationID, SenderPrincipalID: n.SenderPrincipalID}
		raw, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		if _, err = v.validateStationStopResult(a, raw, n); err != nil {
			return nil, err
		}
		return raw, nil
	}, sql.LevelRepeatableRead)
}

func (v *Verifier) validateStationStopOriginal(ctx context.Context, tx *sql.Tx, c Claims, raw []byte) error {
	var body, nativeRaw, payload []byte
	if err := tx.QueryRowContext(ctx, `SELECT request_body,native_binding,payload FROM mindlink_message_outbox WHERE issuer=? AND operation_id=? AND outbox_id=?`, c.Issuer, c.OperationID, messageOutboxID(c)).Scan(&body, &nativeRaw, &payload); err != nil {
		return ErrUnknownResult
	}
	var n MessageNativeBinding
	if DecodeStrictObject(nativeRaw, &n, []string{"channel_type", "native_conversation_id", "sender_principal_id", "target_principal_id", "sub_channel"}) != nil {
		return ErrUnknownResult
	}
	r, err := v.validateStationStopResult(&Authorization{verifier: v, claims: c, body: body}, raw, n)
	if err != nil {
		return err
	}
	var b StationStopRequest
	if json.Unmarshal(body, &b) != nil {
		return ErrUnknownResult
	}
	var storedHash []byte
	err = tx.QueryRowContext(ctx, `SELECT payload_sha256 FROM mindlink_station_stops WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=? AND event_id=? AND issuer=? AND operation_id=? AND conversation_id=? AND policy_version=? AND system_actor_id=? AND native_conversation_id=? AND sender_principal_id=? AND message_id=? AND msg_seq_no=? AND msg_time=? AND access_id=? AND instance_id=? AND epoch=? AND reason=?`, c.Environment, c.TenantID, c.AppKey, c.OwnerID, r.EventID, c.Issuer, c.OperationID, c.TargetID, r.PolicyVersion, c.ActorID, n.ConversationID, n.SenderPrincipalID, r.MessageID, r.MsgSeqNo, r.MsgTime, b.AccessID, b.InstanceID, b.Epoch, b.Reason).Scan(&storedHash)
	hash := sha256.Sum256(payload)
	if err != nil || !bytes.Equal(storedHash, hash[:]) {
		return ErrUnknownResult
	}
	return nil
}
