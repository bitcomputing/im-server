package services

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"im-server/commons/bases"
	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/errs"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/commonservices"
	groupdbs "im-server/services/group/dbs"
	hisdbs "im-server/services/historymsg/storages/dbs"
	hismodels "im-server/services/historymsg/storages/models"
)

func TestSeenInvalidRequests(t *testing.T) {
	ctx := bases.CreateRpcCtx("tenant", "owner")
	for _, req := range []*pbobjs.SetConverSeenReq{nil, {}, {TargetId: "group", ChannelType: pbobjs.ChannelType_Group, SeenSeq: -1}, {TargetId: "group", ChannelType: pbobjs.ChannelType_Chatroom}, {TargetId: strings.Repeat("x", 33), ChannelType: pbobjs.ChannelType_Group}, {TargetId: "group", SubChannel: strings.Repeat("x", 33), ChannelType: pbobjs.ChannelType_Group}} {
		if code, state := SetConverSeen(ctx, req); code != errs.IMErrorCode_MSG_PARAM_ILLEGAL || state != nil {
			t.Fatalf("invalid request succeeded: %v", req)
		}
	}
	if code, _ := QryConverSeen(ctx, nil); code != errs.IMErrorCode_MSG_PARAM_ILLEGAL {
		t.Fatal("nil query accepted")
	}
	if code, _ := QryConverSeen(bases.CreateRpcCtx("tenant", ""), &pbobjs.QryConverSeenReq{TargetId: "group", ChannelType: pbobjs.ChannelType_Group}); code != errs.IMErrorCode_MSG_PARAM_ILLEGAL {
		t.Fatal("anonymous query accepted")
	}
}

func TestSeenServiceMySQL(t *testing.T) {
	if os.Getenv("JIM_F1_READ_ISOLATED") != "im-f1-read-20261106" {
		t.Skip("requires the independently owned F1-READ MySQL fixture")
	}
	password := os.Getenv("MYSQL_ROOT_PASSWORD")
	if password == "" {
		t.Fatal("isolated password missing")
	}
	configures.Config.Mysql.User = "root"
	configures.Config.Mysql.Password = password
	configures.Config.Mysql.Address = "im-f1-read-20261106-mysql-1:3306"
	configures.Config.Mysql.DbName = "jim_db"
	configures.Config.Mysql.Debug = false
	if err := dbcommons.InitMysql(); err != nil {
		t.Fatal(err)
	}
	app := fmt.Sprintf("ss%x", time.Now().UnixNano())
	db := dbcommons.GetDb()
	db.Config.Logger = logger.Default.LogMode(logger.Silent)
	t.Cleanup(func() {
		for _, table := range []string{"conversation_seen", "groupmembers", "groupinfoexts", "groupmemberexts", "g_hismsgs", "p_hismsgs", "usercleantimes", "convercleantimes"} {
			if err := db.Exec("DELETE FROM "+table+" WHERE app_key=?", app).Error; err != nil {
				t.Error(err)
			}
		}
	})
	if err := (groupdbs.GroupMemberDao{}).Create(groupdbs.GroupMemberDao{AppKey: app, GroupId: "group", MemberId: "owner", CreatedTime: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	ctx := bases.CreateRpcCtx(app, "owner")
	query := func() *pbobjs.ConverSeenState {
		t.Helper()
		code, state := QryConverSeen(ctx, &pbobjs.QryConverSeenReq{TargetId: "group", ChannelType: pbobjs.ChannelType_Group})
		if code != 0 || state == nil {
			t.Fatalf("query failed %d", code)
		}
		return state
	}
	if state := query(); state.LatestSeq != 0 || state.SeenSeq != 0 || state.HasNew {
		t.Fatal("empty history reported new messages")
	}
	for seq := int64(1); seq <= 3; seq++ {
		if err := (hisdbs.GroupHisMsgDao{}).SaveGroupHisMsg(hismodels.GroupHisMsg{HisMsg: hismodels.HisMsg{AppKey: app, ConverId: "group", MsgId: fmt.Sprint(seq), MsgSeqNo: seq, SendTime: time.Now().Add(-10*time.Second).UnixMilli() + seq, ChannelType: pbobjs.ChannelType_Group}}); err != nil {
			t.Fatal(err)
		}
	}
	if state := query(); state.LatestSeq != 3 || !state.HasNew {
		t.Fatal("persisted latest sequence is missing")
	}
	set := func(seq int64) *pbobjs.ConverSeenState {
		t.Helper()
		code, state := SetConverSeen(ctx, &pbobjs.SetConverSeenReq{TargetId: "group", ChannelType: pbobjs.ChannelType_Group, SeenSeq: seq})
		if code != 0 || state == nil {
			t.Fatalf("set failed %d", code)
		}
		return state
	}
	if state := set(2); state.SeenSeq != 2 || !state.HasNew {
		t.Fatal("partial reading advanced too far")
	}
	if state := set(1); state.SeenSeq != 2 {
		t.Fatal("stale device regressed state")
	}
	if state := set(3); state.SeenSeq != 3 || state.HasNew {
		t.Fatal("displayed latest still appears new")
	}
	if code, _ := SetConverSeen(ctx, &pbobjs.SetConverSeenReq{TargetId: "group", ChannelType: pbobjs.ChannelType_Group, SeenSeq: 4}); code != errs.IMErrorCode_MSG_PARAM_ILLEGAL {
		t.Fatal("future position accepted")
	}
	for _, otherCtx := range []struct{ app, user string }{{app, "outsider"}, {app + "x", "owner"}} {
		if code, _ := QryConverSeen(bases.CreateRpcCtx(otherCtx.app, otherCtx.user), &pbobjs.QryConverSeenReq{TargetId: "group", ChannelType: pbobjs.ChannelType_Group}); code != errs.IMErrorCode_GROUP_NOTGROUPMEMBER {
			t.Fatal("unrelated principal read group position")
		}
	}
	if code, state := QryConverSeen(ctx, &pbobjs.QryConverSeenReq{TargetId: "group", SubChannel: "thread", ChannelType: pbobjs.ChannelType_Group}); code != 0 || state.LatestSeq != 0 || state.SeenSeq != 0 {
		t.Fatal("subchannel state leaked")
	}
	// The user-visible latest position excludes portion, deleted and expired messages.
	if err := db.Exec("UPDATE g_hismsgs SET is_portion=1 WHERE app_key=? AND msg_seq_no=3", app).Error; err != nil {
		t.Fatal(err)
	}
	if state := query(); state.LatestSeq != 2 || state.SeenSeq != 3 || state.HasNew {
		t.Fatal("hidden latest sequence leaked or durable seen regressed")
	}
	if state := set(3); state.SeenSeq != 3 {
		t.Fatal("duplicate acknowledgement failed after latest was hidden")
	}
	if err := db.Exec("UPDATE g_hismsgs SET is_delete=1 WHERE app_key=? AND msg_seq_no=2", app).Error; err != nil {
		t.Fatal(err)
	}
	if state := query(); state.LatestSeq != 1 {
		t.Fatal("deleted latest sequence leaked")
	}
	if err := db.Exec("UPDATE g_hismsgs SET destroy_time=1 WHERE app_key=? AND msg_seq_no=1", app).Error; err != nil {
		t.Fatal(err)
	}
	if state := query(); state.LatestSeq != 0 || state.SeenSeq != 3 {
		t.Fatal("expired latest sequence leaked")
	}
	// A private read is scoped by the authenticated participant pair, never a client-supplied conver_id.
	conver := commonservices.GetConversationId("owner", "peer", pbobjs.ChannelType_Private)
	if err := (hisdbs.PrivateHisMsgDao{}).SavePrivateHisMsg(hismodels.PrivateHisMsg{HisMsg: hismodels.HisMsg{AppKey: app, ConverId: conver, SenderId: "peer", ReceiverId: "owner", MsgId: "private", MsgSeqNo: 7, SendTime: time.Now().Add(-time.Second).UnixMilli(), ChannelType: pbobjs.ChannelType_Private}}); err != nil {
		t.Fatal(err)
	}
	if code, state := QryConverSeen(ctx, &pbobjs.QryConverSeenReq{TargetId: "peer", ChannelType: pbobjs.ChannelType_Private}); code != 0 || state.LatestSeq != 7 {
		t.Fatal("private latest missing")
	}
	if code, state := QryConverSeen(bases.CreateRpcCtx(app, "outsider"), &pbobjs.QryConverSeenReq{TargetId: "peer", ChannelType: pbobjs.ChannelType_Private}); code != 0 || state.LatestSeq != 0 {
		t.Fatal("private history crossed participants")
	}
	// DB lookup and write failures must not be returned as an empty/successful state.
	injected := errors.New("injected seen database failure")
	if err := db.Callback().Row().Before("gorm:row").Register("seen_find_error", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "conversation_seen") {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if code, state := QryConverSeen(ctx, &pbobjs.QryConverSeenReq{TargetId: "peer", ChannelType: pbobjs.ChannelType_Private}); code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || state != nil {
		t.Fatal("failed DB read became empty success")
	}
	db.Callback().Row().Remove("seen_find_error")
	if err := db.Callback().Raw().Before("gorm:raw").Register("seen_write_error", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "conversation_seen") {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if code, state := SetConverSeen(ctx, &pbobjs.SetConverSeenReq{TargetId: "peer", ChannelType: pbobjs.ChannelType_Private, SeenSeq: 7}); code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || state != nil {
		t.Fatal("failed DB write became success")
	}
	db.Callback().Raw().Remove("seen_write_error")
	if code, state := QryConverSeen(ctx, &pbobjs.QryConverSeenReq{TargetId: "peer", ChannelType: pbobjs.ChannelType_Private}); code != 0 || state.SeenSeq != 0 {
		t.Fatal("failed write advanced reading position")
	}
	if err := (hisdbs.HisMsgConverCleanTimeDao{}).UpsertDestroyTime(hismodels.HisMsgConverCleanTime{AppKey: app, ConverId: conver, ChannelType: pbobjs.ChannelType_Private, CleanTime: time.Now().UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if code, state := QryConverSeen(ctx, &pbobjs.QryConverSeenReq{TargetId: "peer", ChannelType: pbobjs.ChannelType_Private}); code != 0 || state.LatestSeq != 0 {
		t.Fatal("cleared private history leaked into latest sequence")
	}
	if err := db.Exec("DELETE FROM convercleantimes WHERE app_key=?", app).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Query().Before("gorm:query").Register("seen_history_error", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "g_hismsgs") {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if code, state := QryConverSeen(ctx, &pbobjs.QryConverSeenReq{TargetId: "group", ChannelType: pbobjs.ChannelType_Group}); code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || state != nil {
		t.Fatal("failed history query became empty success")
	}
	db.Callback().Query().Remove("seen_history_error")
	if err := db.Callback().Query().Before("gorm:query").Register("seen_missing_cleanup", func(tx *gorm.DB) {
		if tx.Statement.Table == "convercleantimes" || tx.Statement.Table == "usercleantimes" {
			tx.AddError(mongo.ErrNoDocuments)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if code, state := QryConverSeen(ctx, &pbobjs.QryConverSeenReq{TargetId: "peer", ChannelType: pbobjs.ChannelType_Private}); code != 0 || state == nil || state.LatestSeq != 7 {
		t.Fatal("missing Mongo cleanup record became internal failure")
	}
	db.Callback().Query().Remove("seen_missing_cleanup")
	if err := db.Exec("UPDATE p_hismsgs SET msg_seq_no=-1 WHERE app_key=?", app).Error; err != nil {
		t.Fatal(err)
	}
	if code, state := QryConverSeen(ctx, &pbobjs.QryConverSeenReq{TargetId: "peer", ChannelType: pbobjs.ChannelType_Private}); code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || state != nil {
		t.Fatal("invalid persisted ordering became successful state")
	}
}
