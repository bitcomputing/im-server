package bases

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"google.golang.org/protobuf/proto"
	"im-server/commons/errs"
	"im-server/commons/gmicro/actorsystem"
	"im-server/commons/mindlink"
	"im-server/commons/pbdefines/pbobjs"
)

func TestNativePolicyRejectsInvalidInputs(t *testing.T) {
	if code := NativePolicyCode(CheckNativeIdentities(nil, "fixture-app", nil, nil)); code != errs.IMErrorCode_MINDLINK_INVALID_REQUEST {
		t.Fatal("nil context did not fail closed")
	}
	if code := NativePolicyCode(CheckNativeMessage(context.Background(), "fixture-app", nil, nil, nil)); code != errs.IMErrorCode_MINDLINK_INVALID_REQUEST {
		t.Fatal("nil message did not fail closed")
	}
	if NativePolicyCode(mindlink.ErrForbidden) != errs.IMErrorCode_MINDLINK_FORBIDDEN || NativePolicyCode(mindlink.ErrUnknownResult) != errs.IMErrorCode_MINDLINK_UNKNOWN_RESULT {
		t.Fatal("native denial mapping became success")
	}
}

func TestNativeRequestIdentityNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name, method, service, route, actual string
		msg                                  proto.Message
		users, groups                        []string
	}{
		{"private_hashed_route", "p_msg", "message", "conversation-hash", "peer", &pbobjs.UpMsg{}, []string{"owner", "peer"}, nil},
		{"group_route", "g_msg", "group", "same-id", "", &pbobjs.UpMsg{MsgContent: []byte("opaque-user")}, []string{"owner"}, []string{"same-id"}},
		{"group_settings_route", "set_grp_member_setting", "group", "room", "", &pbobjs.QryGrpMemberSettingsReq{MemberId: "member"}, []string{"owner", "member"}, []string{"room"}},
		{"group_members", "g_add_members", "group", "room", "", &pbobjs.GroupMembersReq{GroupId: "room", MemberIds: []string{"member"}}, []string{"owner", "member"}, []string{"room"}},
		{"private_history", "add_hismsg", "historymsg", "hash", "", &pbobjs.AddHisMsgReq{SenderId: "sender", TargetId: "peer", ChannelType: pbobjs.ChannelType_Private}, []string{"owner", "sender", "peer"}, nil},
		{"group_history", "add_hismsg", "historymsg", "hash", "", &pbobjs.AddHisMsgReq{SenderId: "sender", TargetId: "room", ChannelType: pbobjs.ChannelType_Group}, []string{"owner", "sender"}, []string{"room"}},
		{"stream_private_target", "send_stream_msg", "historymsg", "hash", "", &pbobjs.StreamMsg{TargetId: "peer"}, []string{"owner", "peer"}, nil},
		{"snake_case_user_ids", "user_tags", "usermanager", "route", "", &pbobjs.UserIds{UserIds: []string{"member"}}, []string{"owner", "member"}, nil},
		{"rtc_users", "rtc_invite", "rtcroom", "room", "", &pbobjs.RtcInviteReq{TargetIds: []string{"peer"}}, []string{"owner", "peer"}, nil},
		{"opaque_profile_text", "reg_user", "usermanager", "route", "", &pbobjs.UserInfo{UserId: "member", Nickname: "opaque-user", ExtFields: []*pbobjs.KvItem{{Key: "user_id", Value: "opaque-user"}}}, []string{"owner", "member"}, nil},
		{"bind_device_route", "add_bind_device", "connectmanager", "member", "", &pbobjs.BindDevice{DeviceId: "opaque-device"}, []string{"owner", "member"}, nil},
		{"user_settings_route", "get_user_settings", "usermanager", "member", "", &pbobjs.Nil{}, []string{"owner", "member"}, nil},
		{"friend_payload", "add_friends", "friendmanager", "owner", "", &pbobjs.FriendMembersReq{FriendMembers: []*pbobjs.FriendMember{{FriendId: "peer"}}}, []string{"owner", "peer"}, nil},
		{"favorite_group_receiver", "add_favorite_msgs", "historymsg", "owner", "", &pbobjs.FavoriteMsgIds{Items: []*pbobjs.FavoriteMsgIdItem{{SenderId: "sender", ReceiverId: "room", ChannelType: pbobjs.ChannelType_Group}}}, []string{"owner", "sender"}, []string{"room"}},
		{"global_group_config", "qry_g_conver_conf", "conversation", "room", "", &pbobjs.ConverConfReq{ConverId: "room", ChannelType: pbobjs.ChannelType_Group}, []string{"owner"}, []string{"room"}},
		{"rtc_bind_group_namespace", "rtc_bind_conver", "conversation", "same-id", "", &pbobjs.RtcConverBindReq{ConverId: "same-id", ChannelType: pbobjs.ChannelType_Group}, []string{"owner"}, []string{"same-id"}},
		{"rtc_room_namespace", "rtc_qry", "rtcroom", "opaque-room", "", &pbobjs.Nil{}, []string{"owner"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := CreateRpcCtx("fixture-app", "owner")
			ctx = setCtxValue(ctx, CtxKey_Method, tc.method)
			ctx = setCtxValue(ctx, CtxKey_TargetId, tc.route)
			ctx = setCtxValue(ctx, CtxKey_Exts, map[string]string{"real_targetid": tc.actual})
			users, groups := nativeRequestIdentities(ctx, tc.msg, tc.service)
			if !reflect.DeepEqual(identitySet(users), identitySet(tc.users)) || !reflect.DeepEqual(identitySet(groups), identitySet(tc.groups)) {
				t.Fatalf("selectors users=%v groups=%v, want users=%v groups=%v", users, groups, tc.users, tc.groups)
			}
		})
	}
}

func identitySet(ids []string) []string {
	set := map[string]bool{}
	for _, id := range ids {
		if id != "" {
			set[id] = true
		}
	}
	var result []string
	for id := range set {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

type nativeProbeActor struct {
	BaseActor
	calls int
}

func (a *nativeProbeActor) CreateInputObj() proto.Message            { return &pbobjs.UpMsg{} }
func (a *nativeProbeActor) OnReceive(context.Context, proto.Message) { a.calls++ }

type nativeProbeSender struct {
	actorsystem.DeadLetterActorRef
	reply *pbobjs.RpcMessageWraper
}

func (s *nativeProbeSender) Tell(msg proto.Message, _ actorsystem.ActorRef) {
	s.reply, _ = msg.(*pbobjs.RpcMessageWraper)
}

func TestDecodedNativeActorFailsClosedBeforeBusinessAndKeepsErrorDirection(t *testing.T) {
	for _, kind := range []pbobjs.RpcMsgType{pbobjs.RpcMsgType_UserPub, pbobjs.RpcMsgType_QueryMsg} {
		t.Run(kind.String(), func(t *testing.T) {
			business := &nativeProbeActor{}
			sender := &nativeProbeSender{}
			wrapped := BaseProcessActor(business, "message").(*baseProcessActor)
			wrapped.SetSender(sender)
			wrapped.OnReceive(context.Background(), &pbobjs.RpcMessageWraper{RpcMsgType: kind, AppKey: "fixture-app", RequesterId: "owner", Method: "p_msg", ReqIndex: 73, IsFromApi: true})
			if business.calls != 0 || sender.reply == nil || sender.reply.ResultCode != int32(errs.IMErrorCode_MINDLINK_NOT_READY) || sender.reply.ReqIndex != 73 || sender.reply.MsgId != "" || len(sender.reply.AppDataBytes) != 0 {
				t.Fatalf("uninitialized fence reached business or returned a fake success: calls=%d reply=%v", business.calls, sender.reply)
			}
			want := pbobjs.RpcMsgType_QueryAck
			if kind == pbobjs.RpcMsgType_UserPub {
				want = pbobjs.RpcMsgType_UserPubAck
			}
			if sender.reply.RpcMsgType != want {
				t.Fatal("denial changed ACK direction")
			}
		})
	}
	for _, kind := range []pbobjs.RpcMsgType{pbobjs.RpcMsgType_QueryAck, pbobjs.RpcMsgType_UserPubAck, pbobjs.RpcMsgType_ServerPub} {
		business := &nativeProbeActor{}
		wrapped := BaseProcessActor(business, "message").(*baseProcessActor)
		wrapped.OnReceive(context.Background(), &pbobjs.RpcMessageWraper{RpcMsgType: kind, AppKey: "fixture-app", Method: "connect"})
		if business.calls != 1 {
			t.Fatalf("fence swallowed a reply/delivery direction: %v", kind)
		}
	}
}
