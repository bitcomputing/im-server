package dbs

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/historymsg/storages/models"
)

func TestConversationCleanTimeScopeMySQL(t *testing.T) {
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
	if err := dbcommons.InitMysql(); err != nil {
		t.Fatal(err)
	}
	app := fmt.Sprintf("ct%x", time.Now().UnixNano())
	db := dbcommons.GetDb()
	db.Config.Logger = logger.Default.LogMode(logger.Silent)
	t.Cleanup(func() {
		if err := db.Exec("DELETE FROM convercleantimes WHERE app_key=?", app).Error; err != nil {
			t.Error(err)
		}
	})
	dao := HisMsgConverCleanTimeDao{}
	if err := dao.UpsertDestroyTime(models.HisMsgConverCleanTime{AppKey: app, ConverId: "group", SubChannel: "thread", ChannelType: pbobjs.ChannelType_Group, CleanTime: 42}); err != nil {
		t.Fatal(err)
	}
	item, err := dao.FindOne(app, "group", "thread", pbobjs.ChannelType_Group)
	if err != nil || item == nil || item.CleanTime != 42 {
		t.Fatalf("scoped clean-time lookup failed: %v", err)
	}
	for _, key := range []struct {
		app, target, sub string
		channel          pbobjs.ChannelType
	}{{app + "x", "group", "thread", 2}, {app, "other", "thread", 2}, {app, "group", "other", 2}, {app, "group", "thread", 1}} {
		item, err := dao.FindOne(key.app, key.target, key.sub, key.channel)
		if item != nil || !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("scope isolation or empty-row semantics failed: %v", err)
		}
	}
	injected := errors.New("injected clean-time SQL failure")
	if err := db.Callback().Query().Before("gorm:query").Register("clean_time_error", func(tx *gorm.DB) { tx.AddError(injected) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Callback().Query().Remove("clean_time_error") })
	if item, err := dao.FindOne(app, "group", "thread", pbobjs.ChannelType_Group); item != nil || !errors.Is(err, injected) || errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("SQL failure became an empty record")
	}
}
