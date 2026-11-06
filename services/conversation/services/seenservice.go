package services

import (
	"context"
	"errors"
	"math"
	"unicode/utf8"

	"im-server/commons/bases"
	"im-server/commons/errs"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/commonservices"
	"im-server/services/conversation/storages/dbs"
	groupdbs "im-server/services/group/dbs"
	hisstorages "im-server/services/historymsg/storages"

	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

func QryConverSeen(ctx context.Context, req *pbobjs.QryConverSeenReq) (errs.IMErrorCode, *pbobjs.ConverSeenState) {
	if req == nil {
		return errs.IMErrorCode_MSG_PARAM_ILLEGAL, nil
	}
	return converSeen(ctx, req.TargetId, req.SubChannel, req.ChannelType, 0, false)
}

func SetConverSeen(ctx context.Context, req *pbobjs.SetConverSeenReq) (errs.IMErrorCode, *pbobjs.ConverSeenState) {
	if req == nil {
		return errs.IMErrorCode_MSG_PARAM_ILLEGAL, nil
	}
	return converSeen(ctx, req.TargetId, req.SubChannel, req.ChannelType, req.SeenSeq, true)
}

func converSeen(ctx context.Context, target, sub string, channel pbobjs.ChannelType, seq int64, update bool) (errs.IMErrorCode, *pbobjs.ConverSeenState) {
	app, user := bases.GetAppKeyFromCtx(ctx), bases.GetRequesterIdFromCtx(ctx)
	if app == "" || user == "" || target == "" || !utf8.ValidString(app+user+target+sub) || utf8.RuneCountInString(app) > 20 || utf8.RuneCountInString(user) > 32 || utf8.RuneCountInString(target) > 32 || utf8.RuneCountInString(sub) > 32 || seq < 0 || (channel != pbobjs.ChannelType_Private && channel != pbobjs.ChannelType_Group) {
		return errs.IMErrorCode_MSG_PARAM_ILLEGAL, nil
	}
	code, latest := seenLatest(app, user, target, sub, channel)
	if code != errs.IMErrorCode_SUCCESS {
		return code, nil
	}
	dao := dbs.SeenDao{}
	current, err := dao.Find(app, user, target, sub, channel)
	if err != nil {
		return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, nil
	}
	if update {
		// A deleted/expired latest message may fall behind an already acknowledged position.
		if seq > latest && seq > current {
			return errs.IMErrorCode_MSG_PARAM_ILLEGAL, nil
		}
		if seq > current {
			if err := dao.Advance(app, user, target, sub, channel, seq); err != nil {
				return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, nil
			}
		}
		current, err = dao.Find(app, user, target, sub, channel)
		if err != nil {
			return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, nil
		}
	}
	return errs.IMErrorCode_SUCCESS, &pbobjs.ConverSeenState{SeenSeq: current, LatestSeq: latest, HasNew: latest > current}
}

func seenLatest(app, user, target, sub string, channel pbobjs.ChannelType) (errs.IMErrorCode, int64) {
	conver := commonservices.GetConversationId(user, target, channel)
	var cleanTime int64
	if channel == pbobjs.ChannelType_Group {
		member, err := (groupdbs.GroupMemberDao{}).Find(app, target, user)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.IMErrorCode_GROUP_NOTGROUPMEMBER, 0
		}
		if err != nil {
			return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, 0
		}
		groupExts, err := (groupdbs.GroupExtDao{}).QryExtFields(app, target)
		if err != nil {
			return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, 0
		}
		memberExts, err := (groupdbs.GroupMemberExtDao{}).QryExtFields(app, target, user)
		if err != nil {
			return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, 0
		}
		settings := map[string]string{}
		for _, ext := range groupExts {
			if ext.ItemType == int(commonservices.AttItemType_Setting) {
				settings[ext.ItemKey] = ext.ItemValue
			}
		}
		for _, ext := range memberExts {
			if ext.ItemType == int(commonservices.AttItemType_Setting) {
				settings[ext.ItemKey] = ext.ItemValue
			}
		}
		var policy commonservices.GrpMemberSettings
		commonservices.FillObjField(&policy, settings)
		if policy.HideGrpMsg {
			cleanTime = member.CreatedTime.UnixMilli()
		}
	}
	clean, err := hisstorages.NewHisMsgConverCleanTimeStorage().FindOne(app, conver, sub, channel)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && !errors.Is(err, mongo.ErrNoDocuments) {
		return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, 0
	}
	if clean != nil && clean.CleanTime > cleanTime {
		cleanTime = clean.CleanTime
	}
	userClean, err := hisstorages.NewHisMsgUserCleanTimeStorage().FindOne(app, user, target, sub, channel)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && !errors.Is(err, mongo.ErrNoDocuments) {
		return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, 0
	}
	if userClean != nil && userClean.CleanTime > cleanTime {
		cleanTime = userClean.CleanTime
	}
	if channel == pbobjs.ChannelType_Group {
		msgs, err := hisstorages.NewGroupHisMsgStorage().QryHisMsgsExcludeDel(app, conver, sub, user, math.MaxInt64, 1, false, cleanTime, nil)
		if err != nil {
			return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, 0
		}
		if len(msgs) > 0 {
			if msgs[0].MsgSeqNo <= 0 {
				return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, 0
			}
			return errs.IMErrorCode_SUCCESS, msgs[0].MsgSeqNo
		}
	} else {
		msgs, err := hisstorages.NewPrivateHisMsgStorage().QryHisMsgsExcludeDel(app, conver, sub, user, target, math.MaxInt64, 1, false, cleanTime, nil)
		if err != nil {
			return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, 0
		}
		if len(msgs) > 0 {
			if msgs[0].MsgSeqNo <= 0 {
				return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, 0
			}
			return errs.IMErrorCode_SUCCESS, msgs[0].MsgSeqNo
		}
	}
	return errs.IMErrorCode_SUCCESS, 0
}
