package actors

import (
	"context"
	"fmt"
	"im-server/commons/bases"
	"im-server/commons/gmicro/actorsystem"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/commonservices/logs"
	"im-server/services/conversation/services"

	"google.golang.org/protobuf/proto"
)

type QryConversationsActor struct {
	bases.BaseActor
}

func (actor *QryConversationsActor) OnReceive(ctx context.Context, input proto.Message) {
	if qryConverReq, ok := input.(*pbobjs.QryConversationsReq); ok {
		requesterId := bases.GetRequesterIdFromCtx(ctx)
		logs.WithContext(ctx).Infof("user_id:%s\tstart:%d\tcount:%d\ttarget_id:%s\tchannel_type:%d\ttag:%s", requesterId, qryConverReq.StartTime, qryConverReq.Count, qryConverReq.TargetId, qryConverReq.ChannelType, qryConverReq.Tag)

		code, resp := services.QryConversationsV2(ctx, qryConverReq)
		qryAck := bases.CreateQueryAckWraper(ctx, code, resp)
		actor.Sender.Tell(qryAck, actorsystem.NoSender)
		if resp != nil {
			logs.WithContext(ctx).Infof("code:%d\tlen:%d", code, len(resp.Conversations))
		}
	} else {
		fmt.Println("p_msg, upMsg is illigal.")
	}
}

func (actor *QryConversationsActor) CreateInputObj() proto.Message {
	return &pbobjs.QryConversationsReq{}
}
