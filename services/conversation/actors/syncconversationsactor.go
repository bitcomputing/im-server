package actors

import (
	"context"
	"im-server/commons/bases"
	"im-server/commons/gmicro/actorsystem"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/commonservices/logs"
	"im-server/services/conversation/services"

	"google.golang.org/protobuf/proto"
)

type SyncConversationsActor struct {
	bases.BaseActor
}

func (actor *SyncConversationsActor) OnReceive(ctx context.Context, input proto.Message) {
	if syncConverReq, ok := input.(*pbobjs.SyncConversationsReq); ok {
		appkey := bases.GetAppKeyFromCtx(ctx)
		userId := bases.GetRequesterIdFromCtx(ctx)
		startTime := syncConverReq.StartTime

		logs.WithContext(ctx).Infof("user_id:%s\tstart_time:%d\tcount:%d", userId, startTime, syncConverReq.Count)

		code, resp := services.SyncConversationsV2(ctx, appkey, userId, startTime, syncConverReq.Count)
		qryAck := bases.CreateQueryAckWraper(ctx, code, resp)
		actor.Sender.Tell(qryAck, actorsystem.NoSender)

		if resp != nil {
			logs.WithContext(ctx).Infof("result_len:%d", len(resp.Conversations))
		}
	} else {
		logs.WithContext(ctx).Error("input is illigal.")
	}
}

func (actor *SyncConversationsActor) CreateInputObj() proto.Message {
	return &pbobjs.SyncConversationsReq{}
}
