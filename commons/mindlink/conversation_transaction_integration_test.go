//go:build integration

package mindlink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func conversationProof(t *testing.T) (*Verifier, *Authorization, *DisableAuthoritySource) {
	t.Helper()
	trust, key := fixtureTrust()
	trust.MutationOperations = []string{OperationConversationEnsure}
	trust.QueryOperations = []string{"operations.query"}
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	claims := fixtureClaims("")
	claims["operation_type"], claims["path"], claims["actor_kind"], claims["actor_id"] = OperationConversationEnsure, ConversationEnsurePath, "system", trust.SystemActorID
	claims["request_id"], claims["expected_policy_version"], claims["desired_policy_version"] = strings.Repeat("e", 64), int64(0), int64(1)
	claims["target_id"] = strings.Repeat("c", 64)
	claims["iat"], claims["exp"] = time.Now().Unix(), time.Now().Unix()+60
	var c Claims
	if DecodeStrictObject(fixtureJSON(t, claims), &c, claimFields) != nil {
		t.Fatal("fixture claims")
	}
	b := ConversationEnsureRequest{OperationID: c.OperationID, RequestID: c.RequestID, ConversationID: c.TargetID, Kind: "avatar_pair_group", NativeTargetID: "mg" + strings.Repeat("4", 30), OwnerIMID: "owner-native", DesiredPolicyVersion: 1, Members: []ConversationMember{{"A", "ma" + strings.Repeat("5", 30), 1, 1}, {"B", "ma" + strings.Repeat("6", 30), 1, 1}}}
	b.RequestDigest, err = ConversationEnsureDigest(c, &b)
	if err != nil {
		t.Fatal(err)
	}
	raw := fixtureJSON(t, b)
	claims["request_digest"], claims["body_sha256"] = b.RequestDigest, fmt.Sprintf("%x", sha256.Sum256(raw))
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, claims))
	auth, err := v.Verify(fixtureRequest(t, token, "POST", ConversationEnsurePath, string(raw)), OperationConversationEnsure, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c = auth.claims
	snapshot := map[string]string{"version": "mindlink-conversation-authority-v1", "environment": c.Environment, "tenant_id": c.TenantID, "appkey": c.AppKey, "issuer": c.Issuer, "audience": c.Audience, "system_actor_id": c.ActorID, "owner_id": c.OwnerID, "parent_operation_id": c.RequestID, "parent_digest": strings.Repeat("f", 64), "wire_operation_id": c.OperationID, "wire_request_digest": c.RequestDigest, "wire_body_sha256": c.BodySHA256, "conversation_id": c.TargetID, "lifecycle_state": "active"}
	source, _ := sourceChild(t, string(fixtureJSON(t, snapshot)), 0, 0)
	if err := os.WriteFile(source.configPath, fixtureJSON(t, map[string]any{"Output": string(fixtureJSON(t, snapshot)), "Conversation": true}), 0600); err != nil {
		t.Fatal(err)
	}
	return v, auth, source
}

func conversationFixture(t *testing.T) (*sql.DB, *Verifier, *Authorization, *DisableAuthoritySource, *ConversationEnsureRequest) {
	t.Helper()
	db := nativeFixture(t)
	v, auth, source := conversationProof(t)
	b, err := v.ParseConversationEnsure(auth)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`DELETE FROM mindlink_operations; UPDATE mindlink_owner_barriers SET write_enabled=TRUE; INSERT INTO users(app_key,user_id) VALUES('fixture-app','owner-native')`); err != nil {
		t.Fatal(err)
	}
	for _, m := range b.Members {
		if _, err = db.Exec(`INSERT INTO users(app_key,user_id,user_type,nickname) VALUES('fixture-app',?,1,?)`, m.PrincipalID, m.AvatarID); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`INSERT INTO mindlink_principals(environment,tenant_id,appkey,owner_id,principal_id,avatar_id,avatar_generation,policy_version,state) VALUES('dev','fixture-tenant','fixture-app','42',?,?,1,1,'active')`, m.PrincipalID, m.AvatarID); err != nil {
			t.Fatal(err)
		}
	}
	return db, v, auth, source, b
}

func TestMySQLConversationCreatesThreeMembersAndOriginalReceipt(t *testing.T) {
	db, v, auth, source, b := conversationFixture(t)
	handler, err := NewPolicyHTTPWithDisable(context.Background(), v, db, source)
	if err != nil {
		t.Fatal(err)
	}
	_, key := fixtureTrust()
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, auth.claims))
	auth, err = v.Verify(fixtureRequest(t, token, "POST", ConversationEnsurePath, string(auth.body)), OperationConversationEnsure, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, fixtureRequest(t, token, "POST", ConversationEnsurePath, string(auth.body)))
	var response struct {
		Code int `json:"code"`
		Data struct {
			Operation json.RawMessage `json:"operation"`
			Replayed  bool            `json:"replayed"`
		} `json:"data"`
	}
	if recorder.Code != 200 || json.Unmarshal(recorder.Body.Bytes(), &response) != nil || response.Code != 0 {
		t.Fatalf("HTTP creation: %d %s", recorder.Code, recorder.Body.String())
	}
	first := &MutationResult{Result: response.Data.Operation, Replayed: response.Data.Replayed}
	if first.Replayed || v.validateOperationResult(first.Result, auth.claims) != nil {
		t.Fatal("bad original receipt")
	}
	var n int
	if err = db.QueryRow(`SELECT COUNT(*) FROM groupmembers WHERE app_key='fixture-app' AND group_id=?`, b.NativeTargetID).Scan(&n); err != nil || n != 3 {
		t.Fatalf("native roster: %d %v", n, err)
	}
	var members []byte
	if err = db.QueryRow(`SELECT members_json FROM mindlink_conversations WHERE conversation_id=?`, b.ConversationID).Scan(&members); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(b.Members)
	if !bytes.Equal(members, want) {
		t.Fatal("projection changed roster")
	}
	again, err := v.transactConversationEnsure(context.Background(), db, auth, source)
	if err != nil || !again.Replayed || !bytes.Equal(first.Result, again.Result) {
		t.Fatalf("replay changed: %v", err)
	}
	query := queryProof(t, v, key, auth.claims, func(c *Claims) { c.ExpectedPolicyVersion = 1; c.DesiredPolicyVersion = 1 })
	queried, err := v.QueryOperation(context.Background(), db, query, func(tx *sql.Tx, c, original Claims) error {
		return authorizeOperationRead(context.Background(), tx, c, original)
	})
	if err != nil || !bytes.Equal(first.Result, queried.Result) {
		t.Fatalf("original GET: %v", err)
	}
	if _, err := db.Exec(`UPDATE mindlink_owner_barriers SET read_enabled=FALSE`); err != nil {
		t.Fatal(err)
	}
	if _, err := v.QueryOperation(context.Background(), db, query, func(tx *sql.Tx, c, original Claims) error {
		return authorizeOperationRead(context.Background(), tx, c, original)
	}); err == nil {
		t.Fatal("revoked owner read original")
	}
}

func TestMySQLConversationFailureLeavesNoPartialGroup(t *testing.T) {
	for _, tc := range []struct{ name, sql string }{
		{"revoked-owner", `UPDATE mindlink_owner_barriers SET write_enabled=FALSE`},
		{"disabled-member", `UPDATE mindlink_principals SET state='disabled' WHERE avatar_id='B'`},
		{"stale-member", `UPDATE mindlink_principals SET avatar_generation=2 WHERE avatar_id='B'`},
		{"late-failure", `CREATE TRIGGER fail_conversation_receipt BEFORE UPDATE ON mindlink_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='fixture rollback'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, v, auth, source, b := conversationFixture(t)
			if _, err := db.Exec(tc.sql); err != nil {
				t.Fatal(err)
			}
			if _, err := v.transactConversationEnsure(context.Background(), db, auth, source); err == nil {
				t.Fatal("invalid write succeeded")
			}
			for _, q := range []string{`SELECT COUNT(*) FROM mindlink_operations`, `SELECT COUNT(*) FROM mindlink_tickets`, `SELECT COUNT(*) FROM mindlink_conversations`} {
				var n int
				if err := db.QueryRow(q).Scan(&n); err != nil || n != 0 {
					t.Fatalf("partial result %s: %d %v", q, n, err)
				}
			}
			for _, table := range []string{"groupinfos", "groupmembers"} {
				var n int
				if err := db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE group_id=?", b.NativeTargetID).Scan(&n); err != nil || n != 0 {
					t.Fatalf("partial native group: %d %v", n, err)
				}
			}
		})
	}
}
