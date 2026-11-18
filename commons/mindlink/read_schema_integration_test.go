//go:build integration

package mindlink

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"im-server/commons/configures"
	"im-server/commons/dbcommons"
)

func TestMigrationMetadataReadFailureIsNotAnEmptyVersion(t *testing.T) {
	db := nativeFixture(t)
	cfg := ownedDBConfig(t)
	if err := db.QueryRow("SELECT DATABASE()").Scan(&cfg.DBName); err != nil || !strings.HasPrefix(cfg.DBName, "mindlink_policy_c82_native_") {
		t.Fatal("refusing non-owned metadata failure fixture")
	}
	saved := configures.Config
	defer func() { dbcommons.CloseDB(); configures.Config = saved }()
	configures.Config.Mysql.User, configures.Config.Mysql.Password = cfg.User, cfg.Passwd
	configures.Config.Mysql.Address, configures.Config.Mysql.DbName = cfg.Addr, cfg.DBName
	configures.Config.Mysql.Debug = false
	if err := dbcommons.InitMysql(); err != nil {
		t.Fatal("open owned migration metadata pool")
	}
	dbcommons.CloseDB()
	if _, err := (dbcommons.GlobalConfDao{}).FindByKey(dbcommons.JimDbVersionKey); err == nil {
		t.Fatal("closed SQL pool was reported as an empty successful metadata read")
	}
	if err := dbcommons.Upgrade(); err == nil {
		t.Fatal("migration continued after its metadata read failed")
	}
	var version string
	if err := db.QueryRow("SELECT conf_value FROM globalconfs WHERE conf_key='jimdb_version'").Scan(&version); err != nil || version != "20261117" {
		t.Fatal("failed metadata read changed the migration version", err)
	}
}

func TestReadIntegrationRejectsInvalidExistingVersionBeforeDDL(t *testing.T) {
	for i, invalid := range []string{"", "0", "-1", "not-a-version"} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			db := nativeFixture(t)
			cfg := ownedDBConfig(t)
			if err := db.QueryRow("SELECT DATABASE()").Scan(&cfg.DBName); err != nil || !strings.HasPrefix(cfg.DBName, "mindlink_policy_c82_native_") {
				t.Fatal("refusing non-owned invalid-version fixture")
			}
			if _, err := db.Exec("DROP TABLE apps,message_sequence,conversation_seen"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("UPDATE globalconfs SET conf_value=? WHERE conf_key='jimdb_version'", invalid); err != nil {
				t.Fatal(err)
			}
			if err := upgradeNativeFixture(t, db, cfg, invalid); err == nil {
				t.Fatal("invalid existing version was accepted")
			}
			var recreated int
			if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('apps','message_sequence','conversation_seen')").Scan(&recreated); err != nil || recreated != 0 {
				t.Fatal("invalid existing version replayed DDL", err)
			}
		})
	}
}

func TestReadIntegrationHistorySequenceStoresInt64(t *testing.T) {
	db := nativeFixture(t)
	for _, table := range []string{"p_hismsgs", "g_hismsgs"} {
		for i, seq := range []int64{2147483648, math.MaxInt64} {
			id := fmt.Sprintf("wide-%s-%d", table, i)
			if _, err := db.Exec("INSERT INTO "+table+"(app_key,conver_id,sub_channel,msg_id,msg_seq_no,send_time) VALUES('fixture-app','wide-sequence','',?,?,1700000000000)", id, seq); err != nil {
				t.Fatalf("%s did not store the declared int64 sequence: %v", table, err)
			}
			var stored int64
			if err := db.QueryRow("SELECT msg_seq_no FROM "+table+" WHERE app_key='fixture-app' AND msg_id=?", id).Scan(&stored); err != nil || stored != seq {
				t.Fatal("stored sequence lost integer precision", err)
			}
		}
	}
}

func TestReadIntegrationUpgradesAlreadyCurrentNativeSchema(t *testing.T) {
	for _, existingSeen := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing_seen_%t", existingSeen), func(t *testing.T) {
			db := nativeFixture(t)
			cfg := ownedDBConfig(t)
			if err := db.QueryRow("SELECT DATABASE()").Scan(&cfg.DBName); err != nil || !strings.HasPrefix(cfg.DBName, "mindlink_policy_c82_native_") {
				t.Fatal("refusing non-owned schema fixture")
			}
			for _, q := range []string{
				"DROP TABLE IF EXISTS message_sequence,conversation_seen",
				"ALTER TABLE p_hismsgs MODIFY msg_seq_no INT DEFAULT NULL COMMENT '消息seq'; ALTER TABLE g_hismsgs MODIFY msg_seq_no INT DEFAULT NULL COMMENT '消息seq'",
				"UPDATE globalconfs SET conf_value='20261110' WHERE conf_key='jimdb_version'",
				"INSERT INTO g_hismsgs(app_key,conver_id,sub_channel,msg_id,msg_seq_no,send_time) VALUES('fixture-app','protected-group','','schema-seed-1',5,1700000000000)",
			} {
				if _, err := db.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			if existingSeen {
				if _, err := db.Exec("CREATE TABLE conversation_seen(app_key VARCHAR(20) NOT NULL,user_id VARCHAR(32) NOT NULL,target_id VARCHAR(32) NOT NULL,channel_type TINYINT NOT NULL,sub_channel VARCHAR(32) NOT NULL DEFAULT '',seen_seq BIGINT NOT NULL DEFAULT 0,PRIMARY KEY(app_key,user_id,target_id,channel_type,sub_channel)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin; INSERT INTO conversation_seen VALUES('fixture-app','42','protected-group',2,'',17)"); err != nil {
					t.Fatal(err)
				}
			}
			upgradeNativeFixture(t, db, cfg, "20261117")
			var tables int
			if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('conversation_seen','message_sequence') AND ENGINE='InnoDB' AND TABLE_COLLATION='utf8mb4_bin'").Scan(&tables); err != nil || tables != 2 {
				t.Fatal("native READ/sequence tables were not created", err)
			}
			var floor, sendTime int64
			want := int64(5)
			if existingSeen {
				want = 17
			}
			if err := db.QueryRow("SELECT seq,send_time FROM message_sequence WHERE app_key='fixture-app' AND conver_id='protected-group' AND channel_type=2 AND sub_channel=''").Scan(&floor, &sendTime); err != nil || floor != want || sendTime != 1700000000000 {
				t.Fatalf("legacy floor lost: seq=%d time=%d want=%d err=%v", floor, sendTime, want, err)
			}
			for _, table := range []string{"p_hismsgs", "g_hismsgs"} {
				var kind string
				if err := db.QueryRow("SELECT DATA_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND COLUMN_NAME='msg_seq_no'", table).Scan(&kind); err != nil || kind != "bigint" {
					t.Fatal("old INT column was not upgraded", table, kind, err)
				}
				if _, err := db.Exec("INSERT INTO " + table + "(app_key,conver_id,sub_channel,msg_id,msg_seq_no,send_time) VALUES('fixture-app','upgraded-wide','','schema-wide',2147483648,1700000000001)"); err != nil {
					t.Fatal("upgraded old INT column rejected a wide sequence", table, err)
				}
				var stored int64
				if err := db.QueryRow("SELECT msg_seq_no FROM " + table + " WHERE app_key='fixture-app' AND msg_id='schema-wide'").Scan(&stored); err != nil || stored != 2147483648 {
					t.Fatal("old-schema upgrade lost sequence precision", err)
				}
			}
			if _, err := db.Exec("UPDATE message_sequence SET seq=23 WHERE app_key='fixture-app' AND conver_id='protected-group' AND channel_type=2 AND sub_channel=''; UPDATE globalconfs SET conf_value='20261110' WHERE conf_key='jimdb_version'"); err != nil {
				t.Fatal(err)
			}
			upgradeNativeFixture(t, db, cfg, "20261117")
			if err := db.QueryRow("SELECT seq FROM message_sequence WHERE app_key='fixture-app' AND conver_id='protected-group' AND channel_type=2 AND sub_channel=''").Scan(&floor); err != nil || floor != 23 {
				t.Fatal("reapplied schema regressed allocator", err)
			}
			var seenCount int
			if err := db.QueryRow("SELECT COUNT(*) FROM conversation_seen").Scan(&seenCount); err != nil || existingSeen && seenCount != 1 || !existingSeen && seenCount != 0 {
				t.Fatal("schema installation changed seen ownership", err)
			}
			var original string
			if err := db.QueryRow("SELECT result_json FROM mindlink_operations").Scan(&original); err != nil || original != nativeMigrationOriginal {
				t.Fatal("schema upgrade changed an original operation", err)
			}
		})
	}
}
