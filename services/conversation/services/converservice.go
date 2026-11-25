package services

import (
	"context"
	"encoding/json"
	"fmt"
	"im-server/commons/bases"
	"im-server/commons/errs"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/commonservices"
	"im-server/services/commonservices/msgdefines"
	"im-server/services/conversation/storages"
	"im-server/services/conversation/storages/models"
	"math"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
)

func SaveConversationV2(ctx context.Context, appkey string, userId string, msg *pbobjs.DownMsg, isOffline bool) {
	if msg == nil {
		return
	}
	if msgdefines.IsStoreMsg(msg.Flags) {
		var sortTime int64 = msg.MsgTime
		var unreadIndex int64 = 0
		if msg.IsSend {
			if msgdefines.IsNoAffectSenderConver(msg.Flags) {
				sortTime = 0
			}
			appinfo, exist := commonservices.GetAppInfo(appkey)
			if exist && appinfo != nil && appinfo.RecordGlobalConvers {
				converId := commonservices.GetConversationId(msg.SenderId, msg.TargetId, msg.ChannelType)
				SaveGlobalConversationByCache(&GlobalConversationCacheItem{
					Appkey:      appkey,
					ConverId:    converId,
					SenderId:    msg.SenderId,
					TargetId:    msg.TargetId,
					ChannelType: msg.ChannelType,
					MsgTime:     msg.MsgTime,
				})
			}
		} else {
			unreadIndex = msg.UnreadIndex
		}
		if !isOffline || UserConversContains(appkey, userId) {
			userConvers := getUserConvers(appkey, userId)
			newConver := userConvers.UpsertCovner(models.Conversation{
				AppKey:      appkey,
				UserId:      userId,
				TargetId:    msg.TargetId,
				ChannelType: msg.ChannelType,
				SubChannel:  msg.SubChannel,
				LatestMsgId: msg.MsgId,
				// LatestMsg: msg,
				SortTime:             sortTime,
				SyncTime:             msg.MsgTime,
				LatestUnreadMsgIndex: unreadIndex,
				ConverExts: &pbobjs.ConverExts{
					ConverTags:       toUserConverTags(msg.ConverTags),
					GlobalConverTags: toGlobalConverTags(msg.ConverTags),
				},
			})
			if !msg.IsSend {
				HandleMentionedMsg(appkey, userId, msg, userConvers)
			}
			//save to db
			userConvers.PersistConverV2(newConver)
		} else {
			UpsertOfflineConversation(&ConversationCacheItem{
				Appkey:      appkey,
				UserId:      userId,
				TargetId:    msg.TargetId,
				ChannelType: msg.ChannelType,
				SubChannel:  msg.SubChannel,
				LatestMsgId: msg.MsgId,
				SortTime:    sortTime,
				SyncTime:    msg.MsgTime,
				UnReadIndex: unreadIndex,
				ConverExts: &pbobjs.ConverExts{
					ConverTags:       toUserConverTags(msg.ConverTags),
					GlobalConverTags: toGlobalConverTags(msg.ConverTags),
				},
			})
			if !msg.IsSend {
				HandleMentionedMsg(appkey, userId, msg, nil)
			}
		}
	}
}

func toGlobalConverTags(tags []*pbobjs.ConverTag) map[string]bool {
	m := make(map[string]bool)
	for _, tag := range tags {
		if tag.TagType == pbobjs.ConverTagType_GlobalConverTag {
			m[tag.Tag] = true
		}
	}
	return m
}

func toUserConverTags(tags []*pbobjs.ConverTag) map[string]bool {
	m := make(map[string]bool)
	for _, tag := range tags {
		if tag.TagType == pbobjs.ConverTagType_UserConverTag {
			m[tag.Tag] = true
		}
	}
	return m
}

func SaveNilConversationV2(ctx context.Context, appkey string, userId string, targetId, subChannel string, channelType pbobjs.ChannelType) (errs.IMErrorCode, *pbobjs.Conversation) {
	sortTime := time.Now().UnixMilli()
	resp := &pbobjs.Conversation{
		UserId:      userId,
		TargetId:    targetId,
		SubChannel:  subChannel,
		ChannelType: channelType,
		SortTime:    sortTime,
		SyncTime:    sortTime,
	}
	var grpInfo *GroupInfo
	var targetUserInfo *UserInfo
	if channelType == pbobjs.ChannelType_Private {
		resp.TargetUserInfo = commonservices.GetTargetDisplayUserInfo(ctx, targetId)
		targetUserInfo = &UserInfo{
			UserId:       resp.TargetUserInfo.UserId,
			Nickname:     resp.TargetUserInfo.Nickname,
			UserPortrait: resp.TargetUserInfo.UserPortrait,
			ExtFields:    commonservices.Kvitems2Map(resp.TargetUserInfo.ExtFields),
			UpdatedTime:  resp.TargetUserInfo.UpdatedTime,
		}
	} else if channelType == pbobjs.ChannelType_Group {
		resp.GroupInfo = commonservices.GetGroupInfoFromCache(ctx, targetId)
		grpInfo = &GroupInfo{
			GroupId:       resp.GroupInfo.GroupId,
			GroupName:     resp.GroupInfo.GroupName,
			GroupPortrait: resp.GroupInfo.GroupPortrait,
			UpdatedTime:   resp.GroupInfo.UpdatedTime,
			ExtFields:     commonservices.Kvitems2Map(resp.GroupInfo.ExtFields),
		}
	}
	userConvers := getUserConvers(appkey, userId)
	newConver := userConvers.UpsertCovner(models.Conversation{
		AppKey:      appkey,
		UserId:      userId,
		TargetId:    targetId,
		ChannelType: channelType,
		SortTime:    sortTime,
		SyncTime:    sortTime,
	})
	//save to db
	userConvers.PersistConverV2(newConver)
	//notify other device
	addConver := &AddNilConver{
		Conversation: &Conversation{
			TargetId:       targetId,
			ChannelType:    int32(channelType),
			SortTime:       &sortTime,
			SyncTime:       &sortTime,
			TargetUserInfo: targetUserInfo,
			GroupInfo:      grpInfo,
		},
	}
	flag := msgdefines.SetCmdMsg(0)
	bs, _ := json.Marshal(addConver)
	commonservices.AsyncPrivateMsg(ctx, userId, userId, &pbobjs.UpMsg{
		MsgType:    addConverMsgType,
		MsgContent: bs,
		Flags:      flag,
	})
	return errs.IMErrorCode_SUCCESS, resp
}

func SyncConversationsV2(ctx context.Context, appkey, userId string, startTime int64, count int32) (errs.IMErrorCode, *pbobjs.QryConversationsResp) {
	if count < 0 || count == math.MaxInt32 || appkey != bases.GetAppKeyFromCtx(ctx) {
		return errs.IMErrorCode_MSG_PARAM_ILLEGAL, nil
	}
	resp := &pbobjs.QryConversationsResp{
		Conversations: []*pbobjs.Conversation{},
	}
	userConvers := getUserConvers(appkey, userId)
	code, userConvers, visible := nativeConverVisibility(ctx, userConvers)
	if code != 0 {
		return code, nil
	}
	convers := userConvers.SyncConvers(startTime, count+1, visible)
	if len(convers) > int(count) {
		convers = convers[:count]
	} else {
		resp.IsFinished = true
	}
	code, conversations := fillConvers(ctx, userId, convers, userConvers)
	if code != 0 {
		return code, nil
	}
	resp.Conversations = append(resp.Conversations, conversations...)
	return errs.IMErrorCode_SUCCESS, resp
}

func QryConverV2(ctx context.Context, userId string, req *pbobjs.QryConverReq) (errs.IMErrorCode, *pbobjs.Conversation) {
	appkey := bases.GetAppKeyFromCtx(ctx)
	if code := bases.NativePolicyCode(bases.CheckNativeMessage(ctx, appkey, req, []string{userId}, nil)); code != 0 {
		return code, nil
	}
	userConvers := getUserConvers(appkey, userId)
	conver := userConvers.QryConver(req.TargetId, req.SubChannel, req.ChannelType)
	if conver == nil {
		return errs.IMErrorCode_SUCCESS, &pbobjs.Conversation{
			TargetId:    req.TargetId,
			ChannelType: req.ChannelType,
		}
	}
	if req.IsInner {
		converTags := []*pbobjs.ConverTag{}
		if conver.ConverExts != nil {
			for tag := range conver.ConverExts.ConverTags {
				converTags = append(converTags, &pbobjs.ConverTag{
					Tag:     tag,
					TagType: pbobjs.ConverTagType_UserConverTag,
				})
			}
			for tag := range conver.ConverExts.GlobalConverTags {
				converTags = append(converTags, &pbobjs.ConverTag{
					Tag:     tag,
					TagType: pbobjs.ConverTagType_GlobalConverTag,
				})
			}
		}

		return errs.IMErrorCode_SUCCESS, &pbobjs.Conversation{
			TargetId:          req.TargetId,
			ChannelType:       req.ChannelType,
			UndisturbType:     conver.UndisturbType,
			LatestUnreadIndex: conver.LatestUnreadMsgIndex,
			LatestReadIndex:   conver.LatestReadMsgIndex,
			LatestReadMsgId:   conver.LatestReadMsgId,
			LatestReadMsgTime: conver.LatestReadMsgTime,
			ConverTags:        converTags,
		}
	} else {
		code, conversations := fillConvers(ctx, userId, []*models.Conversation{conver}, userConvers)
		if code != 0 {
			return code, nil
		}
		if len(conversations) <= 0 {
			return errs.IMErrorCode_SUCCESS, &pbobjs.Conversation{
				TargetId:    req.TargetId,
				ChannelType: req.ChannelType,
			}
		}
		return errs.IMErrorCode_SUCCESS, conversations[0]
	}
}

func BatchQryConversV2(ctx context.Context, req *pbobjs.QryConverReq) (errs.IMErrorCode, *pbobjs.QryConversationsResp) {
	appkey := bases.GetAppKeyFromCtx(ctx)
	if code := bases.NativePolicyCode(bases.CheckNativeMessage(ctx, appkey, req, nil, nil)); code != 0 {
		return code, nil
	}
	ret := &pbobjs.QryConversationsResp{
		Conversations: []*pbobjs.Conversation{},
	}
	for _, uId := range req.UserIds {
		userConvers := getUserConvers(appkey, uId)
		conver := userConvers.QryConver(req.TargetId, req.SubChannel, req.ChannelType)
		if conver != nil {
			ret.Conversations = append(ret.Conversations, &pbobjs.Conversation{
				UserId:            conver.UserId,
				TargetId:          conver.TargetId,
				ChannelType:       conver.ChannelType,
				LatestUnreadIndex: conver.LatestUnreadMsgIndex,
				LatestReadMsgTime: conver.LatestReadMsgTime,
				UndisturbType:     conver.UndisturbType,
			})
		}
	}
	return errs.IMErrorCode_SUCCESS, ret
}

func QryConversationsV2(ctx context.Context, req *pbobjs.QryConversationsReq) (errs.IMErrorCode, *pbobjs.QryConversationsResp) {
	if req == nil || req.Count < 0 || req.Count == math.MaxInt32 {
		return errs.IMErrorCode_MSG_PARAM_ILLEGAL, nil
	}
	appkey := bases.GetAppKeyFromCtx(ctx)
	userId := bases.GetRequesterIdFromCtx(ctx)
	startTime := req.StartTime
	isPositiveOrder := false
	if req.Order == 0 {
		if startTime <= 0 {
			startTime = time.Now().UnixMilli()
		}
	} else {
		isPositiveOrder = true
	}
	count := req.Count
	resp := &pbobjs.QryConversationsResp{
		Conversations: []*pbobjs.Conversation{},
	}
	userConvers := getUserConvers(appkey, userId)
	code, userConvers, visible := nativeConverVisibility(ctx, userConvers)
	if code != 0 {
		return code, nil
	}
	convers := userConvers.QryConvers(startTime, count+1, isPositiveOrder, req.TargetId, req.ChannelType, req.Tag, req.ExceptTag, visible)
	if len(convers) > int(count) {
		convers = convers[:count]
	} else {
		resp.IsFinished = true
	}
	code, conversations := fillConvers(ctx, userId, convers, userConvers)
	if code != 0 {
		return code, nil
	}
	resp.Conversations = append(resp.Conversations, conversations...)
	if isPositiveOrder {
		sort.Slice(resp.Conversations, func(i, j int) bool {
			return resp.Conversations[i].SortTime > resp.Conversations[j].SortTime
		})
	}
	return errs.IMErrorCode_SUCCESS, resp
}

func ClearTotalUnreadV2(ctx context.Context, userId string) errs.IMErrorCode {
	appkey := bases.GetAppKeyFromCtx(ctx)
	userConvers := getUserConvers(appkey, userId)
	if err := userConvers.ClearTotalUnread(); err != nil {
		return errs.IMErrorCode_API_INTERNAL_RESP_FAIL
	}
	//clear mention msgs
	mentionStorage := storages.NewMentionMsgStorage()
	mentionStorage.CleanMentionMsgsBaseUserId(appkey, userId)
	commonservices.AsyncPrivateMsg(ctx, userId, userId, &pbobjs.UpMsg{
		MsgType:    msgdefines.CmdMsgType_ClearTotalUnread,
		MsgContent: []byte(fmt.Sprintf(`{"clear_time":%d}`, time.Now().UnixMilli())),
		Flags:      msgdefines.SetCmdMsg(0),
	})
	return errs.IMErrorCode_SUCCESS
}

func ClearUnreadV2(ctx context.Context, userId string, convers []*pbobjs.Conversation, noCmdMsg bool) errs.IMErrorCode {
	appkey := bases.GetAppKeyFromCtx(ctx)
	if len(convers) > 0 {
		clearUnreadConvers := &ClearUnreadConvers{
			Conversations: []*ClearUnreadConver{},
		}
		userConvers := getUserConvers(appkey, userId)
		affected := false
		for _, conver := range convers {
			var msgIndex int64 = conver.LatestReadIndex
			clearUnreadConvers.Conversations = append(clearUnreadConvers.Conversations, &ClearUnreadConver{
				TargetId:           conver.TargetId,
				ChannelType:        int32(conver.ChannelType),
				LatestReadMsgIndex: msgIndex,
				SubChannel:         conver.SubChannel,
			})
			if msgIndex > 0 {
				readMsgId := conver.LatestReadMsgId
				readMsgTime := conver.LatestReadMsgTime
				if conver.LatestReadMsgTime <= 0 {
					readMsgTime = time.Now().UnixMilli()
				}
				rowsAffected := userConvers.ClearUnread(conver.TargetId, conver.SubChannel, conver.ChannelType, msgIndex, readMsgId, readMsgTime)
				if rowsAffected {
					affected = true
					// userConvers.PersistConver(conver.TargetId, conver.SubChannel, conver.ChannelType)
					newConver := userConvers.QryConver(conver.TargetId, conver.SubChannel, conver.ChannelType)
					userConvers.PersistConverV2(newConver)
				}
			} else {
				//TODO
				rowsAffected := userConvers.DefaultClearUnread(conver.TargetId, conver.SubChannel, conver.ChannelType)
				if rowsAffected {
					affected = true
					// userConvers.PersistConver(conver.TargetId, conver.SubChannel, conver.ChannelType)
					newConver := userConvers.QryConver(conver.TargetId, conver.SubChannel, conver.ChannelType)
					userConvers.PersistConverV2(newConver)
				}
			}
		}
		if affected && !noCmdMsg {
			//Notify other device to clear unread
			flag := msgdefines.SetCmdMsg(0)
			bs, _ := json.Marshal(clearUnreadConvers)
			exts := bases.GetExtsFromCtx(ctx)
			if len(convers) == 1 {
				exts[commonservices.RpcExtKey_UniqTag] = fmt.Sprintf("%s_%d", convers[0].TargetId, convers[0].ChannelType)
			}
			commonservices.AsyncPrivateMsg(ctx, userId, userId, &pbobjs.UpMsg{
				MsgType:    msgdefines.CmdMsgType_ClearUnread,
				MsgContent: bs,
				Flags:      flag,
			}, &bases.ExtsOption{Exts: exts})
		}
	}
	return errs.IMErrorCode_SUCCESS
}

func DelConversationV2(ctx context.Context, userId string, convers []*pbobjs.Conversation) errs.IMErrorCode {
	appkey := bases.GetAppKeyFromCtx(ctx)
	if len(convers) > 0 {
		delConvers := &DelConvers{
			Conversations: []*Conversation{},
		}
		userConvers := getUserConvers(appkey, userId)
		for _, conver := range convers {
			delConvers.Conversations = append(delConvers.Conversations, &Conversation{
				TargetId:    conver.TargetId,
				ChannelType: int32(conver.ChannelType),
			})
			affected := userConvers.DelConversation(conver.TargetId, conver.SubChannel, conver.ChannelType)
			if affected {
				// userConvers.PersistConver(conver.TargetId, conver.SubChannel, conver.ChannelType)
				newConver := userConvers.QryConver(conver.TargetId, conver.SubChannel, conver.ChannelType)
				userConvers.PersistConverV2(newConver)
			}
		}
		// bases.AsyncRpcCall(ctx, "del_conver_cache", userId, &pbobjs.ConversationsReq{
		// 	Conversations: convers,
		// })
		//notify other device
		flag := msgdefines.SetCmdMsg(0)
		bs, _ := json.Marshal(delConvers)
		commonservices.AsyncPrivateMsg(ctx, userId, userId, &pbobjs.UpMsg{
			MsgType:    delConversMsgType,
			MsgContent: bs,
			Flags:      flag,
		})
	}
	return errs.IMErrorCode_SUCCESS
}

func MarkUnreadV2(ctx context.Context, userId string, req *pbobjs.ConversationsReq) errs.IMErrorCode {
	appkey := bases.GetAppKeyFromCtx(ctx)
	if len(req.Conversations) > 0 {
		markUnreadConvers := &MarkUnreadConvers{
			Conversations: []*MarkUnreadConver{},
		}
		userConvers := getUserConvers(appkey, userId)
		affected := false
		for _, conver := range req.Conversations {
			markUnreadConvers.Conversations = append(markUnreadConvers.Conversations, &MarkUnreadConver{
				TargetId:    conver.TargetId,
				ChannelType: int32(conver.ChannelType),
				UnreadTag:   int(conver.UnreadTag),
			})
			rowsAffected := userConvers.UpdateUnreadTag(conver.TargetId, conver.SubChannel, conver.ChannelType, int(conver.UnreadTag))
			if rowsAffected {
				// userConvers.PersistConver(conver.TargetId, conver.SubChannel, conver.ChannelType)
				newConver := userConvers.QryConver(conver.TargetId, conver.SubChannel, conver.ChannelType)
				userConvers.PersistConverV2(newConver)
				affected = true
			}
		}
		if affected {
			//Notify other device to mark unread
			flag := msgdefines.SetCmdMsg(0)
			bs, _ := json.Marshal(markUnreadConvers)
			commonservices.AsyncPrivateMsg(ctx, userId, userId, &pbobjs.UpMsg{
				MsgType:    msgdefines.CmdMsgType_MarkUnread,
				MsgContent: bs,
				Flags:      flag,
			})
		}
	}
	return errs.IMErrorCode_SUCCESS
}

func MarkMentionRead(ctx context.Context, userId string, req *pbobjs.MarkReadReq) errs.IMErrorCode {
	appkey := bases.GetAppKeyFromCtx(ctx)
	if len(req.Msgs) > 0 && UserConversContains(appkey, userId) {
		userConvers := getUserConvers(appkey, userId)
		msgIds := map[string]bool{}
		for _, msg := range req.Msgs {
			msgIds[msg.MsgId] = true
		}
		userConvers.MarkMentionRead(req.TargetId, req.SubChannel, req.ChannelType, msgIds)
	}
	return errs.IMErrorCode_SUCCESS
}

func SetTopConversV2(ctx context.Context, req *pbobjs.ConversationsReq) (errs.IMErrorCode, int64) {
	appkey := bases.GetAppKeyFromCtx(ctx)
	userId := bases.GetRequesterIdFromCtx(ctx)
	topConvers := &TopConvers{
		Conversations: []*Conversation{},
	}
	var cmdMsgTime int64 = 0
	currTime := time.Now().UnixMilli()
	if len(req.Conversations) > 0 {
		userConvers := getUserConvers(appkey, userId)
		for _, conver := range req.Conversations {
			var t int64 = 0
			if conver.IsTop > 0 {
				t = currTime
			}
			affected := userConvers.UpdTopState(conver.TargetId, conver.SubChannel, conver.ChannelType, int(conver.IsTop), t)
			if affected {
				// userConvers.PersistConver(conver.TargetId, conver.SubChannel, conver.ChannelType)
				newConver := userConvers.QryConver(conver.TargetId, conver.SubChannel, conver.ChannelType)
				userConvers.PersistConverV2(newConver)
			}
			topConvers.Conversations = append(topConvers.Conversations, &Conversation{
				TargetId:      conver.TargetId,
				ChannelType:   int32(conver.ChannelType),
				IsTop:         conver.IsTop,
				TopUpdateTime: t,
			})
		}
		//notify other device
		flag := msgdefines.SetCmdMsg(0)
		bs, _ := json.Marshal(topConvers)
		code, _, msgTime, _ := commonservices.SyncPrivateMsg(ctx, userId, userId, &pbobjs.UpMsg{
			MsgType:    topConversMsgType,
			MsgContent: bs,
			Flags:      flag,
		})
		if code == errs.IMErrorCode_SUCCESS {
			cmdMsgTime = msgTime
		}
	}
	return errs.IMErrorCode_SUCCESS, cmdMsgTime
}

func QryTopConversV2(ctx context.Context, userId string, req *pbobjs.QryTopConversReq) (errs.IMErrorCode, *pbobjs.QryConversationsResp) {
	appkey := bases.GetAppKeyFromCtx(ctx)
	resp := &pbobjs.QryConversationsResp{
		Conversations: []*pbobjs.Conversation{},
		IsFinished:    true,
	}
	userConvers := getUserConvers(appkey, userId)
	code, userConvers, visible := nativeConverVisibility(ctx, userConvers)
	if code != 0 {
		return code, nil
	}
	convers := userConvers.QryTopConvers(req.StartTime, 101, req.SortType, req.Order > 0, visible)
	code, conversations := fillConvers(ctx, userId, convers, userConvers)
	if code != 0 {
		return code, nil
	}
	resp.Conversations = append(resp.Conversations, conversations...)
	if len(resp.Conversations) > 100 {
		resp.Conversations = resp.Conversations[:100]
		resp.IsFinished = false
	}
	return errs.IMErrorCode_SUCCESS, resp
}

func QryTotalUnreadCountV2(ctx context.Context, userId string, req *pbobjs.QryTotalUnreadCountReq) (errs.IMErrorCode, *pbobjs.QryTotalUnreadCountResp) {
	if req == nil {
		return errs.IMErrorCode_MSG_PARAM_ILLEGAL, nil
	}
	appkey := bases.GetAppKeyFromCtx(ctx)
	userConvers := getUserConvers(appkey, userId)
	code, userConvers, visible := nativeConverVisibility(ctx, userConvers)
	if code != 0 {
		return code, nil
	}
	return errs.IMErrorCode_SUCCESS, &pbobjs.QryTotalUnreadCountResp{
		TotalCount: userConvers.totalUnreadCount(req.Filter, visible),
	}
}

func UndisturbConversV2(ctx context.Context, req *pbobjs.UndisturbConversReq) errs.IMErrorCode {
	appkey := bases.GetAppKeyFromCtx(ctx)
	userId := bases.GetTargetIdFromCtx(ctx)
	convers := &UndisturbConvers{
		Conversations: []*UndisturbConver{},
	}
	userConvers := getUserConvers(appkey, userId)
	needUnCacheConvers := []*pbobjs.Conversation{}
	for _, item := range req.Items {
		affected := userConvers.UpdateUndisturbType(item.TargetId, item.SubChannel, item.ChannelType, item.UndisturbType)
		if affected {
			// userConvers.PersistConver(item.TargetId, item.SubChannel, item.ChannelType)
			newConver := userConvers.QryConver(item.TargetId, item.SubChannel, item.ChannelType)
			userConvers.PersistConverV2(newConver)
			needUnCacheConvers = append(needUnCacheConvers, &pbobjs.Conversation{
				TargetId:    item.TargetId,
				ChannelType: item.ChannelType,
				SubChannel:  item.SubChannel,
			})
		}
		convers.Conversations = append(convers.Conversations, &UndisturbConver{
			TargetId:      item.TargetId,
			ChannelType:   int32(item.ChannelType),
			UndisturbType: item.UndisturbType,
			SubChannel:    item.SubChannel,
		})
	}
	if len(needUnCacheConvers) > 0 {
		bases.AsyncRpcCall(ctx, "del_conver_cache", userId, &pbobjs.ConversationsReq{
			Conversations: needUnCacheConvers,
		})
	}
	//Notify other device to update undisturb
	flag := msgdefines.SetCmdMsg(0)
	bs, _ := json.Marshal(convers)
	commonservices.AsyncPrivateMsg(ctx, userId, userId, &pbobjs.UpMsg{
		MsgType:    undisturbMsgType,
		MsgContent: bs,
		Flags:      flag,
	})
	return errs.IMErrorCode_SUCCESS
}

func fillConvers(ctx context.Context, userId string, convers []*models.Conversation, userConvers *UserConversations) (errs.IMErrorCode, []*pbobjs.Conversation) {
	appkey := bases.GetAppKeyFromCtx(ctx)
	retConvers := []*pbobjs.Conversation{}

	for _, conver := range convers {
		if conver.IsDeleted == 0 && conver.MentionInfo == nil {
			conver.MentionInfo = userConvers.GetMentionInfo(conver.TargetId, conver.SubChannel, conver.ChannelType)
		}
	}
	if err := prepareNativeReadRows(ctx, appkey, userId, convers); err != nil {
		return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, nil
	}
	for _, conver := range convers {
		unreadCount := conver.LatestUnreadMsgIndex - conver.LatestReadMsgIndex
		conversation := &pbobjs.Conversation{
			UserId:            conver.UserId,
			TargetId:          conver.TargetId,
			ChannelType:       conver.ChannelType,
			SubChannel:        conver.SubChannel,
			SortTime:          conver.SortTime,
			SyncTime:          conver.SyncTime,
			UnreadCount:       unreadCount,
			IsTop:             int32(conver.IsTop),
			TopUpdatedTime:    conver.TopUpdatedTime,
			UndisturbType:     conver.UndisturbType,
			LatestUnreadIndex: conver.LatestUnreadMsgIndex,
			LatestReadIndex:   conver.LatestReadMsgIndex,
			IsDelete:          int32(conver.IsDeleted),
			UnreadTag:         int32(conver.UnreadTag),
		}
		//add latest msg
		var downMsg *pbobjs.DownMsg
		if len(conver.LatestMsg) > 0 {
			downMsg = &pbobjs.DownMsg{}
			if err := proto.Unmarshal(conver.LatestMsg, downMsg); err != nil {
				return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, nil
			}
		}
		if conver.ChannelType == pbobjs.ChannelType_Private {
			if downMsg != nil {
				conversation.TargetUserInfo = downMsg.TargetUserInfo
				conversation.FriendInfo = downMsg.FriendInfo
				if userId == downMsg.SenderId {
					downMsg.IsSend = true
					downMsg.TargetId = conver.TargetId
					// conversation.TargetUserInfo = commonservices.GetTargetDisplayUserInfo(ctx, downMsg.TargetId)
					downMsg.TargetUserInfo = downMsg.ReceiverInfo
					downMsg.FriendInfo = downMsg.SenderFriendInfo
					conversation.TargetUserInfo = downMsg.ReceiverInfo
					conversation.FriendInfo = downMsg.SenderFriendInfo
				}
			}
			if conversation.TargetUserInfo == nil {
				conversation.TargetUserInfo = commonservices.GetTargetDisplayUserInfo(ctx, conver.TargetId)
			}
		} else if conver.ChannelType == pbobjs.ChannelType_Group {
			if downMsg != nil {
				conversation.GroupInfo = downMsg.GroupInfo
				conversation.TargetUserInfo = downMsg.TargetUserInfo
			}
			if conversation.GroupInfo == nil {
				conversation.GroupInfo = commonservices.GetGroupInfoFromCache(ctx, conver.TargetId)
			}
		}
		conversation.Msg = downMsg
		if conver.IsDeleted == 0 {
			//target userinfo/groupinfo
			// if conversation.ChannelType == pbobjs.ChannelType_Private {
			// 	conversation.TargetUserInfo = commonservices.GetTargetDisplayUserInfo(ctx, conversation.TargetId)
			// } else if conversation.ChannelType == pbobjs.ChannelType_Group {
			// 	conversation.GroupInfo = commonservices.GetGroupInfoFromCache(ctx, conversation.TargetId)
			// }
			//mentions
			mentionInfo := conver.MentionInfo
			if mentionInfo == nil {
				return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, nil
			}
			if mentionInfo != nil {
				conversation.Mentions = &pbobjs.Mentions{
					IsMentioned:     mentionInfo.IsMentioned,
					MentionMsgCount: int32(mentionInfo.MentionMsgCount),
					Senders:         []*pbobjs.UserInfo{},
					MentionMsgs:     []*pbobjs.MentionMsg{},
				}
				for _, senderId := range mentionInfo.SenderIds {
					userInfo := commonservices.GetTargetDisplayUserInfo(ctx, senderId)
					conversation.Mentions.Senders = append(conversation.Mentions.Senders, userInfo)
				}
				for _, mentionMsg := range mentionInfo.MentionMsgs {
					conversation.Mentions.MentionMsgs = append(conversation.Mentions.MentionMsgs, &pbobjs.MentionMsg{
						SenderId:    mentionMsg.SenderId,
						MsgId:       mentionMsg.MsgId,
						MsgTime:     mentionMsg.MsgTime,
						MentionType: mentionMsg.MentionType,
					})
				}
			}
			//conver tags
			if conver.ConverExts != nil {
				conversation.ConverTags = []*pbobjs.ConverTag{}
				for tag := range conver.ConverExts.ConverTags {
					conversation.ConverTags = append(conversation.ConverTags, &pbobjs.ConverTag{
						Tag:     tag,
						TagType: pbobjs.ConverTagType_UserConverTag,
					})
				}
				for tag := range conver.ConverExts.GlobalConverTags {
					conversation.ConverTags = append(conversation.ConverTags, &pbobjs.ConverTag{
						Tag:     tag,
						TagType: pbobjs.ConverTagType_GlobalConverTag,
					})
				}
			}
		}
		if code := bases.NativePolicyCode(bases.CheckNativeMessage(ctx, appkey, conversation, []string{userId}, nil)); code != 0 {
			return code, nil
		}
		switch conversation.ChannelType {
		case pbobjs.ChannelType_Private:
			conversation.Business = &pbobjs.ConversationBusiness{Type: "normal_dm"}
		case pbobjs.ChannelType_Group:
			conversation.Business = &pbobjs.ConversationBusiness{Type: "normal_group"}
		}
		retConvers = append(retConvers, conversation)
	}
	return errs.IMErrorCode_SUCCESS, retConvers
}
