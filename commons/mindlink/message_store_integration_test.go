//go:build integration

package mindlink

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"google.golang.org/protobuf/proto"
	"im-server/commons/pbdefines/pbobjs"
)

func reliableConfig(t *testing.T) *mysql.Config {
	t.Helper()
	path := os.Getenv("MINDLINK_RELIABLE_TEST_DSN_FILE")
	if path == "" {
		t.Skip("requires the explicitly owned reliable3f7 loopback fixture; not DB coverage")
	}
	raw, err := os.ReadFile(path)
	var cfg struct {
		DSN string `json:"dsn"`
	}
	if err != nil || json.Unmarshal(raw, &cfg) != nil {
		t.Fatal("invalid owned reliable fixture file")
	}
	m, err := mysql.ParseDSN(cfg.DSN)
	if err != nil {
		t.Fatal("invalid owned reliable fixture DSN")
	}
	host, port, err := net.SplitHostPort(m.Addr)
	if err != nil || host != "127.0.0.1" || port != "33606" || m.Net != "tcp" || m.DBName != "mindlink_reliable_3f7" || m.User != "reliable3f7" {
		t.Fatal("refusing a database outside the owned3f7 fixture")
	}
	m.Logger = log.New(io.Discard, "", 0)
	return m
}

func reliableConnection(t *testing.T) *sql.DB {
	t.Helper()
	m := reliableConfig(t)
	connector, err := mysql.NewConnector(m)
	if err != nil {
		t.Fatal("fixture connector failed")
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(16)
	t.Cleanup(func() { db.Close() })
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatal("owned reliable fixture unavailable")
	}
	var dataDir string
	var serverPort int
	if db.QueryRow("SELECT @@datadir,@@port").Scan(&dataDir, &serverPort) != nil || serverPort != 33606 || !strings.EqualFold(filepath.Clean(strings.TrimRight(dataDir, `\/`)), filepath.Join(filepath.Dir(os.Getenv("MINDLINK_RELIABLE_TEST_DSN_FILE")), "data")) {
		t.Fatal("owned server process/data namespace identity unavailable")
	}
	return db
}

func reliableDB(t *testing.T) *sql.DB {
	t.Helper()
	db := reliableConnection(t)
	var initialized int
	if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='globalconfs'").Scan(&initialized); err != nil {
		t.Fatal(err)
	}
	paths := []string{"../dbcommons/sqls/20261112.sql"}
	if initialized == 0 {
		paths = append([]string{"../../sql/imserver.sql"}, paths...)
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(raw)); err != nil {
			t.Fatalf("owned schema install failed: %v", err)
		}
	}
	for _, table := range []string{"mindlink_message_outbox", "mindlink_send_conversations", "mindlink_tickets", "mindlink_operations", "mindlink_owner_barriers", "mindlink_principals", "message_sequence", "conversation_seen", "p_hismsgs", "g_hismsgs", "users", "groupmembers", "groupinfos", "banusers", "p_delhismsgs", "g_delhismsgs", "convercleantimes", "usercleantimes", "groupinfoexts", "groupmemberexts", "apps", "appexts", "blocks", "friendrels"} {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func reliableProjection(t *testing.T, db *sql.DB, kind string) MessageNativeBinding {
	t.Helper()
	if _, err := db.Exec("INSERT INTO apps(app_key,app_secret,app_secure_key,app_status) VALUES('fixture-app','owned-fixture-only','owned-fixture-only',0)"); err != nil {
		t.Fatal(err)
	}
	members := []MessageConversationMember{{ActorKind: "avatar", ActorID: "avatar-A", PrincipalID: disablePrincipal, AvatarGeneration: 3}, {ActorKind: "owner", ActorID: "42", PrincipalID: "43"}}
	b := messageNativeBinding()
	if kind != "avatar_owner_dm" {
		b.ChannelType, b.ConversationID, b.TargetPrincipalID = 2, "mg"+strings.Repeat("1", 30), "mg"+strings.Repeat("1", 30)
	}
	if kind == "avatar_pair_group" {
		members = append(members[:1], MessageConversationMember{ActorKind: "avatar", ActorID: "avatar-B", PrincipalID: "ma" + strings.Repeat("2", 30), AvatarGeneration: 4}, members[1])
	}
	for _, m := range members {
		if _, err := db.Exec("INSERT INTO users(app_key,user_id,nickname) VALUES('fixture-app',?,?)", m.PrincipalID, m.ActorID); err != nil {
			t.Fatal(err)
		}
		if m.ActorKind == "avatar" {
			if _, err := db.Exec("INSERT INTO mindlink_principals(environment,tenant_id,appkey,owner_id,principal_id,avatar_id,avatar_generation,policy_version,state) VALUES('dev','fixture-tenant','fixture-app','42',?,?,?,7,'active')", m.PrincipalID, m.ActorID, m.AvatarGeneration); err != nil {
				t.Fatal(err)
			}
		}
		if b.ChannelType == 2 {
			if _, err := db.Exec("INSERT INTO groupmembers(app_key,group_id,member_id) VALUES('fixture-app',?,?)", b.ConversationID, m.PrincipalID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if b.ChannelType == 2 {
		if _, err := db.Exec("INSERT INTO groupinfos(app_key,group_id,group_name) VALUES('fixture-app',?,'owned-fixture')", b.ConversationID); err != nil {
			t.Fatal(err)
		}
	}
	raw := fixtureJSON(t, members)
	hash := sha256.Sum256(raw)
	for _, statement := range []struct {
		q    string
		args []any
	}{
		{"INSERT INTO mindlink_owner_barriers(environment,tenant_id,appkey,owner_id,read_enabled,write_enabled) VALUES('dev','fixture-tenant','fixture-app','42',1,1)", nil},
		{"INSERT INTO mindlink_send_conversations(environment,tenant_id,appkey,owner_id,conversation_id,kind,policy_version,state,channel_type,native_conversation_id,sub_channel,members_json,members_hash) VALUES('dev','fixture-tenant','fixture-app','42',?,?,7,'active',?,?,'',?,?)", []any{strings.Repeat("c", 64), kind, b.ChannelType, b.ConversationID, raw, hash[:]}},
	} {
		if _, err := db.Exec(statement.q, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	return b
}

func reliableProof(t *testing.T, request MessageSendRequest, actor string, generation int64) (*Verifier, *Authorization) {
	t.Helper()
	trust, key := fixtureTrust()
	trust.MutationOperations, trust.QueryOperations = []string{OperationMessageSend}, []string{OperationMessageQuery}
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	c := fixtureClaims("")
	c["operation_type"], c["path"] = OperationMessageSend, MessageSendPath
	c["operation_id"], c["request_id"], c["target_id"] = request.OperationID, request.RequestID, request.ConversationID
	c["expected_policy_version"], c["desired_policy_version"] = request.PolicyVersion, request.PolicyVersion
	if actor != "owner" {
		c["actor_kind"], c["actor_id"], c["avatar_generation"] = "avatar", actor, generation
	}
	encoded := fixtureJSON(t, c)
	var claims Claims
	_ = json.Unmarshal(encoded, &claims)
	request.RequestDigest, err = MessageSendDigest(claims, &request)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(fixtureJSON(t, request))
	c["request_digest"], c["body_sha256"] = request.RequestDigest, fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
	c["iat"], c["exp"], c["jti"] = time.Now().Unix(), time.Now().Unix()+60, fmt.Sprintf("%064x", time.Now().UnixNano())
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
	auth, err := v.Verify(fixtureRequest(t, token, "POST", MessageSendPath, raw), OperationMessageSend, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return v, auth
}

func reliableRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMySQLReliableMessageNativeCommit(t *testing.T) {
	for _, kind := range []string{"avatar_owner_dm", "mindlink_group"} {
		t.Run(kind, func(t *testing.T) {
			db := reliableDB(t)
			binding := reliableProjection(t, db, kind)
			store, err := NewMessageStore(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			v, auth := reliableProof(t, messageRequest(), "owner", 0)
			result, err := store.Send(context.Background(), v, auth)
			if err != nil || result == nil || result.Replayed {
				t.Fatalf("send failed: %v", err)
			}
			receipt, err := v.ValidateMessageSendResult(auth, result.Result, binding)
			if err != nil {
				t.Fatal(err)
			}
			table := "p_hismsgs"
			if binding.ChannelType == 2 {
				table = "g_hismsgs"
			}
			var raw []byte
			var id, seq string
			if err := db.QueryRow("SELECT msg_id,CAST(msg_seq_no AS CHAR),msg_body FROM "+table+" WHERE app_key='fixture-app' AND conver_id=?", binding.ConversationID).Scan(&id, &seq, &raw); err != nil {
				t.Fatal(err)
			}
			var pb pbobjs.DownMsg
			if proto.Unmarshal(raw, &pb) != nil || pb.MsgId != receipt.MessageID || pb.MsgSeqNo != 1 || pb.SenderId != binding.SenderPrincipalID || seq != receipt.MsgSeqNo || id != receipt.MessageID || pb.Flags&8 == 0 {
				t.Fatal("not a real native history PB/sequence binding")
			}
			var content struct {
				Content string `json:"content"`
			}
			if json.Unmarshal(pb.MsgContent, &content) != nil || content.Content != "hello" {
				t.Fatal("native text payload lost")
			}
			if reliableRows(t, db, "mindlink_message_outbox") != 1 || reliableRows(t, db, "mindlink_operations") != 1 || reliableRows(t, db, "inboxmsgs") != 0 {
				t.Fatal("missing atomic outbox/journal or pre-dispatched inbox")
			}
			replayed, err := store.Send(context.Background(), v, auth)
			if err != nil || replayed == nil || !replayed.Replayed || string(replayed.Result) != string(result.Result) || reliableRows(t, db, table) != 1 || reliableRows(t, db, "mindlink_message_outbox") != 1 {
				t.Fatalf("replay repeated an effect: %v", err)
			}
			changed := messageRequest()
			changed.Text = "conflicting text"
			_, conflict := reliableProof(t, changed, "owner", 0)
			if got, err := store.Send(context.Background(), v, conflict); got != nil || !errors.Is(err, ErrForbidden) {
				// Authorization belongs to its exact Verifier instance.
				t.Fatal("foreign verifier accepted")
			}
			cv, conflict := reliableProof(t, changed, "owner", 0)
			if got, err := store.Send(context.Background(), cv, conflict); got != nil || !errors.Is(err, ErrRequestConflict) {
				t.Fatalf("same key conflict not rejected: %v", err)
			}
		})
	}
}

func reliableQueryProof(t *testing.T, original *Authorization, policy int64, actor ...string) (*Verifier, *Authorization) {
	t.Helper()
	trust, key := fixtureTrust()
	trust.QueryOperations = []string{OperationMessageQuery}
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	c := fixtureClaims("")
	c["operation_type"], c["method"], c["path"] = OperationMessageQuery, "GET", MessageResultPathPrefix+original.claims.RequestID
	c["operation_id"], c["request_id"], c["request_digest"], c["target_id"] = original.claims.OperationID, original.claims.RequestID, original.claims.RequestDigest, original.claims.TargetID
	c["expected_policy_version"], c["desired_policy_version"] = policy, policy
	if len(actor) > 0 {
		c["actor_kind"], c["actor_id"], c["avatar_generation"] = "avatar", actor[0], int64(3)
	}
	c["iat"], c["exp"], c["jti"] = time.Now().Unix(), time.Now().Unix()+60, fmt.Sprintf("%064x", time.Now().UnixNano())
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
	a, err := v.Verify(fixtureRequest(t, token, "GET", c["path"].(string), ""), OperationMessageQuery, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return v, a
}

func TestMySQLReliableMessageOriginalRecovery(t *testing.T) {
	db := reliableDB(t)
	reliableProjection(t, db, "avatar_owner_dm")
	store, err := NewMessageStore(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	v, auth := reliableProof(t, messageRequest(), "avatar-A", 3)
	first, err := store.Send(context.Background(), v, auth)
	if err != nil {
		t.Fatal(err)
	}
	// Discarding the first response is not a resend; a current owner reads the original.
	qv, query := reliableQueryProof(t, auth, 7)
	original, err := store.Query(context.Background(), qv, query)
	if err != nil || original == nil || !original.Replayed || string(original.Result) != string(first.Result) {
		t.Fatalf("original result recovery failed: %v", err)
	}
	for _, statement := range []string{
		"UPDATE mindlink_owner_barriers SET write_enabled=0",
		"UPDATE mindlink_principals SET state='disabled',avatar_generation=4,policy_version=8",
		"UPDATE mindlink_send_conversations SET state='disabled',policy_version=8",
		"DELETE FROM p_hismsgs",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	qv, query = reliableQueryProof(t, auth, 8)
	after, err := store.Query(context.Background(), qv, query)
	if err != nil || after == nil || string(after.Result) != string(first.Result) || reliableRows(t, db, "mindlink_operations") != 1 || reliableRows(t, db, "mindlink_message_outbox") != 1 {
		t.Fatalf("history retention/revocation rewrote the original: %v", err)
	}
	if _, err := db.Exec("UPDATE mindlink_owner_barriers SET read_enabled=0"); err != nil {
		t.Fatal(err)
	}
	qv, query = reliableQueryProof(t, auth, 8)
	if got, err := store.Query(context.Background(), qv, query); got != nil || !errors.Is(err, ErrForbidden) {
		t.Fatalf("revoked read recovered result: %v", err)
	}
}

func TestMySQLReliableMessageRollback(t *testing.T) {
	for _, stage := range []string{"history", "outbox", "receipt"} {
		t.Run(stage, func(t *testing.T) {
			db := reliableDB(t)
			reliableProjection(t, db, "avatar_owner_dm")
			store, err := NewMessageStore(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			name := "_reliable_fail_" + stage
			statement := "CREATE TRIGGER " + name + " BEFORE INSERT ON p_hismsgs FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned fixture history failure'"
			if stage == "outbox" {
				statement = "CREATE TRIGGER " + name + " BEFORE INSERT ON mindlink_message_outbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned fixture outbox failure'"
			}
			if stage == "receipt" {
				statement = "CREATE TRIGGER " + name + " BEFORE UPDATE ON mindlink_operations FOR EACH ROW BEGIN IF NEW.result_json IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned fixture receipt failure'; END IF; END"
			}
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := db.Exec("DROP TRIGGER IF EXISTS " + name); err != nil {
					t.Error(err)
				}
			})
			v, auth := reliableProof(t, messageRequest(), "owner", 0)
			if got, err := store.Send(context.Background(), v, auth); got != nil || !errors.Is(err, ErrUnknownResult) {
				t.Fatalf("failed transaction returned a receipt: %v", err)
			}
			for _, table := range []string{"p_hismsgs", "mindlink_message_outbox", "mindlink_operations", "mindlink_tickets", "message_sequence"} {
				if reliableRows(t, db, table) != 0 {
					t.Fatalf("partial effect retained in %s", table)
				}
			}
		})
	}
}

func TestMySQLReliableMessageAuthority(t *testing.T) {
	for _, tc := range []struct {
		name, kind, actor, change string
		want                      error
	}{
		{"owner_revoked", "avatar_owner_dm", "owner", "UPDATE mindlink_owner_barriers SET write_enabled=0", ErrForbidden},
		{"avatar_disabled", "avatar_owner_dm", "avatar-A", "UPDATE mindlink_principals SET state='disabled'", ErrPrincipalDisabled},
		{"generation_changed", "avatar_owner_dm", "avatar-A", "UPDATE mindlink_principals SET avatar_generation=4", ErrStalePolicy},
		{"policy_changed", "avatar_owner_dm", "owner", "UPDATE mindlink_send_conversations SET policy_version=8", ErrStalePolicy},
		{"native_user_missing", "avatar_owner_dm", "owner", "DELETE FROM users WHERE user_id='43'", ErrNotReady},
		{"members_corrupted", "avatar_owner_dm", "owner", "UPDATE mindlink_send_conversations SET members_hash=REPEAT('x',32)", ErrNotReady},
		{"group_extra_member", "mindlink_group", "owner", "INSERT INTO groupmembers(app_key,group_id,member_id) SELECT app_key,group_id,'unexpected' FROM groupinfos", ErrForbidden},
		{"group_muted", "mindlink_group", "owner", "UPDATE groupinfos SET is_mute=1", ErrForbidden},
		{"sender_muted", "mindlink_group", "owner", "UPDATE groupmembers SET is_mute=1 WHERE member_id='43'", ErrForbidden},
		{"pair_owner_denied", "avatar_pair_group", "owner", "", ErrForbidden},
		{"sender_banned", "avatar_owner_dm", "owner", "INSERT INTO banusers(app_key,user_id,scope_key,end_time) VALUES('fixture-app','43','default',0)", ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := reliableDB(t)
			reliableProjection(t, db, tc.kind)
			store, err := NewMessageStore(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			if tc.change != "" {
				if _, err := db.Exec(tc.change); err != nil {
					t.Fatal(err)
				}
			}
			generation := int64(0)
			if tc.actor != "owner" {
				generation = 3
			}
			v, auth := reliableProof(t, messageRequest(), tc.actor, generation)
			if got, err := store.Send(context.Background(), v, auth); got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("authority result: %v, want %v", err, tc.want)
			}
			for _, table := range []string{"p_hismsgs", "g_hismsgs", "mindlink_message_outbox", "mindlink_operations", "message_sequence"} {
				if reliableRows(t, db, table) != 0 {
					t.Fatalf("rejected request committed %s", table)
				}
			}
		})
	}
}

func TestMySQLReliableMessageHTTPResponseLoss(t *testing.T) {
	db := reliableDB(t)
	reliableProjection(t, db, "avatar_owner_dm")
	store, err := NewMessageStore(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	v, auth := reliableProof(t, messageRequest(), "owner", 0)
	h := NewMessageHTTP(v, store)
	committed := make(chan json.RawMessage, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if rec.Code != http.StatusOK {
				t.Errorf("private producer HTTP status=%d", rec.Code)
				w.WriteHeader(rec.Code)
				return
			}
			var envelope struct {
				Data struct {
					Operation json.RawMessage `json:"operation"`
				} `json:"data"`
			}
			if json.Unmarshal(rec.Body.Bytes(), &envelope) != nil {
				t.Error("invalid private HTTP receipt")
			}
			committed <- append(json.RawMessage(nil), envelope.Data.Operation...)
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			conn.Close()
			return
		}
		h.ServeHTTP(w, r)
	}))
	defer server.Close()
	_, key := fixtureTrust()
	call := func(a *Authorization) (*http.Response, error) {
		token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, a.claims))
		r, err := http.NewRequest(a.claims.Method, server.URL+a.claims.Path, strings.NewReader(string(a.body)))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		if a.claims.Method == "POST" {
			r.Header.Set("Content-Type", "application/json")
		}
		client := http.Client{Timeout: 5 * time.Second}
		return client.Do(r)
	}
	if resp, err := call(auth); err == nil {
		resp.Body.Close()
		t.Fatal("response was not lost")
	}
	var original json.RawMessage
	select {
	case original = <-committed:
	case <-time.After(5 * time.Second):
		t.Fatal("commit was not observed; recovery not verified")
	}
	_, query := reliableQueryProof(t, auth, 7)
	resp, err := call(query)
	if err != nil {
		t.Fatal("original-key HTTP recovery unavailable")
	}
	defer resp.Body.Close()
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Operation json.RawMessage `json:"operation"`
			Replayed  bool            `json:"replayed"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&envelope) != nil || resp.StatusCode != 200 || envelope.Code != 0 || !envelope.Data.Replayed || len(original) == 0 || string(envelope.Data.Operation) != string(original) {
		t.Fatal("HTTP query did not return the original committed receipt")
	}
	for _, table := range []string{"p_hismsgs", "mindlink_operations", "mindlink_message_outbox", "message_sequence"} {
		if reliableRows(t, db, table) != 1 {
			t.Fatalf("response loss duplicated %s", table)
		}
	}
}
