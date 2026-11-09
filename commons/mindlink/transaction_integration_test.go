//go:build integration

package mindlink

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"

	"im-server/commons/configures"
	"im-server/commons/dbcommons"
)

func ownedDBConfig(t *testing.T) *mysql.Config {
	t.Helper()
	path := os.Getenv("MINDLINK_TEST_DSN_FILE")
	if path == "" {
		t.Skip("requires the owned mindlink_policy_c82 fixture; not DB coverage")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read owned fixture configuration")
	}
	var config struct {
		DSN string `json:"dsn"`
	}
	if json.Unmarshal(raw, &config) != nil {
		t.Fatal("invalid owned fixture configuration")
	}
	cfg, err := mysql.ParseDSN(config.DSN)
	if err != nil {
		t.Fatal("invalid owned fixture DSN")
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil || host != "127.0.0.1" || cfg.Net != "tcp" || cfg.DBName != "mindlink_policy_c82" {
		t.Fatal("refusing a non-owned database")
	}
	return cfg
}

func ownedDB(t *testing.T) *sql.DB {
	t.Helper()
	cfg := ownedDBConfig(t)
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open owned database")
	}
	db.SetMaxOpenConns(16)
	t.Cleanup(func() { db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal("owned MySQL is not reachable")
	}
	return db
}

func prepareBindingDB(t *testing.T) *sql.DB {
	t.Helper()
	db := ownedDB(t)
	migration, err := os.ReadFile("../dbcommons/sqls/20261108.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"CREATE TABLE IF NOT EXISTS _mindlink_test_authority (owner_id VARBINARY(20) NOT NULL, actor_id VARBINARY(100) NOT NULL, generation BIGINT NOT NULL, policy_version BIGINT NOT NULL, disabled BOOLEAN NOT NULL DEFAULT FALSE, PRIMARY KEY(owner_id,actor_id)) ENGINE=InnoDB",
		"CREATE TABLE IF NOT EXISTS _mindlink_test_effects (operation_id BINARY(64) NOT NULL PRIMARY KEY) ENGINE=InnoDB",
		"DELETE FROM mindlink_tickets", "DELETE FROM mindlink_operations", "DELETE FROM _mindlink_test_effects", "DELETE FROM _mindlink_test_authority",
		"INSERT INTO _mindlink_test_authority(owner_id,actor_id,generation,policy_version) VALUES('42','stable-avatar',3,7)",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func currentAuthority(tx *sql.Tx, c Claims, replayed bool) error {
	var generation, version int64
	var disabled bool
	err := tx.QueryRow("SELECT generation,policy_version,disabled FROM _mindlink_test_authority WHERE owner_id=? AND actor_id=? FOR UPDATE", c.OwnerID, c.ActorID).Scan(&generation, &version, &disabled)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	if disabled {
		return ErrPrincipalDisabled
	}
	if generation != c.AvatarGeneration || !replayed && version != c.ExpectedPolicyVersion {
		return ErrStalePolicy
	}
	return nil
}

func proofForDB(t *testing.T, v *Verifier, key ed25519.PrivateKey, change func(map[string]any)) *Authorization {
	t.Helper()
	p := fixtureClaims(`{"fixture":"metadata"}`)
	p["actor_kind"], p["actor_id"], p["avatar_generation"] = "avatar", "stable-avatar", int64(3)
	p["iat"], p["exp"] = time.Now().Unix(), time.Now().Unix()+60
	if change != nil {
		change(p)
	}
	body, method := `{"fixture":"metadata"}`, "POST"
	if p["method"] == "GET" {
		body, method = "", "GET"
		p["body_sha256"] = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	}
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, p))
	auth, err := v.Verify(fixtureRequest(t, token, method, fixturePath, body), p["operation_type"].(string), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func dbVerifier(t *testing.T) (*Verifier, ed25519.PrivateKey) {
	t.Helper()
	trust, key := fixtureTrust()
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	return v, key
}

func effectFor(auth *Authorization, called *atomic.Int32) func(*sql.Tx) (json.RawMessage, error) {
	return func(tx *sql.Tx) (json.RawMessage, error) {
		if called != nil {
			called.Add(1)
		}
		_, err := tx.Exec("INSERT INTO _mindlink_test_effects(operation_id) VALUES(?)", auth.Claims().OperationID)
		return json.RawMessage(`{"persisted":true}`), err
	}
}

func assertRows(t *testing.T, db *sql.DB, operations, tickets, effects int) {
	t.Helper()
	for _, tc := range []struct {
		table string
		want  int
	}{{"mindlink_operations", operations}, {"mindlink_tickets", tickets}, {"_mindlink_test_effects", effects}} {
		var got int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + tc.table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("%s rows=%d want=%d", tc.table, got, tc.want)
		}
	}
}

func TestMySQLTransactCommitsAuthorizedEffect(t *testing.T) {
	db := prepareBindingDB(t)
	v, key := dbVerifier(t)
	auth := proofForDB(t, v, key, nil)
	result, err := v.Transact(context.Background(), db, auth, currentAuthority, effectFor(auth, nil))
	if err != nil {
		t.Fatalf("authorized request not committed: %v", err)
	}
	if result == nil || result.Replayed || string(result.Result) != `{"persisted":true}` {
		t.Fatal("missing original committed result")
	}
	assertRows(t, db, 1, 1, 1)
}

func TestMySQLTransactRejectsCurrentAuthorityBeforeEffects(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
		update string
		want   error
	}{
		{"old generation", func(p map[string]any) { p["avatar_generation"] = int64(2) }, "", ErrStalePolicy},
		{"old policy", func(p map[string]any) { p["expected_policy_version"] = int64(6) }, "", ErrStalePolicy},
		{"foreign actor", func(p map[string]any) { p["actor_id"] = "other-avatar" }, "", ErrForbidden},
		{"owner cannot impersonate member", func(p map[string]any) {
			p["actor_kind"], p["actor_id"], p["avatar_generation"] = "owner", "42", int64(0)
		}, "", ErrForbidden},
		{"disabled", nil, "UPDATE _mindlink_test_authority SET disabled=TRUE", ErrPrincipalDisabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := prepareBindingDB(t)
			if tc.update != "" {
				if _, err := db.Exec(tc.update); err != nil {
					t.Fatal(err)
				}
			}
			v, key := dbVerifier(t)
			auth := proofForDB(t, v, key, tc.change)
			var calls atomic.Int32
			result, err := v.Transact(context.Background(), db, auth, currentAuthority, effectFor(auth, &calls))
			if result != nil || !errors.Is(err, tc.want) || calls.Load() != 0 {
				t.Fatalf("unauthorized effect: result=%t calls=%d err=%v", result != nil, calls.Load(), err)
			}
			assertRows(t, db, 0, 0, 0)
		})
	}
}

func TestMySQLTransactRejectsZeroForeignAndQueryProof(t *testing.T) {
	db := prepareBindingDB(t)
	v, key := dbVerifier(t)
	other, _ := dbVerifier(t)
	for _, auth := range []*Authorization{nil, {}, proofForDB(t, other, key, nil), proofForDB(t, v, key, func(p map[string]any) { p["method"], p["operation_type"] = "GET", "fixture.query" })} {
		var calls atomic.Int32
		if result, err := v.Transact(context.Background(), db, auth, currentAuthority, func(*sql.Tx) (json.RawMessage, error) { calls.Add(1); return json.RawMessage(`{}`), nil }); result != nil || !errors.Is(err, ErrForbidden) || calls.Load() != 0 {
			t.Fatalf("unusable proof passed: %v", err)
		}
	}
	assertRows(t, db, 0, 0, 0)
}

func TestMySQLTransactReplaysOriginalAndRejectsRebinding(t *testing.T) {
	db := prepareBindingDB(t)
	v, key := dbVerifier(t)
	auth := proofForDB(t, v, key, nil)
	var calls atomic.Int32
	first, err := v.Transact(context.Background(), db, auth, currentAuthority, effectFor(auth, &calls))
	if err != nil {
		t.Fatal(err)
	}
	// The caller may lose this response; a renewed ticket still refers to the original operation.
	renewed := proofForDB(t, v, key, func(p map[string]any) { p["jti"] = strings.Repeat("d", 64) })
	again, err := v.Transact(context.Background(), db, renewed, currentAuthority, effectFor(auth, &calls))
	if err != nil || !again.Replayed || string(again.Result) != string(first.Result) || calls.Load() != 1 {
		t.Fatalf("retry executed again: calls=%d err=%v", calls.Load(), err)
	}
	assertRows(t, db, 1, 2, 1)
	for _, change := range []func(map[string]any){
		func(p map[string]any) {
			p["request_digest"], p["jti"] = strings.Repeat("e", 64), strings.Repeat("e", 64)
		},
		func(p map[string]any) { p["operation_id"], p["jti"] = strings.Repeat("f", 64), strings.Repeat("f", 64) },
		func(p map[string]any) {
			p["operation_id"], p["request_id"] = strings.Repeat("f", 64), "another-request"
		},
	} {
		bad := proofForDB(t, v, key, change)
		if result, err := v.Transact(context.Background(), db, bad, currentAuthority, effectFor(bad, &calls)); result != nil || !errors.Is(err, ErrRequestConflict) {
			t.Fatalf("rebound intent accepted: %v", err)
		}
		assertRows(t, db, 1, 2, 1)
	}
}

func TestMySQLTransactReplaysPolicyChangeWithoutApplyingItTwice(t *testing.T) {
	db := prepareBindingDB(t)
	v, key := dbVerifier(t)
	auth := proofForDB(t, v, key, nil)
	var calls atomic.Int32
	changePolicy := func(tx *sql.Tx) (json.RawMessage, error) {
		calls.Add(1)
		_, err := tx.Exec("UPDATE _mindlink_test_authority SET policy_version=8 WHERE owner_id='42' AND actor_id='stable-avatar' AND policy_version=7")
		return json.RawMessage(`{"effective_policy_version":8}`), err
	}
	if _, err := v.Transact(context.Background(), db, auth, currentAuthority, changePolicy); err != nil {
		t.Fatal(err)
	}
	var effective int64
	if err := db.QueryRow("SELECT policy_version FROM _mindlink_test_authority WHERE owner_id='42' AND actor_id='stable-avatar'").Scan(&effective); err != nil || effective != 8 {
		t.Fatal("policy effect did not actually update version")
	}
	renewed := proofForDB(t, v, key, func(p map[string]any) { p["jti"] = strings.Repeat("e", 64) })
	result, err := v.Transact(context.Background(), db, renewed, currentAuthority, changePolicy)
	if err != nil || result == nil || !result.Replayed || calls.Load() != 1 || string(result.Result) != `{"effective_policy_version":8}` {
		t.Fatalf("original policy result was not recoverable: calls=%d error=%v", calls.Load(), err)
	}
}

func TestMySQLTransactConcurrentRetriesHaveOneEffect(t *testing.T) {
	db := prepareBindingDB(t)
	v, key := dbVerifier(t)
	auth := proofForDB(t, v, key, nil)
	var calls, first atomic.Int32
	var wait sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := v.Transact(context.Background(), db, auth, currentAuthority, effectFor(auth, &calls))
			if err != nil {
				failures <- err
				return
			}
			if !result.Replayed {
				first.Add(1)
			}
		}()
	}
	wait.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if calls.Load() != 1 || first.Load() != 1 {
		t.Fatalf("duplicate effects=%d first=%d", calls.Load(), first.Load())
	}
	assertRows(t, db, 1, 1, 1)
}

func TestMySQLTransactRollsBackEffectFailure(t *testing.T) {
	db := prepareBindingDB(t)
	v, key := dbVerifier(t)
	auth := proofForDB(t, v, key, nil)
	_, err := v.Transact(context.Background(), db, auth, currentAuthority, func(tx *sql.Tx) (json.RawMessage, error) {
		if _, err := tx.Exec("INSERT INTO _mindlink_test_effects(operation_id) VALUES(?)", auth.Claims().OperationID); err != nil {
			return nil, err
		}
		return nil, errors.New("fixture effect failure")
	})
	if !errors.Is(err, ErrUnknownResult) {
		t.Fatalf("failed SQL effect reported success: %v", err)
	}
	assertRows(t, db, 0, 0, 0)
}

func TestMySQLTransactRejectsExpiredTicketAfterAuthorityWait(t *testing.T) {
	db := prepareBindingDB(t)
	v, key := dbVerifier(t)
	auth := proofForDB(t, v, key, func(p map[string]any) { expired := time.Now().Unix() - 4; p["iat"], p["exp"] = expired-60, expired })
	var calls atomic.Int32
	result, err := v.Transact(context.Background(), db, auth, func(tx *sql.Tx, c Claims, replayed bool) error {
		if err := currentAuthority(tx, c, replayed); err != nil {
			return err
		}
		time.Sleep(2 * time.Second)
		return nil
	}, effectFor(auth, &calls))
	if result != nil || !errors.Is(err, ErrForbidden) || calls.Load() != 0 {
		t.Fatalf("expired queued request committed: calls=%d err=%v", calls.Load(), err)
	}
	assertRows(t, db, 0, 0, 0)
}

func TestMySQLTransactRejectsExpiredTicketAfterTicketLockWait(t *testing.T) {
	db := prepareBindingDB(t)
	v, key := dbVerifier(t)
	auth := proofForDB(t, v, key, func(p map[string]any) {
		expired := time.Now().Unix() - 3
		p["iat"], p["exp"] = expired-60, expired
	})
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	locker, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Rollback()
	c := auth.Claims()
	if _, err := locker.ExecContext(ctx, "INSERT INTO mindlink_tickets(issuer,jti,ticket_hash,operation_id) VALUES(?,?,?,?)", c.Issuer, c.JTI, make([]byte, 32), strings.Repeat("f", 64)); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	done := make(chan error, 1)
	go func() {
		_, err := v.Transact(ctx, db, auth, currentAuthority, effectFor(auth, &calls))
		done <- err
	}()
	for {
		var waiting int
		err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks l ON l.ENGINE_LOCK_ID=w.REQUESTING_ENGINE_LOCK_ID AND l.ENGINE=w.ENGINE WHERE l.OBJECT_SCHEMA=DATABASE() AND l.OBJECT_NAME='mindlink_tickets'`).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("request did not wait for ticket lock: %v", err)
		case <-ctx.Done():
			t.Fatal("no actual ticket lock wait observed")
		case <-time.After(5 * time.Millisecond):
		}
	}
	select {
	case <-time.After(time.Until(time.Unix(c.ExpiresAt, 0).Add(5*time.Second + 20*time.Millisecond))):
	case <-ctx.Done():
		t.Fatal("ticket wait did not finish before test deadline")
	}
	if err := locker.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrForbidden) || calls.Load() != 0 {
		t.Fatalf("expired ticket lock waiter committed: calls=%d err=%v", calls.Load(), err)
	}
	assertRows(t, db, 0, 0, 0)
}

func TestMySQLTransactRollsBackTicketExpiredDuringEffect(t *testing.T) {
	db := prepareBindingDB(t)
	v, key := dbVerifier(t)
	auth := proofForDB(t, v, key, func(p map[string]any) {
		expired := time.Now().Unix() - 4
		p["iat"], p["exp"] = expired-60, expired
	})
	result, err := v.Transact(context.Background(), db, auth, currentAuthority, func(tx *sql.Tx) (json.RawMessage, error) {
		body, err := effectFor(auth, nil)(tx)
		time.Sleep(2 * time.Second)
		return body, err
	})
	if result != nil || !errors.Is(err, ErrForbidden) {
		t.Fatalf("expired effect transaction committed: %v", err)
	}
	assertRows(t, db, 0, 0, 0)
}

func TestMySQLTransactDoesNotReexecuteAnIncompleteOriginalRecord(t *testing.T) {
	db := prepareBindingDB(t)
	v, key := dbVerifier(t)
	auth := proofForDB(t, v, key, nil)
	var calls atomic.Int32
	if _, err := v.Transact(context.Background(), db, auth, currentAuthority, effectFor(auth, &calls)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE mindlink_operations SET result_json=NULL"); err != nil {
		t.Fatal(err)
	}
	if result, err := v.Transact(context.Background(), db, auth, currentAuthority, effectFor(auth, &calls)); result != nil || !errors.Is(err, ErrUnknownResult) || calls.Load() != 1 {
		t.Fatalf("incomplete result was treated as permission to repeat: calls=%d err=%v", calls.Load(), err)
	}
	assertRows(t, db, 1, 1, 1)
}

func TestMySQLTransactReadsOriginalAfterConnectionPoolReplacement(t *testing.T) {
	db := prepareBindingDB(t)
	v, key := dbVerifier(t)
	auth := proofForDB(t, v, key, nil)
	var calls atomic.Int32
	first, err := v.Transact(context.Background(), db, auth, currentAuthority, effectFor(auth, &calls))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := ownedDB(t)
	original, err := v.Transact(context.Background(), reopened, auth, currentAuthority, effectFor(auth, &calls))
	if err != nil || original == nil || !original.Replayed || string(first.Result) != string(original.Result) || calls.Load() != 1 {
		t.Fatalf("persistent original result lost after reconnect: %v", err)
	}
	assertRows(t, reopened, 1, 1, 1)
}

func TestMySQLRevocationSharesTheCommitLock(t *testing.T) {
	db := prepareBindingDB(t)
	v, key := dbVerifier(t)
	auth := proofForDB(t, v, key, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	locked, release, committed := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	go func() {
		_, err := v.Transact(ctx, db, auth, func(tx *sql.Tx, c Claims, replayed bool) error {
			if err := currentAuthority(tx, c, replayed); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		}, effectFor(auth, nil))
		committed <- err
	}()
	select {
	case <-locked:
	case <-ctx.Done():
		t.Fatal("transaction did not acquire authority lock")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	var connectionID int64
	if err := conn.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&connectionID); err != nil {
		t.Fatal(err)
	}
	revoked := make(chan error, 1)
	go func() {
		_, err := conn.ExecContext(ctx, "UPDATE _mindlink_test_authority SET generation=4 WHERE owner_id='42' AND actor_id='stable-avatar'")
		revoked <- err
	}()
	waited := false
	for !waited {
		select {
		case err := <-revoked:
			t.Fatalf("revocation did not wait on the authority lock: %v", err)
		default:
		}
		var waiting int
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM performance_schema.data_lock_waits AS w JOIN performance_schema.threads AS t ON t.THREAD_ID=w.REQUESTING_THREAD_ID WHERE t.PROCESSLIST_ID=?", connectionID).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		waited = waiting > 0
		if !waited {
			select {
			case <-ctx.Done():
				t.Fatal("no actual MySQL lock wait observed")
			case <-time.After(5 * time.Millisecond):
			}
		}
	}
	close(release)
	released = true
	if err := <-committed; err != nil {
		t.Fatal(err)
	}
	if err := <-revoked; err != nil {
		t.Fatal(err)
	}
	old := proofForDB(t, v, key, func(p map[string]any) {
		p["operation_id"], p["request_id"], p["jti"] = strings.Repeat("e", 64), "after-revocation", strings.Repeat("e", 64)
	})
	if result, err := v.Transact(ctx, db, old, currentAuthority, effectFor(old, nil)); result != nil || !errors.Is(err, ErrStalePolicy) {
		t.Fatalf("old generation committed after revocation: %v", err)
	}
	assertRows(t, db, 1, 1, 1)
}

func TestMySQLFreshAndUpgradeKeepExistingData(t *testing.T) {
	db := ownedDB(t)
	initial, err := os.ReadFile("../../sql/imserver.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../dbcommons/sqls/20261108.sql")
	if err != nil {
		t.Fatal(err)
	}
	marker := []byte("-- Mindlink original request binding")
	cut := strings.Index(string(initial), string(marker))
	if cut < 0 {
		t.Fatal("new database schema lacks mindlink metadata")
	}
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	names := []string{"mindlink_policy_c82_fresh_" + suffix, "mindlink_policy_c82_upgrade_" + suffix}
	for _, name := range names {
		if _, err := db.Exec("CREATE DATABASE IF NOT EXISTS `" + name + "`"); err != nil {
			t.Fatal(err)
		}
	}
	var config struct {
		DSN string `json:"dsn"`
	}
	raw, _ := os.ReadFile(os.Getenv("MINDLINK_TEST_DSN_FILE"))
	json.Unmarshal(raw, &config)
	cfg, _ := mysql.ParseDSN(config.DSN)
	var definitions [][]string
	for _, fresh := range []bool{true, false} {
		cfg.DBName = names[1]
		if fresh {
			cfg.DBName = names[0]
		}
		other, err := sql.Open("mysql", cfg.FormatDSN())
		if err != nil {
			t.Fatal(err)
		}
		defer other.Close()
		base := string(initial[:cut])
		if fresh {
			base = string(initial)
		}
		if _, err := other.Exec(base); err != nil {
			t.Fatal(err)
		}
		if !fresh {
			if _, err := other.Exec("UPDATE globalconfs SET conf_value='20260908' WHERE conf_key='jimdb_version'"); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := other.Exec("INSERT IGNORE INTO users(app_key,user_id,nickname) VALUES('guard-app','upgrade-sentinel','original')"); err != nil {
			t.Fatal(err)
		}
		saved := configures.Config
		configures.Config.Mysql.User = cfg.User
		configures.Config.Mysql.Password = cfg.Passwd
		configures.Config.Mysql.Address = cfg.Addr
		configures.Config.Mysql.DbName = cfg.DBName
		configures.Config.Mysql.Debug = false
		if err := dbcommons.InitMysql(); err != nil {
			t.Fatal("owned migration database failed to open")
		}
		dbcommons.Upgrade()
		dbcommons.CloseDB()
		configures.Config = saved
		var version string
		if err := other.QueryRow("SELECT conf_value FROM globalconfs WHERE conf_key='jimdb_version'").Scan(&version); err != nil || version != "20261111" {
			t.Fatal("real migration entry did not finish new metadata version")
		}
		var nickname string
		if err := other.QueryRow("SELECT nickname FROM users WHERE app_key='guard-app' AND user_id='upgrade-sentinel'").Scan(&nickname); err != nil || nickname != "original" {
			t.Fatal("upgrade changed existing data")
		}
		for _, table := range []string{"p_hismsgs", "g_hismsgs"} {
			var kind string
			if err := other.QueryRow("SELECT DATA_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND COLUMN_NAME='msg_seq_no'", table).Scan(&kind); err != nil || kind != "bigint" {
				t.Fatalf("fresh=%t %s does not support the declared int64 sequence: %s err=%v", fresh, table, kind, err)
			}
		}
		var pair []string
		for _, table := range []string{"mindlink_operations", "mindlink_tickets", "mindlink_owner_barriers", "mindlink_principals", "conversation_seen", "message_sequence"} {
			var name, ddl string
			if err := other.QueryRow("SHOW CREATE TABLE "+table).Scan(&name, &ddl); err != nil {
				t.Fatal(err)
			}
			pair = append(pair, ddl)
		}
		definitions = append(definitions, pair)
		// Inspect Upgrade's result before a replay could repair missing tables.
		if _, err := other.Exec(string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	if fmt.Sprint(definitions[0]) != fmt.Sprint(definitions[1]) {
		t.Fatal("new and upgraded metadata schemas differ")
	}
}
