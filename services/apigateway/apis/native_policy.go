package apis

import (
	"im-server/commons/bases"
	"im-server/commons/errs"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/commons/tools"
	"im-server/services/apigateway/models"
	"im-server/services/apigateway/services"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
)

// Admission must reject before an async handler promises a message ID or success.
func nativeHTTPAllowed(ctx *gin.Context, users, groups []string, msg proto.Message) bool {
	var err error
	if msg == nil {
		err = bases.CheckNativeIdentities(ctx.Request.Context(), services.GetAppkeyFromCtx(ctx), users, groups)
	} else {
		err = bases.CheckNativeMessage(ctx.Request.Context(), services.GetAppkeyFromCtx(ctx), msg, users, groups)
	}
	code := bases.NativePolicyCode(err)
	if code != errs.IMErrorCode_SUCCESS {
		tools.ErrorHttpResp(ctx, code)
		return false
	}
	return true
}

func nativeSendAllowed(ctx *gin.Context, req models.SendMsgReq, targets []string, channel pbobjs.ChannelType) bool {
	if channel != pbobjs.ChannelType_System && req.MentionInfo != nil && len(req.MentionInfo.TargetUserIds) == 0 {
		for _, user := range req.MentionInfo.TargetUsers {
			if user == nil {
				tools.ErrorHttpResp(ctx, errs.IMErrorCode_API_REQ_BODY_ILLEGAL)
				return false
			}
		}
	}
	users, groups := nativeHTTPIdentities(req.SenderId, targets, channel)
	msg := &pbobjs.UpMsg{PushData: handlePushData(req.PushData)}
	if channel != pbobjs.ChannelType_System {
		msg.MentionInfo, msg.ReferMsg = handleMentionInfo(req.MentionInfo), handleReferMsg(req.ReferMsg)
	}
	if channel == pbobjs.ChannelType_Group {
		msg.ToUserIds = req.ToUserIds
	}
	return nativeHTTPAllowed(ctx, users, groups, msg)
}

func nativeHTTPIdentities(sender string, targets []string, channel pbobjs.ChannelType) (users, groups []string) {
	users = []string{sender}
	if channel == pbobjs.ChannelType_Group {
		groups = targets
	} else if channel == pbobjs.ChannelType_Private || channel == pbobjs.ChannelType_System {
		users = append(users, targets...)
	}
	return
}
