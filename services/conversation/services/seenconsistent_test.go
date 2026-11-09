package services

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"im-server/commons/bases"
	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/errs"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/commonservices"
	groupdbs "im-server/services/group/dbs"
	hisdbs "im-server/services/historymsg/storages/dbs"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func consistentSeenFixture(t *testing.T) (*gorm.DB, string, int64) {
	t.Helper()
	if os.Getenv("JIM_F1_READ_ISOLATED") != "im-f1-read-20261106" {
		t.Skip("requires the owned F1-READ MySQL fixture")
	}
	password := os.Getenv("MYSQL_ROOT_PASSWORD")
	if password == "" {
		t.Fatal("isolated password missing")
	}
	configures.Config.Mysql.User, configures.Config.Mysql.Password = "root", password
	configures.Config.Mysql.Address, configures.Config.Mysql.DbName = "im-f1-read-20261106-mysql-1:3306", "jim_db"
	configures.Config.Mysql.Debug = false
	if err := dbcommons.InitMysql(); err != nil {
		t.Fatal(err)
	}
	db := dbcommons.GetDb()
	db.Config.Logger = logger.Default.LogMode(logger.Silent)
	app := fmt.Sprintf("sc%x", time.Now().UnixNano())
	stamp := time.Now().Add(-time.Minute).UnixMilli()
	t.Cleanup(func() {
		for _, table := range []string{"conversation_seen", "p_hismsgs", "g_hismsgs", "p_delhismsgs", "g_delhismsgs", "g_portionrels", "groupmembers", "groupinfoexts", "groupmemberexts", "convercleantimes", "usercleantimes"} {
			if err := db.Exec("DELETE FROM "+table+" WHERE app_key=?", app).Error; err != nil {
				t.Error(err)
			}
		}
	})
	if err := db.Create(&groupdbs.GroupMemberDao{AppKey: app, GroupId: "group", MemberId: "owner", CreatedTime: time.UnixMilli(stamp + 2)}).Error; err != nil {
		t.Fatal(err)
	}
	for seq := int64(1); seq <= 3; seq++ {
		stageSeenHistory(t, db, app, pbobjs.ChannelType_Private, seq, stamp+seq)
		stageSeenHistory(t, db, app, pbobjs.ChannelType_Group, seq, stamp+seq)
	}
	return db, app, stamp
}

func stageSeenHistory(t *testing.T, db *gorm.DB, app string, channel pbobjs.ChannelType, seq, stamp int64) {
	t.Helper()
	if channel == pbobjs.ChannelType_Private {
		if err := db.Create(&hisdbs.PrivateHisMsgDao{AppKey: app, ConverId: commonservices.GetConversationId("owner", "peer", channel), MsgId: fmt.Sprintf("sc-%x", seq), MsgSeqNo: seq, SendTime: stamp, SenderId: "peer", ReceiverId: "owner", ChannelType: int(channel)}).Error; err != nil {
			t.Fatal(err)
		}
	} else if err := db.Create(&hisdbs.GroupHisMsgDao{AppKey: app, ConverId: "group", MsgId: fmt.Sprintf("sc-%x", seq), MsgSeqNo: seq, SendTime: stamp, SenderId: "peer", ChannelType: int(channel)}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestSeenTransactionalVisibilityMySQL(t *testing.T) {
	db, app, stamp := consistentSeenFixture(t)
	ctx := bases.CreateRpcCtx(app, "owner")
	for _, scope := range []struct {
		target  string
		channel pbobjs.ChannelType
		table   string
	}{{"peer", pbobjs.ChannelType_Private, "p_hismsgs"}, {"group", pbobjs.ChannelType_Group, "g_hismsgs"}} {
		req := &pbobjs.QryConverSeenReq{TargetId: scope.target, ChannelType: scope.channel}
		cases := []struct {
			name string
			want int64
			edit func(*gorm.DB) error
		}{
			{"staged history", 4, func(tx *gorm.DB) error { stageSeenHistory(t, tx, app, scope.channel, 4, stamp+4); return nil }},
			{"user clean", 0, func(tx *gorm.DB) error {
				return tx.Create(&hisdbs.HisMsgUserCleanTimeDao{AppKey: app, UserId: "owner", TargetId: scope.target, ChannelType: int(scope.channel), CleanTime: stamp + 4}).Error
			}},
			{"conversation clean", 0, func(tx *gorm.DB) error {
				return tx.Create(&hisdbs.HisMsgConverCleanTimeDao{AppKey: app, ConverId: commonservices.GetConversationId("owner", scope.target, scope.channel), ChannelType: int(scope.channel), CleanTime: stamp + 4}).Error
			}},
			{"global deletion", 2, func(tx *gorm.DB) error {
				return tx.Table(scope.table).Where("app_key=? AND msg_id=?", app, "sc-3").Update("is_delete", 1).Error
			}},
			{"expiry", 2, func(tx *gorm.DB) error {
				return tx.Table(scope.table).Where("app_key=? AND msg_id=?", app, "sc-3").Update("destroy_time", time.Now().Add(-time.Second).UnixMilli()).Error
			}},
			{"recipient deletion", 2, func(tx *gorm.DB) error {
				if scope.channel == pbobjs.ChannelType_Private {
					return tx.Create(&hisdbs.PrivateDelHisMsgDao{AppKey: app, UserId: "owner", TargetId: scope.target, MsgId: "sc-3"}).Error
				}
				return tx.Create(&hisdbs.GroupDelHisMsgDao{AppKey: app, UserId: "owner", TargetId: scope.target, MsgId: "sc-3"}).Error
			}},
		}
		for _, tc := range cases {
			t.Run(scope.target+"/"+tc.name, func(t *testing.T) {
				tx := db.Begin()
				if tx.Error != nil {
					t.Fatal(tx.Error)
				}
				defer tx.Rollback()
				if err := tc.edit(tx); err != nil {
					t.Fatal(err)
				}
				code, state := QryConverSeenWithTx(ctx, tx, req)
				if code != 0 || state == nil || state.LatestSeq != tc.want || state.SeenSeq != 0 || state.HasNew != (tc.want > 0) {
					t.Errorf("staged %s was not observed: code=%d state=%v want_latest=%d", tc.name, code, state, tc.want)
				}
				if tc.name == "staged history" {
					if code, state := SetConverSeenWithTx(ctx, tx, &pbobjs.SetConverSeenReq{TargetId: scope.target, ChannelType: scope.channel, SeenSeq: tc.want}); code != 0 || state == nil || state.SeenSeq != tc.want {
						t.Fatal("advance did not use its transaction's visible history")
					}
				}
				if code, state := QryConverSeen(ctx, req); code != 0 || state == nil || state.LatestSeq != 3 || state.SeenSeq != 0 {
					t.Fatal("uncommitted fixture escaped to native/global visibility")
				}
			})
		}
	}
	t.Run("staged member and history", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()
		if err := tx.Create(&groupdbs.GroupMemberDao{AppKey: app, GroupId: "new-group", MemberId: "owner", CreatedTime: time.UnixMilli(stamp - 1)}).Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Create(&hisdbs.GroupHisMsgDao{AppKey: app, ConverId: "new-group", MsgId: "new-group-msg", MsgSeqNo: 1, SendTime: stamp}).Error; err != nil {
			t.Fatal(err)
		}
		req := &pbobjs.QryConverSeenReq{TargetId: "new-group", ChannelType: pbobjs.ChannelType_Group}
		if code, state := QryConverSeenWithTx(ctx, tx, req); code != 0 || state == nil || state.LatestSeq != 1 {
			t.Fatal("staged member/history were read outside caller transaction")
		}
		if code, state := QryConverSeen(ctx, req); code != errs.IMErrorCode_GROUP_NOTGROUPMEMBER || state != nil {
			t.Fatal("staged member escaped to global readers")
		}
	})
	t.Run("staged member removal", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()
		if err := tx.Where("app_key=? AND group_id=?", app, "group").Delete(&groupdbs.GroupMemberDao{}).Error; err != nil {
			t.Fatal(err)
		}
		if code, state := QryConverSeenWithTx(ctx, tx, &pbobjs.QryConverSeenReq{TargetId: "group", ChannelType: pbobjs.ChannelType_Group}); code != errs.IMErrorCode_GROUP_NOTGROUPMEMBER || state != nil {
			t.Fatalf("staged membership removal ignored: code=%d state=%v", code, state)
		}
	})
	t.Run("staged settings", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()
		if err := tx.Create(&groupdbs.GroupExtDao{AppKey: app, GroupId: "group", ItemType: 1, ItemKey: "hide_grp_msg", ItemValue: "true", UpdatedTime: time.Now()}).Error; err != nil {
			t.Fatal(err)
		}
		req := &pbobjs.QryConverSeenReq{TargetId: "group", ChannelType: pbobjs.ChannelType_Group}
		if code, state := QryConverSeenWithTx(ctx, tx, req); code != 0 || state == nil || state.LatestSeq != 3 {
			t.Fatal("hide-before-join hid a newer visible message")
		}
		if err := tx.Table("g_hismsgs").Where("app_key=? AND msg_id=?", app, "sc-3").Update("is_delete", 1).Error; err != nil {
			t.Fatal(err)
		}
		if code, state := QryConverSeenWithTx(ctx, tx, req); code != 0 || state == nil || state.LatestSeq != 0 {
			t.Fatalf("pre-join boundary or staged settings ignored: code=%d state=%v", code, state)
		}
		if err := tx.Create(&groupdbs.GroupMemberExtDao{AppKey: app, GroupId: "group", MemberId: "owner", ItemType: 1, ItemKey: "hide_grp_msg", ItemValue: "false", UpdatedTime: time.Now()}).Error; err != nil {
			t.Fatal(err)
		}
		if code, state := QryConverSeenWithTx(ctx, tx, req); code != 0 || state == nil || state.LatestSeq != 2 {
			t.Fatal("member setting did not override group setting")
		}
	})
	t.Run("staged portion recipient", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()
		if err := tx.Table("g_hismsgs").Where("app_key=? AND msg_id=?", app, "sc-3").Update("is_portion", 1).Error; err != nil {
			t.Fatal(err)
		}
		req := &pbobjs.QryConverSeenReq{TargetId: "group", ChannelType: pbobjs.ChannelType_Group}
		if code, state := QryConverSeenWithTx(ctx, tx, req); code != 0 || state == nil || state.LatestSeq != 2 {
			t.Fatalf("portion recipient visibility ignored: code=%d state=%v", code, state)
		}
		if err := tx.Create(&hisdbs.GroupPortionRelDao{AppKey: app, ConverId: "group", ChannelType: 2, UserId: "owner", MsgId: "sc-3"}).Error; err != nil {
			t.Fatal(err)
		}
		if code, state := QryConverSeenWithTx(ctx, tx, req); code != 0 || state == nil || state.LatestSeq != 3 {
			t.Fatal("staged portion recipient was not observed")
		}
	})
}

func TestSeenCommittedVisibilityParityMySQL(t *testing.T) {
	db, app, stamp := consistentSeenFixture(t)
	t.Cleanup(func() {
		for _, table := range []string{"p_hismsgs", "g_hismsgs"} {
			if err := db.Exec("DELETE FROM "+table+" WHERE app_key=?", app+"x").Error; err != nil {
				t.Error(err)
			}
		}
	})
	ctx := bases.CreateRpcCtx(app, "owner")
	for _, scope := range []struct {
		target  string
		channel pbobjs.ChannelType
	}{{"peer", pbobjs.ChannelType_Private}, {"group", pbobjs.ChannelType_Group}} {
		t.Run(scope.target, func(t *testing.T) {
			req := &pbobjs.QryConverSeenReq{TargetId: scope.target, ChannelType: scope.channel}
			check := func(want int64) {
				t.Helper()
				code, native := QryConverSeen(ctx, req)
				if code != 0 || native == nil || native.LatestSeq != want {
					t.Fatalf("native parity fixture unavailable: code=%d state=%v want=%d", code, native, want)
				}
				tx := db.Begin()
				defer tx.Rollback()
				code, transactional := QryConverSeenWithTx(ctx, tx, req)
				if code != 0 || transactional == nil || transactional.LatestSeq != native.LatestSeq || transactional.SeenSeq != native.SeenSeq || transactional.HasNew != native.HasNew {
					t.Fatalf("native/transaction visibility differs: native=%v code=%d tx=%v", native, code, transactional)
				}
			}
			// These higher sequences must not cross tenant or subchannel boundaries.
			conver := commonservices.GetConversationId("owner", scope.target, scope.channel)
			if scope.channel == pbobjs.ChannelType_Private {
				for _, foreign := range []hisdbs.PrivateHisMsgDao{{AppKey: app + "x", ConverId: conver, MsgId: "foreign", MsgSeqNo: 99, SendTime: stamp + 99}, {AppKey: app, ConverId: conver, SubChannel: "thread", MsgId: "thread", MsgSeqNo: 88, SendTime: stamp + 88}} {
					if err := db.Create(&foreign).Error; err != nil {
						t.Fatal(err)
					}
				}
			} else {
				for _, foreign := range []hisdbs.GroupHisMsgDao{{AppKey: app + "x", ConverId: conver, MsgId: "foreign", MsgSeqNo: 99, SendTime: stamp + 99}, {AppKey: app, ConverId: conver, SubChannel: "thread", MsgId: "thread", MsgSeqNo: 88, SendTime: stamp + 88}} {
					if err := db.Create(&foreign).Error; err != nil {
						t.Fatal(err)
					}
				}
			}
			check(3)
			future := time.Now().Add(time.Hour).UnixMilli()
			if scope.channel == pbobjs.ChannelType_Private {
				if err := db.Create(&hisdbs.PrivateDelHisMsgDao{AppKey: app, UserId: "owner", TargetId: "peer", MsgId: "sc-3", EffectiveTime: future}).Error; err != nil {
					t.Fatal(err)
				}
				check(2)
			} else {
				deleted := &hisdbs.GroupDelHisMsgDao{AppKey: app, UserId: "owner", TargetId: "group", MsgId: "sc-3", EffectiveTime: future}
				if err := db.Create(deleted).Error; err != nil {
					t.Fatal(err)
				}
				check(3)
				if err := db.Model(deleted).Update("effective_time", int64(0)).Error; err != nil {
					t.Fatal(err)
				}
				check(2)
			}
		})
	}
}

func TestSeenTransactionRejectsUnsupportedStoreAndCancellationMySQL(t *testing.T) {
	db, app, stamp := consistentSeenFixture(t)
	ctx := bases.CreateRpcCtx(app, "owner")
	req := &pbobjs.QryConverSeenReq{TargetId: "peer", ChannelType: pbobjs.ChannelType_Private}
	for _, engine := range []string{configures.MsgStoreEngine_Mongo, "unsupported"} {
		tx := db.Begin()
		previous := configures.Config.MsgStoreEngine
		configures.Config.MsgStoreEngine = engine
		code, state := QryConverSeenWithTx(ctx, tx, req)
		configures.Config.MsgStoreEngine = previous
		if code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || state != nil {
			t.Fatal("non-MySQL history silently became an SQL zero snapshot")
		}
		if err := tx.Exec("SELECT 1").Error; err == nil {
			t.Fatal("unsupported store did not abort transaction")
		}
	}
	tx := db.Begin()
	defer tx.Rollback()
	stageSeenHistory(t, tx, app, pbobjs.ChannelType_Private, 4, stamp+4)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if code, state := QryConverSeenWithTx(cancelled, tx, req); code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || state != nil {
		t.Fatal("cancelled query returned success")
	}
	if err := tx.Exec("SELECT 1").Error; err == nil {
		t.Fatal("cancelled visibility read left a reusable transaction")
	}
	if code, state := QryConverSeen(ctx, req); code != 0 || state == nil || state.LatestSeq != 3 {
		t.Fatal("failed visibility read committed staged history")
	}
}
