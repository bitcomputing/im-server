//go:build integration

package mindlink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"im-server/commons/pbdefines/pbobjs"
)

func stationStopFixture(t *testing.T) (*sql.DB, *Verifier, *Authorization, *ConversationEnsureRequest) {
	t.Helper()
	db, cv, ca, source, b := conversationFixture(t)
	if _, err := cv.transactConversationEnsure(context.Background(), db, ca, source); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE mindlink_conversations SET kind='mindlink_group'`); err != nil {
		t.Fatal(err)
	}
	v, a := stationStopProof(t, nil)
	return db, v, a, b
}

func stationRows(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	for _, table := range []string{"g_hismsgs", "mindlink_station_stops", "mindlink_message_outbox"} {
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != want {
			t.Fatalf("%s rows=%d want=%d err=%v", table, n, want, err)
		}
	}
}

func TestMySQLStationStoppedAtomicReplayQueryAndOutbox(t *testing.T) {
	db, v, a, b := stationStopFixture(t)
	ctx := context.Background()
	ownerVerifier, ownerSend := currentMessageProof(t, "owner")
	if _, err := ownerVerifier.SendMessage(ctx, db, ownerSend); !errors.Is(err, ErrForbidden) {
		t.Fatal("read-only native owner gained normal send", err)
	}
	_, key := fixtureTrust()
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, a.claims))
	out := httptest.NewRecorder()
	NewPolicyHTTP(v, db).ServeHTTP(out, fixtureRequest(t, token, "POST", StationStoppedPath, string(a.body)))
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Operation json.RawMessage `json:"operation"`
			Replayed  bool            `json:"replayed"`
		} `json:"data"`
	}
	if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &envelope) != nil || envelope.Code != 0 || envelope.Data.Replayed {
		t.Fatalf("station HTTP mutation status=%d", out.Code)
	}
	first := envelope.Data.Operation
	var r StationStopResult
	if json.Unmarshal(first, &r) != nil || r.SenderPrincipalID != b.OwnerIMID || r.NativeConversationID != b.NativeTargetID || r.EventID != strings.Repeat("d", 64) {
		t.Fatal("native anchor receipt mismatch")
	}
	// Simulate a lost response and concurrent retries of the identical original.
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			again, err := v.StationStopped(ctx, db, a)
			if err != nil || again == nil || !again.Replayed || !bytes.Equal(again.Result, first) {
				t.Error("lost-response retry changed original", err)
			}
		}()
	}
	wg.Wait()
	stationRows(t, db, 1)
	for _, actor := range []string{"system", "owner"} {
		q := queryProof(t, v, key, a.claims, func(c *Claims) {
			c.ExpectedPolicyVersion, c.DesiredPolicyVersion = 1, 1
			if actor == "owner" {
				c.ActorKind, c.ActorID, c.JTI = "owner", c.OwnerID, strings.Repeat("e", 64)
			}
		})
		got, err := v.QueryOperation(ctx, db, q, func(tx *sql.Tx, c, original Claims) error { return authorizeOperationRead(ctx, tx, c, original) })
		if err != nil || !bytes.Equal(got.Result, first) {
			t.Fatal("original query failed", err)
		}
	}
	id := messageOutboxID(a.claims)
	if _, err := db.Exec(`CREATE TRIGGER stop_inbox_failure BEFORE INSERT ON sendboxmsgs FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='fixture rollback'`); err != nil {
		t.Fatal(err)
	}
	if ok, err := v.MaterializeOwnerInbox(ctx, db, id); ok || err == nil {
		t.Fatal("failed inbox write reported success")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM conversations`).Scan(&n); err != nil || n != 0 {
		t.Fatal("partial conversation survived inbox rollback", err)
	}
	if _, err := db.Exec(`DROP TRIGGER stop_inbox_failure`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		ok, err := v.MaterializeOwnerInbox(ctx, db, id)
		if err != nil || ok != (i == 0) {
			t.Fatal("outbox did not materialize exactly once", err)
		}
	}
	var raw []byte
	if err := db.QueryRow(`SELECT msg_body FROM sendboxmsgs WHERE msg_id=?`, r.MessageID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var down pbobjs.DownMsg
	if proto.Unmarshal(raw, &down) != nil || down.MsgId != r.MessageID || down.SenderId != b.OwnerIMID {
		t.Fatal("native inbox payload changed")
	}
	fence, err := NewNativeFence(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := fence.OwnerInboxReadable(ctx, a.claims.AppKey, b.OwnerIMID, &down); err != nil || !ok {
		t.Fatal("owner cannot read durable stop inbox", err)
	}
	calls := 0
	for range 2 {
		if err := NewPolicyHTTP(v, db).SignalOwnerInbox(ctx, id, func(_ context.Context, app, owner string, stamp int64) error {
			calls++
			if app != a.claims.AppKey || owner != b.OwnerIMID || stamp <= 0 {
				t.Fatal("notification binding")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatal("duplicate notification")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sendboxmsgs WHERE msg_id=?`, r.MessageID).Scan(&n); err != nil || n != 1 {
		t.Fatal("duplicate owner inbox", err)
	}
	if _, err := db.Exec(`UPDATE mindlink_conversations SET policy_version=2,state='disabled'`); err != nil {
		t.Fatal(err)
	}
	q := queryProof(t, v, key, a.claims, func(c *Claims) {
		c.ExpectedPolicyVersion, c.DesiredPolicyVersion, c.JTI = 1, 1, strings.Repeat("f", 64)
	})
	got, err := v.QueryOperation(ctx, db, q, func(tx *sql.Tx, c, original Claims) error { return authorizeOperationRead(ctx, tx, c, original) })
	if err != nil || !bytes.Equal(got.Result, first) {
		t.Fatal("policy drift rewrote original", err)
	}
	if replayed, err := v.StationStopped(ctx, db, a); err != nil || !replayed.Replayed || !bytes.Equal(replayed.Result, first) {
		t.Fatal("policy drift blocked original POST replay", err)
	}
	if _, err := db.Exec(`UPDATE mindlink_owner_barriers SET read_enabled=0`); err != nil {
		t.Fatal(err)
	}
	if _, err := v.QueryOperation(ctx, db, q, func(tx *sql.Tx, c, original Claims) error { return authorizeOperationRead(ctx, tx, c, original) }); !errors.Is(err, ErrForbidden) {
		t.Fatal("revoked owner queried original", err)
	}
	if _, err := v.StationStopped(ctx, db, a); !errors.Is(err, ErrForbidden) {
		t.Fatal("revoked owner replayed original POST", err)
	}
	stationRows(t, db, 1)
}

func TestMySQLStationStoppedHistoricalRecoveryKeepsCurrentReadAndOriginalTrust(t *testing.T) {
	db, v, a, _ := stationStopFixture(t)
	ctx := context.Background()
	first, err := v.StationStopped(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE mindlink_conversations SET policy_version=2`); err != nil {
		t.Fatal(err)
	}
	// A fresh write still needs the current policy; the exception only reads an existing original.
	nv, fresh := stationStopProof(t, func(b *StationStopRequest, c *Claims) {
		b.EventID, b.Epoch = strings.Repeat("e", 64), 8
		b.OperationID = stationStopOperationID(*c, b.EventID)
		b.RequestID = b.OperationID
		c.OperationID, c.RequestID, c.JTI = b.OperationID, b.RequestID, strings.Repeat("2", 64)
	})
	if _, err := nv.StationStopped(ctx, db, fresh); !errors.Is(err, ErrStalePolicy) {
		t.Fatal("historical recovery exception admitted a stale fresh write", err)
	}
	_, key := fixtureTrust()
	q := queryProof(t, v, key, a.claims, func(c *Claims) { c.ExpectedPolicyVersion, c.DesiredPolicyVersion = 1, 1 })
	serve := func(handler *PolicyHTTP, c Claims, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, fixtureRequest(t, token, c.Method, c.Path, string(body)))
		return out
	}
	handler := NewPolicyHTTP(v, db)
	for _, change := range []func(*Claims){
		func(c *Claims) { c.TenantID = "forged-tenant" },
		func(c *Claims) { c.OwnerID = "43" },
		func(c *Claims) { c.ActorID = "forged-system" },
		func(c *Claims) { c.ActorKind, c.ActorID, c.AvatarGeneration = "avatar", "A", 1 },
	} {
		c := q.claims
		change(&c)
		if out := serve(handler, c, nil); out.Code != 403 {
			t.Fatalf("historical recovery accepted forged scope/actor: %d", out.Code)
		}
	}
	for _, tc := range []struct {
		name, change, restore string
		status                int
	}{
		{"read_revoked", `UPDATE mindlink_owner_barriers SET read_enabled=0`, `UPDATE mindlink_owner_barriers SET read_enabled=1`, 403},
		{"current_scope_changed", `UPDATE mindlink_conversations SET owner_id='43'`, `UPDATE mindlink_conversations SET owner_id='42'`, 403},
		{"current_kind_changed", `UPDATE mindlink_conversations SET kind='avatar_pair_group'`, `UPDATE mindlink_conversations SET kind='mindlink_group'`, 403},
		{"original_fact_changed", `UPDATE mindlink_station_stops SET reason='auth_failed'`, `UPDATE mindlink_station_stops SET reason='tls_failed'`, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.Exec(tc.change); err != nil {
				t.Fatal(err)
			}
			for _, input := range []struct {
				c    Claims
				body []byte
			}{{q.claims, nil}, {a.claims, a.body}} {
				if out := serve(handler, input.c, input.body); out.Code != tc.status {
					t.Fatalf("historical recovery ignored current/original protection: status=%d", out.Code)
				}
			}
			if _, err := db.Exec(tc.restore); err != nil {
				t.Fatal(err)
			}
		})
	}
	trust, _ := fixtureTrust()
	trust.MutationOperations = nil
	trust.QueryOperations = []string{"operations.query"}
	ungranted, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	if out := serve(NewPolicyHTTP(ungranted, db), q.claims, nil); out.Code != 403 {
		t.Fatalf("historical original ignored configured system permission: %d", out.Code)
	}
	queried, err := v.QueryOperation(ctx, db, q, func(tx *sql.Tx, c, original Claims) error { return authorizeOperationRead(ctx, tx, c, original) })
	if err != nil || !bytes.Equal(queried.Result, first.Result) {
		t.Fatal("restored current authority could not recover original", err)
	}
	stationRows(t, db, 1)
}

func TestMySQLStationStoppedRejectsScopePolicyAndAtomicFailures(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		want      error
	}{
		{"pair", `UPDATE mindlink_conversations SET kind='avatar_pair_group'`, ErrForbidden},
		{"dm", `UPDATE mindlink_conversations SET kind='owner_avatar_dm',channel_type=1,native_target_id='ma555555555555555555555555555555',members_json='[{"avatar_id":"A","principal_id":"ma555555555555555555555555555555","avatar_generation":1,"principal_policy_version":1}]'`, ErrForbidden},
		{"ordinary_group", `DELETE FROM mindlink_conversations`, ErrNotReady},
		{"owner_scope", `UPDATE mindlink_conversations SET owner_id='43'`, ErrScopeMismatch},
		{"tenant_scope", `UPDATE mindlink_conversations SET tenant_id='other'`, ErrScopeMismatch},
		{"write_revoked", `UPDATE mindlink_owner_barriers SET write_enabled=0`, ErrForbidden},
		{"stale_policy", `UPDATE mindlink_conversations SET policy_version=2`, ErrStalePolicy},
		{"disabled_member", `UPDATE mindlink_principals SET state='disabled'`, ErrPrincipalDisabled},
		{"ledger_failure", `CREATE TRIGGER stop_ledger_failure BEFORE INSERT ON mindlink_station_stops FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='fixture rollback'`, ErrUnknownResult},
		{"outbox_failure", `CREATE TRIGGER stop_outbox_failure BEFORE INSERT ON mindlink_message_outbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='fixture rollback'`, ErrUnknownResult},
		{"receipt_failure", `CREATE TRIGGER stop_receipt_failure BEFORE UPDATE ON mindlink_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='fixture rollback'`, ErrUnknownResult},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, v, a, _ := stationStopFixture(t)
			if _, err := db.Exec(tc.sql); err != nil {
				t.Fatal(err)
			}
			if r, err := v.StationStopped(context.Background(), db, a); r != nil || !errors.Is(err, tc.want) {
				t.Fatalf("rejected operation got=%v want=%v", err, tc.want)
			}
			stationRows(t, db, 0)
			for _, table := range []string{"mindlink_operations", "mindlink_tickets"} {
				var n int
				if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 1 {
					t.Fatal("rejected operation changed prior journal/ticket", err, n)
				}
			}
		})
	}
}

func TestMySQLStationStoppedConflictsNeverDuplicateNative(t *testing.T) {
	db, v, a, _ := stationStopFixture(t)
	ctx := context.Background()
	if _, err := v.StationStopped(ctx, db, a); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*StationStopRequest, *Claims){
		func(b *StationStopRequest, c *Claims) { b.Reason = "auth_failed" },
		func(b *StationStopRequest, c *Claims) { b.Text = "changed" },
		func(b *StationStopRequest, c *Claims) {
			b.EventID = strings.Repeat("e", 64)
			b.OperationID = stationStopOperationID(*c, b.EventID)
			b.RequestID = b.OperationID
			c.OperationID, c.RequestID, c.JTI = b.OperationID, b.RequestID, strings.Repeat("f", 64)
		},
	} {
		cv, ca := stationStopProof(t, change)
		if _, err := cv.StationStopped(ctx, db, ca); !errors.Is(err, ErrRequestConflict) {
			t.Fatal("conflicting original not rejected", err)
		}
	}
	stationRows(t, db, 1)
}

func stationHistoryHTTP(t *testing.T, db *sql.DB, v *Verifier, a *Authorization, prefix string) HistoryResult {
	t.Helper()
	c := a.claims
	c.ActorKind, c.ActorID, c.OperationType, c.Method = "owner", c.OwnerID, OperationHistoryRead, "GET"
	c.Path = prefix + c.TargetID + "/0/20"
	c.RequestDigest = fmt.Sprintf("%x", sha256.Sum256([]byte(c.Path)))
	c.BodySHA256 = fmt.Sprintf("%x", sha256.Sum256(nil))
	c.IssuedAt, c.ExpiresAt = time.Now().Unix(), time.Now().Unix()+60
	_, key := fixtureTrust()
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
	out := httptest.NewRecorder()
	NewPolicyHTTP(v, db).ServeHTTP(out, fixtureRequest(t, token, "GET", c.Path, ""))
	var envelope struct {
		Data struct {
			Operation HistoryResult `json:"operation"`
		} `json:"data"`
	}
	if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &envelope) != nil {
		t.Fatalf("history status=%d", out.Code)
	}
	return envelope.Data.Operation
}

func TestMySQLStationStoppedHistoryTrustedCategoryAndLegacyCompatibility(t *testing.T) {
	db, v, a, b := stationStopFixture(t)
	ctx := context.Background()
	stop, err := v.StationStopped(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	var r StationStopResult
	if json.Unmarshal(stop.Result, &r) != nil {
		t.Fatal("receipt")
	}
	mv, ma := currentMessageProof(t, "A")
	regular, err := mv.SendMessage(ctx, db, ma)
	if err != nil {
		t.Fatal(err)
	}
	var mr MessageSendResult
	if json.Unmarshal(regular.Result, &mr) != nil {
		t.Fatal("message receipt")
	}
	var raw []byte
	if err := db.QueryRow(`SELECT msg_body FROM g_hismsgs WHERE msg_id=?`, mr.MessageID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var down pbobjs.DownMsg
	if proto.Unmarshal(raw, &down) != nil {
		t.Fatal("message")
	}
	down.MsgContent = []byte(`{"content":"ordinary","extra":{"category":"station_stopped"},"category":"station_stopped"}`)
	raw, err = proto.Marshal(&down)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE g_hismsgs SET msg_body=? WHERE msg_id=?`, raw, mr.MessageID); err != nil {
		t.Fatal(err)
	}
	page := stationHistoryHTTP(t, db, v, a, HistoryV3PathPrefix)
	if page.Version != "mindlink-history-v3" || len(page.Messages) != 2 || page.RequesterID != b.OwnerIMID {
		t.Fatal("v3 page binding")
	}
	if page.Messages[0].ID != mr.MessageID || page.Messages[0].Category != "message" || page.Messages[1].ID != r.MessageID || page.Messages[1].Category != "station_stopped" || page.Messages[1].SenderID != page.RequesterID {
		t.Fatal("untrusted category or native author")
	}
	legacy := stationHistoryHTTP(t, db, v, a, HistoryPathPrefix)
	if legacy.Version != "mindlink-history-v1" || len(legacy.Messages) != 2 || legacy.Messages[0].Category != "" || legacy.Messages[1].Category != "" {
		t.Fatal("legacy history changed")
	}
	if _, err := db.Exec(`UPDATE mindlink_conversations SET state='disabled'`); err != nil {
		t.Fatal(err)
	}
	if len(stationHistoryHTTP(t, db, v, a, HistoryV3PathPrefix).Messages) != 2 {
		t.Fatal("retirement erased history")
	}
}
