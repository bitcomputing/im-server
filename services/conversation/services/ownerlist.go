package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"im-server/commons/errs"
	"im-server/services/conversation/storages/dbs"
	"im-server/services/conversation/storages/models"
	"math"
	"sort"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"im-server/commons/bases"
	"im-server/commons/dbcommons"
	"im-server/commons/mindlink"
	"im-server/commons/pbdefines/pbobjs"
)

// Both queries share one snapshot so a readiness/permission change cannot mix
// one account's classification with another snapshot's message previews.
func ownerListRows(ctx context.Context, target string) ([]*pbobjs.Conversation, bool, error) {
	app, owner := bases.GetAppKeyFromCtx(ctx), bases.GetRequesterIdFromCtx(ctx)
	if app == "" || owner == "" {
		return nil, false, errors.New("missing owner list identity")
	}
	const scope = " FROM mindlink_conversations m JOIN mindlink_owner_barriers b ON b.environment=m.environment AND b.tenant_id=m.tenant_id AND b.appkey=m.appkey AND b.owner_id=m.owner_id "
	const allowed = " WHERE m.appkey=? AND BINARY m.owner_im_id=BINARY ? AND b.read_enabled AND b.list_projection_ready "
	var result []*pbobjs.Conversation
	var positions []int64
	var ready bool
	err := dbcommons.GetDb().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw("SELECT EXISTS(SELECT 1"+scope+allowed+")", app, owner).Scan(&ready).Error; err != nil || !ready {
			return err
		}
		var retiredIDs []string
		if err := tx.Raw("SELECT DISTINCT p.native_principal_id"+scope+" JOIN mindlink_principals p ON p.environment=m.environment AND p.tenant_id=m.tenant_id AND p.appkey=m.appkey AND p.owner_id=m.owner_id"+allowed+" AND p.state='disabled'", app, owner).Scan(&retiredIDs).Error; err != nil {
			return err
		}
		retired := make(map[string]bool, len(retiredIDs))
		for _, id := range retiredIDs {
			retired[id] = true
		}
		filter := ""
		args := []any{app, owner}
		if target != "" {
			filter = " AND BINARY m.native_target_id=BINARY ?"
			args = append(args, target)
		}
		rows, err := tx.Raw("SELECT m.kind,m.conversation_id,m.members_json,m.state,b.write_enabled,c.target_id,c.channel_type,c.sort_time,c.sync_time,c.is_top,c.top_updated_time,c.undisturb_type,c.latest_unread_msg_index,c.latest_read_msg_index,COALESCE(s.seen_seq,0)"+scope+
			" JOIN conversations c ON c.app_key=m.native_appkey AND BINARY c.user_id=BINARY m.owner_im_id AND BINARY c.target_id=BINARY m.native_target_id AND c.channel_type=m.channel_type AND c.sub_channel='' AND c.is_deleted=0 "+
			" LEFT JOIN conversation_seen s ON s.app_key=c.app_key AND BINARY s.user_id=BINARY c.user_id AND BINARY s.target_id=BINARY c.target_id AND s.channel_type=c.channel_type AND s.sub_channel='' "+allowed+
			" AND m.state IN ('active','disabled') AND (m.channel_type=1 OR EXISTS (SELECT 1 FROM groupmembers g WHERE g.app_key=c.app_key AND BINARY g.group_id=BINARY c.target_id AND BINARY g.member_id=BINARY c.user_id))"+filter, args...).Rows()
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			b := &pbobjs.ConversationBusiness{}
			r := &pbobjs.Conversation{UserId: owner, Business: b}
			var members []byte
			var state string
			var writeEnabled bool
			var seen int64
			if err := rows.Scan(&b.Type, &b.BusinessId, &members, &state, &writeEnabled, &r.TargetId, &r.ChannelType, &r.SortTime, &r.SyncTime, &r.IsTop, &r.TopUpdatedTime, &r.UndisturbType, &r.LatestUnreadIndex, &r.LatestReadIndex, &seen); err != nil {
				return err
			}
			b.ReadOnly = !writeEnabled || state == "disabled" || b.Type == "avatar_pair_group"
			if b.Type != "mindlink_group" {
				var roster []mindlink.ConversationMember
				if err := json.Unmarshal(members, &roster); err != nil {
					return err
				}
				for _, m := range roster {
					if m.AvatarID == "" {
						return errors.New("missing mapped avatar")
					}
					b.AvatarIds = append(b.AvatarIds, m.AvatarID)
					if retired[m.PrincipalID] {
						b.ReadOnly = true
					}
				}
				if !(b.Type == "owner_avatar_dm" && len(roster) == 1 && r.ChannelType == pbobjs.ChannelType_Private || b.Type == "avatar_pair_group" && len(roster) == 2 && roster[0].AvatarID != roster[1].AvatarID && r.ChannelType == pbobjs.ChannelType_Group) {
					return errors.New("invalid mapped conversation")
				}
			}
			if b.Type != "avatar_pair_group" {
				r.UnreadCount = max(0, r.LatestUnreadIndex-r.LatestReadIndex)
			}
			result = append(result, r)
			positions = append(positions, seen)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for i, r := range result {
			if r.Business.Type != "avatar_pair_group" && positions[i] > 0 && r.UnreadCount > 0 {
				index, err := (dbs.SeenDao{DB: tx}).OwnerReadIndex(app, owner, r.TargetId, r.ChannelType, positions[i])
				if err != nil {
					return err
				}
				r.LatestReadIndex = max(r.LatestReadIndex, min(index, r.LatestUnreadIndex))
				r.UnreadCount = max(0, r.LatestUnreadIndex-r.LatestReadIndex)
			}
			last, err := (dbs.SeenDao{DB: tx}).LatestPreview(app, owner, r.TargetId, "", r.ChannelType)
			if err != nil {
				return err
			}
			if last.MsgId == "" {
				continue
			}
			msg := &pbobjs.DownMsg{}
			if err = proto.Unmarshal(last.MsgBody, msg); err != nil {
				return err
			}
			if msg.MsgId != last.MsgId || msg.MsgSeqNo != last.MsgSeqNo || msg.ChannelType != r.ChannelType || msg.SubChannel != "" {
				return errors.New("inconsistent native history preview")
			}
			msg.IsSend = msg.SenderId == owner
			msg.TargetId = r.TargetId
			r.Msg = msg
			r.Business.HasNew = last.MsgSeqNo > positions[i]
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return result, ready, err
}

type ownerListKey struct {
	Time    int64
	Target  string
	Channel pbobjs.ChannelType
	Sub     string
}

func pageOwnerList(ctx context.Context, view *UserConversations, visible map[string]bool, internal []*pbobjs.Conversation, req *pbobjs.QryConversationsReq) (errs.IMErrorCode, *pbobjs.QryConversationsResp) {
	var after ownerListKey
	if req.OwnerListCursor != "" && (len(req.OwnerListCursor) > 1024 || json.Unmarshal([]byte(req.OwnerListCursor), &after) != nil || after.Target == "") {
		return errs.IMErrorCode_MSG_PARAM_ILLEGAL, nil
	}
	// Use the sort index rather than every cached item, since deletion tombstones
	// belong in the synchronization feed and must not reappear in a full list.
	normal := view.QryConvers(math.MaxInt64, math.MaxInt32, false, req.TargetId, req.ChannelType, "", "", visible)
	standard := make(map[ownerListKey]*models.Conversation, len(normal))
	special := make(map[ownerListKey]*pbobjs.Conversation, len(internal))
	keys := make([]ownerListKey, 0, len(normal)+len(internal))
	for _, r := range normal {
		if r.IsDeleted != 0 || req.TargetId != "" && req.TargetId != r.TargetId {
			continue
		}
		k := ownerListKey{r.SortTime, r.TargetId, r.ChannelType, r.SubChannel}
		standard[k] = r
		keys = append(keys, k)
	}
	for _, r := range internal {
		if req.TargetId != "" && req.TargetId != r.TargetId || req.ChannelType != pbobjs.ChannelType_Unknown && req.ChannelType != r.ChannelType {
			continue
		}
		k := ownerListKey{r.SortTime, r.TargetId, r.ChannelType, r.SubChannel}
		special[k] = r
		keys = append(keys, k)
	}
	less := func(a, b ownerListKey) bool {
		if a.Time != b.Time {
			if req.Order != 0 {
				return a.Time < b.Time
			}
			return a.Time > b.Time
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		if a.Channel != b.Channel {
			return a.Channel < b.Channel
		}
		return a.Sub < b.Sub
	}
	sort.Slice(keys, func(i, j int) bool { return less(keys[i], keys[j]) })
	start := sort.Search(len(keys), func(i int) bool {
		if req.OwnerListCursor != "" {
			return less(after, keys[i])
		}
		if req.StartTime == 0 {
			return true
		}
		if req.Order != 0 {
			return keys[i].Time > req.StartTime
		}
		return keys[i].Time < req.StartTime
	})
	page := keys[start:]
	count := int(req.Count)
	resp := &pbobjs.QryConversationsResp{OwnerBusinessList: true, IsFinished: len(page) <= count}
	if !resp.IsFinished {
		page = page[:count]
		if len(page) > 0 {
			raw, _ := json.Marshal(page[len(page)-1])
			resp.OwnerListCursor = string(raw)
		}
	}
	filtered := make([]*models.Conversation, 0, len(page))
	for _, k := range page {
		if r := standard[k]; r != nil {
			filtered = append(filtered, r)
		}
	}
	code, formatted := fillConvers(ctx, view.UserId, filtered, view)
	if code != 0 {
		return code, nil
	}
	idx := 0
	for _, k := range page {
		if r := special[k]; r != nil {
			resp.Conversations = append(resp.Conversations, r)
		} else {
			resp.Conversations = append(resp.Conversations, formatted[idx])
			idx++
		}
	}
	return errs.IMErrorCode_SUCCESS, resp
}
