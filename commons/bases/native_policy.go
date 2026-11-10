package bases

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"im-server/commons/errs"
	"im-server/commons/gmicro/actorsystem"
	"im-server/commons/mindlink"
	"im-server/commons/pbdefines/pbobjs"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var nativePolicy atomic.Pointer[mindlink.NativeFence]

func InitNativePolicy(ctx context.Context, db *sql.DB) error {
	fence, err := mindlink.NewNativeFence(ctx, db)
	nativePolicy.Store(fence)
	return err
}

// A disabled public API never reopens internal identities on legacy protocols.
func CheckNativeIdentities(ctx context.Context, appkey string, users, groups []string) error {
	if ctx == nil {
		return mindlink.ErrInvalidRequest
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return nativePolicy.Load().Check(ctx, appkey, users, groups)
}

func CheckNativeMessage(ctx context.Context, appkey string, msg proto.Message, users, groups []string) error {
	if msg == nil || !msg.ProtoReflect().IsValid() {
		return mindlink.ErrInvalidRequest
	}
	users, groups = appendNativeIdentities(msg.ProtoReflect(), users, groups)
	return CheckNativeIdentities(ctx, appkey, users, groups)
}

// FilterNativeMessages classifies only supplied selectors; callers must load actual
// history/mentions and check requester/final authority separately.
func FilterNativeMessages(ctx context.Context, appkey string, messages []proto.Message) ([]bool, error) {
	if ctx == nil || appkey == "" {
		return nil, mindlink.ErrInvalidRequest
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	identities := make([]mindlink.NativeIdentities, len(messages))
	for i, msg := range messages {
		if msg == nil || !msg.ProtoReflect().IsValid() {
			return nil, mindlink.ErrInvalidRequest
		}
		if ctx.Err() != nil {
			return nil, mindlink.ErrUnknownResult
		}
		identities[i].Users, identities[i].Groups = appendNativeIdentities(msg.ProtoReflect(), nil, nil)
	}
	return nativePolicy.Load().Filter(ctx, appkey, identities)
}

// FilterOwnerInboxMessages only relaxes the read fence for exact persisted owner
// inbox entries. Login, publishing, membership and history fences remain strict.
func FilterOwnerInboxMessages(ctx context.Context, appkey, owner string, messages []*pbobjs.DownMsg) ([]*pbobjs.DownMsg, error) {
	if ctx == nil || owner == "" {
		return nil, mindlink.ErrInvalidRequest
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	inputs := make([]proto.Message, len(messages))
	for i, m := range messages {
		inputs[i] = m
	}
	mask, err := FilterNativeMessages(ctx, appkey, inputs)
	if err != nil {
		return nil, err
	}
	result := make([]*pbobjs.DownMsg, 0, len(messages))
	for i, m := range messages {
		allowed := mask[i]
		if !allowed {
			allowed, err = nativePolicy.Load().OwnerInboxReadable(ctx, appkey, owner, m)
			if err != nil {
				return nil, err
			}
		}
		if allowed {
			result = append(result, m)
		}
	}
	return result, nil
}

func NativePolicyCode(err error) errs.IMErrorCode {
	if err == nil {
		return errs.IMErrorCode_SUCCESS
	}
	switch {
	case errors.Is(err, mindlink.ErrForbidden):
		return errs.IMErrorCode_MINDLINK_FORBIDDEN
	case errors.Is(err, mindlink.ErrNotReady):
		return errs.IMErrorCode_MINDLINK_NOT_READY
	case errors.Is(err, mindlink.ErrInvalidRequest):
		return errs.IMErrorCode_MINDLINK_INVALID_REQUEST
	default:
		return errs.IMErrorCode_MINDLINK_UNKNOWN_RESULT
	}
}

func nativeRequestIdentities(ctx context.Context, msg proto.Message, service string) ([]string, []string) {
	users := []string{GetRequesterIdFromCtx(ctx)}
	groups := []string{GetGroupIdFromCtx(ctx)}
	method := GetMethodFromCtx(ctx)
	if service == "group" {
		groups = append(groups, GetTargetIdFromCtx(ctx))
	} else if method == "p_msg" || method == "imp_pri_msg" || method == "s_msg" {
		users = append(users, GetExtsFromCtx(ctx)["real_targetid"])
	} else if nativeRouteSelectsUser(service, method) {
		users = append(users, GetTargetIdFromCtx(ctx))
	}
	users = append(users, GetTargetIdsFromCtx(ctx)...)
	return appendNativeIdentities(msg.ProtoReflect(), users, groups)
}

func nativeRouteSelectsUser(service, method string) bool {
	switch method {
	case "add_bind_device", "del_bind_device", "qry_bind_devices", "set_user_settings", "get_user_settings", "set_user_status", "set_user_undisturb", "get_user_undisturb",
		"sendbox", "sync_msgs", "push_switch", "upd_push_status", "check_block_user", "msg_ack", "msg_dispatch", "user_push", "sync_friend_rels", "qry_friend_infos",
		"rtc_member_rooms", "rtc_grab_member", "rtc_sync_member":
		return true
	}
	switch service {
	case "friendmanager", "pushmanager", "botmsgmanager":
		return true
	case "conversation":
		return method != "qry_g_conver_conf" && method != "set_g_conver_conf" && method != "qry_global_convers" && method != "rtc_bind_conver"
	case "statussubscriptions":
		return method == "pub_user_status"
	}
	return false
}

func appendNativeIdentities(msg protoreflect.Message, users, groups []string) ([]string, []string) {
	channel := pbobjs.ChannelType_Unknown
	fields := msg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if normalizedNativeField(field) == "channeltype" && field.Kind() == protoreflect.EnumKind {
			channel = pbobjs.ChannelType(msg.Get(field).Enum())
		}
	}
	msg.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.IsMap() {
			if field.MapValue().Kind() == protoreflect.MessageKind {
				value.Map().Range(func(_ protoreflect.MapKey, v protoreflect.Value) bool {
					users, groups = appendNativeIdentities(v.Message(), users, groups)
					return true
				})
			}
			return true
		}
		kind := 0
		switch normalizedNativeField(field) {
		case "userid", "userids", "memberid", "memberids", "senderid", "ownerid", "operatorid", "fromuserid", "touserids", "friendid", "friendids", "blockuserid":
			kind = 1
		case "groupid", "groupids":
			kind = 2
		case "targetid", "targetids", "receiverid":
			if channel == pbobjs.ChannelType_Group {
				kind = 2
			}
			if channel == pbobjs.ChannelType_Private || channel == pbobjs.ChannelType_System || msg.Descriptor().Name() == "StreamMsg" || msg.Descriptor().Name() == "RtcInviteReq" {
				kind = 1
			}
		case "converid":
			if channel == pbobjs.ChannelType_Group {
				kind = 2
			}
		}
		consume := func(v protoreflect.Value) {
			if field.Kind() == protoreflect.MessageKind {
				users, groups = appendNativeIdentities(v.Message(), users, groups)
			} else if field.Kind() == protoreflect.StringKind {
				if kind == 1 {
					users = append(users, v.String())
				}
				if kind == 2 {
					groups = append(groups, v.String())
				}
			}
		}
		if field.IsList() {
			for i := 0; i < value.List().Len(); i++ {
				consume(value.List().Get(i))
			}
		} else {
			consume(value)
		}
		return true
	})
	return users, groups
}

func normalizedNativeField(field protoreflect.FieldDescriptor) string {
	return strings.ReplaceAll(strings.ToLower(string(field.Name())), "_", "")
}

func rejectNativeRequest(ctx context.Context, msg proto.Message, service string, sender actorsystem.ActorRef) bool {
	kind := GetRpcTypeFromCtx(ctx)
	if kind != pbobjs.RpcMsgType_UserPub && kind != pbobjs.RpcMsgType_QueryMsg && kind != pbobjs.RpcMsgType_ServerPub && GetMethodFromCtx(ctx) != "msg_ack" {
		return false
	}
	users, groups := nativeRequestIdentities(ctx, msg, service)
	code := NativePolicyCode(CheckNativeIdentities(ctx, GetAppKeyFromCtx(ctx), users, groups))
	if code == errs.IMErrorCode_SUCCESS {
		return false
	}
	if sender != nil {
		if kind == pbobjs.RpcMsgType_QueryMsg {
			sender.Tell(CreateQueryAckWraper(ctx, code, nil), actorsystem.NoSender)
		} else if kind == pbobjs.RpcMsgType_UserPub {
			sender.Tell(CreateUserPubAckWraper(ctx, code, "", 0, 0, "", nil), actorsystem.NoSender)
		}
	}
	return true
}
