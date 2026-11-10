package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"im-server/commons/dbcommons"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/commonservices"
	"im-server/services/conversation/storages/models"
	hisStorages "im-server/services/historymsg/storages"
	hisModels "im-server/services/historymsg/storages/models"

	"github.com/bytedance/gopkg/collection/zset"
	"google.golang.org/protobuf/proto"
)

func cloneNativeMentionInfo(src *models.ConverMentionInfo) *models.ConverMentionInfo {
	if src == nil {
		return nil
	}
	dst := *src
	dst.SenderIds = append([]string(nil), src.SenderIds...)
	dst.MentionMsgs = make([]*models.MentionMsg, len(src.MentionMsgs))
	for i, msg := range src.MentionMsgs {
		if msg != nil {
			copy := *msg
			dst.MentionMsgs[i] = &copy
		}
	}
	return &dst
}

// This is a request-local read snapshot, not another persistent conversation authority.
func snapshotNativeConversations(uc *UserConversations) (*UserConversations, error) {
	view := &UserConversations{Appkey: uc.Appkey, UserId: uc.UserId, ConverItemMap: map[string]*models.Conversation{}, SyncTimeIndex: zset.NewFloat64(), SortTimeIndex: zset.NewFloat64(), TopIndexBaseTopTime: zset.NewFloat64(), TopIndexBaseSortTime: zset.NewFloat64()}
	lock := userLocks.GetLocks(getUserConverCacheKey(uc.Appkey, uc.UserId))
	lock.RLock()
	defer lock.RUnlock()
	for key, item := range uc.ConverItemMap {
		if item == nil {
			return nil, errors.New("missing native conversation cache row")
		}
		copy := *item
		copy.LatestMsg = nil // Only the current LatestMsgId's persisted body is authoritative for rendering.
		copy.MentionInfo = cloneNativeMentionInfo(item.MentionInfo)
		if copy.ConverExts != nil {
			copy.ConverExts = proto.Clone(copy.ConverExts).(*pbobjs.ConverExts)
		}
		view.ConverItemMap[key] = &copy
	}
	for _, indexes := range [][2]*zset.Float64Set{{uc.SyncTimeIndex, view.SyncTimeIndex}, {uc.SortTimeIndex, view.SortTimeIndex}, {uc.TopIndexBaseTopTime, view.TopIndexBaseTopTime}, {uc.TopIndexBaseSortTime, view.TopIndexBaseSortTime}} {
		if indexes[0] == nil {
			return nil, errors.New("missing native cache index")
		}
		for _, node := range indexes[0].RangeByScore(-math.MaxFloat64, math.MaxFloat64) {
			indexes[1].Add(node.Score, node.Value)
		}
	}
	return view, nil
}

// Resolve current history and cold mentions before either pagination or aggregation.
func prepareNativeReadRows(ctx context.Context, appkey, userID string, rows []*models.Conversation) error {
	for _, row := range rows {
		row.LatestMsg = nil
	}
	for _, channel := range []pbobjs.ChannelType{pbobjs.ChannelType_Private, pbobjs.ChannelType_Group} {
		selected := []*models.Conversation{}
		for _, row := range rows {
			if row.IsDeleted == 0 && row.ChannelType == channel && row.LatestMsgId != "" {
				selected = append(selected, row)
			}
		}
		for start := 0; start < len(selected); start += 256 {
			if err := ctx.Err(); err != nil {
				return err
			}
			end := start + 256
			if end > len(selected) {
				end = len(selected)
			}
			items := make([]hisModels.ConverItem, 0, end-start)
			for _, row := range selected[start:end] {
				items = append(items, hisModels.ConverItem{ConverId: commonservices.GetConversationId(userID, row.TargetId, channel), SubChannel: row.SubChannel, MsgId: row.LatestMsgId})
			}
			bodies := map[string][]byte{}
			decode := func(id string, body []byte, isRead bool) error {
				msg := &pbobjs.DownMsg{}
				if err := proto.Unmarshal(body, msg); err != nil {
					return err
				}
				if msg.MsgId != id {
					return errors.New("native history message identity mismatch")
				}
				msg.IsSend = msg.SenderId == userID
				if channel == pbobjs.ChannelType_Private {
					msg.IsRead = isRead
				}
				encoded, err := proto.Marshal(msg)
				if err != nil {
					return err
				}
				bodies[id] = encoded
				return nil
			}
			if channel == pbobjs.ChannelType_Private {
				messages, err := hisStorages.NewPrivateHisMsgStorage().FindByConvers(appkey, items)
				if err != nil {
					return err
				}
				for _, msg := range messages {
					if err := decode(msg.MsgId, msg.MsgBody, msg.IsRead > 0); err != nil {
						return err
					}
				}
			} else {
				messages, err := hisStorages.NewGroupHisMsgStorage().FindByConvers(appkey, items)
				if err != nil {
					return err
				}
				for _, msg := range messages {
					if err := decode(msg.MsgId, msg.MsgBody, false); err != nil {
						return err
					}
				}
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			for _, row := range selected[start:end] {
				row.LatestMsg = bodies[row.LatestMsgId]
			}
		}
	}
	cold := []*models.Conversation{}
	for _, row := range rows {
		if row.IsDeleted > 0 {
			row.MentionInfo = nil
			continue
		}
		if row.MentionInfo == nil {
			row.MentionInfo = &models.ConverMentionInfo{}
			cold = append(cold, row)
		} else {
			for _, msg := range row.MentionInfo.MentionMsgs {
				if msg == nil {
					return errors.New("invalid cached native mention")
				}
			}
		}
	}
	onlyUnread := false
	if len(cold) > 0 {
		if app, ok := commonservices.GetAppInfo(appkey); ok && app != nil {
			onlyUnread = app.LoadMentionWayInConver > 0
		}
	}
	for start := 0; start < len(cold); start += 128 {
		end := start + 128
		if end > len(cold) {
			end = len(cold)
		}
		var inputs []string
		args := []any{}
		for i, row := range cold[start:end] {
			inputs = append(inputs, fmt.Sprintf("SELECT %d AS ordinal,CONVERT(? USING utf8mb4) COLLATE utf8mb4_0900_ai_ci AS target_id,? AS channel_type,CONVERT(? USING utf8mb4) COLLATE utf8mb4_0900_ai_ci AS sub_channel,? AS read_index", i))
			args = append(args, row.TargetId, int(row.ChannelType), row.SubChannel, row.LatestReadMsgIndex)
		}
		condition := "m.msg_index>scope.read_index"
		if onlyUnread {
			condition = "m.is_read=0"
		}
		query := "SELECT ordinal,sender_id,msg_id,msg_time,msg_index,mention_type FROM (SELECT scope.ordinal,m.sender_id,m.msg_id,m.msg_time,m.msg_index,m.mention_type,ROW_NUMBER() OVER (PARTITION BY scope.ordinal ORDER BY m.msg_index DESC) AS ranking FROM (" + strings.Join(inputs, " UNION ALL ") + ") scope JOIN mentionmsgs m ON m.app_key=? AND m.user_id=? AND m.target_id=scope.target_id AND m.channel_type=scope.channel_type AND m.sub_channel=scope.sub_channel WHERE " + condition + ") ranked WHERE ranking<=100 ORDER BY ordinal,msg_index"
		args = append(args, appkey, userID)
		var messages []struct {
			Ordinal           int
			SenderId, MsgId   string
			MsgTime, MsgIndex int64
			MentionType       int
		}
		if err := dbcommons.GetDb().WithContext(ctx).Raw(query, args...).Scan(&messages).Error; err != nil {
			return err
		}
		for _, msg := range messages {
			if msg.Ordinal < 0 || msg.Ordinal >= end-start {
				return errors.New("invalid native mention ordinal")
			}
			info := cold[start+msg.Ordinal].MentionInfo
			info.MentionMsgs = append(info.MentionMsgs, &models.MentionMsg{SenderId: msg.SenderId, MsgId: msg.MsgId, MsgTime: msg.MsgTime, MsgIndex: msg.MsgIndex, MentionType: pbobjs.MentionType(msg.MentionType)})
			if !slices.Contains(info.SenderIds, msg.SenderId) {
				info.SenderIds = append(info.SenderIds, msg.SenderId)
			}
			info.IsMentioned = true
			info.MentionMsgCount++
		}
	}
	return ctx.Err()
}
