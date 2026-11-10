//go:build integration

package mindlink

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"im-server/commons/configures"
	"im-server/commons/dbcommons"
)

const nativeMigrationOriginal = `{"fixture":"native-index-migration"}`

func nativeFixture(t *testing.T) *sql.DB {
	t.Helper()
	root := ownedDB(t)
	cfg := ownedDBConfig(t)
	name := fmt.Sprintf("mindlink_policy_c82_native_%x", time.Now().UnixNano())
	if !strings.HasPrefix(name, "mindlink_policy_c82_native_") || len(name) > 64 {
		t.Fatal("invalid owned scratch name")
	}
	t.Log("owned native migration scratch:", name)
	if _, err := root.Exec("CREATE DATABASE `" + name + "`"); err != nil {
		t.Fatal(err)
	}
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open owned native scratch")
	}
	t.Cleanup(func() {
		db.Close()
		if _, err := root.Exec("DROP DATABASE `" + name + "`"); err != nil {
			t.Error("owned scratch cleanup failed")
		}
	})
	initial, err := os.ReadFile("../../sql/imserver.sql")
	if err != nil {
		t.Fatal(err)
	}
	cut := bytes.Index(initial, []byte("-- Mindlink original request binding"))
	if cut < 0 {
		t.Fatal("missing initial metadata boundary")
	}
	if _, err := db.Exec(string(initial[:cut])); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"20261108", "20261109"} {
		migration, err := os.ReadFile("../dbcommons/sqls/" + version + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		"UPDATE globalconfs SET conf_value='20261109' WHERE conf_key='jimdb_version'",
		"INSERT INTO apps(app_key,app_secret,app_secure_key) VALUES('fixture-app','fixture-secret','0123456789abcdef')",
		"INSERT INTO users(app_key,user_id,nickname) VALUES('fixture-app','ma111111111111111111111111111111','fixture-avatar'),('fixture-app','42','legacy-owner'),('fixture-app','ma222222222222222222222222222222','ordinary-shaped'),('other-app','ma111111111111111111111111111111','other-app-user')",
		"INSERT INTO groupinfos(app_key,group_id,group_name) VALUES('fixture-app','protected-group','fixture'),('fixture-app','ordinary-group','ordinary'),('fixture-app','ma111111111111111111111111111111','namespace-collision')",
		"INSERT INTO groupmembers(app_key,group_id,member_id) VALUES('fixture-app','protected-group','ma111111111111111111111111111111'),('fixture-app','protected-group','42'),('fixture-app','ordinary-group','42'),('fixture-app','ma111111111111111111111111111111','42')",
		"INSERT INTO mindlink_owner_barriers(environment,tenant_id,appkey,owner_id,read_enabled) VALUES('dev','fixture-tenant','fixture-app','42',TRUE)",
		"INSERT INTO mindlink_principals(environment,tenant_id,appkey,owner_id,principal_id,avatar_id,avatar_generation,policy_version,state) VALUES('dev','fixture-tenant','fixture-app','42','ma111111111111111111111111111111','avatar-A',2,7,'active')",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO mindlink_operations(issuer,operation_id,request_key,binding_hash,binding_json,reservation_id,result_json) VALUES('fixture-biz',?,UNHEX(REPEAT('11',32)),UNHEX(REPEAT('22',32)),'{}',UNHEX(REPEAT('33',32)),?)", strings.Repeat("a", 64), nativeMigrationOriginal); err != nil {
		t.Fatal(err)
	}
	upgradeNativeFixture(t, db, cfg, "20261113")
	return db
}

func upgradeNativeFixture(t *testing.T, db *sql.DB, cfg *mysql.Config, wantVersion string) error {
	t.Helper()
	saved := configures.Config
	configures.Config.Mysql.User = cfg.User
	configures.Config.Mysql.Password = cfg.Passwd
	configures.Config.Mysql.Address = cfg.Addr
	configures.Config.Mysql.DbName = cfg.DBName
	configures.Config.Mysql.Debug = false
	if err := dbcommons.InitMysql(); err != nil {
		t.Fatal("open migration entry on owned scratch")
	}
	migrationErr := dbcommons.Upgrade()
	dbcommons.CloseDB()
	configures.Config = saved
	var version string
	if err := db.QueryRow("SELECT conf_value FROM globalconfs WHERE conf_key='jimdb_version'").Scan(&version); err != nil || version != wantVersion {
		t.Fatal("real native migration entry did not reach required version")
	}
	return migrationErr
}

func TestNativeMigrationResumesAfterCommittedDDL(t *testing.T) {
	for _, partial := range []bool{true, false} {
		t.Run(fmt.Sprintf("partial_%t", partial), func(t *testing.T) {
			db := nativeFixture(t)
			if partial {
				if _, err := db.Exec("ALTER TABLE mindlink_principals DROP INDEX uniq_mindlink_native_identity, DROP COLUMN native_appkey, DROP COLUMN native_principal_id"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec("UPDATE globalconfs SET conf_value='20261109' WHERE conf_key='jimdb_version'; UPDATE mindlink_owner_barriers SET write_enabled=TRUE"); err != nil {
				t.Fatal(err)
			}
			cfg := ownedDBConfig(t)
			if err := db.QueryRow("SELECT DATABASE()").Scan(&cfg.DBName); err != nil {
				t.Fatal(err)
			}
			upgradeNativeFixture(t, db, cfg, "20261113")
			if _, err := NewNativeFence(context.Background(), db); err != nil {
				t.Fatal("resumed migration did not restore fence", err)
			}
			var write bool
			var receipt string
			if err := db.QueryRow("SELECT write_enabled FROM mindlink_owner_barriers").Scan(&write); err != nil || !write {
				t.Fatal("resumed migration reset an existing write grant")
			}
			if err := db.QueryRow("SELECT result_json FROM mindlink_operations").Scan(&receipt); err != nil || receipt != nativeMigrationOriginal {
				t.Fatal("resumed migration changed original receipt")
			}
		})
	}
}

func TestNativeMigrationRejectsIncompatibleExistingDDL(t *testing.T) {
	for name, ddl := range map[string]string{
		"write_default":    "ALTER TABLE mindlink_owner_barriers ALTER COLUMN write_enabled SET DEFAULT TRUE",
		"generated_length": "ALTER TABLE mindlink_principals MODIFY native_appkey VARCHAR(21) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci GENERATED ALWAYS AS (CONVERT(appkey USING utf8mb4)) STORED",
		"partial_index":    "ALTER TABLE mindlink_principals DROP INDEX uniq_mindlink_native_identity",
	} {
		t.Run(name, func(t *testing.T) {
			db := nativeFixture(t)
			if _, err := db.Exec(ddl); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("UPDATE globalconfs SET conf_value='20261109' WHERE conf_key='jimdb_version'"); err != nil {
				t.Fatal(err)
			}
			cfg := ownedDBConfig(t)
			if err := db.QueryRow("SELECT DATABASE()").Scan(&cfg.DBName); err != nil {
				t.Fatal(err)
			}
			upgradeNativeFixture(t, db, cfg, "20261109")
			if name == "generated_length" {
				if fence, err := NewNativeFence(context.Background(), db); fence != nil || !errors.Is(err, ErrNotReady) {
					t.Fatal("incompatible generated length activated fence", err)
				}
			}
		})
	}
}

func TestNativeMigrationKeepsProjectionAndRequiresSeparateWriteGrant(t *testing.T) {
	db := nativeFixture(t)
	var read, write bool
	if err := db.QueryRow("SELECT read_enabled,write_enabled FROM mindlink_owner_barriers WHERE owner_id='42'").Scan(&read, &write); err != nil {
		t.Fatal(err)
	}
	if !read || write {
		t.Fatal("migration lost read grant or implicitly granted writes")
	}
	var avatar, state, legacyApp, legacyID string
	var generation, policy int64
	if err := db.QueryRow("SELECT avatar_id,avatar_generation,policy_version,state,native_appkey,native_principal_id FROM mindlink_principals").Scan(&avatar, &generation, &policy, &state, &legacyApp, &legacyID); err != nil {
		t.Fatal(err)
	}
	if avatar != "avatar-A" || generation != 2 || policy != 7 || state != "active" || legacyApp != "fixture-app" || legacyID != disablePrincipal {
		t.Fatal("native index changed old projection")
	}
	var original, nickname string
	if err := db.QueryRow("SELECT result_json FROM mindlink_operations").Scan(&original); err != nil || original != nativeMigrationOriginal {
		t.Fatal("migration changed original journal bytes")
	}
	if err := db.QueryRow("SELECT nickname FROM users WHERE app_key='fixture-app' AND user_id='42'").Scan(&nickname); err != nil || nickname != "legacy-owner" {
		t.Fatal("migration changed ordinary user")
	}
	_, err := db.Exec("INSERT INTO mindlink_principals(environment,tenant_id,appkey,owner_id,principal_id,avatar_id,avatar_generation,policy_version,state) VALUES('dev','fixture-tenant','Fixture-App','43',?,'avatar-B',1,1,'active')", disablePrincipal)
	var duplicate *mysql.MySQLError
	if !errors.As(err, &duplicate) || duplicate.Number != 1062 {
		t.Fatalf("native-equivalent app/principal rebind not rejected as a duplicate: %v", err)
	}
}

func TestNativeFenceUsesNativeIdentityComparison(t *testing.T) {
	db := nativeFixture(t)
	f, err := NewNativeFence(context.Background(), db)
	if err != nil {
		t.Fatalf("supported native schema rejected: %v", err)
	}
	for _, tc := range []struct{ name, app, user string }{
		{"exact", "fixture-app", disablePrincipal}, {"upper ID", "fixture-app", strings.ToUpper(disablePrincipal)},
		{"accent ID", "fixture-app", "mà" + disablePrincipal[2:]}, {"width ID", "fixture-app", "ｍａ" + disablePrincipal[2:]},
		{"ignorable ID", "fixture-app", "m\u200ba" + disablePrincipal[2:]}, {"upper app", "FIXTURE-APP", disablePrincipal},
		{"accent app", "fìxture-app", disablePrincipal}, {"unrelated app", "other-app", disablePrincipal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The legacy users table is the independent oracle, not the fence index.
			var nativeMatches int
			if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE app_key=? AND user_id=? AND nickname='fixture-avatar'", tc.app, tc.user).Scan(&nativeMatches); err != nil {
				t.Fatal(err)
			}
			err := f.Check(context.Background(), tc.app, []string{tc.user}, nil)
			if nativeMatches > 0 && !errors.Is(err, ErrForbidden) || nativeMatches == 0 && err != nil {
				t.Fatalf("native comparison mismatch: matches=%d err=%v", nativeMatches, err)
			}
			if (tc.name == "exact" || tc.name == "upper ID") && nativeMatches != 1 {
				t.Fatal("essential native equality fixture was not exercised")
			}
		})
	}
	if _, err := db.Exec("UPDATE mindlink_principals SET state='disabled',avatar_generation=3,policy_version=8"); err != nil {
		t.Fatal(err)
	}
	if err := f.Check(context.Background(), "fixture-app", []string{disablePrincipal}, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("permanent tombstone became native-addressable: %v", err)
	}
}

func TestNativeFenceSeparatesUserAndGroupScopes(t *testing.T) {
	db := nativeFixture(t)
	f, err := NewNativeFence(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, app     string
		users, groups []string
		want          error
	}{
		{"ordinary user", "fixture-app", []string{"42"}, nil, nil},
		{"ordinary shaped user", "fixture-app", []string{"ma222222222222222222222222222222"}, nil, nil},
		{"other app same ID", "other-app", []string{disablePrincipal}, nil, nil},
		{"managed group", "fixture-app", []string{"42"}, []string{"protected-group"}, ErrForbidden},
		{"group alias", "fixture-app", []string{"42"}, []string{"PROTECTED-GROUP"}, ErrForbidden},
		{"ordinary group", "fixture-app", []string{"42"}, []string{"ordinary-group"}, nil},
		{"group namespace collision", "fixture-app", []string{"42"}, []string{disablePrincipal}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := f.Check(context.Background(), tc.app, tc.users, tc.groups); !errors.Is(err, tc.want) {
				t.Fatalf("scope check got %v want %v", err, tc.want)
			}
		})
	}
	users := make([]string, 600)
	for i := range users {
		users[i] = fmt.Sprintf("ordinary-%d", i)
	}
	users[len(users)-1] = disablePrincipal
	if err := f.Check(context.Background(), "fixture-app", users, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("protected identity after a batch boundary was missed: %v", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM mindlink_principals").Scan(&count); err != nil || count != 1 {
		t.Fatal("native check created or changed a projection")
	}
}

func TestNativeFenceFailsClosedForUnsupportedSchemaAndDatabaseErrors(t *testing.T) {
	db := nativeFixture(t)
	f, err := NewNativeFence(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.Check(ctx, "fixture-app", []string{disablePrincipal}, nil); !errors.Is(err, ErrUnknownResult) {
		t.Fatalf("canceled read treated as absent principal: %v", err)
	}
	if _, err := db.Exec("ALTER TABLE users MODIFY user_id varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT ''"); err != nil {
		t.Fatal(err)
	}
	if invalid, err := NewNativeFence(context.Background(), db); invalid != nil || !errors.Is(err, ErrNotReady) {
		t.Fatalf("unsupported native comparison activated: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Check(context.Background(), "fixture-app", []string{disablePrincipal}, nil); !errors.Is(err, ErrUnknownResult) {
		t.Fatalf("closed database treated as absent authority: %v", err)
	}
}
