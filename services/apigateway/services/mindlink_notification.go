package services

import (
	"context"
	"google.golang.org/protobuf/proto"
	"im-server/commons/bases"
	"im-server/commons/gmicro/actorsystem"
	"im-server/commons/mindlink"
	"im-server/commons/pbdefines/pbobjs"
	"time"
)

type ownerHintAck struct {
	actorsystem.UntypedActor
	result chan bool
}

func (a *ownerHintAck) finish(ok bool) {
	select {
	case a.result <- ok:
	default:
	}
}
func (a *ownerHintAck) OnReceive(_ context.Context, input proto.Message) {
	r, ok := input.(*pbobjs.RpcMessageWraper)
	if !ok || r.RpcMsgType != pbobjs.RpcMsgType_ServerPubAck {
		a.finish(false)
		return
	}
	var status pbobjs.OnlineStatus
	a.finish(proto.Unmarshal(r.AppDataBytes, &status) == nil && status.Type == pbobjs.OnlineType_Online)
}
func (a *ownerHintAck) OnTimeout()                    { a.finish(false) }
func (a *ownerHintAck) CreateInputObj() proto.Message { return &pbobjs.RpcMessageWraper{} }

func SignalMindlinkOwner(ctx context.Context, appkey, owner string, syncTime int64) error {
	if ctx == nil || appkey == "" || owner == "" || syncTime <= 0 {
		return mindlink.ErrInvalidRequest
	}
	ack := &ownerHintAck{result: make(chan bool, 1)}
	rpc := bases.CreateServerPubWraper(bases.CreateRpcCtx(appkey, owner), owner, owner, "ntf", &pbobjs.Notify{Type: pbobjs.NotifyType_Msg, SyncTime: syncTime})
	bases.UnicastRouteWithCallback(rpc, ack, 3*time.Second)
	select {
	case ok := <-ack.result:
		if ok {
			return nil
		}
		return mindlink.ErrNotReady
	case <-ctx.Done():
		return ctx.Err()
	}
}
