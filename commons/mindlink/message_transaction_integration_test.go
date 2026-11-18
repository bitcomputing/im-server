//go:build integration

package mindlink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"im-server/commons/pbdefines/pbobjs"
)

func currentMessageProof(t *testing.T, actor string) (*Verifier, *Authorization) {
	t.Helper()
	trust, key := fixtureTrust()
	trust.MutationOperations = []string{OperationMessageSend}
	trust.QueryOperations = []string{OperationMessageQuery}
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	fields := fixtureClaims("")
	fields["jti"] = strings.Repeat("7", 64)
	fields["operation_type"], fields["path"] = OperationMessageSend, MessageSendPath
	fields["iat"], fields["exp"] = time.Now().Unix(), time.Now().Unix()+60
	fields["operation_id"], fields["request_id"], fields["target_id"] = strings.Repeat("9", 64), strings.Repeat("8", 64), strings.Repeat("c", 64)
	fields["expected_policy_version"], fields["desired_policy_version"] = int64(1), int64(1)
	if actor != "owner" {
		fields["actor_kind"], fields["actor_id"], fields["avatar_generation"] = "avatar", actor, int64(1)
	}
	var claims Claims
	if err = json.Unmarshal(fixtureJSON(t, fields), &claims); err != nil {
		t.Fatal(err)
	}
	b := messageRequest()
	b.OperationID, b.RequestID, b.ConversationID, b.PolicyVersion = claims.OperationID, claims.RequestID, claims.TargetID, 1
	b.Text = "真实消息 <&> 😀"
	b.Mentions = []string{"B"}
	b.RequestDigest, err = MessageSendDigest(claims, &b)
	if err != nil {
		t.Fatal(err)
	}
	raw := fixtureJSON(t, b)
	fields["request_digest"], fields["body_sha256"] = b.RequestDigest, fmt.Sprintf("%x", sha256.Sum256(raw))
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, fields))
	auth, err := v.Verify(fixtureRequest(t, token, "POST", MessageSendPath, string(raw)), OperationMessageSend, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return v, auth
}

func TestMySQLReliableMessageAtomicNativeHistory(t *testing.T) {
	db, cv, ca, source, conversation := conversationFixture(t)
	if _, err := cv.transactConversationEnsure(context.Background(), db, ca, source); err != nil {
		t.Fatal(err)
	}
	v, a := currentMessageProof(t, "A")
	_, key := fixtureTrust()
	sendToken := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, a.claims))
	a, err := v.Verify(fixtureRequest(t, sendToken, "POST", MessageSendPath, string(a.body)), OperationMessageSend, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	h := NewPolicyHTTP(v, db)
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, fixtureRequest(t, sendToken, "POST", MessageSendPath, string(a.body)))
	var response struct {
		Code int `json:"code"`
		Data struct {
			Operation json.RawMessage `json:"operation"`
			Replayed  bool            `json:"replayed"`
		} `json:"data"`
	}
	if recorder.Code != 200 || json.Unmarshal(recorder.Body.Bytes(), &response) != nil || response.Code != 0 {
		t.Fatalf("HTTP send: %d %s", recorder.Code, recorder.Body.String())
	}
	first := &MutationResult{Result: response.Data.Operation, Replayed: response.Data.Replayed}
	native := MessageNativeBinding{ChannelType: 2, ConversationID: conversation.NativeTargetID, SenderPrincipalID: conversation.Members[0].PrincipalID, TargetPrincipalID: conversation.NativeTargetID}
	receipt, err := v.ValidateMessageSendResult(a, first.Result, native)
	if err != nil || first.Replayed {
		t.Fatalf("receipt: %v", err)
	}
	var payload []byte
	var seq, stamp int64
	if err = db.QueryRow(`SELECT msg_body,msg_seq_no,send_time FROM g_hismsgs WHERE app_key='fixture-app' AND msg_id=?`, receipt.MessageID).Scan(&payload, &seq, &stamp); err != nil {
		t.Fatal(err)
	}
	var down pbobjs.DownMsg
	if err = proto.Unmarshal(payload, &down); err != nil {
		t.Fatal(err)
	}
	var content struct {
		Content string `json:"content"`
	}
	if json.Unmarshal(down.MsgContent, &content) != nil || content.Content != "真实消息 <&> 😀" || down.MsgId != receipt.MessageID || down.MsgSeqNo != seq || down.MsgTime != stamp || down.SenderId != native.SenderPrincipalID || down.TargetId != native.TargetPrincipalID || down.Flags != 8 || down.MentionInfo == nil || len(down.MentionInfo.TargetUsers) != 1 || down.MentionInfo.TargetUsers[0].UserId != conversation.Members[1].PrincipalID {
		t.Fatal("native message lost body, identity, sequence or mention")
	}
	var queued []byte
	if err = db.QueryRow(`SELECT payload FROM mindlink_message_outbox WHERE outbox_id=?`, receipt.OutboxID).Scan(&queued); err != nil || !bytes.Equal(payload, queued) {
		t.Fatalf("outbox differs from committed native message: %v", err)
	}
	again, err := v.SendMessage(context.Background(), db, a)
	if err != nil || !again.Replayed || !bytes.Equal(first.Result, again.Result) {
		t.Fatalf("replay changed result: %v", err)
	}
	for _, table := range []string{"g_hismsgs", "mindlink_message_outbox"} {
		var n int
		if err = db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 1 {
			t.Fatalf("%s duplicated: %d %v", table, n, err)
		}
	}
	queryClaims := a.claims
	queryClaims.OperationType, queryClaims.Method, queryClaims.Path = OperationMessageQuery, "GET", MessageResultPathPrefix+queryClaims.RequestID
	queryClaims.BodySHA256 = fmt.Sprintf("%x", sha256.Sum256(nil))
	queryClaims.JTI = strings.Repeat("6", 64)
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, queryClaims))
	query, err := v.Verify(fixtureRequest(t, token, "GET", queryClaims.Path, ""), OperationMessageQuery, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := v.QueryMessageResult(context.Background(), db, query)
	if err != nil || !recovered.Replayed || !bytes.Equal(recovered.Result, first.Result) {
		t.Fatalf("original message recovery: %v", err)
	}
	recorder = httptest.NewRecorder()
	h.ServeHTTP(recorder, fixtureRequest(t, token, "GET", queryClaims.Path, ""))
	if recorder.Code != 200 || json.Unmarshal(recorder.Body.Bytes(), &response) != nil || response.Code != 0 || !response.Data.Replayed || !bytes.Equal(response.Data.Operation, first.Result) {
		t.Fatal("HTTP query changed original receipt")
	}
	for _, change := range []string{
		"UPDATE mindlink_principals SET policy_version=policy_version+1",
		"UPDATE mindlink_principals SET avatar_generation=avatar_generation+1",
		"UPDATE mindlink_principals SET state='disabled'; UPDATE mindlink_conversations SET state='disabled',policy_version=policy_version+1",
	} {
		if _, err = db.Exec(change); err != nil {
			t.Fatal(err)
		}
		recovered, err = v.QueryMessageResult(context.Background(), db, query)
		if err != nil || !recovered.Replayed || !bytes.Equal(recovered.Result, first.Result) {
			t.Fatalf("committed result lost after lifecycle change: %v", err)
		}
		var count int
		if err = db.QueryRow("SELECT COUNT(*) FROM g_hismsgs").Scan(&count); err != nil || count != 1 {
			t.Fatal("query duplicated native history", err)
		}
	}
	if _, err = db.Exec(`UPDATE mindlink_owner_barriers SET read_enabled=FALSE`); err != nil {
		t.Fatal(err)
	}
	if _, err = v.QueryMessageResult(context.Background(), db, query); !errors.Is(err, ErrForbidden) {
		t.Fatalf("revoked result read: %v", err)
	}
}

func TestMySQLReliableMessageRejectsAuthorityAndRollsBack(t *testing.T) {
	for _, kind := range []string{"owner_pair", "disabled_member", "stale_conversation", "outbox_failure", "receipt_failure"} {
		t.Run(kind, func(t *testing.T) {
			db, cv, ca, source, b := conversationFixture(t)
			if _, err := cv.transactConversationEnsure(context.Background(), db, ca, source); err != nil {
				t.Fatal(err)
			}
			actor := "A"
			if kind == "owner_pair" {
				actor = "owner"
			}
			v, a := currentMessageProof(t, actor)
			var err error
			switch kind {
			case "disabled_member":
				_, err = db.Exec(`UPDATE mindlink_principals SET state='disabled' WHERE principal_id=?`, b.Members[1].PrincipalID)
			case "stale_conversation":
				_, err = db.Exec(`UPDATE mindlink_conversations SET policy_version=2`)
			case "outbox_failure":
				_, err = db.Exec(`CREATE TRIGGER fail_message_outbox BEFORE INSERT ON mindlink_message_outbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='outbox failure'`)
			case "receipt_failure":
				_, err = db.Exec(`CREATE TRIGGER fail_message_receipt BEFORE UPDATE ON mindlink_operations FOR EACH ROW BEGIN IF NEW.result_json IS NOT NULL AND OLD.result_json IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='receipt failure'; END IF; END`)
			}
			if err != nil {
				t.Fatal(err)
			}
			want := ErrUnknownResult
			switch kind {
			case "owner_pair":
				want = ErrForbidden
			case "disabled_member":
				want = ErrPrincipalDisabled
			case "stale_conversation":
				want = ErrStalePolicy
			}
			if result, err := v.SendMessage(context.Background(), db, a); !errors.Is(err, want) || result != nil {
				t.Fatalf("wrong failure: %v, want %v", err, want)
			}
			for _, table := range []string{"g_hismsgs", "message_sequence", "mindlink_message_outbox"} {
				var n int
				if err = db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 0 {
					t.Fatalf("%s survived failed transaction: %d %v", table, n, err)
				}
			}
			var n int
			if err = db.QueryRow(`SELECT COUNT(*) FROM mindlink_operations WHERE operation_id=?`, a.claims.OperationID).Scan(&n); err != nil || n != 0 {
				t.Fatal("failed message left an original receipt")
			}
		})
	}
}

func TestMySQLReliableDMUsesNativeReceiverHistory(t *testing.T) {
	db, cv, ca, source, b := conversationFixture(t)
	if _, err := cv.transactConversationEnsure(context.Background(), db, ca, source); err != nil {
		t.Fatal(err)
	}
	// This fixture isolates the message producer; C1 DM provisioning is tested separately.
	roster := fixtureJSON(t, b.Members[1:])
	if _, err := db.Exec(`UPDATE mindlink_conversations SET kind='owner_avatar_dm',channel_type=1,native_target_id=?,members_json=?`, b.Members[1].PrincipalID, roster); err != nil {
		t.Fatal(err)
	}
	v, a := currentMessageProof(t, "B")
	first, err := v.SendMessage(context.Background(), db, a)
	if err != nil {
		t.Fatal(err)
	}
	native := MessageNativeBinding{ChannelType: 1, ConversationID: min(b.OwnerIMID, b.Members[1].PrincipalID) + ":" + max(b.OwnerIMID, b.Members[1].PrincipalID), SenderPrincipalID: b.Members[1].PrincipalID, TargetPrincipalID: b.OwnerIMID}
	receipt, err := v.ValidateMessageSendResult(a, first.Result, native)
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	var sender, receiver, conver string
	if err = db.QueryRow(`SELECT msg_body,sender_id,receiver_id,conver_id FROM p_hismsgs WHERE msg_id=?`, receipt.MessageID).Scan(&raw, &sender, &receiver, &conver); err != nil {
		t.Fatal(err)
	}
	var down pbobjs.DownMsg
	if proto.Unmarshal(raw, &down) != nil || sender != native.SenderPrincipalID || receiver != native.TargetPrincipalID || conver != native.ConversationID || down.TargetId != sender || down.IsSend || down.Flags != 10 {
		t.Fatal("private history must preserve native receiver-view payload")
	}
}

func changedMessageProof(t *testing.T, v *Verifier, a *Authorization, change func(*MessageSendRequest, *Claims)) *Authorization {
	t.Helper()
	var body MessageSendRequest
	if json.Unmarshal(a.body, &body) != nil {
		t.Fatal("fixture body")
	}
	c := a.claims
	change(&body, &c)
	var err error
	body.RequestDigest, err = MessageSendDigest(c, &body)
	if err != nil {
		t.Fatal(err)
	}
	c.RequestDigest = body.RequestDigest
	raw := fixtureJSON(t, body)
	c.BodySHA256 = fmt.Sprintf("%x", sha256.Sum256(raw))
	_, key := fixtureTrust()
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
	proof, err := v.Verify(fixtureRequest(t, token, "POST", MessageSendPath, string(raw)), OperationMessageSend, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return proof
}

func TestMySQLReliableReplyAndOriginalConflict(t *testing.T) {
	db, cv, ca, source, _ := conversationFixture(t)
	if _, err := cv.transactConversationEnsure(context.Background(), db, ca, source); err != nil {
		t.Fatal(err)
	}
	v, a := currentMessageProof(t, "A")
	first, err := v.SendMessage(context.Background(), db, a)
	if err != nil {
		t.Fatal(err)
	}
	var original MessageSendResult
	if json.Unmarshal(first.Result, &original) != nil {
		t.Fatal("original receipt")
	}
	conflict := changedMessageProof(t, v, a, func(b *MessageSendRequest, c *Claims) { b.Text = "changed"; c.JTI = strings.Repeat("2", 64) })
	if _, err = v.SendMessage(context.Background(), db, conflict); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("same-key body conflict: %v", err)
	}
	reply := changedMessageProof(t, v, a, func(b *MessageSendRequest, c *Claims) {
		b.OperationID = strings.Repeat("4", 64)
		b.RequestID = strings.Repeat("5", 64)
		c.OperationID = b.OperationID
		c.RequestID = b.RequestID
		c.JTI = strings.Repeat("3", 64)
		b.ReplyTo = &original.MessageID
	})
	second, err := v.SendMessage(context.Background(), db, reply)
	if err != nil {
		t.Fatal(err)
	}
	var result MessageSendResult
	if json.Unmarshal(second.Result, &result) != nil {
		t.Fatal("reply receipt")
	}
	var raw []byte
	if err = db.QueryRow(`SELECT msg_body FROM g_hismsgs WHERE msg_id=?`, result.MessageID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var down pbobjs.DownMsg
	if proto.Unmarshal(raw, &down) != nil || down.ReferMsg == nil || down.ReferMsg.MsgId != original.MessageID || down.ReferMsg.ReferMsg != nil {
		t.Fatal("reply lost its scoped native reference")
	}
	foreign := changedMessageProof(t, v, a, func(b *MessageSendRequest, c *Claims) {
		b.OperationID = strings.Repeat("1", 64)
		b.RequestID = strings.Repeat("2", 64)
		c.OperationID = b.OperationID
		c.RequestID = b.RequestID
		c.JTI = strings.Repeat("0", 64)
		b.ReplyTo = stringPointer("missing-message")
	})
	if _, err = v.SendMessage(context.Background(), db, foreign); !errors.Is(err, ErrNotReady) {
		t.Fatalf("missing reply reference accepted: %v", err)
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM g_hismsgs`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("reply failure left history: %d %v", count, err)
	}
}
