package convercache

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/pbdefines/pbobjs"
	hisdbs "im-server/services/historymsg/storages/dbs"
	hismodels "im-server/services/historymsg/storages/models"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func sequenceTestDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	if os.Getenv("JIM_SEQUENCE_ISOLATED") != "im-f1-sequence-20261106" {
		t.Skip("requires the owned sequence MySQL fixture")
	}
	configures.Config.Mysql.User = "root"
	configures.Config.Mysql.Password = os.Getenv("MYSQL_ROOT_PASSWORD")
	configures.Config.Mysql.Address = "im-f1-sequence-mysql:3306"
	configures.Config.Mysql.DbName = "jim_db"
	if err := dbcommons.InitMysql(); err != nil {
		t.Fatal(err)
	}
	db := dbcommons.GetDb()
	db.Config.Logger = logger.Default.LogMode(logger.Silent)
	app := fmt.Sprintf("sq%x", time.Now().UnixNano())
	t.Cleanup(func() {
		for _, table := range []string{"g_hismsgs", "p_hismsgs", "message_sequence", "conversation_seen"} {
			if err := db.Exec("DELETE FROM "+table+" WHERE app_key=?", app).Error; err != nil {
				t.Error(err)
			}
		}
	})
	return db, app
}

func TestSequenceAfterRetentionMySQL(t *testing.T) {
	_, app := sequenceTestDB(t)
	if err := (hisdbs.GroupHisMsgDao{}).SaveGroupHisMsg(hismodels.GroupHisMsg{HisMsg: hismodels.HisMsg{AppKey: app, ConverId: "group", ChannelType: pbobjs.ChannelType_Group, MsgId: "old", MsgSeqNo: 3, SendTime: 100}}); err != nil {
		t.Fatal(err)
	}
	first := &MsgConverItem{cacheKey: app + "/group", appKey: app, converId: "group", channelType: pbobjs.ChannelType_Group, LatestMsgSeq: 3, LatestMsgTime: 100}
	_, _, before, err := first.GenerateMsgId("group", pbobjs.ChannelType_Group, 101, 8)
	if err != nil {
		t.Fatal(err)
	}
	if err := (hisdbs.GroupHisMsgDao{}).DelMsgsBaseTime(app, 1000); err != nil {
		t.Fatal(err)
	}
	cold := &MsgConverItem{cacheKey: app + "/group", appKey: app, converId: "group", channelType: pbobjs.ChannelType_Group}
	_, _, after, err := cold.GenerateMsgId("group", pbobjs.ChannelType_Group, 102, 8)
	if err != nil {
		t.Fatal(err)
	}
	if after <= before {
		t.Fatalf("retention/cold-start reused sequence: before=%d after=%d", before, after)
	}
}

func TestSequenceFloorAndScopeMySQL(t *testing.T) {
	db, app := sequenceTestDB(t)
	// A READ position may remain after all history was physically removed.
	if err := db.Exec("INSERT INTO conversation_seen (app_key,user_id,target_id,channel_type,sub_channel,seen_seq) VALUES (?,?,?,?,?,?),(?,?,?,?,?,?),(?,?,?,?,?,?)", app, "owner", "peer", 1, "", 100, app, "other", "peer", 1, "", 900, app, "owner", "group", 2, "", 200).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		conver, sub string
		channel     pbobjs.ChannelType
		want        int64
	}{
		{"owner:peer", "", 1, 101}, {"owner:peer", "thread", 1, 1}, {"group", "", 2, 201}, {"other-group", "", 2, 1}, {"owner:peer", "", 2, 1},
	} {
		got, err := reserveSequence(app, tc.conver, tc.sub, tc.channel, sequenceWatermark{}, 1000)
		if err != nil || got.Seq != tc.want {
			t.Fatalf("scope floor: got=%+v want=%d err=%v", got, tc.want, err)
		}
	}
	otherApp := app + "b"
	t.Cleanup(func() {
		if err := db.Exec("DELETE FROM message_sequence WHERE app_key=?", otherApp).Error; err != nil {
			t.Error(err)
		}
	})
	other, err := reserveSequence(otherApp, "owner:peer", "", 1, sequenceWatermark{}, 1000)
	if err != nil || other.Seq != 1 {
		t.Fatalf("sequence crossed tenant scope: %+v %v", other, err)
	}
	// Ordering by send_time alone must not reduce an existing message sequence floor.
	if err := db.Exec("INSERT INTO p_hismsgs (app_key,conver_id,sub_channel,msg_id,msg_seq_no,send_time) VALUES (?,?,?,?,?,?),(?,?,?,?,?,?)", app, "old:pair", "", "one", 300, 500, app, "old:pair", "", "two", 250, 600).Error; err != nil {
		t.Fatal(err)
	}
	got, err := reserveSequence(app, "old:pair", "", 1, sequenceWatermark{}, 10)
	if err != nil || got.Seq != 301 || got.SendTime != 601 {
		t.Fatalf("history max floor: %+v err=%v", got, err)
	}
}

func TestSequenceConcurrentReservationsMySQL(t *testing.T) {
	_, app := sequenceTestDB(t)
	var wg sync.WaitGroup
	results := make(chan sequenceWatermark, 24)
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			next, err := reserveSequence(app, "group", "", 2, sequenceWatermark{}, 1000)
			if err != nil {
				t.Error(err)
				return
			}
			results <- next
		}()
	}
	wg.Wait()
	close(results)
	seqs, times := map[int64]bool{}, map[int64]bool{}
	for next := range results {
		if seqs[next.Seq] || times[next.SendTime] {
			t.Fatal("independent reservations collided")
		}
		seqs[next.Seq] = true
		times[next.SendTime] = true
	}
	if len(seqs) != 24 || !seqs[24] {
		t.Fatalf("lost reservations: %d", len(seqs))
	}
}

func TestSequenceFailuresAndOverflowMySQL(t *testing.T) {
	db, app := sequenceTestDB(t)
	first, err := reserveSequence(app, "group", "", 2, sequenceWatermark{}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected sequence update failure")
	if err := db.Callback().Raw().Before("gorm:raw").Register("sequence_update_error", func(tx *gorm.DB) {
		if strings.HasPrefix(tx.Statement.SQL.String(), "UPDATE message_sequence SET") {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	failed, err := reserveSequence(app, "group", "", 2, first, 1000)
	db.Callback().Raw().Remove("sequence_update_error")
	if !errors.Is(err, injected) || failed != (sequenceWatermark{}) {
		t.Fatalf("failed reservation returned success: %+v %v", failed, err)
	}
	next, err := reserveSequence(app, "group", "", 2, sequenceWatermark{}, 1000)
	if err != nil || next.Seq != first.Seq+1 {
		t.Fatalf("rollback lost watermark: %+v %v", next, err)
	}
	if err := db.Callback().Row().Before("gorm:row").Register("sequence_bootstrap_error", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "MAX(msg_seq_no)") {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	failed, err = reserveSequence(app, "uninitialized", "", 2, sequenceWatermark{}, 1000)
	db.Callback().Row().Remove("sequence_bootstrap_error")
	if !errors.Is(err, injected) || failed != (sequenceWatermark{}) {
		t.Fatalf("failed bootstrap became empty success: %+v %v", failed, err)
	}
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM message_sequence WHERE app_key=? AND conver_id='uninitialized'", app).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("failed bootstrap left a durable reservation")
	}
	if err := db.Exec("UPDATE message_sequence SET seq=? WHERE app_key=? AND conver_id='group'", int64(math.MaxInt64), app).Error; err != nil {
		t.Fatal(err)
	}
	item := &MsgConverItem{cacheKey: app + "/group", appKey: app, converId: "group", channelType: 2}
	id, stamp, seq, err := item.GenerateMsgId("group", 2, 1000, 8)
	if err == nil || id != "" || stamp != 0 || seq != 0 {
		t.Fatal("exhausted sequence became a publishable allocation")
	}
}

func TestConversationCacheKeyScope(t *testing.T) {
	if getMsgConverCacheKey("a_2_b", "c", "", 2) == getMsgConverCacheKey("a", "b_2_c", "", 2) {
		t.Fatal("conversation cache key crosses tenant scope")
	}
}

func TestSequenceNonstoredAndOtherChannels(t *testing.T) {
	item := &MsgConverItem{cacheKey: "no-store"}
	_, _, seq, err := item.GenerateMsgId("target", pbobjs.ChannelType_Private, 1000, 0)
	if err != nil || seq != -1 {
		t.Fatal("nonstored message required a durable reservation")
	}
	for _, channel := range []pbobjs.ChannelType{pbobjs.ChannelType_System, pbobjs.ChannelType_BroadCast, pbobjs.ChannelType_GroupCast, pbobjs.ChannelType_SubStatus} {
		item := &MsgConverItem{cacheKey: fmt.Sprint(channel)}
		_, _, seq, err := item.GenerateMsgId("target", channel, 1000, 8)
		if err != nil || seq != 1 {
			t.Fatalf("legacy channel %v behavior changed", channel)
		}
	}
}
