package services

import (
	"context"
	"time"

	"im-server/commons/bases"
	"im-server/commons/errs"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/conversation/storages/models"

	"google.golang.org/protobuf/proto"
)

// Snapshot identity fields under the cache lock; never perform policy SQL while holding it.
func nativeConverVisibility(ctx context.Context, uc *UserConversations) (errs.IMErrorCode, *UserConversations, map[string]bool) {
	if ctx == nil || uc == nil || uc.UserId == "" {
		return errs.IMErrorCode_MINDLINK_INVALID_REQUEST, nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	appkey := bases.GetAppKeyFromCtx(ctx)
	if appkey != uc.Appkey {
		return errs.IMErrorCode_MINDLINK_INVALID_REQUEST, nil, nil
	}
	if code := bases.NativePolicyCode(bases.CheckNativeIdentities(ctx, appkey, []string{uc.UserId}, nil)); code != 0 {
		return code, nil, nil
	}
	view, err := snapshotNativeConversations(uc)
	if err != nil {
		return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, nil, nil
	}
	rows := make([]*models.Conversation, 0, len(view.ConverItemMap))
	for _, row := range view.ConverItemMap {
		rows = append(rows, row)
	}
	if err := prepareNativeReadRows(ctx, appkey, uc.UserId, rows); err != nil {
		return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, nil, nil
	}
	items := make([]proto.Message, 0, len(rows))
	for _, item := range rows {
		conver := &pbobjs.Conversation{UserId: uc.UserId, TargetId: item.TargetId, ChannelType: item.ChannelType, SubChannel: item.SubChannel}
		if len(item.LatestMsg) > 0 {
			conver.Msg = &pbobjs.DownMsg{}
			if err := proto.Unmarshal(item.LatestMsg, conver.Msg); err != nil {
				return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, nil, nil
			}
		}
		if item.MentionInfo != nil {
			conver.Mentions = &pbobjs.Mentions{}
			for _, sender := range item.MentionInfo.SenderIds {
				conver.Mentions.Senders = append(conver.Mentions.Senders, &pbobjs.UserInfo{UserId: sender})
			}
			for _, msg := range item.MentionInfo.MentionMsgs {
				conver.Mentions.MentionMsgs = append(conver.Mentions.MentionMsgs, &pbobjs.MentionMsg{SenderId: msg.SenderId})
			}
		}
		items = append(items, conver)
	}
	visible := make(map[string]bool, len(items))
	mask, err := bases.FilterNativeMessages(ctx, appkey, items)
	if err != nil {
		return bases.NativePolicyCode(err), nil, nil
	}
	for i, allowed := range mask {
		if allowed {
			row := rows[i]
			visible[getConverItemKey(row.TargetId, row.SubChannel, row.ChannelType)] = true
		}
	}
	return errs.IMErrorCode_SUCCESS, view, visible
}
