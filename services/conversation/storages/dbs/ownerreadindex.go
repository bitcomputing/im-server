package dbs

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/commonservices"
)

// OwnerReadIndex translates the durable displayed history sequence to the owner's
// delivery counter. Outgoing messages have a sequence but do not increment that counter.
func (dao SeenDao) OwnerReadIndex(app, owner, target string, channel pbobjs.ChannelType, seen int64) (int64, error) {
	if seen <= 0 {
		return 0, nil
	}
	var latest struct{ LatestMsg []byte }
	err := dao.DB.Raw("SELECT latest_msg FROM conversations WHERE app_key=? AND user_id=? AND target_id=? AND channel_type=? AND sub_channel=''", app, owner, target, channel).Scan(&latest).Error
	if err != nil {
		return 0, err
	}
	var msg pbobjs.DownMsg
	if err = proto.Unmarshal(latest.LatestMsg, &msg); err != nil {
		return 0, err
	}
	if msg.MsgSeqNo > 0 && msg.MsgSeqNo <= seen {
		return msg.UnreadIndex, nil
	}
	table := "p_hismsgs"
	if channel == pbobjs.ChannelType_Group {
		table = "g_hismsgs"
	}
	// Each inbox copy retains the recipient-specific index at delivery. Restrict
	// the join to this conversation and never acknowledge a message past seen.
	var row struct{ MsgBody []byte }
	query := fmt.Sprintf("SELECT i.msg_body FROM %s h JOIN inboxmsgs i ON i.app_key=h.app_key AND i.user_id=? AND i.target_id=? AND i.channel_type=? AND i.msg_id=h.msg_id WHERE h.app_key=? AND h.conver_id=? AND h.sub_channel='' AND h.msg_seq_no<=? ORDER BY h.msg_seq_no DESC LIMIT 1", table)
	err = dao.DB.Raw(query, owner, target, channel, app, commonservices.GetConversationId(owner, target, channel), seen).Scan(&row).Error
	if err != nil {
		return 0, err
	}
	if err = proto.Unmarshal(row.MsgBody, &msg); err != nil {
		return 0, err
	}
	if msg.MsgSeqNo > seen || msg.TargetId != target && msg.MsgId != "" {
		return 0, fmt.Errorf("inconsistent owner inbox position")
	}
	return msg.UnreadIndex, nil
}
