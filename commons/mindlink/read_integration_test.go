//go:build integration

package mindlink

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMySQLReadSnapshotAndFailedSettlement(t *testing.T) {
	db, cv, ca, source, conversation := conversationFixture(t)
	ctx := context.Background()
	if _, err := cv.transactConversationEnsure(ctx, db, ca, source); err != nil {
		t.Fatal(err)
	}
	trust, key := fixtureTrust()
	trust.QueryOperations = []string{OperationSeenQuery}
	trust.MutationOperations = []string{OperationSeenAdvance}
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	b := ReadBinding{AppKey: ca.claims.AppKey, RequesterID: conversation.OwnerIMID, TargetID: conversation.NativeTargetID, Kind: conversation.Kind, ChannelType: 2, Members: []string{conversation.OwnerIMID}}
	for _, m := range conversation.Members {
		b.Members = append(b.Members, m.PrincipalID)
	}
	sort.Strings(b.Members)
	proof := func(label string, position *int64) *Authorization {
		c := ca.claims
		c.OperationID = fmt.Sprintf("%x", sha256.Sum256([]byte(label)))
		c.RequestID = c.OperationID
		c.ActorKind, c.ActorID, c.AvatarGeneration = "owner", c.OwnerID, 0
		c.ExpectedPolicyVersion, c.DesiredPolicyVersion = 1, 1
		c.Method, c.OperationType = "GET", OperationSeenQuery
		if position != nil {
			c.Method, c.OperationType = "PUT", OperationSeenAdvance
		}
		c.Path = "/apigateway/mindlink/conversations/" + conversation.ConversationID + "/seen"
		c.TargetID = conversation.ConversationID
		c.RequestDigest = ReadDigest(c, b, position)
		var body []byte
		if position != nil {
			body = fixtureJSON(t, map[string]string{"operation_id": c.OperationID, "request_id": c.RequestID, "request_digest": c.RequestDigest, "seen_seq": strconv.FormatInt(*position, 10)})
		}
		c.BodySHA256 = fmt.Sprintf("%x", sha256.Sum256(body))
		c.IssuedAt = time.Now().Unix()
		c.ExpiresAt = c.IssuedAt + 60
		c.JTI = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprint(time.Now().UnixNano()))))
		token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
		a, e := v.Verify(fixtureRequest(t, token, c.Method, c.Path, string(body)), c.OperationType, time.Now())
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	// This adapter isolates journal guarantees; real native adapter HTTP/SQL is checked on Rocky separately.
	latest := int64(2)
	calls := 0
	native := func(ctx context.Context, tx *sql.Tx, b ReadBinding, position *int64) (ReadState, error) {
		calls++
		if position != nil {
			if _, e := tx.ExecContext(ctx, "INSERT INTO conversation_seen(app_key,user_id,target_id,channel_type,sub_channel,seen_seq) VALUES(?,?,?,2,'',?) ON DUPLICATE KEY UPDATE seen_seq=GREATEST(seen_seq,VALUES(seen_seq))", b.AppKey, b.RequesterID, b.TargetID, *position); e != nil {
				return ReadState{}, e
			}
		}
		var seen int64
		e := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(seen_seq),0) FROM conversation_seen WHERE app_key=? AND user_id=? AND target_id=?", b.AppKey, b.RequesterID, b.TargetID).Scan(&seen)
		return ReadState{strconv.FormatInt(seen, 10), strconv.FormatInt(latest, 10), latest > seen}, e
	}
	first, err := v.Read(ctx, db, proof("query", nil), native)
	if err != nil || first.Replayed {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM conversation_seen").Scan(&count); err != nil || count != 0 {
		t.Fatal("query wrote seen", err)
	}
	latest = 3
	repeated, err := v.Read(ctx, db, proof("query", nil), native)
	if err != nil || !repeated.Replayed || string(repeated.Result) != string(first.Result) || calls != 1 {
		t.Fatal("query not immutable", err)
	}
	position := int64(2)
	if _, err = db.Exec("CREATE TRIGGER reject_read BEFORE UPDATE ON mindlink_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned failure'"); err != nil {
		t.Fatal(err)
	}
	if _, err = v.Read(ctx, db, proof("advance", &position), native); err == nil {
		t.Fatal("settlement failure accepted")
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM conversation_seen").Scan(&count); err != nil || count != 0 {
		t.Fatal("seen escaped rollback", err)
	}
	if _, err = db.Exec("DROP TRIGGER reject_read"); err != nil {
		t.Fatal(err)
	}
	advanced, err := v.Read(ctx, db, proof("advance", &position), native)
	if err != nil {
		t.Fatal(err)
	}
	latest = 4
	again, err := v.Read(ctx, db, proof("advance", &position), native)
	if err != nil || !again.Replayed {
		t.Fatal(err)
	}
	var original, current ReadResult
	json.Unmarshal(advanced.Result, &original)
	json.Unmarshal(again.Result, &current)
	if current.State.LatestSeq != "4" || *current.Operation.CommitVersion != *original.Operation.CommitVersion {
		t.Fatal("stale state or changed receipt")
	}
	if _, err = db.Exec("UPDATE mindlink_owner_barriers SET read_enabled=0"); err != nil {
		t.Fatal(err)
	}
	if _, err = v.Read(ctx, db, proof("query", nil), native); err == nil {
		t.Fatal("revoked query replay")
	}
	if strings.Contains(string(first.Result), "seen_seq\":2") {
		t.Fatal("numeric JSON position")
	}
}
