package services

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/conversation/storages/dbs"
	"im-server/services/conversation/storages/models"

	"github.com/bytedance/gopkg/collection/zset"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Only the owned F0 Docker network is supported; there is no target URL override.
func persistenceTestTenant(t *testing.T) string {
	t.Helper()
	if os.Getenv("JIM_F1_PERSIST_ISOLATED") != "im-f0-20261105" {
		t.Skip("requires the inspected local im-f0-20261105 MySQL fixture")
	}
	password := os.Getenv("MYSQL_ROOT_PASSWORD")
	if password == "" {
		t.Fatal("isolated fixture password missing")
	}
	configures.Config.Mysql.User = "root"
	configures.Config.Mysql.Password = password
	configures.Config.Mysql.Address = "im-f0-20261105-mysql-1:3306"
	configures.Config.Mysql.DbName = "jim_db"
	if err := dbcommons.InitMysql(); err != nil {
		t.Fatal("isolated MySQL initialization failed")
	}
	dbcommons.GetDb().Config.Logger = logger.Default.LogMode(logger.Silent)
	app := fmt.Sprintf("fp%x", time.Now().UnixNano())
	t.Logf("isolated test tenant=%s", app)
	t.Cleanup(func() {
		if err := dbcommons.GetDb().Exec("DELETE FROM conversations WHERE app_key IN (?, ?, ?, ?)", app, app+"b", app+"_a", app+"f").Error; err != nil {
			t.Error("test tenant cleanup failed")
		}
	})
	return app
}

func TestConversationPersistenceQueueMySQL(t *testing.T) {
	app := persistenceTestTenant(t)
	makeUser := func(tenant string) *UserConversations {
		return &UserConversations{Appkey: tenant, UserId: "same-user", ConverItemMap: map[string]*models.Conversation{},
			SyncTimeIndex: zset.NewFloat64(), SortTimeIndex: zset.NewFloat64(),
			TopIndexBaseTopTime: zset.NewFloat64(), TopIndexBaseSortTime: zset.NewFloat64()}
	}
	flush := func(user *UserConversations, snapshot *models.Conversation) {
		user.PersistConverV2(snapshot)
		key := converPersistKey(user.Appkey, user.UserId, snapshot.TargetId, snapshot.SubChannel, snapshot.ChannelType)
		persistCache.Remove(key) // Real eviction callback and actual SQL, with a controlled flush.
	}
	assertRow := func(tenant, id string, deleted int) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			var row struct {
				LatestMsgID string
				IsDeleted   int
			}
			query := dbcommons.GetDb().Raw("SELECT latest_msg_id, is_deleted FROM conversations WHERE app_key=? AND user_id=? AND target_id=? AND channel_type=? AND sub_channel=''", tenant, "same-user", "target", pbobjs.ChannelType_Group).Scan(&row)
			if query.Error != nil {
				t.Fatal("actual conversation query failed")
			}
			if query.RowsAffected == 1 && row.LatestMsgID == id && row.IsDeleted == deleted {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("actual DB did not persist expected snapshot: tenant=%s id=%s deleted=%d", tenant, id, deleted)
	}
	user := makeUser(app)
	first := user.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "first", SyncTime: 1, SortTime: 1})
	flush(user, first)
	assertRow(app, "first", 0)
	latest := user.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "latest", SyncTime: 2, SortTime: 2})
	flush(user, latest)
	assertRow(app, "latest", 0)
	if !user.DelConversation("target", "", pbobjs.ChannelType_Group) {
		t.Fatal("delete did not change conversation")
	}
	flush(user, user.QryConver("target", "", pbobjs.ChannelType_Group))
	assertRow(app, "latest", 1)
	other := makeUser(app + "b")
	flush(other, other.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "other", SyncTime: 3, SortTime: 3}))
	assertRow(app+"b", "other", 0)
	assertRow(app, "latest", 1)

	// These identities collided in the old underscore-joined persistence key.
	left := makeUser(app + "_a")
	right := makeUser(app)
	right.UserId = "a_same-user"
	leftSnapshot := left.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "left", SyncTime: 4, SortTime: 4})
	rightSnapshot := right.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "right", SyncTime: 4, SortTime: 4})
	left.PersistConverV2(leftSnapshot)
	right.PersistConverV2(rightSnapshot)
	persistCache.Remove(converPersistKey(left.Appkey, left.UserId, "target", "", pbobjs.ChannelType_Group))
	persistCache.Remove(converPersistKey(right.Appkey, right.UserId, "target", "", pbobjs.ChannelType_Group))
	assertRow(app+"_a", "left", 0)
	var rightCount int64
	if err := dbcommons.GetDb().Raw("SELECT COUNT(*) FROM conversations WHERE app_key=? AND user_id=? AND latest_msg_id=?", app, "a_same-user", "right").Scan(&rightCount).Error; err != nil || rightCount != 1 {
		t.Fatal("second colliding identity not stored independently")
	}

	frozen := makeUser(app + "f")
	snapshot := frozen.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "frozen", SyncTime: 5, SortTime: 5, ConverExts: &pbobjs.ConverExts{ConverTags: map[string]bool{"before": true}}})
	frozen.PersistConverV2(snapshot)
	snapshot.ConverExts.ConverTags["after"] = true
	delete(frozen.ConverItemMap, getConverItemKey("target", "", pbobjs.ChannelType_Group))
	persistCache.Remove(converPersistKey(frozen.Appkey, frozen.UserId, "target", "", pbobjs.ChannelType_Group))
	assertRow(app+"f", "frozen", 0)
	dao := dbs.ConversationDao{}
	stored, err := dao.FindOne(app+"f", "same-user", "target", "", pbobjs.ChannelType_Group)
	if err != nil || stored.ConverExts == nil || !stored.ConverExts.ConverTags["before"] || stored.ConverExts.ConverTags["after"] {
		t.Fatal("queued snapshot did not survive cache eviction independently")
	}

	var injected atomic.Bool
	callbacks := dbcommons.GetDb().Callback().Raw()
	err = callbacks.Before("gorm:raw").Register("f1_transient_error", func(tx *gorm.DB) {
		if strings.HasPrefix(tx.Statement.SQL.String(), "INSERT INTO conversations") && len(tx.Statement.Vars) > 7 && tx.Statement.Vars[0] == app+"b" && tx.Statement.Vars[7] == "retry" && injected.CompareAndSwap(false, true) {
			tx.AddError(errors.New("injected transient storage failure"))
		}
	})
	if err != nil {
		t.Fatal("fault injection setup failed")
	}
	t.Cleanup(func() { callbacks.Remove("f1_transient_error") })
	flush(other, other.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "retry", SyncTime: 6, SortTime: 6}))
	assertRow(app+"b", "retry", 0)
	if !injected.Load() {
		t.Fatal("transient failure branch was not exercised")
	}

	started, release := make(chan struct{}), make(chan struct{})
	secondAtStorage := make(chan struct{}, 1)
	var blockOnce atomic.Bool
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	err = callbacks.Before("gorm:raw").Register("f1_write_order", func(tx *gorm.DB) {
		if strings.HasPrefix(tx.Statement.SQL.String(), "INSERT INTO conversations") && len(tx.Statement.Vars) > 17 && tx.Statement.Vars[0] == app && tx.Statement.Vars[7] == "slow" && tx.Statement.Vars[17] == 1 {
			select {
			case secondAtStorage <- struct{}{}:
			default:
			}
		}
		if strings.HasPrefix(tx.Statement.SQL.String(), "INSERT INTO conversations") && len(tx.Statement.Vars) > 7 && tx.Statement.Vars[0] == app && tx.Statement.Vars[7] == "slow" && blockOnce.CompareAndSwap(false, true) {
			close(started)
			<-release
		}
	})
	if err != nil {
		t.Fatal("write-order injection setup failed")
	}
	t.Cleanup(func() { callbacks.Remove("f1_write_order") })
	flush(user, user.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "slow", SyncTime: 7, SortTime: 7}))
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("first write did not enter controlled storage boundary")
	}
	if !user.DelConversation("target", "", pbobjs.ChannelType_Group) {
		t.Fatal("concurrent deletion did not update cache")
	}
	flushed := make(chan struct{})
	go func() { flush(user, user.QryConver("target", "", pbobjs.ChannelType_Group)); close(flushed) }()
	select {
	case <-flushed:
	case <-time.After(3 * time.Second):
		t.Fatal("second flush deadlocked")
	}
	if len(persistSem) < 2 {
		t.Fatal("second flush was not admitted while the first write remained blocked")
	}
	select {
	case <-secondAtStorage:
		t.Fatal("new deletion reached storage before the blocked old write finished")
	case <-time.After(150 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	deadline := time.Now().Add(3 * time.Second)
	for len(persistSem) > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(persistSem) > 0 {
		t.Fatal("persistence workers did not finish")
	}
	assertRow(app, "slow", 1)
}

func TestConversationUserCacheTenantIsolationMySQL(t *testing.T) {
	app := persistenceTestTenant(t)
	left := getUserConvers(app+"_a", "u")
	right := getUserConvers(app, "a_u")
	t.Cleanup(func() {
		userConverCache.Remove(getUserConverCacheKey(app+"_a", "u"))
		userConverCache.Remove(getUserConverCacheKey(app, "a_u"))
	})
	if left == right || right.Appkey != app || right.UserId != "a_u" {
		t.Fatal("real user cache aliases different tenant/user identities")
	}
}

func TestConversationPermanentFailureDoesNotBlockQueueMySQL(t *testing.T) {
	app := persistenceTestTenant(t)
	bad := getUserConvers(app, "bad")
	snapshot := bad.UpsertCovner(models.Conversation{TargetId: strings.Repeat("x", 100), ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "invalid", SyncTime: 1, SortTime: 1})
	dao := dbs.ConversationDao{}
	err := dao.Upsert(*snapshot)
	var mysqlError *mysqldriver.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError.Number != 1406 {
		t.Fatal("expected actual MySQL data-too-long error")
	}
	key := converPersistKey(app, "bad", snapshot.TargetId, "", pbobjs.ChannelType_Group)
	bad.PersistConverV2(snapshot)
	persistCache.Remove(key)
	go func() { bad.PersistConverV2(snapshot); persistCache.Remove(key) }()
	good := getUserConvers(app, "good")
	done := make(chan struct{})
	go func() {
		s := good.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "good", SyncTime: 2, SortTime: 2})
		good.PersistConverV2(s)
		persistCache.Remove(converPersistKey(app, "good", "target", "", pbobjs.ChannelType_Group))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("permanent SQL error blocked unrelated queue admission")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var count int64
		q := dbcommons.GetDb().Raw("SELECT COUNT(*) FROM conversations WHERE app_key=? AND user_id=? AND latest_msg_id=?", app, "good", "good").Scan(&count)
		if q.Error != nil {
			t.Fatal("query of successful independent write failed")
		}
		if count == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("independent write did not reach actual MySQL")
}

func TestConversationLatestSnapshotSurvivesEnqueueInterleaveMySQL(t *testing.T) {
	app := persistenceTestTenant(t)
	key := converPersistKey(app, "interleave", "target", "", pbobjs.ChannelType_Group)
	old := &models.Conversation{AppKey: app, UserId: "interleave", TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "old", SyncTime: 1, SortTime: 1}
	latest := *old
	latest.LatestMsgId = "latest"
	latest.SyncTime = 2
	latest.SortTime = 2
	// Reproduce Store(A), Store(B), Add(B), Add(A) at the real queue boundary.
	persistLatest.Store(key, old)
	persistLatest.Store(key, &latest)
	persistCache.Add(key, &latest)
	persistCache.Add(key, old)
	persistCache.Remove(key)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var id string
		q := dbcommons.GetDb().Raw("SELECT latest_msg_id FROM conversations WHERE app_key=? AND user_id=?", app, "interleave").Scan(&id)
		if q.Error != nil {
			t.Fatal("interleaved persistence query failed")
		}
		if id == "latest" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("stale queue notification lost the authoritative newer snapshot")
}

func TestConversationFullWorkerQuotaDoesNotBlockAdmissionMySQL(t *testing.T) {
	app := persistenceTestTenant(t)
	deadline := time.Now().Add(3 * time.Second)
	for len(persistSem) > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(persistSem) > 0 {
		t.Fatal("previous persistence workers did not finish")
	}
	for i := 0; i < persistWorkerCount; i++ {
		persistSem <- struct{}{}
	}
	reserved := true
	defer func() {
		if reserved {
			for i := 0; i < persistWorkerCount; i++ {
				<-persistSem
			}
		}
	}()
	user := getUserConvers(app, "quota")
	snapshot := user.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "quota", SyncTime: 1, SortTime: 1})
	done := make(chan struct{})
	go func() {
		user.PersistConverV2(snapshot)
		persistCache.Remove(converPersistKey(app, "quota", "target", "", pbobjs.ChannelType_Group))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("full worker quota blocked cache admission")
	}
	for i := 0; i < persistWorkerCount; i++ {
		<-persistSem
	}
	reserved = false
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var count int64
		q := dbcommons.GetDb().Raw("SELECT COUNT(*) FROM conversations WHERE app_key=? AND user_id=? AND latest_msg_id=?", app, "quota", "quota").Scan(&count)
		if q.Error != nil {
			t.Fatal("deferred write query failed")
		}
		if count == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("quota-delayed snapshot was dropped instead of retried")
}

func TestConversationRecoverableMySQLErrorsRetryMySQL(t *testing.T) {
	app := persistenceTestTenant(t)
	user := getUserConvers(app, "resource")
	for _, code := range []uint16{1203, 1114, 1021} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			id := fmt.Sprintf("resource-%d", code)
			var injected atomic.Bool
			name := fmt.Sprintf("f1_resource_%d", code)
			callbacks := dbcommons.GetDb().Callback().Raw()
			err := callbacks.Before("gorm:raw").Register(name, func(tx *gorm.DB) {
				if strings.HasPrefix(tx.Statement.SQL.String(), "INSERT INTO conversations") && len(tx.Statement.Vars) > 7 && tx.Statement.Vars[0] == app && tx.Statement.Vars[7] == id && injected.CompareAndSwap(false, true) {
					tx.AddError(&mysqldriver.MySQLError{Number: code, Message: "injected resource capacity error"})
				}
			})
			if err != nil {
				t.Fatal("resource fault injection setup failed")
			}
			t.Cleanup(func() { callbacks.Remove(name) })
			snapshot := user.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: id, SyncTime: int64(code), SortTime: int64(code)})
			user.PersistConverV2(snapshot)
			persistCache.Remove(converPersistKey(app, "resource", "target", "", pbobjs.ChannelType_Group))
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				var stored string
				q := dbcommons.GetDb().Raw("SELECT latest_msg_id FROM conversations WHERE app_key=? AND user_id=?", app, "resource").Scan(&stored)
				if q.Error != nil {
					t.Fatal("resource recovery query failed")
				}
				if stored == id && injected.Load() {
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatal("recoverable capacity error discarded the pending snapshot")
		})
	}
}

func TestConversationClearAllUnreadSurvivesQueuedSnapshotMySQL(t *testing.T) {
	app := persistenceTestTenant(t)
	user := getUserConvers(app, "all-read")
	t.Cleanup(func() { userConverCache.Remove(getUserConverCacheKey(app, "all-read")) })
	snapshot := user.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "all-read", SyncTime: 1, SortTime: 1, LatestUnreadMsgIndex: 1})
	dao := dbs.ConversationDao{}
	if err := dao.Upsert(*snapshot); err != nil {
		t.Fatal("existing unread row could not be seeded")
	}
	user.PersistConverV2(snapshot)
	if err := user.ClearTotalUnread(); err != nil {
		t.Fatal("real clear-all SQL failed")
	}
	key := converPersistKey(app, "all-read", "target", "", pbobjs.ChannelType_Group)
	persistCache.Remove(key)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, pending := persistLatest.Load(key)
		if !pending && len(persistSem) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stored, err := dao.FindOne(app, "all-read", "target", "", pbobjs.ChannelType_Group)
	if err != nil || stored.LatestReadMsgIndex != 1 || stored.LatestUnreadMsgIndex != 1 || stored.UnreadTag != 0 {
		t.Fatal("queued snapshot resurrected unread state after clear-all")
	}
	userConverCache.Remove(getUserConverCacheKey(app, "all-read"))
	reloaded := getUserConvers(app, "all-read")
	if reloaded.TotalUnreadCount(nil) != 0 {
		t.Fatal("DB reload resurrected cleared unread state")
	}
}

func TestConversationClearAllUnreadWaitsForInFlightSnapshotMySQL(t *testing.T) {
	app := persistenceTestTenant(t)
	user := getUserConvers(app, "all-inflight")
	snapshot := user.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "all-inflight", SyncTime: 1, SortTime: 1, LatestUnreadMsgIndex: 1})
	started, release := make(chan struct{}), make(chan struct{})
	var once atomic.Bool
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	callbacks := dbcommons.GetDb().Callback().Raw()
	if err := callbacks.Before("gorm:raw").Register("f1_all_inflight", func(tx *gorm.DB) {
		if strings.HasPrefix(tx.Statement.SQL.String(), "INSERT INTO conversations") && len(tx.Statement.Vars) > 7 && tx.Statement.Vars[0] == app && once.CompareAndSwap(false, true) {
			close(started)
			<-release
		}
	}); err != nil {
		t.Fatal("all-read ordering setup failed")
	}
	t.Cleanup(func() {
		callbacks.Remove("f1_all_inflight")
		userConverCache.Remove(getUserConverCacheKey(app, "all-inflight"))
	})
	key := converPersistKey(app, "all-inflight", "target", "", pbobjs.ChannelType_Group)
	user.PersistConverV2(snapshot)
	persistCache.Remove(key)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("old snapshot did not reach controlled SQL boundary")
	}
	cleared := make(chan error, 1)
	go func() { cleared <- user.ClearTotalUnread() }()
	select {
	case <-cleared:
		t.Fatal("clear-all returned before an earlier SQL write completed")
	case <-time.After(150 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-cleared:
		if err != nil {
			t.Fatal("ordered clear-all SQL failed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("clear-all deadlocked after earlier write completed")
	}
	persistCache.Remove(key)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, pending := persistLatest.Load(key)
		if !pending && len(persistSem) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	dao := dbs.ConversationDao{}
	stored, err := dao.FindOne(app, "all-inflight", "target", "", pbobjs.ChannelType_Group)
	if err != nil || stored.LatestReadMsgIndex != 1 || stored.LatestUnreadMsgIndex != 1 {
		t.Fatal("in-flight old write reverted clear-all read state")
	}
}

func TestConversationClearAllSQLFailureKeepsUnreadMySQL(t *testing.T) {
	app := persistenceTestTenant(t)
	user := getUserConvers(app, "clear-fail")
	snapshot := user.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "clear-fail", SyncTime: 1, SortTime: 1, LatestUnreadMsgIndex: 1})
	dao := dbs.ConversationDao{}
	if err := dao.Upsert(*snapshot); err != nil {
		t.Fatal("unread seed write failed")
	}
	var injected atomic.Bool
	callbacks := dbcommons.GetDb().Callback().Raw()
	if err := callbacks.Before("gorm:raw").Register("f1_clear_error", func(tx *gorm.DB) {
		if strings.HasPrefix(tx.Statement.SQL.String(), "UPDATE conversations set latest_read_msg_index") && len(tx.Statement.Vars) > 0 && tx.Statement.Vars[0] == app && injected.CompareAndSwap(false, true) {
			tx.AddError(errors.New("injected clear-all SQL failure"))
		}
	}); err != nil {
		t.Fatal("clear failure setup failed")
	}
	t.Cleanup(func() {
		callbacks.Remove("f1_clear_error")
		userConverCache.Remove(getUserConverCacheKey(app, "clear-fail"))
	})
	if err := user.ClearTotalUnread(); err == nil || !injected.Load() {
		t.Fatal("clear-all SQL failure was reported as successful")
	}
	if user.TotalUnreadCount(nil) != 1 {
		t.Fatal("failed SQL changed cached read state")
	}
	stored, err := dao.FindOne(app, "clear-fail", "target", "", pbobjs.ChannelType_Group)
	if err != nil || stored.LatestReadMsgIndex != 0 {
		t.Fatal("failed clear changed actual DB state")
	}
}

func TestConversationClearAllUnreadSurvivesEvictedSnapshotMySQL(t *testing.T) {
	app := persistenceTestTenant(t)
	user := getUserConvers(app, "evicted-read")
	snapshot := user.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "evicted-read", SyncTime: 1, SortTime: 1, LatestUnreadMsgIndex: 1})
	user.PersistConverV2(snapshot)
	userKey := getUserConverCacheKey(app, "evicted-read")
	userConverCache.Remove(userKey)
	t.Cleanup(func() { userConverCache.Remove(userKey) })
	reloaded := getUserConvers(app, "evicted-read")
	if item := reloaded.QryConver("target", "", pbobjs.ChannelType_Group); item == nil || item.LatestMsgId != "evicted-read" {
		t.Fatal("user reload did not restore its pending snapshot")
	}
	if err := reloaded.ClearTotalUnread(); err != nil {
		t.Fatal("clear-all after user eviction failed")
	}
	key := converPersistKey(app, "evicted-read", "target", "", pbobjs.ChannelType_Group)
	persistCache.Remove(key)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, pending := persistLatest.Load(key)
		if !pending && len(persistSem) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	dao := dbs.ConversationDao{}
	stored, err := dao.FindOne(app, "evicted-read", "target", "", pbobjs.ChannelType_Group)
	if err != nil || stored.LatestReadMsgIndex != 1 || stored.LatestUnreadMsgIndex != 1 || stored.LatestMsgId != "evicted-read" {
		t.Fatal("evicted dirty snapshot resurrected unread state after clear-all")
	}
}

func TestConversationClearAllUnreadPreservesNewerPendingAfterReloadMySQL(t *testing.T) {
	app := persistenceTestTenant(t)
	user := getUserConvers(app, "stale-reload")
	old := user.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "old", SyncTime: 1, SortTime: 1, LatestUnreadMsgIndex: 1})
	dao := dbs.ConversationDao{}
	if err := dao.Upsert(*old); err != nil {
		t.Fatal("old conversation seed failed")
	}
	latest := user.UpsertCovner(models.Conversation{TargetId: "target", ChannelType: pbobjs.ChannelType_Group, LatestMsgId: "new", SyncTime: 2, SortTime: 2, LatestUnreadMsgIndex: 2})
	user.PersistConverV2(latest)
	userKey := getUserConverCacheKey(app, "stale-reload")
	userConverCache.Remove(userKey)
	t.Cleanup(func() { userConverCache.Remove(userKey) })
	reloaded := getUserConvers(app, "stale-reload")
	if item := reloaded.QryConver("target", "", pbobjs.ChannelType_Group); item == nil || item.LatestMsgId != "new" || item.LatestUnreadMsgIndex != 2 {
		t.Fatal("user reload replaced its pending snapshot with stale SQL")
	}
	if err := reloaded.ClearTotalUnread(); err != nil {
		t.Fatal("clear-all with old DB row failed")
	}
	key := converPersistKey(app, "stale-reload", "target", "", pbobjs.ChannelType_Group)
	persistCache.Remove(key)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, pending := persistLatest.Load(key)
		if !pending && len(persistSem) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stored, err := dao.FindOne(app, "stale-reload", "target", "", pbobjs.ChannelType_Group)
	if err != nil || stored.LatestReadMsgIndex != 2 || stored.LatestUnreadMsgIndex != 2 || stored.LatestMsgId != "new" || stored.SortTime != 2 {
		t.Fatal("clear-all replaced newer dirty snapshot with stale DB state")
	}
}
