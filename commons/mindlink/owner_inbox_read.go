package mindlink

import (
	"context"
	"google.golang.org/protobuf/proto"
	"im-server/commons/pbdefines/pbobjs"
)

// OwnerInboxReadable is a read-only exception for an exact durable inbox entry.
// Selectors or a matching owner alone never authorize a supplied message body.
func (f *NativeFence) OwnerInboxReadable(ctx context.Context, appkey, owner string, msg *pbobjs.DownMsg) (bool, error) {
	if f == nil || f.db == nil || !f.verified {
		return false, ErrNotReady
	}
	if ctx == nil || appkey == "" || owner == "" || msg == nil || msg.MsgId == "" {
		return false, ErrInvalidRequest
	}
	box := "inboxmsgs"
	if msg.IsSend {
		box = "sendboxmsgs"
	}
	rows, err := f.db.QueryContext(ctx, `SELECT i.msg_body FROM `+box+` i
 JOIN mindlink_conversations c ON c.native_appkey=i.app_key AND BINARY c.owner_im_id=BINARY i.user_id AND BINARY c.native_target_id=BINARY i.target_id AND c.channel_type=i.channel_type
 JOIN mindlink_owner_barriers b ON b.environment=c.environment AND b.tenant_id=c.tenant_id AND b.appkey=c.appkey AND b.owner_id=c.owner_id
 WHERE i.app_key=? AND BINARY i.user_id=? AND BINARY i.msg_id=? AND BINARY i.target_id=? AND i.channel_type=? AND c.state IN ('active','disabled') AND b.read_enabled=TRUE AND ?=''`, appkey, owner, msg.MsgId, msg.TargetId, int32(msg.ChannelType), msg.SubChannel)
	if err != nil {
		return false, ErrUnknownResult
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var raw []byte
		var stored pbobjs.DownMsg
		if rows.Scan(&raw) != nil || proto.Unmarshal(raw, &stored) != nil {
			return false, ErrUnknownResult
		}
		if found {
			return false, ErrUnknownResult
		}
		if !proto.Equal(&stored, msg) {
			return false, nil
		}
		found = true
	}
	if rows.Err() != nil {
		return false, ErrUnknownResult
	}
	return found, nil
}
