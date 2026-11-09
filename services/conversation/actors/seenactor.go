package actors

import (
	"context"
	"google.golang.org/protobuf/proto"
	"im-server/commons/bases"
	"im-server/commons/errs"
	"im-server/commons/gmicro/actorsystem"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/conversation/services"
)

type QryConverSeenActor struct{ bases.BaseActor }

func (actor *QryConverSeenActor) OnReceive(ctx context.Context, input proto.Message) {
	code := errs.IMErrorCode_PBILLEGAL
	var state *pbobjs.ConverSeenState
	if req, ok := input.(*pbobjs.QryConverSeenReq); ok {
		code, state = services.QryConverSeen(ctx, req)
	}
	actor.Sender.Tell(bases.CreateQueryAckWraper(ctx, code, state), actorsystem.NoSender)
}
func (*QryConverSeenActor) CreateInputObj() proto.Message { return &pbobjs.QryConverSeenReq{} }

type SetConverSeenActor struct{ bases.BaseActor }

func (actor *SetConverSeenActor) OnReceive(ctx context.Context, input proto.Message) {
	code := errs.IMErrorCode_PBILLEGAL
	var state *pbobjs.ConverSeenState
	if req, ok := input.(*pbobjs.SetConverSeenReq); ok {
		code, state = services.SetConverSeen(ctx, req)
	}
	actor.Sender.Tell(bases.CreateQueryAckWraper(ctx, code, state), actorsystem.NoSender)
}
func (*SetConverSeenActor) CreateInputObj() proto.Message { return &pbobjs.SetConverSeenReq{} }
