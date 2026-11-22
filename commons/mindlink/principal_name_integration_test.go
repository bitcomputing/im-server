//go:build integration

package mindlink

import (
	"strings"
	"testing"
)

func TestMySQLPrincipalDisplayNameCapacity(t *testing.T) {
	db := nativeFixture(t)
	name := strings.Repeat("😀", 80)
	if _, err := db.Exec("INSERT INTO users(app_key,user_id,nickname) VALUES('fixture-app','wide-display-name',?)", name); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := db.QueryRow("SELECT nickname FROM users WHERE app_key='fixture-app' AND user_id='wide-display-name'").Scan(&got); err != nil || got != name {
		t.Fatal("display name truncated", err)
	}
	if err := db.QueryRow("SELECT nickname FROM users WHERE app_key='fixture-app' AND user_id='42'").Scan(&got); err != nil || got != "legacy-owner" {
		t.Fatal("legacy user changed", err)
	}
}

func TestMySQLPrincipalNameUpgradePreservesExistingUsers(t *testing.T) {
	db := nativeFixture(t)
	cfg := ownedDBConfig(t)
	if err := db.QueryRow("SELECT DATABASE()").Scan(&cfg.DBName); err != nil || !strings.HasPrefix(cfg.DBName, "mindlink_policy_c82_native_") {
		t.Fatal("unowned database")
	}
	if _, err := db.Exec("ALTER TABLE users MODIFY COLUMN nickname varchar(50) DEFAULT NULL COMMENT '昵称'; UPDATE globalconfs SET conf_value='20261111' WHERE conf_key='jimdb_version'"); err != nil {
		t.Fatal(err)
	}
	legacy := strings.Repeat("原", 50)
	if _, err := db.Exec("UPDATE users SET nickname=? WHERE app_key='fixture-app' AND user_id='42'", legacy); err != nil {
		t.Fatal(err)
	}
	if err := upgradeNativeFixture(t, db, cfg, "20261122"); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := db.QueryRow("SELECT nickname FROM users WHERE app_key='fixture-app' AND user_id='42'").Scan(&name); err != nil || name != legacy {
		t.Fatal("upgrade changed existing name", err)
	}
	name = strings.Repeat("😀", 80)
	if _, err := db.Exec("UPDATE users SET nickname=? WHERE app_key='fixture-app' AND user_id='42'", name); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := db.QueryRow("SELECT nickname FROM users WHERE app_key='fixture-app' AND user_id='42'").Scan(&got); err != nil || got != name {
		t.Fatal("upgraded readback differs", err)
	}
	if err := upgradeNativeFixture(t, db, cfg, "20261122"); err != nil {
		t.Fatal(err)
	}
}
