//go:build integration

package services

import (
	"google.golang.org/protobuf/proto"
	"im-server/commons/dbcommons"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/conversation/storages/dbs"
	"testing"
)

// The owner has seen an outgoing message between two incoming messages. Its
// history sequence must not clear the unread counter of the next inbox delivery.
func TestOwnerSeenMapsPartialHistoryToDeliveredIndex(t *testing.T) {
	db, _, _ := nativeReadFixture(t)
	const app, owner, target = "list-app", "42", "ordinary-peer"
	for _, item := range []struct {
		id         string
		seq, index int64
	}{{"incoming1", 5, 1}, {"incoming2", 10, 2}} {
		body, e := proto.Marshal(&pbobjs.DownMsg{MsgId: item.id, TargetId: target, ChannelType: 1, MsgSeqNo: item.seq, UnreadIndex: item.index})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec("INSERT INTO p_hismsgs(app_key,conver_id,sub_channel,msg_id,msg_seq_no) VALUES(?,?,'',?,?)", app, owner+":"+target, item.id, item.seq); e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec("INSERT INTO inboxmsgs(app_key,user_id,target_id,channel_type,msg_id,msg_body) VALUES(?,?,?,1,?,?)", app, owner, target, item.id, body); e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec("INSERT INTO conversations(app_key,user_id,target_id,channel_type,sub_channel,latest_msg) VALUES(?,?,?,1,'',?) ON DUPLICATE KEY UPDATE latest_msg=VALUES(latest_msg)", app, owner, target, body); e != nil {
			t.Fatal(e)
		}
	}
	for _, tc := range []struct{ seen, want int64 }{{0, 0}, {4, 0}, {5, 1}, {8, 1}, {10, 2}, {12, 2}} {
		got, e := (dbs.SeenDao{DB: dbcommons.GetDb()}).OwnerReadIndex(app, owner, target, 1, tc.seen)
		if e != nil || got != tc.want {
			t.Fatalf("seen=%d got=%d want=%d err=%v", tc.seen, got, tc.want, e)
		}
	}
}
