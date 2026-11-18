//go:build integration

package mindlink

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMySQLOwnerHistoryNativePageAndRetirement(t *testing.T) {
	db, cv, ca, source, _ := conversationFixture(t)
	ctx := context.Background()
	if _, err := cv.transactConversationEnsure(ctx, db, ca, source); err != nil {
		t.Fatal(err)
	}
	v, a := currentMessageProof(t, "A")
	sent, err := v.SendMessage(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	var receipt MessageSendResult
	if err = json.Unmarshal(sent.Result, &receipt); err != nil {
		t.Fatal(err)
	}
	trust, key := fixtureTrust()
	trust.QueryOperations = []string{OperationHistoryRead}
	hv, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	read := func(owner, cursor string) *httptest.ResponseRecorder {
		fields := fixtureClaims("")
		path := HistoryPathPrefix + strings.Repeat("c", 64) + "/" + cursor + "/1"
		fields["operation_type"], fields["method"], fields["path"] = OperationHistoryRead, "GET", path
		fields["owner_id"], fields["actor_id"], fields["target_id"] = owner, owner, strings.Repeat("c", 64)
		fields["expected_policy_version"], fields["desired_policy_version"] = int64(1), int64(1)
		fields["request_digest"] = fmt.Sprintf("%x", sha256.Sum256([]byte(path)))
		fields["iat"], fields["exp"] = time.Now().Unix(), time.Now().Unix()+60
		token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, fields))
		out := httptest.NewRecorder()
		NewPolicyHTTP(hv, db).ServeHTTP(out, fixtureRequest(t, token, "GET", path, ""))
		return out
	}
	parse := func(out *httptest.ResponseRecorder) HistoryResult {
		t.Helper()
		var envelope struct {
			Code int `json:"code"`
			Data struct {
				Operation HistoryResult `json:"operation"`
			} `json:"data"`
		}
		if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &envelope) != nil || envelope.Code != 0 {
			t.Fatalf("history rejected: %d %s", out.Code, out.Body.String())
		}
		return envelope.Data.Operation
	}
	var seenBefore, operationsBefore int
	db.QueryRow("SELECT COUNT(*) FROM conversation_seen").Scan(&seenBefore)
	db.QueryRow("SELECT COUNT(*) FROM mindlink_operations").Scan(&operationsBefore)
	page := parse(read("42", "0"))
	if len(page.Messages) != 1 || page.Messages[0].ID != receipt.MessageID || page.Messages[0].Text != "真实消息 <&> 😀" || page.Messages[0].Sequence != receipt.MsgSeqNo || page.NextCursor != nil {
		t.Fatalf("lost native history: %+v", page)
	}
	seq, _ := strconv.ParseInt(receipt.MsgSeqNo, 10, 64)
	if len(parse(read("42", strconv.FormatInt(seq, 10))).Messages) != 0 {
		t.Fatal("cursor repeated boundary")
	}
	if _, err = db.Exec("UPDATE g_hismsgs SET destroy_time=1 WHERE msg_id=?", receipt.MessageID); err != nil {
		t.Fatal(err)
	}
	if len(parse(read("42", "0")).Messages) != 0 {
		t.Fatal("expired message exposed")
	}
	if _, err = db.Exec("UPDATE g_hismsgs SET destroy_time=0,is_portion=1 WHERE msg_id=?", receipt.MessageID); err != nil {
		t.Fatal(err)
	}
	if len(parse(read("42", "0")).Messages) != 0 {
		t.Fatal("private portion exposed")
	}
	if _, err = db.Exec("UPDATE g_hismsgs SET is_portion=0 WHERE msg_id=?", receipt.MessageID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO g_delhismsgs(app_key,user_id,target_id,msg_id) VALUES(?,?,?,?)", a.claims.AppKey, page.RequesterID, page.TargetID, receipt.MessageID); err != nil {
		t.Fatal(err)
	}
	if len(parse(read("42", "0")).Messages) != 0 {
		t.Fatal("owner-deleted message exposed")
	}
	if _, err = db.Exec("DELETE FROM g_delhismsgs WHERE msg_id=?", receipt.MessageID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO usercleantimes(app_key,user_id,target_id,channel_type,clean_time) VALUES(?,?,?,?,?)", a.claims.AppKey, page.RequesterID, page.TargetID, 2, receipt.MsgTime); err != nil {
		t.Fatal(err)
	}
	if len(parse(read("42", "0")).Messages) != 0 {
		t.Fatal("owner-cleared message exposed")
	}
	if _, err = db.Exec("DELETE FROM usercleantimes WHERE user_id=?", page.RequesterID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE mindlink_conversations SET state='disabled' WHERE conversation_id=?", a.claims.TargetID); err != nil {
		t.Fatal(err)
	}
	if len(parse(read("42", "0")).Messages) != 1 {
		t.Fatal("retirement erased history")
	}
	if out := read("43", "0"); out.Code == 200 {
		t.Fatal("another owner read history")
	}
	if _, err = db.Exec("UPDATE mindlink_owner_barriers SET read_enabled=0 WHERE owner_id='42'"); err != nil {
		t.Fatal(err)
	}
	if out := read("42", "0"); out.Code == 200 {
		t.Fatal("revoked owner read history")
	}
	var seenAfter, operationsAfter int
	if err = db.QueryRow("SELECT COUNT(*) FROM conversation_seen").Scan(&seenAfter); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM mindlink_operations").Scan(&operationsAfter); err != nil {
		t.Fatal(err)
	}
	if seenAfter != seenBefore || operationsAfter != operationsBefore {
		t.Fatal("GET mutated read state or journal")
	}
}
