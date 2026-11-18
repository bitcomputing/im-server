//go:build integration

package mindlink

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"im-server/commons/configures"
	"im-server/commons/dbcommons"
)

func TestMySQLReliableSchemaNewAndUpgrade(t *testing.T) {
	current, err := os.ReadFile("../../sql/imserver.sql")
	if err != nil {
		t.Fatal(err)
	}
	tables := regexp.MustCompile("(?m)^CREATE TABLE(?: IF NOT EXISTS)? `([a-z0-9_]+)`").FindAllSubmatch(current, -1)
	if len(tables) < 70 {
		t.Fatal("native initialization table inventory incomplete")
	}
	for _, mode := range []string{"new", "upgrade"} {
		t.Run(mode, func(t *testing.T) {
			db := reliableConnection(t)
			// The connection helper rejects every schema/account/port except this owned fixture.
			for _, table := range tables {
				if _, err := db.Exec("DROP TABLE IF EXISTS `" + string(table[1]) + "`"); err != nil {
					t.Fatal(err)
				}
			}
			source := current
			if mode == "upgrade" {
				source, err = exec.Command("git", "show", "c325af8c0715e9d1af00bba57e6b3f81ad9fdfbc:sql/imserver.sql").Output()
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(string(source)); err != nil {
				t.Fatal(err)
			}
			if mode == "upgrade" {
				if _, err := db.Exec("INSERT INTO users(app_key,user_id,nickname) VALUES('fixture-app','legacy-owner','retained'); INSERT INTO p_hismsgs(app_key,conver_id,msg_id,msg_seq_no,send_time) VALUES('fixture-app','legacy','legacy-message',2147483648,1700000000000)"); err != nil {
					t.Fatal(err)
				}
				if _, err := NewMessageStore(context.Background(), db); !errors.Is(err, ErrNotReady) {
					t.Fatal("old version enabled producer")
				}
				cfg := reliableConfig(t)
				saved := configures.Config
				defer func() { dbcommons.CloseDB(); configures.Config = saved }()
				configures.Config.Mysql.User, configures.Config.Mysql.Password = cfg.User, cfg.Passwd
				configures.Config.Mysql.Address, configures.Config.Mysql.DbName = cfg.Addr, cfg.DBName
				configures.Config.Mysql.Debug = false
				if err := dbcommons.InitMysql(); err != nil {
					t.Fatal("owned migration pool unavailable")
				}
				if err := dbcommons.Upgrade(); err != nil {
					t.Fatal(err)
				}
				var name string
				var seq int64
				if db.QueryRow("SELECT nickname FROM users WHERE user_id='legacy-owner'").Scan(&name) != nil || name != "retained" || db.QueryRow("SELECT msg_seq_no FROM p_hismsgs WHERE msg_id='legacy-message'").Scan(&seq) != nil || seq != 2147483648 {
					t.Fatal("upgrade changed native rows")
				}
			}
			var version string
			if db.QueryRow("SELECT conf_value FROM globalconfs WHERE conf_key='jimdb_version'").Scan(&version) != nil || version != "20261112" {
				t.Fatal("migration version not recorded")
			}
			if _, err := NewMessageStore(context.Background(), db); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMySQLReliableSchemaRejectsUnready(t *testing.T) {
	for _, mode := range []string{"table_missing", "non_transactional", "narrow_sequence", "mongo"} {
		t.Run(mode, func(t *testing.T) {
			db := reliableDB(t)
			switch mode {
			case "table_missing":
				if _, err := db.Exec("RENAME TABLE mindlink_message_outbox TO _reliable_outbox"); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if _, err := db.Exec("RENAME TABLE _reliable_outbox TO mindlink_message_outbox"); err != nil {
						t.Error(err)
					}
				}()
			case "non_transactional":
				if _, err := db.Exec("ALTER TABLE users ENGINE=MyISAM"); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if _, err := db.Exec("ALTER TABLE users ENGINE=InnoDB"); err != nil {
						t.Error(err)
					}
				}()
			case "narrow_sequence":
				if _, err := db.Exec("ALTER TABLE p_hismsgs MODIFY msg_seq_no INT DEFAULT 0"); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if _, err := db.Exec("ALTER TABLE p_hismsgs MODIFY msg_seq_no BIGINT DEFAULT 0"); err != nil {
						t.Error(err)
					}
				}()
			case "mongo":
				saved := configures.Config.MsgStoreEngine
				configures.Config.MsgStoreEngine = configures.MsgStoreEngine_Mongo
				defer func() { configures.Config.MsgStoreEngine = saved }()
			}
			if _, err := NewMessageStore(context.Background(), db); !errors.Is(err, ErrNotReady) {
				t.Fatalf("unready store accepted: %v", err)
			}
		})
	}
}

func TestMySQLReliableMessageSequenceFloor(t *testing.T) {
	db := reliableDB(t)
	binding := reliableProjection(t, db, "avatar_owner_dm")
	if _, err := db.Exec("INSERT INTO conversation_seen(app_key,user_id,target_id,channel_type,sub_channel,seen_seq) VALUES('fixture-app','43',?,1,'',2147483648)", disablePrincipal); err != nil {
		t.Fatal(err)
	}
	store, err := NewMessageStore(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"2147483649", "2147483650"} {
		request := messageRequest()
		if i == 1 {
			request.OperationID, request.RequestID = strings.Repeat("e", 64), strings.Repeat("f", 64)
			if _, err := db.Exec("DELETE FROM p_hismsgs"); err != nil {
				t.Fatal(err)
			}
		}
		v, auth := reliableProof(t, request, "owner", 0)
		result, err := store.Send(context.Background(), v, auth)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := v.ValidateMessageSendResult(auth, result.Result, binding)
		if err != nil || receipt.MsgSeqNo != want {
			t.Fatalf("seen/retention floor: %v", err)
		}
	}
	if _, err := db.Exec("UPDATE message_sequence SET seq=9223372036854775807"); err != nil {
		t.Fatal(err)
	}
	request := messageRequest()
	request.OperationID, request.RequestID = strings.Repeat("2", 64), strings.Repeat("3", 64)
	v, auth := reliableProof(t, request, "owner", 0)
	if result, err := store.Send(context.Background(), v, auth); result != nil || !errors.Is(err, ErrUnknownResult) {
		t.Fatalf("exhausted sequence wrapped: %v", err)
	}
	if reliableRows(t, db, "mindlink_operations") != 2 || reliableRows(t, db, "mindlink_message_outbox") != 2 {
		t.Fatal("overflow committed a partial effect")
	}
}
