//go:build integration

package mindlink

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

func seedQueryOperation(t *testing.T) (*sql.DB, *Verifier, ed25519.PrivateKey, Claims, []byte) {
	t.Helper()
	db := prepareBindingDB(t)
	v, key, original, raw := queryFixture(t)
	if _, err := db.Exec("INSERT INTO _mindlink_test_authority(owner_id,actor_id,generation,policy_version) VALUES('42','42',0,7)"); err != nil {
		t.Fatal(err)
	}
	c := original
	c.IssuedAt = time.Now().Unix()
	c.ExpiresAt = c.IssuedAt + 60
	c.JTI = strings.Repeat("b", 64)
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
	a, err := v.Verify(fixtureRequest(t, token, c.Method, c.Path, string(fixtureJSON(t, disableBody()))), OperationPrincipalDisable, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// This is a journal/read fixture, not a production disable or authority projection.
	result, err := v.Transact(context.Background(), db, a, currentAuthority, func(tx *sql.Tx) (json.RawMessage, error) {
		if _, err := tx.Exec("UPDATE _mindlink_test_authority SET policy_version=8 WHERE owner_id='42' AND actor_id='42'"); err != nil {
			return nil, err
		}
		if _, err := tx.Exec("INSERT INTO _mindlink_test_effects(operation_id) VALUES(?)", c.OperationID); err != nil {
			return nil, err
		}
		return raw, nil
	})
	if err != nil || result.Replayed {
		t.Fatalf("fixture original commit failed: %v", err)
	}
	return db, v, key, original, raw
}

func queryReadAuthority(tx *sql.Tx, query, original Claims) error {
	if query.OwnerID != original.OwnerID {
		return ErrForbidden
	}
	var version int64
	var revoked bool
	err := tx.QueryRow("SELECT policy_version,disabled FROM _mindlink_test_authority WHERE owner_id=? AND actor_id=? FOR UPDATE", query.OwnerID, query.ActorID).Scan(&version, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	if revoked {
		return ErrForbidden
	}
	if version != query.ExpectedPolicyVersion || version != query.DesiredPolicyVersion {
		return ErrStalePolicy
	}
	return nil
}

func assertQueryOriginalUnchanged(t *testing.T, db *sql.DB, original Claims, raw []byte, tickets int) {
	t.Helper()
	assertRows(t, db, 1, tickets, 1)
	var stored []byte
	if err := db.QueryRow("SELECT result_json FROM mindlink_operations WHERE issuer=? AND operation_id=?", original.Issuer, original.OperationID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, raw) {
		t.Fatal("query rewrote original receipt")
	}
}

func TestMySQLQueryOperationReturnsOriginalAfterPolicyChange(t *testing.T) {
	db, v, key, original, raw := seedQueryOperation(t)
	a := queryProof(t, v, key, original, nil)
	for i := 0; i < 2; i++ {
		r, err := v.QueryOperation(context.Background(), db, a, queryReadAuthority)
		if err != nil || r == nil || !r.Replayed || !bytes.Equal(r.Result, raw) {
			t.Fatalf("original query failed: %v", err)
		}
		assertQueryOriginalUnchanged(t, db, original, raw, 2)
	}
	// A fresh pool represents a new consumer, not proof of a server process restart.
	reopened := ownedDB(t)
	r, err := v.QueryOperation(context.Background(), reopened, queryProof(t, v, key, original, func(c *Claims) { c.JTI = strings.Repeat("e", 64) }), queryReadAuthority)
	if err != nil || r == nil || !bytes.Equal(r.Result, raw) {
		t.Fatalf("reopened query failed: %v", err)
	}
	assertQueryOriginalUnchanged(t, db, original, raw, 3)
}

func TestMySQLQueryOperationAllowsRenewedKeyAndCurrentSystemReader(t *testing.T) {
	db, _, _, original, raw := seedQueryOperation(t)
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8}, ed25519.SeedSize))
	trust, _ := fixtureTrust()
	trust.PublicKey = key.Public().(ed25519.PublicKey)
	trust.QueryOperations = []string{"operations.query"}
	trust.MutationOperations = nil
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO _mindlink_test_authority(owner_id,actor_id,generation,policy_version) VALUES('42','fixture-system',0,8)"); err != nil {
		t.Fatal(err)
	}
	a := queryProof(t, v, key, original, func(c *Claims) { c.ActorKind = "system"; c.ActorID = "fixture-system" })
	r, err := v.QueryOperation(context.Background(), db, a, queryReadAuthority)
	if err != nil || r == nil || !r.Replayed || !bytes.Equal(r.Result, raw) {
		t.Fatalf("renewed current reader lost original actor/version: %v", err)
	}
	assertQueryOriginalUnchanged(t, db, original, raw, 2)
}

func TestMySQLQueryOperationRejectsCurrentReadRevocation(t *testing.T) {
	for _, tc := range []struct {
		name, statement string
		want            error
	}{
		{"version changed", "UPDATE _mindlink_test_authority SET policy_version=9 WHERE actor_id='42'", ErrStalePolicy},
		{"read revoked", "UPDATE _mindlink_test_authority SET disabled=TRUE WHERE actor_id='42'", ErrForbidden},
		{"authority removed", "DELETE FROM _mindlink_test_authority WHERE actor_id='42'", ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, v, key, original, raw := seedQueryOperation(t)
			if _, err := db.Exec(tc.statement); err != nil {
				t.Fatal(err)
			}
			r, err := v.QueryOperation(context.Background(), db, queryProof(t, v, key, original, nil), queryReadAuthority)
			if r != nil || !errors.Is(err, tc.want) {
				t.Fatalf("revoked/stale read accepted: %v", err)
			}
			assertQueryOriginalUnchanged(t, db, original, raw, 1)
		})
	}
}

func TestMySQLQueryOperationRejectsIntentAndTicketChanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Claims)
		want   error
	}{
		{"request", func(c *Claims) { c.RequestID = "other" }, ErrRequestConflict},
		{"digest", func(c *Claims) { c.RequestDigest = strings.Repeat("e", 64) }, ErrRequestConflict},
		{"target", func(c *Claims) { c.TargetID = "ma222222222222222222222222222222" }, ErrRequestConflict},
		{"owner", func(c *Claims) { c.OwnerID = "43"; c.ActorID = "43" }, ErrScopeMismatch},
		{"mutation jti reused", func(c *Claims) { c.JTI = strings.Repeat("b", 64) }, ErrRequestConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, v, key, original, raw := seedQueryOperation(t)
			r, err := v.QueryOperation(context.Background(), db, queryProof(t, v, key, original, tc.change), queryReadAuthority)
			if r != nil || !errors.Is(err, tc.want) {
				t.Fatalf("rebound query accepted: %v", err)
			}
			assertQueryOriginalUnchanged(t, db, original, raw, 1)
		})
	}
}

func TestMySQLQueryOperationRejectsIssuerAndAudienceFallback(t *testing.T) {
	for _, field := range []string{"issuer", "audience"} {
		t.Run(field, func(t *testing.T) {
			db, _, key, original, raw := seedQueryOperation(t)
			trust, _ := fixtureTrust()
			trust.QueryOperations = []string{"operations.query"}
			trust.MutationOperations = []string{OperationPrincipalDisable}
			query := original
			want := ErrUnknownResult
			if field == "issuer" {
				trust.Issuer = "other-biz"
				query.Issuer = trust.Issuer
			} else {
				trust.Audience = "other-im"
				query.Audience = trust.Audience
				want = ErrScopeMismatch
			}
			v, err := NewVerifier([]Trust{trust})
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			r, err := v.QueryOperation(context.Background(), db, queryProof(t, v, key, query, nil), func(*sql.Tx, Claims, Claims) error { calls++; return nil })
			if r != nil || !errors.Is(err, want) || calls != 0 {
				t.Fatalf("original binding fallback: %v, authority calls=%d", err, calls)
			}
			assertQueryOriginalUnchanged(t, db, original, raw, 1)
		})
	}
}

func TestMySQLQueryOperationRejectsCorruptOrMissingOriginal(t *testing.T) {
	for _, tc := range []struct{ name, statement string }{
		{"missing", "DELETE FROM mindlink_operations"},
		{"null result", "UPDATE mindlink_operations SET result_json=NULL"},
		{"empty result", "UPDATE mindlink_operations SET result_json='{}'"},
		{"binding damaged", "UPDATE mindlink_operations SET binding_json='{}'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, v, key, original, _ := seedQueryOperation(t)
			if _, err := db.Exec(tc.statement); err != nil {
				t.Fatal(err)
			}
			r, err := v.QueryOperation(context.Background(), db, queryProof(t, v, key, original, nil), queryReadAuthority)
			if r != nil || !errors.Is(err, ErrUnknownResult) {
				t.Fatalf("invalid original read as success: %v", err)
			}
			var tickets, effects int
			if err := db.QueryRow("SELECT COUNT(*) FROM mindlink_tickets").Scan(&tickets); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow("SELECT COUNT(*) FROM _mindlink_test_effects").Scan(&effects); err != nil {
				t.Fatal(err)
			}
			if tickets != 1 || effects != 1 {
				t.Fatalf("bad query changed effects/tickets: %d/%d", effects, tickets)
			}
		})
	}
}

func TestMySQLQueryOperationRejectsExpiredLockWaiters(t *testing.T) {
	for _, table := range []string{"mindlink_operations", "_mindlink_test_authority", "mindlink_tickets"} {
		t.Run(table, func(t *testing.T) {
			db, v, key, original, raw := seedQueryOperation(t)
			a := queryProof(t, v, key, original, func(c *Claims) { c.IssuedAt = time.Now().Unix() - 59; c.ExpiresAt = time.Now().Unix() - 2 })
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			locker, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer locker.Rollback()
			switch table {
			case "mindlink_operations":
				_, err = locker.ExecContext(ctx, "UPDATE mindlink_operations SET operation_id=operation_id WHERE issuer=? AND operation_id=?", original.Issuer, original.OperationID)
			case "_mindlink_test_authority":
				_, err = locker.ExecContext(ctx, "UPDATE _mindlink_test_authority SET policy_version=policy_version WHERE actor_id='42'")
			case "mindlink_tickets":
				_, err = locker.ExecContext(ctx, "INSERT INTO mindlink_tickets(issuer,jti,ticket_hash,operation_id) VALUES(?,?,?,?)", original.Issuer, a.claims.JTI, a.ticketHash[:], original.OperationID)
			}
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				r, err := v.QueryOperation(ctx, db, a, queryReadAuthority)
				if r != nil {
					done <- fmt.Errorf("expired query returned original")
					return
				}
				done <- err
			}()
			deadline := time.Now().Add(5 * time.Second)
			observed := false
			for time.Now().Before(deadline) {
				var waits int
				err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks l ON l.ENGINE_LOCK_ID=w.REQUESTING_ENGINE_LOCK_ID AND l.ENGINE=w.ENGINE WHERE l.OBJECT_SCHEMA=DATABASE() AND l.OBJECT_NAME=?`, table).Scan(&waits)
				if err != nil {
					t.Fatal(err)
				}
				if waits > 0 {
					observed = true
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !observed {
				t.Fatal("no actual query lock wait observed")
			}
			time.Sleep(time.Until(time.Unix(a.claims.ExpiresAt+5, 0)) + 100*time.Millisecond)
			if err := locker.Rollback(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, ErrForbidden) {
					t.Fatalf("expired lock waiter: got %v want FORBIDDEN", err)
				}
			case <-ctx.Done():
				t.Fatal("query waiter did not finish")
			}
			assertQueryOriginalUnchanged(t, db, original, raw, 1)
		})
	}
}

func TestMySQLQueryOperationSQLFailureDoesNotReturnReceipt(t *testing.T) {
	db, v, key, original, raw := seedQueryOperation(t)
	r, err := v.QueryOperation(context.Background(), db, queryProof(t, v, key, original, nil), func(tx *sql.Tx, query, original Claims) error {
		if err := queryReadAuthority(tx, query, original); err != nil {
			return err
		}
		_, err := tx.Exec("SELECT * FROM _mindlink_test_nonexistent_query_table")
		return err
	})
	if r != nil || !errors.Is(err, ErrUnknownResult) {
		t.Fatalf("SQL failure returned a receipt: %v", err)
	}
	assertQueryOriginalUnchanged(t, db, original, raw, 1)
}

func TestMySQLQueryOperationLostCommitResponseIsUnknown(t *testing.T) {
	db, v, key, original, raw := seedQueryOperation(t)
	private, err := os.ReadFile(os.Getenv("MINDLINK_TEST_DSN_FILE"))
	if err != nil {
		t.Fatal("read owned fixture configuration")
	}
	var config struct {
		DSN string `json:"dsn"`
	}
	if json.Unmarshal(private, &config) != nil {
		t.Fatal("invalid owned fixture configuration")
	}
	cfg, err := mysql.ParseDSN(config.DSN)
	if err != nil {
		t.Fatal("invalid fixture DSN")
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil || host != "127.0.0.1" || cfg.Net != "tcp" || cfg.DBName != "mindlink_policy_c82" || cfg.TLSConfig != "" {
		t.Fatal("refusing non-owned or encrypted proxy fixture")
	}
	addr, dropped := loseOneCommitResponse(t, cfg.Addr)
	cfg.Addr = addr
	proxied, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open fixture proxy")
	}
	defer proxied.Close()
	a := queryProof(t, v, key, original, nil)
	r, err := v.QueryOperation(context.Background(), proxied, a, queryReadAuthority)
	if r != nil || !errors.Is(err, ErrUnknownResult) {
		t.Fatalf("lost commit ACK returned success: %v", err)
	}
	if !dropped.Load() {
		t.Fatal("no actual COMMIT response was dropped")
	}
	// The server really committed the query ticket despite the missing response.
	assertQueryOriginalUnchanged(t, db, original, raw, 2)
	r, err = v.QueryOperation(context.Background(), db, a, queryReadAuthority)
	if err != nil || r == nil || !r.Replayed || !bytes.Equal(r.Result, raw) {
		t.Fatalf("same query did not recover original after lost ACK: %v", err)
	}
	assertQueryOriginalUnchanged(t, db, original, raw, 2)
}

// The owned MySQL fixture uses plain TCP. Forward packets unchanged, then drop
// the server's COMMIT reply after MySQL has committed; never record packet data.
func loseOneCommitResponse(t *testing.T, upstream string) (string, *atomic.Bool) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	dropped := new(atomic.Bool)
	go func() {
		client, err := l.Accept()
		if err != nil {
			return
		}
		defer client.Close()
		server, err := net.DialTimeout("tcp", upstream, 3*time.Second)
		if err != nil {
			return
		}
		defer server.Close()
		client.SetDeadline(time.Now().Add(15 * time.Second))
		server.SetDeadline(time.Now().Add(15 * time.Second))
		var committing atomic.Bool
		done := make(chan struct{}, 1)
		go func() {
			defer func() { done <- struct{}{} }()
			for {
				packet, err := mysqlFixturePacket(client)
				if err != nil {
					return
				}
				if len(packet) > 5 && packet[4] == 3 && string(packet[5:]) == "COMMIT" {
					committing.Store(true)
				}
				if _, err := server.Write(packet); err != nil {
					return
				}
			}
		}()
		for {
			packet, err := mysqlFixturePacket(server)
			if err != nil {
				break
			}
			if committing.Load() {
				dropped.Store(true)
				break
			}
			if _, err := client.Write(packet); err != nil {
				break
			}
		}
		client.Close()
		server.Close()
		<-done
	}()
	return l.Addr().String(), dropped
}

func mysqlFixturePacket(r io.Reader) ([]byte, error) {
	h := make([]byte, 4)
	if _, err := io.ReadFull(r, h); err != nil {
		return nil, err
	}
	n := int(h[0]) | int(h[1])<<8 | int(h[2])<<16
	packet := make([]byte, 4+n)
	copy(packet, h)
	_, err := io.ReadFull(r, packet[4:])
	return packet, err
}
