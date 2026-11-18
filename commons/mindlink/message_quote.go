package mindlink

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"google.golang.org/protobuf/proto"
	"im-server/commons/pbdefines/pbobjs"
)

// A quote redistributes content to the frozen roster. Lock each native visibility
// boundary, including absent delete/clear rows, in the send's repeatable-read Tx.
func lockMessageQuote(ctx context.Context, tx *sql.Tx, c Claims, b MessageNativeBinding, p *messageProjection, id string) (*pbobjs.DownMsg, error) {
	if err := lockMessageOwner(ctx, tx, c, false); err != nil {
		return nil, err
	}
	table, deleted, portion := "p_hismsgs", "p_delhismsgs", ""
	if b.ChannelType == 2 {
		table, deleted, portion = "g_hismsgs", "g_delhismsgs", " AND is_portion=0"
	}
	var raw []byte
	var seq, sent int64
	err := tx.QueryRowContext(ctx, "SELECT msg_body,msg_seq_no,send_time FROM "+table+" WHERE app_key=? AND conver_id=? AND sub_channel=? AND msg_id=? AND is_delete=0 AND destroy_time=0 AND life_time_after_read=0"+portion+" FOR UPDATE", c.AppKey, b.ConversationID, b.SubChannel, id).Scan(&raw, &seq, &sent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, ErrUnknownResult
	}
	var original pbobjs.DownMsg
	if len(raw) > MaxBodyBytes || proto.Unmarshal(raw, &original) != nil || original.MsgId != id || original.MsgSeqNo != seq || original.MsgTime != sent || seq <= 0 || sent <= 0 || int32(original.ChannelType) != b.ChannelType {
		return nil, ErrUnknownResult
	}
	var text struct {
		Content string `json:"content"`
	}
	// Restricted/ephemeral messages and non-text payloads cannot become permanent quotes.
	if original.MsgType != "jg:text" || DecodeStrictObject(original.MsgContent, &text, []string{"content"}) != nil || len(text.Content) == 0 || len(text.Content) > 65536 {
		return nil, ErrNotReady
	}
	var clear int64
	err = tx.QueryRowContext(ctx, "SELECT clean_time FROM convercleantimes WHERE app_key=? AND conver_id=? AND channel_type=? AND sub_channel=? FOR UPDATE", c.AppKey, b.ConversationID, b.ChannelType, b.SubChannel).Scan(&clear)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnknownResult
	}
	if clear >= sent {
		return nil, ErrForbidden
	}
	for _, m := range p.members {
		target := b.ConversationID
		if b.ChannelType == 1 {
			for _, peer := range p.members {
				if peer.PrincipalID != m.PrincipalID {
					target = peer.PrincipalID
				}
			}
		}
		var removed string
		err = tx.QueryRowContext(ctx, "SELECT msg_id FROM "+deleted+" WHERE app_key=? AND user_id=? AND target_id=? AND sub_channel=? AND msg_id=? FOR UPDATE", c.AppKey, m.PrincipalID, target, b.SubChannel, id).Scan(&removed)
		if err == nil {
			return nil, ErrForbidden
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUnknownResult
		}
		clear = 0
		err = tx.QueryRowContext(ctx, "SELECT clean_time FROM usercleantimes WHERE app_key=? AND user_id=? AND target_id=? AND channel_type=? AND sub_channel=? FOR UPDATE", c.AppKey, m.PrincipalID, target, b.ChannelType, b.SubChannel).Scan(&clear)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUnknownResult
		}
		if clear >= sent {
			return nil, ErrForbidden
		}
		if b.ChannelType == 2 {
			var joined int64
			if tx.QueryRowContext(ctx, "SELECT CAST(UNIX_TIMESTAMP(created_time)*1000 AS SIGNED) FROM groupmembers WHERE app_key=? AND group_id=? AND member_id=? FOR UPDATE", c.AppKey, b.ConversationID, m.PrincipalID).Scan(&joined) != nil {
				return nil, ErrUnknownResult
			}
			hide := false
			for _, setting := range []struct {
				q    string
				args []any
			}{
				{"SELECT item_value FROM groupinfoexts WHERE app_key=? AND group_id=? AND item_key='hide_grp_msg' AND item_type=1 FOR UPDATE", []any{c.AppKey, b.ConversationID}},
				{"SELECT item_value FROM groupmemberexts WHERE app_key=? AND group_id=? AND member_id=? AND item_key='hide_grp_msg' AND item_type=1 FOR UPDATE", []any{c.AppKey, b.ConversationID, m.PrincipalID}},
			} {
				var value string
				err = tx.QueryRowContext(ctx, setting.q, setting.args...).Scan(&value)
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				if err != nil {
					return nil, ErrUnknownResult
				}
				hide, err = strconv.ParseBool(value)
				if err != nil {
					return nil, ErrNotReady
				}
			}
			if hide && (joined <= 0 || sent <= joined) {
				return nil, ErrForbidden
			}
		}
	}
	// Do not copy nested references or unvalidated recipient/extension metadata.
	return &pbobjs.DownMsg{MsgId: id, MsgSeqNo: seq, MsgTime: sent, MsgType: original.MsgType, MsgContent: original.MsgContent, SenderId: original.SenderId, TargetId: original.TargetId, ChannelType: original.ChannelType}, nil
}
