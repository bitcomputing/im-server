//go:build integration

package mindlink

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"im-server/commons/pbdefines/pbobjs"
)

func TestMySQLMediaNativeHistoryPreservesAssetsReferenceAndOriginalResult(t *testing.T) {
	db, cv, ca, source, _ := conversationFixture(t)
	ctx := context.Background()
	if _, err := cv.transactConversationEnsure(ctx, db, ca, source); err != nil {
		t.Fatal(err)
	}
	v, a := currentMessageProof(t, "A")
	original, err := v.SendMessage(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	var first MessageSendResult
	if json.Unmarshal(original.Result, &first) != nil {
		t.Fatal("invalid original receipt")
	}
	var body MessageSendRequest
	if json.Unmarshal(a.body, &body) != nil {
		t.Fatal("invalid fixture")
	}
	body.Version, body.MessageType, body.Text = "mindlink-reliable-v2", "jg:image", ""
	body.AssetIDs = []string{"up_a"}
	body.Assets = []MessageAsset{{FileID: "up_a", Name: "photo.png", MIMEType: "image/png", Size: 20, ExpiresAt: time.Now().Add(time.Hour).Unix()}}
	body.ReplyTo = &first.MessageID
	body.OperationID, body.RequestID = strings.Repeat("6", 64), strings.Repeat("5", 64)
	claims := a.claims
	claims.OperationID, claims.RequestID, claims.JTI = body.OperationID, body.RequestID, strings.Repeat("4", 64)
	body.RequestDigest, err = MessageSendDigest(claims, &body)
	if err != nil {
		t.Fatal(err)
	}
	raw := fixtureJSON(t, body)
	var fields map[string]any
	if json.Unmarshal(fixtureJSON(t, claims), &fields) != nil {
		t.Fatal("invalid claims")
	}
	fields["request_digest"], fields["body_sha256"] = body.RequestDigest, fmt.Sprintf("%x", sha256.Sum256(raw))
	_, key := fixtureTrust()
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, fields))
	mediaAuth, err := v.Verify(fixtureRequest(t, token, "POST", MessageSendPath, string(raw)), OperationMessageSend, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	result, err := v.SendMessage(ctx, db, mediaAuth)
	if err != nil {
		t.Fatal(err)
	}
	if replay, err := v.SendMessage(ctx, db, mediaAuth); err != nil || string(replay.Result) != string(result.Result) {
		t.Fatal("media retry changed original result", err)
	}
	var receipt MessageSendResult
	if json.Unmarshal(result.Result, &receipt) != nil {
		t.Fatal("invalid media receipt")
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM g_hismsgs WHERE msg_id=?", receipt.MessageID).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate native message", count, err)
	}
	var stored []byte
	if err = db.QueryRow("SELECT msg_body FROM g_hismsgs WHERE msg_id=?", receipt.MessageID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	var down pbobjs.DownMsg
	if proto.Unmarshal(stored, &down) != nil {
		t.Fatal("invalid native protobuf")
	}
	m, err := historyContent(&down)
	if err != nil || m.ContentType != "image" || m.ReplyTo != first.MessageID || len(m.Assets) != 1 || m.Assets[0] != body.Assets[0] {
		t.Fatal("native media/reference lost", m, err)
	}
}
