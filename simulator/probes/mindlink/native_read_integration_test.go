//go:build integration

package mindlink

import (
	"context"
	"database/sql"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/commonservices/msgdefines"
	"im-server/simulator/utils"
	"im-server/simulator/wsclients"

	"google.golang.org/protobuf/proto"
)

func startNativeReadProcess(t *testing.T, ctx context.Context, configPath string, log *os.File) func() {
	t.Helper()
	cmd := exec.CommandContext(ctx, "/opt/imserver", "-config", configPath)
	cmd.Dir = filepath.Dir(configPath)
	cmd.Stdout, cmd.Stderr = log, log
	if cmd.Start() != nil {
		t.Fatal("start rebuilt owned IM process")
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	stop := func() {
		select {
		case <-done:
			return
		default:
		}
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}
	t.Cleanup(stop)
	health := http.Client{Timeout: time.Second}
	for deadline := time.Now().Add(12 * time.Second); time.Now().Before(deadline); {
		select {
		case <-done:
			t.Fatal("rebuilt IM exited before health; inspect private fixture log")
		default:
		}
		resp, err := health.Get("http://127.0.0.1:9003/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				t.Log("owned rebuilt process healthy, pid:", cmd.Process.Pid)
				return stop
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("rebuilt IM did not become healthy")
	return stop
}

func nativeReadState(t *testing.T, c *wsclients.WsImClient, target string, channel pbobjs.ChannelType, seq int64) (int32, *pbobjs.ConverSeenState) {
	t.Helper()
	method := "qry_conver_seen"
	var request proto.Message = &pbobjs.QryConverSeenReq{TargetId: target, ChannelType: channel}
	if seq >= 0 {
		method = "set_conver_seen"
		request = &pbobjs.SetConverSeenReq{TargetId: target, ChannelType: channel, SeenSeq: seq}
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	transport, ack := c.Query(method, c.UserId, raw)
	if ack == nil {
		t.Fatal("native READ transport failed", transport)
	}
	if transport != utils.Trans2ClientErrorCoce(ack.Code) {
		t.Fatal("native READ transport/ACK mismatch", transport, ack.Code)
	}
	if ack.Code != 0 {
		if len(ack.Data) != 0 {
			t.Fatal("native READ denial included state")
		}
		return ack.Code, nil
	}
	state := &pbobjs.ConverSeenState{}
	if proto.Unmarshal(ack.Data, state) != nil {
		t.Fatal("native READ success could not be decoded")
	}
	return 0, state
}

func nativeDisplayedSequence(t *testing.T, c *wsclients.WsImClient, target string, channel pbobjs.ChannelType, id string, want int64) int64 {
	t.Helper()
	transport, history := c.QryHistoryMsgs(&pbobjs.QryHisMsgsReq{TargetId: target, ChannelType: channel, Count: 10})
	if transport != utils.ClientErrorCode_Success || history == nil {
		t.Fatal("native display/history unavailable")
	}
	for _, msg := range history.Msgs {
		if msg.MsgId == id {
			if msg.MsgSeqNo != want {
				t.Fatalf("history sequence lost precision: got=%d want=%d", msg.MsgSeqNo, want)
			}
			return msg.MsgSeqNo
		}
	}
	t.Fatal("actual SDK history omitted received message ID")
	return 0
}

func exerciseNativeRead(t *testing.T, db *sql.DB, owner, peer *wsclients.WsImClient, received *sync.Map) {
	t.Helper()
	if code, state := nativeReadState(t, owner, nativePrincipal, pbobjs.ChannelType_Group, -1); code != 0 || state.SeenSeq != 0 || state.LatestSeq != 1 || !state.HasNew {
		t.Fatal("ordinary group/user namespace READ query failed", code)
	}
	for _, seq := range []int64{-1, 0} {
		for _, channel := range []pbobjs.ChannelType{pbobjs.ChannelType_Private, pbobjs.ChannelType_Group} {
			target := nativePrincipal
			if channel == pbobjs.ChannelType_Group {
				target = "protected-group"
			}
			if code, _ := nativeReadState(t, owner, target, channel, seq); code != 19008 {
				t.Fatal("projected native READ query/advance escaped refusal", code)
			}
		}
	}
	for _, q := range []string{
		"INSERT INTO groupinfos(app_key,group_id,group_name) VALUES('fixture-app','wide-group','wide ordinary fixture')",
		"INSERT INTO groupmembers(app_key,group_id,member_id) VALUES('fixture-app','wide-group','42'),('fixture-app','wide-group','43')",
		"INSERT INTO message_sequence(app_key,conver_id,channel_type,sub_channel,seq,send_time) VALUES('fixture-app','wide-group',2,'',2147483647,0)",
		"UPDATE message_sequence SET seq=2147483647 WHERE app_key='fixture-app' AND conver_id='42:43' AND channel_type=1 AND sub_channel=''",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	pc := wsclients.NewWsImClient("ws://127.0.0.1:9003", nativeApp, nativeToken(t, "42"), nil, nil, nil)
	pc.Verbose, pc.Platform, pc.DeviceId = false, "PC", "native-read-PC"
	if transport, ack := pc.Connect("", ""); transport != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 {
		t.Fatal("second ordinary READ device failed to connect")
	}
	defer pc.Disconnect()
	_, private := owner.SendPrivateMsg("43", &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"wide private fixture"}`), Flags: msgdefines.SetStoreMsg(0)})
	if private == nil || private.Code != 0 || private.MsgId == "" {
		t.Fatal("wide private native send was not accepted")
	}
	waitNativeHistory(t, db, received, "p_hismsgs", private.MsgId, 2147483648)
	privateSeq := nativeDisplayedSequence(t, peer, "42", pbobjs.ChannelType_Private, private.MsgId, 2147483648)
	if code, state := nativeReadState(t, peer, "42", pbobjs.ChannelType_Private, privateSeq); code != 0 || state.SeenSeq != privateSeq || state.HasNew {
		t.Fatal("private wide display could not advance seen", code)
	}
	for _, want := range []int64{2147483648, math.MaxInt64} {
		if want == math.MaxInt64 {
			if _, err := db.Exec("UPDATE message_sequence SET seq=? WHERE app_key='fixture-app' AND conver_id='wide-group' AND channel_type=2 AND sub_channel=''", want-1); err != nil {
				t.Fatal(err)
			}
		}
		_, sent := owner.SendGroupMsg("wide-group", &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"wide group fixture"}`), Flags: msgdefines.SetStoreMsg(0)})
		if sent == nil || sent.Code != 0 || sent.MsgId == "" {
			t.Fatal("wide group native send was not accepted")
		}
		waitNativeHistory(t, db, received, "g_hismsgs", sent.MsgId, want)
		seq := nativeDisplayedSequence(t, owner, "wide-group", pbobjs.ChannelType_Group, sent.MsgId, want)
		_ = nativeDisplayedSequence(t, peer, "wide-group", pbobjs.ChannelType_Group, sent.MsgId, want)
		if code, state := nativeReadState(t, owner, "wide-group", pbobjs.ChannelType_Group, seq); code != 0 || state.SeenSeq != seq || state.LatestSeq != want || state.HasNew {
			t.Fatal("group display could not advance the exact wide sequence", code)
		}
		if code, state := nativeReadState(t, pc, "wide-group", pbobjs.ChannelType_Group, seq-1); code != 0 || state.SeenSeq != seq || state.HasNew {
			t.Fatal("second device regressed a wide reading position", code)
		}
		if !eventually(func() bool {
			_, summary := peer.QryConversation(&pbobjs.QryConverReq{TargetId: "wide-group", ChannelType: pbobjs.ChannelType_Group})
			return summary != nil && summary.GetMsg().GetMsgId() == sent.MsgId && summary.UnreadCount == 0
		}) {
			t.Fatal("stored no-count message lost summary or changed ordinary unread")
		}
	}
	_, exhausted := owner.SendGroupMsg("wide-group", &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"must not persist"}`), Flags: msgdefines.SetStoreMsg(0)})
	if exhausted == nil || exhausted.Code != 10009 || exhausted.MsgId != "" {
		t.Fatal("exhausted allocator returned a message success")
	}
	var count int
	if db.QueryRow("SELECT COUNT(*) FROM g_hismsgs WHERE app_key='fixture-app' AND conver_id='wide-group'").Scan(&count) != nil || count != 2 {
		t.Fatal("exhausted allocator left an extra history row")
	}
	if _, err := db.Exec("DELETE FROM p_hismsgs WHERE app_key='fixture-app' AND conver_id='42:43'"); err != nil {
		t.Fatal("owned private retention fixture failed", err)
	}
	t.Log("native READ: Web/PC share exact seen, stored no-count summary/unread checked, private2147483648 and group MaxInt64 actually received/displayed/stored; exhaustion denied; scoped private history removed for restart retention test")
}

func waitNativeHistory(t *testing.T, db *sql.DB, received *sync.Map, table, id string, want int64) {
	t.Helper()
	if table != "p_hismsgs" && table != "g_hismsgs" {
		t.Fatal("invalid owned history table")
	}
	if !eventually(func() bool {
		var count int
		var minSeq, maxSeq int64
		_, got := received.Load("43:" + id)
		return got && db.QueryRow("SELECT COUNT(*),COALESCE(MIN(msg_seq_no),0),COALESCE(MAX(msg_seq_no),0) FROM "+table+" WHERE app_key=? AND msg_id=?", nativeApp, id).Scan(&count, &minSeq, &maxSeq) == nil && count == 1 && minSeq == want && maxSeq == want
	}) {
		t.Fatal("SDK receipt and exact history sequence did not both appear")
	}
}

func exerciseNativeReadAfterRestart(t *testing.T, db *sql.DB, owner, peer *wsclients.WsImClient, received *sync.Map) {
	t.Helper()
	if code, state := nativeReadState(t, owner, "wide-group", pbobjs.ChannelType_Group, -1); code != 0 || state.SeenSeq != math.MaxInt64 || state.LatestSeq != math.MaxInt64 || state.HasNew {
		t.Fatal("wide group state did not survive actual IM restart", code)
	}
	if code, state := nativeReadState(t, peer, "42", pbobjs.ChannelType_Private, -1); code != 0 || state.SeenSeq != 2147483648 || state.LatestSeq != 0 || state.HasNew {
		t.Fatal("retained private seen did not survive IM-only restart", code)
	}
	_, sent := owner.SendPrivateMsg("43", &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"post-restart private fixture"}`), Flags: msgdefines.SetStoreMsg(0)})
	if sent == nil || sent.Code != 0 || sent.MsgId == "" {
		t.Fatal("post-restart private send failed")
	}
	waitNativeHistory(t, db, received, "p_hismsgs", sent.MsgId, 2147483649)
	seq := nativeDisplayedSequence(t, peer, "42", pbobjs.ChannelType_Private, sent.MsgId, 2147483649)
	if code, state := nativeReadState(t, peer, "42", pbobjs.ChannelType_Private, -1); code != 0 || state.SeenSeq != 2147483648 || state.LatestSeq != seq || !state.HasNew {
		t.Fatal("retention/restart reused sequence and hid the new entry", code)
	}
	var protected int
	if err := db.QueryRow("SELECT COUNT(*) FROM conversation_seen WHERE app_key='fixture-app' AND (target_id='protected-group' OR target_id=?)", nativePrincipal).Scan(&protected); err != nil || protected != 0 {
		// The same-named ordinary group was only queried, never advanced.
		t.Fatal("denied projected native READ left seen state", err)
	}
	t.Log("IM-only restart: MaxInt64 group state retained; private seen/watermark outlived physical history; next actual SDK/history sequence2147483649 produces has_new=true")
}
