//go:build integration

package mindlink

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"im-server/commons/pbdefines/pbobjs"
)

func TestMySQLReliableMessageQuoteAndMentions(t *testing.T) {
	for _, mode := range []string{"same_conversation", "read_revoked", "history_unavailable", "other_conversation"} {
		t.Run(mode, func(t *testing.T) {
			db := reliableDB(t)
			binding := reliableProjection(t, db, "avatar_owner_dm")
			store, err := NewMessageStore(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			v, auth := reliableProof(t, messageRequest(), "owner", 0)
			first, err := store.Send(context.Background(), v, auth)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := v.ValidateMessageSendResult(auth, first.Result, binding)
			if err != nil {
				t.Fatal(err)
			}
			request := messageRequest()
			request.OperationID, request.RequestID = strings.Repeat("e", 64), strings.Repeat("f", 64)
			request.Text, request.ReplyTo, request.Mentions = "next", &receipt.MessageID, []string{"avatar-A"}
			want := error(nil)
			switch mode {
			case "read_revoked":
				if _, err := db.Exec("UPDATE mindlink_owner_barriers SET read_enabled=0"); err != nil {
					t.Fatal(err)
				}
				want = ErrForbidden
			case "history_unavailable":
				if _, err := db.Exec("RENAME TABLE p_hismsgs TO _reliable_p_hismsgs"); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if _, err := db.Exec("RENAME TABLE _reliable_p_hismsgs TO p_hismsgs"); err != nil {
						t.Error(err)
					}
				}()
				want = ErrUnknownResult
			case "other_conversation":
				if _, err := db.Exec("UPDATE p_hismsgs SET conver_id='other'"); err != nil {
					t.Fatal(err)
				}
				want = ErrForbidden
			}
			v, auth = reliableProof(t, request, "owner", 0)
			result, err := store.Send(context.Background(), v, auth)
			if want != nil {
				if result != nil || !errors.Is(err, want) {
					t.Fatalf("quote result: %v, want %v", err, want)
				}
				if reliableRows(t, db, "mindlink_message_outbox") != 1 || reliableRows(t, db, "mindlink_operations") != 1 {
					t.Fatal("rejected quote committed")
				}
				return
			}
			if err != nil || result == nil {
				t.Fatalf("quote send: %v", err)
			}
			var raw []byte
			if err := db.QueryRow("SELECT msg_body FROM p_hismsgs WHERE msg_seq_no=2").Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var message pbobjs.DownMsg
			if proto.Unmarshal(raw, &message) != nil || message.ReferMsg == nil || message.ReferMsg.MsgId != receipt.MessageID || message.ReferMsg.MsgSeqNo != 1 || message.MentionInfo == nil || message.MentionInfo.MentionType != pbobjs.MentionType_Someone || len(message.MentionInfo.TargetUsers) != 1 || message.MentionInfo.TargetUsers[0].UserId != disablePrincipal {
				t.Fatal("native quote/mention binding lost")
			}
		})
	}
}

func TestMySQLReliableMessagePairAvatar(t *testing.T) {
	db := reliableDB(t)
	reliableProjection(t, db, "avatar_pair_group")
	store, err := NewMessageStore(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	v, auth := reliableProof(t, messageRequest(), "avatar-A", 3)
	if result, err := store.Send(context.Background(), v, auth); err != nil || result == nil {
		t.Fatalf("pair avatar send: %v", err)
	}
	var raw []byte
	if err := db.QueryRow("SELECT msg_body FROM g_hismsgs").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var message pbobjs.DownMsg
	if proto.Unmarshal(raw, &message) != nil || message.SenderId != disablePrincipal || message.MemberCount != 2 {
		t.Fatal("pair native membership lost")
	}
}

func TestMySQLReliableMessageRejectsHiddenQuotes(t *testing.T) {
	for _, tc := range []struct{ name, kind, change string }{
		{"expired", "avatar_owner_dm", "UPDATE p_hismsgs SET destroy_time=1"},
		{"ephemeral", "avatar_owner_dm", "UPDATE p_hismsgs SET life_time_after_read=60000"},
		{"private_personal_delete", "avatar_owner_dm", "INSERT INTO p_delhismsgs(app_key,user_id,target_id,msg_id) SELECT app_key,'43',receiver_id,msg_id FROM p_hismsgs"},
		{"conversation_cleared", "avatar_owner_dm", "INSERT INTO convercleantimes(app_key,conver_id,channel_type,sub_channel,clean_time) SELECT app_key,conver_id,1,'',send_time FROM p_hismsgs"},
		{"user_cleared", "avatar_owner_dm", "INSERT INTO usercleantimes(app_key,user_id,target_id,channel_type,sub_channel,clean_time) SELECT app_key,'43',receiver_id,1,'',send_time FROM p_hismsgs"},
		{"group_personal_delete", "mindlink_group", "INSERT INTO g_delhismsgs(app_key,user_id,target_id,msg_id) SELECT app_key,'43',conver_id,msg_id FROM g_hismsgs"},
		{"portion", "mindlink_group", "UPDATE g_hismsgs SET is_portion=1"},
		{"pre_join", "mindlink_group", "INSERT INTO groupinfoexts(app_key,group_id,item_type,item_key,item_value) SELECT app_key,group_id,1,'hide_grp_msg','true' FROM groupinfos; UPDATE groupmembers SET created_time=FROM_UNIXTIME(2000000000)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := reliableDB(t)
			binding := reliableProjection(t, db, tc.kind)
			store, err := NewMessageStore(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			v, auth := reliableProof(t, messageRequest(), "owner", 0)
			first, err := store.Send(context.Background(), v, auth)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := v.ValidateMessageSendResult(auth, first.Result, binding)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.change); err != nil {
				t.Fatal(err)
			}
			request := messageRequest()
			request.OperationID, request.RequestID = strings.Repeat("e", 64), strings.Repeat("f", 64)
			request.ReplyTo = &receipt.MessageID
			v, auth = reliableProof(t, request, "owner", 0)
			if got, err := store.Send(context.Background(), v, auth); got != nil || !errors.Is(err, ErrForbidden) {
				t.Fatalf("hidden quote copied: %v", err)
			}
			if reliableRows(t, db, "mindlink_message_outbox") != 1 || reliableRows(t, db, "mindlink_operations") != 1 {
				t.Fatal("hidden quote committed")
			}
		})
	}
}

func TestMySQLReliableMessageInsertRace(t *testing.T) {
	for _, mode := range []string{"roster", "ban"} {
		t.Run(mode, func(t *testing.T) {
			db := reliableDB(t)
			kind, table, changed := "mindlink_group", "g_hismsgs", "groupmembers"
			if mode == "ban" {
				kind, table, changed = "avatar_owner_dm", "p_hismsgs", "banusers"
			}
			binding := reliableProjection(t, db, kind)
			store, err := NewMessageStore(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			gate := "mindlink-reliable-3f7-insert-" + mode
			var acquired int
			if conn.QueryRowContext(ctx, "SELECT GET_LOCK(?,0)", gate).Scan(&acquired) != nil || acquired != 1 {
				t.Fatal("owned gate unavailable")
			}
			defer conn.ExecContext(context.Background(), "DO RELEASE_LOCK(?)", gate)
			trigger := "_reliable_insert_pause"
			if _, err := db.Exec("CREATE TRIGGER " + trigger + " BEFORE INSERT ON " + table + " FOR EACH ROW BEGIN DECLARE acquired INT; SET acquired=GET_LOCK('" + gate + "',10); IF acquired IS NULL OR acquired<>1 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned fixture gate failed'; END IF; SET acquired=RELEASE_LOCK('" + gate + "'); END"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := db.Exec("DROP TRIGGER IF EXISTS " + trigger); err != nil {
					t.Error(err)
				}
			})
			v, auth := reliableProof(t, messageRequest(), "owner", 0)
			sent := make(chan error, 1)
			go func() { _, e := store.Send(ctx, v, auth); sent <- e }()
			reliableWait(t, db, "SELECT COUNT(*) FROM performance_schema.metadata_locks WHERE OBJECT_TYPE='USER LEVEL LOCK' AND OBJECT_NAME=? AND LOCK_STATUS='PENDING'", gate)
			inserted := make(chan error, 1)
			go func() {
				var e error
				if mode == "roster" {
					_, e = db.ExecContext(ctx, "INSERT INTO groupmembers(app_key,group_id,member_id) VALUES('fixture-app',?,'unexpected')", binding.ConversationID)
				} else {
					_, e = db.ExecContext(ctx, "INSERT INTO banusers(app_key,user_id,scope_key,end_time) VALUES('fixture-app','43','default',0)")
				}
				inserted <- e
			}()
			blocked := false
			for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
				select {
				case err := <-inserted:
					if err != nil {
						t.Fatal(err)
					}
					goto release
				default:
				}
				var n int
				if err := db.QueryRow("SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks l ON l.ENGINE=w.ENGINE AND l.ENGINE_LOCK_ID=w.REQUESTING_ENGINE_LOCK_ID WHERE l.OBJECT_SCHEMA=DATABASE() AND l.OBJECT_NAME=?", changed).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n > 0 {
					blocked = true
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
		release:
			if _, err := conn.ExecContext(ctx, "DO RELEASE_LOCK(?)", gate); err != nil {
				t.Fatal(err)
			}
			if err := <-sent; err != nil {
				t.Fatal(err)
			}
			if !blocked {
				t.Fatal(fmt.Sprintf("%s insert committed inside the checked send authority window", mode))
			}
			if err := <-inserted; err != nil {
				t.Fatal(err)
			}
			if reliableRows(t, db, "mindlink_message_outbox") != 1 {
				t.Fatal("insert race lost original send")
			}
		})
	}
}

func TestMySQLReliableMessageNativeRules(t *testing.T) {
	for _, tc := range []struct {
		name, kind, change string
		want               error
	}{
		{"recipient_block", "avatar_owner_dm", "INSERT INTO blocks(app_key,user_id,block_user_id) VALUES('fixture-app','" + disablePrincipal + "','43')", ErrForbidden},
		{"friend_required", "avatar_owner_dm", "INSERT INTO appexts(app_key,app_item_key,app_item_value) VALUES('fixture-app','msg_friend_check','true')", ErrForbidden},
		{"friend_allowed", "avatar_owner_dm", "INSERT INTO appexts(app_key,app_item_key,app_item_value) VALUES('fixture-app','msg_friend_check','true'); INSERT INTO friendrels(app_key,user_id,friend_id) VALUES('fixture-app','" + disablePrincipal + "','43')", nil},
		{"mute_expired", "mindlink_group", "UPDATE groupmembers SET is_mute=1,mute_end_at=1 WHERE member_id='43'", nil},
		{"mute_allowlist", "mindlink_group", "UPDATE groupinfos SET is_mute=1; UPDATE groupmembers SET is_allow=1 WHERE member_id='43'", nil},
		{"app_missing", "avatar_owner_dm", "DELETE FROM apps", ErrNotReady},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := reliableDB(t)
			reliableProjection(t, db, tc.kind)
			store, err := NewMessageStore(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.change); err != nil {
				t.Fatal(err)
			}
			v, auth := reliableProof(t, messageRequest(), "owner", 0)
			got, err := store.Send(context.Background(), v, auth)
			if tc.want == nil {
				if err != nil || got == nil {
					t.Fatalf("native allowance rejected: %v", err)
				}
			} else {
				if got != nil || !errors.Is(err, tc.want) {
					t.Fatalf("native rule: %v, want %v", err, tc.want)
				}
				if reliableRows(t, db, "mindlink_message_outbox") != 0 || reliableRows(t, db, "mindlink_operations") != 0 {
					t.Fatal("native rejection committed")
				}
			}
		})
	}
}

func TestMySQLReliableMessageAvatarReadMembership(t *testing.T) {
	db := reliableDB(t)
	reliableProjection(t, db, "mindlink_group")
	store, err := NewMessageStore(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	v, auth := reliableProof(t, messageRequest(), "owner", 0)
	first, err := store.Send(context.Background(), v, auth)
	if err != nil {
		t.Fatal(err)
	}
	qv, query := reliableQueryProof(t, auth, 7, "avatar-A")
	if got, err := store.Query(context.Background(), qv, query); err != nil || got == nil {
		t.Fatalf("current member cannot recover: %v", err)
	}
	if _, err := db.Exec("DELETE FROM groupmembers WHERE member_id=?", disablePrincipal); err != nil {
		t.Fatal(err)
	}
	qv, query = reliableQueryProof(t, auth, 7, "avatar-A")
	if got, err := store.Query(context.Background(), qv, query); got != nil || !errors.Is(err, ErrForbidden) {
		t.Fatalf("expelled avatar recovered: %v", err)
	}
	qv, query = reliableQueryProof(t, auth, 7)
	if got, err := store.Query(context.Background(), qv, query); err != nil || got == nil || string(got.Result) != string(first.Result) {
		t.Fatalf("owner archival result lost: %v", err)
	}
}
