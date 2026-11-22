//go:build integration

package mindlink

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"im-server/commons/pbdefines/pbobjs"
)

func TestMySQLOwnerInboxAtomicAndIdempotent(t *testing.T) {
	for _, kind := range []string{"avatar_pair_group", "mindlink_group", "owner_avatar_dm", "owner_send"} {
		t.Run(kind, func(t *testing.T) {
			db, cv, ca, source, b := conversationFixture(t)
			ctx := context.Background()
			if _, err := cv.transactConversationEnsure(ctx, db, ca, source); err != nil {
				t.Fatal(err)
			}
			target, channel := b.NativeTargetID, 2
			if kind == "owner_avatar_dm" || kind == "owner_send" {
				target, channel = b.Members[1].PrincipalID, 1
				if _, err := db.Exec(`UPDATE mindlink_conversations SET members_json=?`, fixtureJSON(t, b.Members[1:])); err != nil {
					t.Fatal(err)
				}
			}
			storedKind, actor, box := kind, "B", "inboxmsgs"
			if kind == "owner_send" {
				storedKind, actor, box = "owner_avatar_dm", "owner", "sendboxmsgs"
			}
			if _, err := db.Exec(`UPDATE mindlink_conversations SET kind=?,native_target_id=?,channel_type=?`, storedKind, target, channel); err != nil {
				t.Fatal(err)
			}
			v, a := currentMessageProof(t, actor)
			result, err := v.SendMessage(ctx, db, a)
			if err != nil {
				t.Fatal(err)
			}
			var receipt MessageSendResult
			if err = json.Unmarshal(result.Result, &receipt); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`CREATE TRIGGER inbox_failure BEFORE INSERT ON ` + box + ` FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='inbox fixture'`); err != nil {
				t.Fatal(err)
			}
			if stored, err := v.MaterializeOwnerInbox(ctx, db, receipt.OutboxID); err == nil || stored {
				t.Fatal("inbox failure claimed success")
			}
			var count int
			if err = db.QueryRow(`SELECT COUNT(*) FROM conversations`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("conversation survived rollback: %d %v", count, err)
			}
			if _, err = db.Exec(`DROP TRIGGER inbox_failure`); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				_, stored, err := NewPolicyHTTP(v, db).DrainOwnerInbox(ctx, "")
				want := 0
				if i == 0 {
					want = 1
				}
				if err != nil || stored != want {
					t.Fatalf("attempt %d stored=%v err=%v", i, stored, err)
				}
			}
			var raw []byte
			h := NewPolicyHTTP(v, db)
			calls := 0
			failedSignal := func(context.Context, string, string, int64) error { calls++; return ErrNotReady }
			if err := h.SignalOwnerInbox(ctx, receipt.OutboxID, failedSignal); err == nil || calls != 1 {
				t.Fatal("failed route marked successful", err, calls)
			}
			var phase string
			if err := db.QueryRow(`SELECT state FROM mindlink_message_outbox WHERE outbox_id=?`, receipt.OutboxID).Scan(&phase); err != nil || phase != "inboxed" {
				t.Fatal("failed route lost pending hint", err, phase)
			}
			if _, err := db.Exec(`UPDATE mindlink_owner_barriers SET read_enabled=FALSE`); err != nil {
				t.Fatal(err)
			}
			calls = 0
			if err := h.SignalOwnerInbox(ctx, receipt.OutboxID, failedSignal); err == nil || calls != 0 {
				t.Fatal("revoked notification escaped", err, calls)
			}
			if _, err := db.Exec(`UPDATE mindlink_owner_barriers SET read_enabled=TRUE`); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := h.SignalOwnerInbox(ctx, receipt.OutboxID, func(_ context.Context, app, owner string, syncTime int64) error {
					calls++
					var storedTime int64
					if err := db.QueryRow(`SELECT MAX(send_time) FROM ` + box).Scan(&storedTime); err != nil {
						t.Fatal(err)
					}
					if app != "fixture-app" || owner != b.OwnerIMID || syncTime <= 0 || syncTime != storedTime {
						t.Fatal("notification target mismatch")
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			if calls != 1 {
				t.Fatal("missing or duplicate synchronization hint", calls)
			}
			var inboxTarget, recipient string
			if err = db.QueryRow(`SELECT msg_body,target_id,user_id FROM `+box).Scan(&raw, &inboxTarget, &recipient); err != nil {
				t.Fatal(err)
			}
			var down pbobjs.DownMsg
			if err = proto.Unmarshal(raw, &down); err != nil {
				t.Fatal(err)
			}
			fence, err := NewNativeFence(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			if ok, err := fence.OwnerInboxReadable(ctx, "fixture-app", b.OwnerIMID, &down); err != nil || !ok {
				t.Fatal("owner cannot read committed inbox", err)
			}
			if ok, err := fence.OwnerInboxReadable(ctx, "fixture-app", "another-owner", &down); err != nil || ok {
				t.Fatal("foreign owner inbox", err)
			}
			forged := proto.Clone(&down).(*pbobjs.DownMsg)
			forged.MsgContent = []byte("forged")
			if ok, err := fence.OwnerInboxReadable(ctx, "fixture-app", b.OwnerIMID, forged); err != nil || ok {
				t.Fatal("forged payload accepted", err)
			}
			if _, err = db.Exec(`UPDATE mindlink_owner_barriers SET read_enabled=FALSE`); err != nil {
				t.Fatal(err)
			}
			if ok, err := fence.OwnerInboxReadable(ctx, "fixture-app", b.OwnerIMID, &down); err != nil || ok {
				t.Fatal("revoked read accepted", err)
			}
			wantUnread := int64(1)
			if kind == "avatar_pair_group" || kind == "owner_send" {
				wantUnread = 0
			}
			if down.MsgId != receipt.MessageID || down.TargetId != target || inboxTarget != target || recipient != b.OwnerIMID || down.UnreadIndex != wantUnread || down.IsSend != (kind == "owner_send") || kind == "avatar_pair_group" && down.Flags != 8 {
				t.Fatalf("owner inbox binding: %+v", &down)
			}
			var unread int64
			if err = db.QueryRow(`SELECT latest_unread_msg_index FROM conversations WHERE user_id=? AND target_id=?`, b.OwnerIMID, target).Scan(&unread); err != nil || unread != wantUnread {
				t.Fatalf("unread=%d %v", unread, err)
			}
			if err = db.QueryRow(`SELECT COUNT(*) FROM ` + box).Scan(&count); err != nil || count != 1 {
				t.Fatalf("duplicate inbox: %d %v", count, err)
			}
		})
	}
}

func TestMySQLQuietPairRecoversOnlySynchronization(t *testing.T) {
	db, cv, ca, source, b := conversationFixture(t)
	ctx := context.Background()
	if _, err := cv.transactConversationEnsure(ctx, db, ca, source); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE mindlink_conversations SET kind='avatar_pair_group'`); err != nil {
		t.Fatal(err)
	}
	v, a := currentMessageProof(t, "B")
	result, err := v.SendMessage(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	var receipt MessageSendResult
	if err := json.Unmarshal(result.Result, &receipt); err != nil {
		t.Fatal(err)
	}
	if stored, err := v.MaterializeOwnerInbox(ctx, db, receipt.OutboxID); err != nil || !stored {
		t.Fatal("materialize fixture", stored, err)
	}
	if _, err := db.Exec(`UPDATE mindlink_message_outbox SET state='quiet' WHERE outbox_id=?`, receipt.OutboxID); err != nil {
		t.Fatal(err)
	}
	h, calls := NewPolicyHTTP(v, db), 0
	for range 2 {
		if _, err := h.DrainOwnerNotifications(ctx, "", func(_ context.Context, app, owner string, syncTime int64) error {
			calls++
			if app != "fixture-app" || owner != b.OwnerIMID || syncTime <= 0 {
				t.Fatal("wrong recovery target")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	var state string
	var count, unread int
	if err := db.QueryRow(`SELECT state FROM mindlink_message_outbox WHERE outbox_id=?`, receipt.OutboxID).Scan(&state); err != nil || state != "signalled" || calls != 1 {
		t.Fatal("quiet synchronization not recovered exactly once", state, calls, err)
	}
	if stored, err := v.MaterializeOwnerInbox(ctx, db, receipt.OutboxID); err != nil || stored {
		t.Fatal("recovery replayed inbox", stored, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM inboxmsgs`).Scan(&count); err != nil || count != 1 {
		t.Fatal("recovery duplicated message", count, err)
	}
	if err := db.QueryRow(`SELECT latest_unread_msg_index FROM conversations`).Scan(&unread); err != nil || unread != 0 {
		t.Fatal("quiet recovery counted unread", unread, err)
	}
}

func TestMySQLOwnerInboxPreservesConversationOrder(t *testing.T) {
	db, cv, ca, source, _ := conversationFixture(t)
	ctx := context.Background()
	if _, err := cv.transactConversationEnsure(ctx, db, ca, source); err != nil {
		t.Fatal(err)
	}
	v, a := currentMessageProof(t, "A")
	first, err := v.SendMessage(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	b := changedMessageProof(t, v, a, func(body *MessageSendRequest, c *Claims) {
		body.OperationID = strings.Repeat("3", 64)
		body.RequestID = strings.Repeat("4", 64)
		body.Text = "second"
		c.OperationID = body.OperationID
		c.RequestID = body.RequestID
		c.JTI = strings.Repeat("2", 64)
	})
	second, err := v.SendMessage(ctx, db, b)
	if err != nil {
		t.Fatal(err)
	}
	var one, two MessageSendResult
	if json.Unmarshal(first.Result, &one) != nil || json.Unmarshal(second.Result, &two) != nil {
		t.Fatal("receipts")
	}
	if ok, err := v.MaterializeOwnerInbox(ctx, db, two.OutboxID); err == nil || ok {
		t.Fatal("later message passed pending predecessor")
	}
	for _, id := range []string{one.OutboxID, two.OutboxID} {
		if ok, err := v.MaterializeOwnerInbox(ctx, db, id); err != nil || !ok {
			t.Fatal(err)
		}
	}
	var latest string
	if err = db.QueryRow(`SELECT latest_msg_id FROM conversations`).Scan(&latest); err != nil || latest != two.MessageID {
		t.Fatal("summary order", err)
	}
}

func TestMySQLOwnerInboxConcurrentRecoveryAndTampering(t *testing.T) {
	for _, mutation := range []string{"concurrent", "payload", "recipient", "scope", "receipt"} {
		t.Run(mutation, func(t *testing.T) {
			db, cv, ca, source, _ := conversationFixture(t)
			ctx := context.Background()
			if _, err := cv.transactConversationEnsure(ctx, db, ca, source); err != nil {
				t.Fatal(err)
			}
			v, a := currentMessageProof(t, "A")
			result, err := v.SendMessage(ctx, db, a)
			if err != nil {
				t.Fatal(err)
			}
			var receipt MessageSendResult
			if err = json.Unmarshal(result.Result, &receipt); err != nil {
				t.Fatal(err)
			}
			changes := map[string]string{
				"payload":   `UPDATE mindlink_message_outbox SET payload='bad'`,
				"recipient": `UPDATE mindlink_message_outbox SET recipients_json='["someone-else"]'`,
				"scope":     `UPDATE mindlink_conversations SET owner_id='another-owner'`,
				"receipt":   `UPDATE mindlink_operations SET result_json='{}' WHERE operation_id=REPEAT('9',64)`,
			}
			if mutation != "concurrent" {
				if _, err = db.Exec(changes[mutation]); err != nil {
					t.Fatal(err)
				}
				if ok, err := v.MaterializeOwnerInbox(ctx, db, receipt.OutboxID); err == nil || ok {
					t.Fatal("corrupt projection accepted")
				}
			} else {
				var wg sync.WaitGroup
				for i := 0; i < 8; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						if _, err := v.MaterializeOwnerInbox(ctx, db, receipt.OutboxID); err != nil {
							t.Error(err)
						}
					}()
				}
				wg.Wait()
			}
			var count int
			want := 0
			if mutation == "concurrent" {
				want = 1
			}
			if err = db.QueryRow(`SELECT COUNT(*) FROM inboxmsgs`).Scan(&count); err != nil || count != want {
				t.Fatalf("inbox count=%d want=%d err=%v", count, want, err)
			}
		})
	}
}

func TestMySQLOwnerInboxRechecksReadBarrier(t *testing.T) {
	db, cv, ca, source, _ := conversationFixture(t)
	ctx := context.Background()
	if _, err := cv.transactConversationEnsure(ctx, db, ca, source); err != nil {
		t.Fatal(err)
	}
	v, a := currentMessageProof(t, "A")
	result, err := v.SendMessage(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	var receipt MessageSendResult
	if err = json.Unmarshal(result.Result, &receipt); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE mindlink_owner_barriers SET read_enabled=FALSE`); err != nil {
		t.Fatal(err)
	}
	if ok, err := v.MaterializeOwnerInbox(ctx, db, receipt.OutboxID); err == nil || ok {
		t.Fatal("revoked owner received inbox")
	}
	var n int
	if err = db.QueryRow(`SELECT COUNT(*) FROM inboxmsgs`).Scan(&n); err != nil || n != 0 {
		t.Fatal("revoked inbox side effect", err)
	}
	if _, err = db.Exec(`UPDATE mindlink_owner_barriers SET read_enabled=TRUE; UPDATE mindlink_conversations SET state='disabled',policy_version=2`); err != nil {
		t.Fatal(err)
	}
	if ok, err := v.MaterializeOwnerInbox(ctx, db, receipt.OutboxID); err != nil || !ok {
		t.Fatal("committed retired conversation lost owner delivery", err)
	}
}
