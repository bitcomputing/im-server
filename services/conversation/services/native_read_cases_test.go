//go:build integration

package services

import (
	"context"
	"math"
	"strings"
	"testing"

	"im-server/commons/bases"
	"im-server/commons/errs"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/conversation/storages/models"

	"google.golang.org/protobuf/proto"
)

func TestNativeReadPaginationAndTombstones(t *testing.T) {
	_, ctx, uc := nativeReadFixture(t)
	for _, tc := range []struct {
		start    int64
		order    int32
		want     string
		finished bool
	}{
		{0, 0, "ordinary-peer", false},
		{98, 0, "ma222222222222222222222222222222", true},
		{0, 1, "ma222222222222222222222222222222", false},
		{97, 1, "ordinary-peer", true},
	} {
		code, resp := QryConversationsV2(ctx, &pbobjs.QryConversationsReq{StartTime: tc.start, Order: tc.order, Count: 1})
		if code != 0 || resp == nil || len(resp.Conversations) != 1 || resp.Conversations[0].TargetId != tc.want || resp.IsFinished != tc.finished {
			t.Fatalf("native pagination start=%d order=%d code=%d: %+v", tc.start, tc.order, code, resp)
		}
		if got := resp.Conversations[0].GetBusiness().GetType(); got != "normal_dm" {
			t.Fatalf("ordinary private conversation classification = %q", got)
		}
	}
	uc.ConverItemMap[getConverItemKey(listAvatar, "", pbobjs.ChannelType_Private)].IsDeleted = 1
	code, syncResp := SyncConversationsV2(ctx, "list-app", "42", 98, 10)
	if code != 0 || syncResp == nil || len(syncResp.Conversations) != 0 || !syncResp.IsFinished {
		t.Fatalf("protected sync tombstone leaked: code=%d %+v", code, syncResp)
	}
	// Ordinary tombstones must still synchronize even though they are absent from the list index.
	ordinary := uc.ConverItemMap[getConverItemKey("ordinary-peer", "", pbobjs.ChannelType_Private)]
	ordinary.IsDeleted = 1
	ordinary.LatestMsgId = ""
	ordinary.ChannelType = pbobjs.ChannelType_Chatroom
	oldKey := getConverItemKey("ordinary-peer", "", pbobjs.ChannelType_Private)
	newKey := getConverItemKey("ordinary-peer", "", pbobjs.ChannelType_Chatroom)
	delete(uc.ConverItemMap, oldKey)
	uc.ConverItemMap[newKey] = ordinary
	uc.SyncTimeIndex.Add(98, newKey)
	uc.SyncTimeIndex.Remove(oldKey)
	code, syncResp = SyncConversationsV2(ctx, "list-app", "42", 97, 10)
	if code != 0 || len(syncResp.Conversations) != 1 || syncResp.Conversations[0].IsDelete != 1 {
		t.Fatalf("ordinary tombstone lost: code=%d %+v", code, syncResp)
	}
}

func TestNativeReadUsesProjectionInsteadOfPrefixes(t *testing.T) {
	db, ctx, uc := nativeReadFixture(t)
	if _, err := db.Exec("UPDATE mindlink_principals SET state='disabled'"); err != nil {
		t.Fatal(err)
	}
	alias := strings.ToUpper(listAvatar)
	uc.ConverItemMap[getConverItemKey(alias, "alias", pbobjs.ChannelType_Private)] = &models.Conversation{TargetId: alias, SubChannel: "alias", ChannelType: pbobjs.ChannelType_Private, LatestUnreadMsgIndex: 80}
	if _, err := db.Exec("INSERT INTO mindlink_owner_barriers(environment,tenant_id,appkey,owner_id,read_enabled) VALUES('dev','other-tenant','other-app','42',TRUE); INSERT INTO mindlink_principals(environment,tenant_id,appkey,owner_id,principal_id,avatar_id,avatar_generation,policy_version,state) VALUES('dev','other-tenant','other-app','42','ma222222222222222222222222222222','other-avatar',1,1,'active')"); err != nil {
		t.Fatal(err)
	}
	code, resp := QryTotalUnreadCountV2(ctx, "42", &pbobjs.QryTotalUnreadCountReq{})
	if code != 0 || resp == nil || resp.TotalCount != 4 {
		t.Fatalf("disabled/alias/tenant/prefix classification incorrect: code=%d %+v", code, resp)
	}
	internal := &UserConversations{Appkey: "list-app", UserId: listAvatar, ConverItemMap: map[string]*models.Conversation{}}
	if code, _, _ := nativeConverVisibility(ctx, internal); code != errs.IMErrorCode_MINDLINK_FORBIDDEN {
		t.Fatalf("protected requester with an empty list must fail: %d", code)
	}
}

func TestNativeUnreadPreservesOrdinaryFiltersAndMarkers(t *testing.T) {
	_, ctx, uc := nativeReadFixture(t)
	peer := uc.ConverItemMap[getConverItemKey("ordinary-peer", "", pbobjs.ChannelType_Private)]
	peer.LatestReadMsgIndex = 1
	peer.ConverExts = &pbobjs.ConverExts{ConverTags: map[string]bool{"team": true}}
	marked := uc.ConverItemMap[getConverItemKey("ma222222222222222222222222222222", "", pbobjs.ChannelType_Private)]
	marked.LatestUnreadMsgIndex = 0
	marked.UnreadTag = 1
	for _, tc := range []struct {
		filter *pbobjs.ConverFilter
		want   int64
	}{
		{nil, 3},
		{&pbobjs.ConverFilter{Tag: "team"}, 2},
		{&pbobjs.ConverFilter{ExceptTag: "team"}, 1},
		{&pbobjs.ConverFilter{ChannelTypes: []pbobjs.ChannelType{pbobjs.ChannelType_Group}}, 0},
		{&pbobjs.ConverFilter{IncludeConvers: []*pbobjs.SimpleConversation{{TargetId: "ordinary-peer", ChannelType: pbobjs.ChannelType_Private}}}, 2},
		{&pbobjs.ConverFilter{ExcludeConvers: []*pbobjs.SimpleConversation{{TargetId: "ordinary-peer", ChannelType: pbobjs.ChannelType_Private}}}, 1},
	} {
		code, resp := QryTotalUnreadCountV2(ctx, "42", &pbobjs.QryTotalUnreadCountReq{Filter: tc.filter})
		if code != 0 || resp == nil || resp.TotalCount != tc.want {
			t.Fatalf("ordinary unread semantics changed: filter=%+v code=%d %+v want=%d", tc.filter, code, resp, tc.want)
		}
	}
	if peer.LatestReadMsgIndex != 1 || marked.UnreadTag != 1 {
		t.Fatal("read-only filtering mutated stored cache state")
	}
}

func TestNativeReadFiltersCurrentMessageAndMentionIdentities(t *testing.T) {
	db, ctx, uc := nativeReadFixture(t)
	peer := uc.ConverItemMap[getConverItemKey("ordinary-peer", "", pbobjs.ChannelType_Private)]
	current, err := proto.Marshal(&pbobjs.DownMsg{MsgId: peer.LatestMsgId, ChannelType: pbobjs.ChannelType_Private, SenderId: listAvatar, TargetId: "42"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE p_hismsgs SET msg_body=? WHERE app_key='list-app' AND msg_id=?", current, peer.LatestMsgId); err != nil {
		t.Fatal(err)
	}
	peer.LatestMsg = nil
	marked := uc.ConverItemMap[getConverItemKey("ma222222222222222222222222222222", "", pbobjs.ChannelType_Private)]
	marked.MentionInfo = &models.ConverMentionInfo{SenderIds: []string{listAvatar}}
	code, resp := QryTotalUnreadCountV2(ctx, "42", &pbobjs.QryTotalUnreadCountReq{})
	if code != 0 || resp == nil || resp.TotalCount != 0 {
		t.Fatalf("cached message/mention identities leaked into aggregation: code=%d %+v", code, resp)
	}
	code, list := QryConversationsV2(ctx, &pbobjs.QryConversationsReq{Count: 1})
	if code != 0 || list == nil || len(list.Conversations) != 0 || !list.IsFinished {
		t.Fatalf("all-protected page must be empty and finished: code=%d %+v", code, list)
	}
	if _, err := db.Exec("UPDATE p_hismsgs SET msg_body=? WHERE app_key='list-app' AND msg_id=?", []byte{0xff}, peer.LatestMsgId); err != nil {
		t.Fatal(err)
	}
	if code, resp := QryTotalUnreadCountV2(ctx, "42", &pbobjs.QryTotalUnreadCountReq{}); code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || resp != nil {
		t.Fatalf("invalid cached protobuf became successful zero: code=%d %+v", code, resp)
	}
}

func TestNativeReadIgnoresStaleCachedMessageBody(t *testing.T) {
	_, ctx, uc := nativeReadFixture(t)
	peer := uc.ConverItemMap[getConverItemKey("ordinary-peer", "", pbobjs.ChannelType_Private)]
	var err error
	peer.LatestMsg, err = proto.Marshal(&pbobjs.DownMsg{MsgId: "obsolete-message", SenderId: listAvatar, ChannelType: pbobjs.ChannelType_Private})
	if err != nil {
		t.Fatal(err)
	}
	code, resp := QryTotalUnreadCountV2(ctx, "42", &pbobjs.QryTotalUnreadCountReq{})
	if code != 0 || resp == nil || resp.TotalCount != 4 {
		t.Fatalf("obsolete body hid a current ordinary message: %d %+v", code, resp)
	}
}

func TestNativeReadResolvesColdMentionsBeforeAggregationAndPaging(t *testing.T) {
	db, ctx, uc := nativeReadFixture(t)
	key := getConverItemKey("ordinary-group", "", pbobjs.ChannelType_Group)
	uc.ConverItemMap[key] = &models.Conversation{AppKey: "list-app", UserId: "42", TargetId: "ordinary-group", ChannelType: pbobjs.ChannelType_Group, SortTime: 200, SyncTime: 200, LatestUnreadMsgIndex: 7}
	uc.SortTimeIndex.Add(200, key)
	uc.SyncTimeIndex.Add(200, key)
	if _, err := db.Exec("INSERT INTO mentionmsgs(app_key,user_id,target_id,channel_type,sub_channel,sender_id,msg_id,msg_index,msg_time,mention_type,is_read) VALUES('list-app','42','ordinary-group',2,'',?,'former-mention',7,200,1,0)", listAvatar); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		code, total := QryTotalUnreadCountV2(ctx, "42", &pbobjs.QryTotalUnreadCountReq{})
		if code != 0 || total == nil || total.TotalCount != 4 {
			t.Fatalf("cold/hot mention state changed aggregation: attempt=%d code=%d %+v", attempt, code, total)
		}
		code, resp := QryConversationsV2(ctx, &pbobjs.QryConversationsReq{Count: 1, StartTime: 1000})
		if code != 0 || resp == nil || len(resp.Conversations) != 1 || resp.Conversations[0].TargetId != "ordinary-peer" {
			t.Fatalf("cold internal mention consumed/failed the first ordinary page: %d %+v", code, resp)
		}
	}
}

func TestNativeReadColdMentionFailureIsNotEmptySuccess(t *testing.T) {
	db, ctx, _ := nativeReadFixture(t)
	if _, err := db.Exec("RENAME TABLE mentionmsgs TO mentionmsgs_unavailable"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("RENAME TABLE mentionmsgs_unavailable TO mentionmsgs"); err != nil {
			t.Error("restore owned mention fixture", err)
		}
	})
	code, resp := QryTotalUnreadCountV2(ctx, "42", &pbobjs.QryTotalUnreadCountReq{})
	if code == 0 || resp != nil {
		t.Fatalf("unknown cold mentions became successful data: %d %+v", code, resp)
	}
}

func TestNativeReadFailuresAreNotSuccessfulEmptyResults(t *testing.T) {
	db, ctx, uc := nativeReadFixture(t)
	check := func(ctx context.Context, want errs.IMErrorCode) {
		t.Helper()
		code, list := QryConversationsV2(ctx, &pbobjs.QryConversationsReq{Count: 1})
		if code == 0 || list != nil || want != 0 && code != want {
			t.Fatalf("list failure hidden: %d %+v", code, list)
		}
		code, sync := SyncConversationsV2(ctx, "list-app", "42", 0, 1)
		if code == 0 || sync != nil || want != 0 && code != want {
			t.Fatalf("sync failure hidden: %d %+v", code, sync)
		}
		code, top := QryTopConversV2(ctx, "42", &pbobjs.QryTopConversReq{})
		if code == 0 || top != nil || want != 0 && code != want {
			t.Fatalf("top failure hidden: %d %+v", code, top)
		}
		code, total := QryTotalUnreadCountV2(ctx, "42", &pbobjs.QryTotalUnreadCountReq{})
		if code == 0 || total != nil || want != 0 && code != want {
			t.Fatalf("total failure hidden: %d %+v", code, total)
		}
		code, global := QryGlobalConvers(ctx, &pbobjs.QryGlobalConversReq{Count: 1})
		if code == 0 || global != nil || want != 0 && code != want {
			t.Fatalf("global failure hidden: %d %+v", code, global)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	check(canceled, 0)
	bases.InitNativePolicy(context.Background(), nil)
	check(ctx, errs.IMErrorCode_MINDLINK_NOT_READY)
	if err := bases.InitNativePolicy(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	// An empty cache must not bypass a failed fence lookup either.
	uc.ConverItemMap = map[string]*models.Conversation{}
	db.Close()
	check(ctx, 0)
}

func TestNativeGlobalReadFiltersTargetsGroupsAndAliases(t *testing.T) {
	db, ctx, _ := nativeReadFixture(t)
	rows := []struct {
		id, sender, target string
		channel            pbobjs.ChannelType
		stamp              int64
	}{
		{"hidden-sender", strings.ToUpper(listAvatar), "42", pbobjs.ChannelType_Private, 105},
		{"hidden-target", "42", listAvatar, pbobjs.ChannelType_Private, 104},
		{"hidden-system", "42", listAvatar, pbobjs.ChannelType_System, 103},
		{"hidden-group", "42", "protected-group", pbobjs.ChannelType_Group, 102},
		{"ordinary", "42", "ordinary-peer", pbobjs.ChannelType_Private, 101},
		{"lookalike", "42", "ma222222222222222222222222222222", pbobjs.ChannelType_Private, 100},
	}
	for _, r := range rows {
		if _, err := db.Exec("INSERT INTO globalconvers(app_key,conver_id,sender_id,target_id,channel_type,sub_channel,updated_time) VALUES('list-app',?,?,?,?, 'scope',?)", r.id, r.sender, r.target, r.channel, r.stamp); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		start int64
		order int32
		want  string
		done  bool
	}{{106, 0, "ordinary", false}, {101, 0, "lookalike", true}, {0, 1, "lookalike", false}, {100, 1, "ordinary", true}} {
		code, resp := QryGlobalConvers(ctx, &pbobjs.QryGlobalConversReq{Start: tc.start, Order: tc.order, Count: 1, SubChannel: "scope"})
		if code != 0 || resp == nil || len(resp.Convers) != 1 || resp.Convers[0].ConverId != tc.want || resp.IsFinished != tc.done {
			t.Fatalf("global pagination/filter failure: code=%d %+v want=%s", code, resp, tc.want)
		}
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM globalconvers").Scan(&count); err != nil || count != len(rows) {
		t.Fatal("filtering changed stored global rows", count, err)
	}
}

func TestNativeGlobalReadRejectsProtectedRequester(t *testing.T) {
	_, _, _ = nativeReadFixture(t)
	ctx := bases.CreateRpcCtx("list-app", listAvatar)
	code, resp := QryGlobalConvers(ctx, &pbobjs.QryGlobalConversReq{Count: 1})
	if code != errs.IMErrorCode_MINDLINK_FORBIDDEN || resp != nil {
		t.Fatalf("empty global list skipped requester authority: code=%d %+v", code, resp)
	}
}

func TestNativeReadsRejectInvalidCounts(t *testing.T) {
	ctx := bases.CreateRpcCtx("list-app", "42")
	for _, count := range []int32{-1, math.MaxInt32} {
		if code, resp := QryConversationsV2(ctx, &pbobjs.QryConversationsReq{Count: count}); code != errs.IMErrorCode_MSG_PARAM_ILLEGAL || resp != nil {
			t.Fatal("invalid list count accepted", count, code)
		}
		if code, resp := SyncConversationsV2(ctx, "list-app", "42", 0, count); code != errs.IMErrorCode_MSG_PARAM_ILLEGAL || resp != nil {
			t.Fatal("invalid sync count accepted", count, code)
		}
		if code, resp := QryGlobalConvers(ctx, &pbobjs.QryGlobalConversReq{Count: count}); code != errs.IMErrorCode_MSG_PARAM_ILLEGAL || resp != nil {
			t.Fatal("invalid global count accepted", count, code)
		}
	}
}
