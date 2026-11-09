//go:build integration

package services

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/conversation/storages/models"
)

func TestNativeReadLargeMixedPopulationUsesBoundedSQL(t *testing.T) {
	db, ctx, uc := nativeReadFixture(t)
	for i := 0; i < 9996; i++ {
		target := fmt.Sprintf("ordinary-%05d", i)
		key := getConverItemKey(target, "", pbobjs.ChannelType_Private)
		stamp := int64(1000 + i)
		uc.ConverItemMap[key] = &models.Conversation{AppKey: uc.Appkey, UserId: uc.UserId, TargetId: target, ChannelType: pbobjs.ChannelType_Private, SortTime: stamp, SyncTime: stamp, LatestUnreadMsgIndex: 1, MentionInfo: &models.ConverMentionInfo{}}
		uc.SortTimeIndex.Add(float64(stamp), key)
		uc.SyncTimeIndex.Add(float64(stamp), key)
	}
	peerKey := getConverItemKey("ordinary-peer", "", pbobjs.ChannelType_Private)
	uc.ConverItemMap[peerKey].SortTime = 50000
	uc.SortTimeIndex.Add(50000, peerKey)
	questions := func() uint64 {
		t.Helper()
		var name, value string
		if err := db.QueryRow("SHOW GLOBAL STATUS LIKE 'Questions'").Scan(&name, &value); err != nil {
			t.Fatal(err)
		}
		count, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		return count
	}
	before := questions()
	started := time.Now()
	code, list := QryConversationsV2(ctx, &pbobjs.QryConversationsReq{Count: 1, StartTime: 100000})
	delta := questions() - before
	t.Logf("owned native Count=1 population=10000 SQL_delta=%d elapsed_ms=%d code=%d", delta, time.Since(started).Milliseconds(), code)
	if code != 0 || list == nil || len(list.Conversations) != 1 || list.Conversations[0].TargetId != "ordinary-peer" || delta > 256 {
		t.Fatalf("large mixed list amplified policy lookups or failed: code=%d SQL_delta=%d", code, delta)
	}
	before = questions()
	started = time.Now()
	code, total := QryTotalUnreadCountV2(ctx, "42", &pbobjs.QryTotalUnreadCountReq{})
	delta = questions() - before
	t.Logf("owned native unread population=10000 SQL_delta=%d elapsed_ms=%d code=%d", delta, time.Since(started).Milliseconds(), code)
	if code != 0 || total == nil || total.TotalCount != 10000 || delta > 256 {
		t.Fatalf("large mixed aggregate amplified policy lookups or changed counts: code=%d SQL_delta=%d", code, delta)
	}
}
