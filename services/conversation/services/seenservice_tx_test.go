package services

import (
	"context"
	"errors"
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
	converdbs "im-server/services/conversation/storages/dbs"
	groupdbs "im-server/services/group/dbs"
	hisdbs "im-server/services/historymsg/storages/dbs"
	hismodels "im-server/services/historymsg/storages/models"

	mysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSeenTxRequiresTransaction(t *testing.T) {
	ctx := bases.CreateRpcCtx("tenant", "owner")
	for _, tx := range []*gorm.DB{nil, {}, {Error: errors.New("failed begin")}} {
		if code, state := QryConverSeenWithTx(ctx, tx, &pbobjs.QryConverSeenReq{TargetId: "peer", ChannelType: pbobjs.ChannelType_Private}); code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || state != nil {
			t.Fatal("query accepted a missing/failed transaction")
		}
		if code, state := SetConverSeenWithTx(ctx, tx, &pbobjs.SetConverSeenReq{TargetId: "peer", ChannelType: pbobjs.ChannelType_Private, SeenSeq: 1}); code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || state != nil {
			t.Fatal("advance accepted a missing/failed transaction")
		}
	}
}

func TestSeenTxMySQL(t *testing.T) {
	if os.Getenv("JIM_F1_READ_ISOLATED") != "im-f1-read-20261106" {
		t.Skip("requires the owned F1-READ MySQL fixture")
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
	db := dbcommons.GetDb()
	db.Config.Logger = logger.Default.LogMode(logger.Silent)
	app := fmt.Sprintf("st%x", time.Now().UnixNano())
	t.Cleanup(func() {
		for _, table := range []string{"conversation_seen", "p_hismsgs", "g_hismsgs", "groupmembers", "groupinfoexts", "groupmemberexts", "convercleantimes", "usercleantimes"} {
			if err := db.Exec("DELETE FROM "+table+" WHERE app_key=?", app).Error; err != nil {
				t.Error(err)
			}
		}
	})
	if err := (groupdbs.GroupMemberDao{}).Create(groupdbs.GroupMemberDao{AppKey: app, GroupId: "group", MemberId: "owner", CreatedTime: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	for seq := int64(1); seq <= 3; seq++ {
		msg := hismodels.HisMsg{AppKey: app, MsgId: fmt.Sprintf("tx-%d", seq), MsgSeqNo: seq, SendTime: time.Now().Add(-time.Second).UnixMilli() + seq}
		msg.ConverId, msg.SenderId, msg.ReceiverId, msg.ChannelType = commonservices.GetConversationId("owner", "peer", pbobjs.ChannelType_Private), "peer", "owner", pbobjs.ChannelType_Private
		if err := (hisdbs.PrivateHisMsgDao{}).SavePrivateHisMsg(hismodels.PrivateHisMsg{HisMsg: msg}); err != nil {
			t.Fatal(err)
		}
		msg.ConverId, msg.ChannelType = "group", pbobjs.ChannelType_Group
		if err := (hisdbs.GroupHisMsgDao{}).SaveGroupHisMsg(hismodels.GroupHisMsg{HisMsg: msg}); err != nil {
			t.Fatal(err)
		}
	}
	for _, scope := range []struct {
		target  string
		channel pbobjs.ChannelType
	}{{"peer", pbobjs.ChannelType_Private}, {"group", pbobjs.ChannelType_Group}} {
		t.Run(scope.target, func(t *testing.T) {
			ctx := bases.CreateRpcCtx(app, "owner")
			query := &pbobjs.QryConverSeenReq{TargetId: scope.target, ChannelType: scope.channel}
			request := &pbobjs.SetConverSeenReq{TargetId: scope.target, ChannelType: scope.channel, SeenSeq: 3}
			assertStored := func(want int64) {
				t.Helper()
				got, err := (converdbs.SeenDao{}).Find(app, "owner", scope.target, "", scope.channel)
				if err != nil || got != want {
					t.Fatalf("committed seen=%d want=%d err=%v", got, want, err)
				}
			}
			if code, state := SetConverSeenWithTx(ctx, db, request); code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || state != nil {
				t.Fatal("ordinary DB handle was accepted as a transaction")
			}
			if err := db.Transaction(func(tx *gorm.DB) error {
				code, state := QryConverSeenWithTx(ctx, tx, query)
				if code != 0 || state == nil || state.SeenSeq != 0 || state.LatestSeq != 3 || !state.HasNew {
					t.Fatal("transaction query did not read visible persisted history")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			var rows int64
			if err := db.Raw("SELECT COUNT(*) FROM conversation_seen WHERE app_key=? AND target_id=?", app, scope.target).Scan(&rows).Error; err != nil || rows != 0 {
				t.Fatal("query created a seen row")
			}
			abort := errors.New("caller rolls back its operation")
			err := db.Transaction(func(tx *gorm.DB) error {
				code, state := SetConverSeenWithTx(ctx, tx, request)
				if code != 0 || state == nil || state.SeenSeq != 3 || state.HasNew {
					t.Fatal("transaction could not read its own seen write")
				}
				code, state = QryConverSeenWithTx(ctx, tx, query)
				if code != 0 || state == nil || state.SeenSeq != 3 {
					t.Fatal("query lost uncommitted seen in caller transaction")
				}
				assertStored(0)
				return abort
			})
			if !errors.Is(err, abort) {
				t.Fatal("caller rollback was not returned")
			}
			assertStored(0)
			request.SeenSeq = 1
			if err := db.Transaction(func(tx *gorm.DB) error {
				if code, state := SetConverSeenWithTx(ctx, tx, request); code != 0 || state == nil || state.SeenSeq != 1 {
					t.Fatal("transaction advance failed")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			assertStored(1)
			tx := db.Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			defer tx.Rollback()
			var earlier int64
			if err := tx.Raw("SELECT seen_seq FROM conversation_seen WHERE app_key=? AND user_id='owner' AND target_id=? AND channel_type=?", app, scope.target, scope.channel).Scan(&earlier).Error; err != nil || earlier != 1 {
				t.Fatal("could not establish caller's earlier snapshot")
			}
			request.SeenSeq = 3
			if code, state := SetConverSeen(ctx, request); code != 0 || state == nil || state.SeenSeq != 3 {
				t.Fatal("other device did not commit a newer max")
			}
			for _, seq := range []int64{1, 2, 3, 2} {
				request.SeenSeq = seq
				if code, state := SetConverSeenWithTx(ctx, tx, request); code != 0 || state == nil || state.SeenSeq != 3 || state.HasNew {
					t.Fatal("caller snapshot hid a newer max or an older device regressed state")
				}
			}
			request.SeenSeq = 4
			if code, state := SetConverSeenWithTx(ctx, tx, request); code != errs.IMErrorCode_MSG_PARAM_ILLEGAL || state != nil {
				t.Fatal("transaction accepted a forged future position")
			}
			if err := tx.Commit().Error; err != nil {
				t.Fatal(err)
			}
			assertStored(3)
			request.SeenSeq = 3
			if code, state := SetConverSeenWithTx(ctx, tx, request); code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || state != nil {
				t.Fatal("finished transaction fell back to a successful global write")
			}
			assertStored(3)
			testSeenTxDeadlock(t, ctx, db, app, scope.target, scope.channel)
			t.Logf("app=%s scope=%s rollback=0 commit=1 newer_device=3 stale_snapshot_max=3 future_rejected=true finished_tx_rejected=true", app, scope.target)
		})
	}
}

type seenTxErrorLog struct {
	logger.Interface
	errors chan error
}

func (l seenTxErrorLog) Trace(_ context.Context, _ time.Time, _ func() (string, int64), err error) {
	if err != nil {
		l.errors <- err
	}
}

func testSeenTxDeadlock(t *testing.T, ctx context.Context, db *gorm.DB, app, target string, channel pbobjs.ChannelType) {
	t.Helper()
	clear := func() {
		t.Helper()
		if err := db.Exec("DELETE FROM conversation_seen WHERE app_key=? AND target_id=?", app, target).Error; err != nil {
			t.Fatal(err)
		}
	}
	clear()
	logged := make(chan error, 8)
	type result struct {
		tx   *gorm.DB
		code int32
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for _, seq := range []int64{1, 3} {
		tx := db.Session(&gorm.Session{Logger: seenTxErrorLog{Interface: db.Logger, errors: logged}}).Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		defer tx.Rollback()
		// Missing unique-key rows can hold compatible gap locks before either caller inserts.
		if _, err := (converdbs.SeenDao{DB: tx}).Find(app, "owner", target, "", channel); err != nil {
			t.Fatal(err)
		}
		go func(tx *gorm.DB, seq int64) {
			<-start
			code, _ := SetConverSeenWithTx(ctx, tx, &pbobjs.SetConverSeenReq{TargetId: target, ChannelType: channel, SeenSeq: seq})
			results <- result{tx: tx, code: int32(code)}
		}(tx, seq)
	}
	close(start)
	var victim *gorm.DB
	for range 2 {
		r := <-results
		if r.code == 0 {
			if err := r.tx.Commit().Error; err != nil {
				t.Fatal(err)
			}
		} else if r.code == int32(errs.IMErrorCode_API_INTERNAL_RESP_FAIL) && victim == nil {
			victim = r.tx
		} else {
			t.Fatalf("unexpected concurrent transaction result %d", r.code)
		}
	}
	var deadlock bool
	for len(logged) > 0 {
		var sqlErr *mysql.MySQLError
		if errors.As(<-logged, &sqlErr) && sqlErr.Number == 1213 {
			deadlock = true
		}
	}
	if victim == nil || !deadlock {
		t.Fatal("fixture did not reproduce a real MySQL 1213 victim")
	}
	t.Logf("scope=%s real_mysql_deadlock=1213", target)
	clear()
	queryCode, queryState := QryConverSeenWithTx(ctx, victim, &pbobjs.QryConverSeenReq{TargetId: target, ChannelType: channel})
	code, state := SetConverSeenWithTx(ctx, victim, &pbobjs.SetConverSeenReq{TargetId: target, ChannelType: channel, SeenSeq: 3})
	victim.Rollback()
	stored, err := (converdbs.SeenDao{}).Find(app, "owner", target, "", channel)
	if queryCode != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || queryState != nil || code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || state != nil || err != nil || stored != 0 {
		t.Fatalf("deadlock victim reused: query_code=%d code=%d state_present=%v persisted_after_rollback=%d err=%v", queryCode, code, state != nil, stored, err)
	}
	t.Logf("scope=%s mysql_deadlock=1213 victim_reuse_rejected=true persisted_after_rollback=0", target)
	if err := db.Transaction(func(tx *gorm.DB) error {
		if code, state := SetConverSeenWithTx(ctx, tx, &pbobjs.SetConverSeenReq{TargetId: target, ChannelType: channel, SeenSeq: 3}); code != 0 || state == nil || state.SeenSeq != 3 {
			return errors.New("fresh transaction could not retry the aborted advancement")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if stored, err := (converdbs.SeenDao{}).Find(app, "owner", target, "", channel); err != nil || stored != 3 {
		t.Fatal("fresh transaction retry did not commit max=3")
	}
}
