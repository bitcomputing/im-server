package dbcommons

import (
	"errors"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"im-server/commons/configures"
)

func upgradeFixture(t *testing.T) *gorm.DB {
	t.Helper()
	if os.Getenv("JIM_SEQUENCE_ISOLATED") != "im-f1-sequence-20261106" {
		t.Skip("requires owned sequence MySQL fixture")
	}
	previous, conf := db, configures.Config
	configures.Config.Mysql.User = "root"
	configures.Config.Mysql.Password = os.Getenv("MYSQL_ROOT_PASSWORD")
	configures.Config.Mysql.Address = "im-f1-sequence-mysql:3306"
	configures.Config.Mysql.DbName = "jim_db"
	if err := InitMysql(); err != nil {
		t.Fatal(err)
	}
	admin := db
	admin.Config.Logger = logger.Default.LogMode(logger.Silent)
	if err := admin.Exec("CREATE DATABASE jim_sequence_upgrade_test").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP DATABASE jim_sequence_upgrade_test").Error; err != nil {
			t.Error(err)
		}
		db = previous
		configures.Config = conf
	})
	for _, table := range []string{"globalconfs", "p_hismsgs", "g_hismsgs", "conversation_seen"} {
		if err := admin.Exec("CREATE TABLE jim_sequence_upgrade_test." + table + " LIKE jim_db." + table).Error; err != nil {
			t.Fatal(err)
		}
	}
	configures.Config.Mysql.DbName = "jim_sequence_upgrade_test"
	if err := InitMysql(); err != nil {
		t.Fatal(err)
	}
	db.Config.Logger = logger.Default.LogMode(logger.Silent)
	if err := db.Exec("INSERT INTO globalconfs(conf_key,conf_value) VALUES ('jimdb_version','20260908')").Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestUpgradeStopsAtFailedMigrationMySQL(t *testing.T) {
	db := upgradeFixture(t)
	injected := errors.New("injected READ migration failure")
	if err := db.Callback().Raw().Before("gorm:raw").Register("upgrade_create_error", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "CREATE TABLE IF NOT EXISTS `conversation_seen`") {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := Upgrade()
	db.Callback().Raw().Remove("upgrade_create_error")
	if !errors.Is(err, injected) {
		t.Fatalf("migration error lost: %v", err)
	}
	var version string
	if err := db.Raw("SELECT conf_value FROM globalconfs WHERE conf_key='jimdb_version'").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	if version != "20260908" {
		t.Fatalf("failed migration skipped by later version: %s", version)
	}
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='message_sequence'").Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("later schema ran after failed prerequisite")
	}
}

func TestUpgradeStopsAtFailedVersionWriteMySQL(t *testing.T) {
	db := upgradeFixture(t)
	injected := errors.New("injected version write failure")
	if err := db.Callback().Raw().Before("gorm:raw").Register("upgrade_version_error", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "INSERT INTO globalconfs") && len(tx.Statement.Vars) > 1 && tx.Statement.Vars[1] == "20261106" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := Upgrade()
	db.Callback().Raw().Remove("upgrade_version_error")
	if !errors.Is(err, injected) {
		t.Fatalf("version error lost: %v", err)
	}
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='message_sequence'").Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("later schema ran after failed version write")
	}
}

func TestUpgradeSeedsWatermarksMySQL(t *testing.T) {
	db := upgradeFixture(t)
	if err := db.Exec("INSERT INTO p_hismsgs(app_key,conver_id,sub_channel,msg_id,msg_seq_no,send_time) VALUES ('seed','owner:peer','','old-private',100,500)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO g_hismsgs(app_key,conver_id,sub_channel,msg_id,msg_seq_no,send_time) VALUES ('seed','group','','old-group',200,600),('seed','Group','','old-case',300,700)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO conversation_seen(app_key,user_id,target_id,channel_type,sub_channel,seen_seq) VALUES ('seed','peer','owner',1,'',150),('seed','owner','group',2,'',250),('seed','a ','a',1,'',400)").Error; err != nil {
		t.Fatal(err)
	}
	if err := Upgrade(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		conver              string
		channel, seq, stamp int64
	}{{"owner:peer", 1, 150, 500}, {"group", 2, 250, 600}, {"Group", 2, 300, 700}, {"a:a ", 1, 400, 0}} {
		var row struct{ Seq, SendTime int64 }
		if err := db.Raw("SELECT seq,send_time FROM message_sequence WHERE app_key='seed' AND conver_id=? AND channel_type=? AND sub_channel=''", tc.conver, tc.channel).Scan(&row).Error; err != nil || row.Seq != tc.seq || row.SendTime != tc.stamp {
			t.Fatalf("seed scope=%q got=%+v err=%v", tc.conver, row, err)
		}
	}
	if err := db.Exec("UPDATE globalconfs SET conf_value='20261106' WHERE conf_key='jimdb_version'").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE message_sequence SET seq=999 WHERE app_key='seed'").Error; err != nil {
		t.Fatal(err)
	}
	if err := Upgrade(); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM message_sequence WHERE app_key='seed' AND seq=999").Scan(&count).Error; err != nil || count != 4 {
		t.Fatal("rerun lowered a durable watermark")
	}
}

func TestUpgradeRejectsVersionReadFailureMySQL(t *testing.T) {
	db := upgradeFixture(t)
	injected := errors.New("injected migration version read failure")
	if err := db.Callback().Query().Before("gorm:query").Register("upgrade_read_error", func(tx *gorm.DB) { tx.AddError(injected) }); err != nil {
		t.Fatal(err)
	}
	err := Upgrade()
	db.Callback().Query().Remove("upgrade_read_error")
	if !errors.Is(err, injected) {
		t.Fatalf("version read error ignored: %v", err)
	}
	if err := db.Exec("UPDATE globalconfs SET conf_value='invalid' WHERE conf_key='jimdb_version'").Error; err != nil {
		t.Fatal(err)
	}
	if err := Upgrade(); err == nil {
		t.Fatal("invalid persisted version accepted")
	}
}
