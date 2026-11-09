//go:build integration

package services

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"im-server/commons/bases"
	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/logs"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/commonservices"
	"im-server/services/conversation/storages/models"

	"github.com/bytedance/gopkg/collection/zset"
	mysql "github.com/go-sql-driver/mysql"
	"google.golang.org/protobuf/proto"
)

const listFixtureOwner = "im-f1-read-lists-20261109"
const listAvatar = "ma111111111111111111111111111111"

func nativeReadFixture(t *testing.T) (*sql.DB, context.Context, *UserConversations) {
	t.Helper()
	if os.Getenv("JIM_READ_LIST_ISOLATED") != listFixtureOwner {
		t.Skip("requires the independently owned READ-list MySQL fixture")
	}
	cfg, err := mysql.ParseDSN(os.Getenv("JIM_READ_LIST_DSN"))
	if err != nil || cfg.Net != "tcp" || !strings.HasPrefix(cfg.Addr, listFixtureOwner+"-mysql:") {
		t.Fatal("invalid owned READ-list database address")
	}
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open owned READ-list database")
	}
	t.Cleanup(func() { root.Close() })
	name := fmt.Sprintf("read_list_%x", time.Now().UnixNano())
	if _, err := root.Exec("CREATE DATABASE `" + name + "`"); err != nil {
		t.Fatal(err)
	}
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open owned READ-list scratch")
	}
	t.Cleanup(func() {
		db.Close()
		if _, err := root.Exec("DROP DATABASE `" + name + "`"); err != nil {
			t.Error("owned READ-list scratch cleanup failed")
		}
	})
	initial, err := os.ReadFile("../../../sql/imserver.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(initial)); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"INSERT INTO apps(app_key,app_secret,app_secure_key) VALUES('list-app','fixture-only','0123456789abcdef')",
		"INSERT INTO users(app_key,user_id,nickname) VALUES('list-app','42','owner'),('list-app','ordinary-peer','peer'),('list-app','" + listAvatar + "','avatar')",
		"INSERT INTO groupinfos(app_key,group_id,group_name) VALUES('list-app','protected-group','protected'),('list-app','ordinary-group','ordinary')",
		"INSERT INTO groupmembers(app_key,group_id,member_id) VALUES('list-app','protected-group','" + listAvatar + "'),('list-app','ordinary-group','42')",
		"INSERT INTO mindlink_owner_barriers(environment,tenant_id,appkey,owner_id,read_enabled) VALUES('dev','list-tenant','list-app','42',TRUE)",
		"INSERT INTO mindlink_principals(environment,tenant_id,appkey,owner_id,principal_id,avatar_id,avatar_generation,policy_version,state) VALUES('dev','list-tenant','list-app','42','" + listAvatar + "','list-avatar',1,1,'active')",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	saved := configures.Config
	configures.Config.Mysql.User = cfg.User
	configures.Config.Mysql.Password = cfg.Passwd
	configures.Config.Mysql.Address = cfg.Addr
	configures.Config.Mysql.DbName = cfg.DBName
	configures.Config.Mysql.Debug = false
	configures.Config.MsgStoreEngine = configures.MsgStoreEngine_MySQL
	configures.Config.Log.LogPath = t.TempDir()
	configures.Config.Log.LogName = "native-read-test"
	logs.InitLogs()
	logs.GetInfoLogger().SetOutput(io.Discard)
	if err := dbcommons.InitMysql(); err != nil {
		t.Fatal("initialize owned READ-list GORM database")
	}
	t.Cleanup(func() {
		dbcommons.CloseDB()
		configures.Config = saved
		bases.InitNativePolicy(context.Background(), nil)
	})
	if err := bases.InitNativePolicy(context.Background(), db); err != nil {
		t.Fatal("real native fence did not initialize", err)
	}
	if err := bases.CheckNativeIdentities(context.Background(), "list-app", []string{listAvatar}, nil); err == nil {
		t.Fatal("fixture projection is not protected by the real native fence")
	}
	ctx := bases.CreateRpcCtx("list-app", "42")
	uc := &UserConversations{
		Appkey: "list-app", UserId: "42",
		ConverItemMap: map[string]*models.Conversation{},
		SyncTimeIndex: zset.NewFloat64(), SortTimeIndex: zset.NewFloat64(),
		TopIndexBaseSortTime: zset.NewFloat64(), TopIndexBaseTopTime: zset.NewFloat64(),
	}
	userConverCache.Add(getUserConverCacheKey(uc.Appkey, uc.UserId), uc)
	t.Cleanup(func() { userConverCache.Remove(getUserConverCacheKey(uc.Appkey, uc.UserId)) })
	for i, item := range []struct {
		target  string
		channel pbobjs.ChannelType
		unread  int64
	}{
		{listAvatar, pbobjs.ChannelType_Private, 100},
		{"protected-group", pbobjs.ChannelType_Group, 100},
		{"ordinary-peer", pbobjs.ChannelType_Private, 3},
		{"ma222222222222222222222222222222", pbobjs.ChannelType_Private, 1},
	} {
		score := int64(100 - i)
		key := getConverItemKey(item.target, "", item.channel)
		msgID := fmt.Sprintf("list-msg-%d", i)
		down := &pbobjs.DownMsg{MsgId: msgID, SenderId: item.target, TargetId: "42", ChannelType: item.channel, TargetUserInfo: &pbobjs.UserInfo{UserId: item.target}}
		table := "p_hismsgs"
		converID := commonservices.GetConversationId("42", item.target, item.channel)
		if item.channel == pbobjs.ChannelType_Group {
			table = "g_hismsgs"
			down.SenderId = "ordinary-peer"
			down.TargetId = item.target
			down.TargetUserInfo.UserId = "ordinary-peer"
			down.GroupInfo = &pbobjs.GroupInfo{GroupId: item.target}
		}
		body, err := proto.Marshal(down)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO "+table+"(app_key,conver_id,sub_channel,sender_id,receiver_id,channel_type,msg_id,msg_body,send_time,msg_seq_no) VALUES('list-app',?,'',?,'42',?,?,?,100,1)", converID, down.SenderId, item.channel, msgID, body); err != nil {
			t.Fatal(err)
		}
		uc.ConverItemMap[key] = &models.Conversation{AppKey: uc.Appkey, UserId: uc.UserId, TargetId: item.target, ChannelType: item.channel, SortTime: score, SyncTime: score, IsTop: 1, TopUpdatedTime: score, LatestMsgId: msgID, LatestUnreadMsgIndex: item.unread}
		uc.SortTimeIndex.Add(float64(score), key)
		uc.SyncTimeIndex.Add(float64(score), key)
		uc.TopIndexBaseSortTime.Add(float64(score), key)
		uc.TopIndexBaseTopTime.Add(float64(score), key)
	}
	return db, ctx, uc
}

func TestNativeUnreadExcludesInternalConversations(t *testing.T) {
	_, ctx, _ := nativeReadFixture(t)
	code, resp := QryTotalUnreadCountV2(ctx, "42", &pbobjs.QryTotalUnreadCountReq{})
	if code != 0 || resp == nil {
		t.Fatalf("native unread failed: code=%d", code)
	}
	if resp.TotalCount != 4 {
		t.Fatalf("native unread includes protected rows: got %d, want ordinary 4", resp.TotalCount)
	}
}

func TestNativeListFiltersBeforePaging(t *testing.T) {
	_, ctx, _ := nativeReadFixture(t)
	code, resp := QryConversationsV2(ctx, &pbobjs.QryConversationsReq{Count: 1})
	if code != 0 || resp == nil || len(resp.Conversations) != 1 || resp.Conversations[0].TargetId != "ordinary-peer" || resp.IsFinished {
		t.Fatalf("hidden native row consumed list page: %+v", resp)
	}
}

func TestNativePinnedListFiltersInternalConversations(t *testing.T) {
	_, ctx, _ := nativeReadFixture(t)
	code, resp := QryTopConversV2(ctx, "42", &pbobjs.QryTopConversReq{})
	if code != 0 || len(resp.Conversations) != 2 || resp.Conversations[0].TargetId != "ordinary-peer" {
		t.Fatalf("pinned list contains protected rows: code=%d response=%+v", code, resp)
	}
}

func TestNativeSyncFiltersInternalConversations(t *testing.T) {
	_, ctx, _ := nativeReadFixture(t)
	code, resp := SyncConversationsV2(ctx, "list-app", "42", 98, 2)
	if code != 0 || resp == nil || len(resp.Conversations) != 0 || !resp.IsFinished {
		t.Fatalf("sync revealed protected rows/tombstones: %+v", resp)
	}
}

func TestNativeGlobalListExcludesInternalBeforeLimit(t *testing.T) {
	db, ctx, _ := nativeReadFixture(t)
	if _, err := db.Exec("INSERT INTO globalconvers(app_key,conver_id,sender_id,target_id,channel_type,sub_channel,updated_time) VALUES('list-app','protected-conver',?,'42',1,'',100),('list-app','ordinary-conver','ordinary-peer','42',1,'',99)", listAvatar); err != nil {
		t.Fatal(err)
	}
	code, resp := QryGlobalConvers(ctx, &pbobjs.QryGlobalConversReq{Start: 101, Count: 1})
	if code != 0 || resp == nil || len(resp.Convers) != 1 || resp.Convers[0].ConverId != "ordinary-conver" {
		t.Fatalf("global native list exposed/consumed protected page slot: %+v", resp.Convers)
	}
}
