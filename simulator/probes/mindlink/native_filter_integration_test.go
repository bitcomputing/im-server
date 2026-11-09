//go:build integration

package mindlink

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"im-server/commons/bases"
	policy "im-server/commons/mindlink"
	"im-server/commons/pbdefines/pbobjs"
)

func TestNativeMessageFilterIncludesNestedSourcesAndKeepsOpaqueText(t *testing.T) {
	db := nativeBoundaryDB(t)
	messages := []proto.Message{
		&pbobjs.Conversation{UserId: "42", TargetId: "43", ChannelType: pbobjs.ChannelType_Private, Msg: &pbobjs.DownMsg{SenderId: "42", TargetId: "43", ChannelType: pbobjs.ChannelType_Private, MsgContent: []byte(nativePrincipal)}},
		&pbobjs.Conversation{TargetId: nativePrincipal, ChannelType: pbobjs.ChannelType_Private},
		&pbobjs.Conversation{TargetId: "43", ChannelType: pbobjs.ChannelType_Private, Msg: &pbobjs.DownMsg{SenderId: strings.ToUpper(nativePrincipal), TargetId: "43", ChannelType: pbobjs.ChannelType_Private}},
		&pbobjs.Conversation{Msg: &pbobjs.DownMsg{SenderId: "42", TargetId: "43", ChannelType: pbobjs.ChannelType_Private, MentionInfo: &pbobjs.MentionInfo{TargetUsers: []*pbobjs.UserInfo{{UserId: nativePrincipal}}}}},
		&pbobjs.Conversation{TargetId: "protected-group", ChannelType: pbobjs.ChannelType_Group},
		&pbobjs.Conversation{TargetId: nativePrincipal, ChannelType: pbobjs.ChannelType_Group},
		&pbobjs.UserInfo{UserId: "42", Nickname: nativePrincipal, ExtFields: []*pbobjs.KvItem{{Key: "user_id", Value: nativePrincipal}}},
		&pbobjs.Nil{},
	}
	want := []bool{true, false, false, false, false, true, true, true}
	for _, state := range []string{"active", "disabled"} {
		if _, err := db.Exec("UPDATE mindlink_principals SET state=?", state); err != nil {
			t.Fatal(err)
		}
		mask, err := bases.FilterNativeMessages(context.Background(), nativeApp, messages)
		if err != nil || !reflect.DeepEqual(mask, want) {
			t.Fatal("bases classifier lost nested source/native ordinal semantics", mask, err)
		}
	}
	if mask, err := bases.FilterNativeMessages(context.Background(), nativeApp, nil); err != nil || len(mask) != 0 {
		t.Fatal("valid empty batch failed", mask, err)
	}
	if _, err := db.Exec("RENAME TABLE groupmembers TO _filter_unavailable_members"); err != nil {
		t.Fatal(err)
	}
	if mask, err := bases.FilterNativeMessages(context.Background(), nativeApp, messages); mask != nil || !errors.Is(err, policy.ErrUnknownResult) {
		t.Fatal("SQL failure returned a usable partial mask", mask, err)
	}
}
