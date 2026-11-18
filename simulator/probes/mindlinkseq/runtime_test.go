package mindlinkseq

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/connectmanager/server/codec"
	hisdbs "im-server/services/historymsg/storages/dbs"
	"im-server/simulator/utils"
	"im-server/simulator/wsclients"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const server = "im-f1-sequence-server"

type fixture struct {
	Key, Secret, Group string
	IDs                map[string]string
	Received           sync.Map `json:"-"`
}

func guard(t *testing.T) {
	t.Helper()
	if os.Getenv("JIM_SEQUENCE_ISOLATED") != "im-f1-sequence-20261106" {
		t.Skip("requires owned sequence runtime fixture")
	}
}

func post(t *testing.T, f *fixture, path string, body any, auth string) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + server + ":9001/apigateway"
	if auth != "" || path == "/admingateway/login" {
		base = "http://" + server + ":8090"
	}
	req, err := http.NewRequest(http.MethodPost, base+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if f.Key != "" {
		nonce, stamp := "seq", fmt.Sprint(time.Now().UnixMilli())
		sig := sha1.Sum([]byte(f.Secret + nonce + stamp))
		req.Header.Set("appkey", f.Key)
		req.Header.Set("nonce", nonce)
		req.Header.Set("timestamp", stamp)
		req.Header.Set("signature", hex.EncodeToString(sig[:]))
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result struct {
		Code int
		Data json.RawMessage
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || resp.StatusCode != 200 || result.Code != 0 {
		t.Fatalf("fixture API %s failed status=%d code=%d err=%v", path, resp.StatusCode, result.Code, err)
	}
	return result.Data
}

func connect(t *testing.T, f *fixture, user, platform string) *wsclients.WsImClient {
	t.Helper()
	var token struct{ Token string }
	if err := json.Unmarshal(post(t, f, "/users/register", map[string]string{"user_id": user}, ""), &token); err != nil || token.Token == "" {
		t.Fatal("fixture registration failed")
	}
	c := wsclients.NewWsImClient("ws://"+server+":9003", f.Key, token.Token, func(msg *pbobjs.DownMsg) { f.Received.Store(user+platform+msg.MsgId, msg.MsgSeqNo) }, nil, nil)
	c.Verbose = false
	c.Platform = platform
	c.DeviceId = "seq-" + platform
	code, ack := c.Connect("", "")
	if code != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 {
		t.Fatal("fixture connection failed")
	}
	t.Cleanup(c.Disconnect)
	return c
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	configures.Config.Mysql.User = "root"
	configures.Config.Mysql.Password = os.Getenv("MYSQL_ROOT_PASSWORD")
	configures.Config.Mysql.Address = "im-f1-sequence-mysql:3306"
	configures.Config.Mysql.DbName = "jim_db"
	if err := dbcommons.InitMysql(); err != nil {
		t.Fatal(err)
	}
	db := dbcommons.GetDb()
	db.Config.Logger = logger.Default.LogMode(logger.Silent)
	return db
}

func eventually(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatal("expected actual receipt/history did not appear")
}

func sendAndVerify(t *testing.T, f *fixture, sender, receiver *wsclients.WsImClient, group bool, want int64) string {
	t.Helper()
	target, channel, kind := "seq-bob", pbobjs.ChannelType_Private, "private"
	send := sender.SendPrivateMsg
	if group {
		target, channel, kind = f.Group, pbobjs.ChannelType_Group, "group"
		send = sender.SendGroupMsg
	}
	code, ack := send(target, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"isolated watermark flow"}`), Flags: 8})
	if code != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 || ack.MsgSeqNo != want {
		t.Fatalf("%s publish sequence: ack=%v code=%v", kind, ack, code)
	}
	eventually(t, func() bool {
		web, w := f.Received.Load("seq-bobWeb" + ack.MsgId)
		pc, p := f.Received.Load("seq-bobPC" + ack.MsgId)
		return w && p && web == want && pc == want
	})
	historyTarget := target
	if !group {
		historyTarget = "seq-alice"
	}
	eventually(t, func() bool {
		code, history := receiver.QryHistoryMsgs(&pbobjs.QryHisMsgsReq{TargetId: historyTarget, ChannelType: channel, Count: 10})
		return code == utils.ClientErrorCode_Success && history != nil && len(history.Msgs) == 1 && history.Msgs[0].MsgId == ack.MsgId && history.Msgs[0].MsgSeqNo == want
	})
	return ack.MsgId
}

func TestSequenceBeforeRestart(t *testing.T) {
	guard(t)
	f := &fixture{IDs: map[string]string{}}
	var auth struct{ Authorization string }
	if err := json.Unmarshal(post(t, f, "/admingateway/login", map[string]string{"account": "admin", "password": "123456"}, ""), &auth); err != nil || auth.Authorization == "" {
		t.Fatal("fixture admin login failed")
	}
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	f.Key = "sw" + hex.EncodeToString(random)
	f.Group = "group-" + hex.EncodeToString(random)
	var app struct {
		Secret string `json:"app_secret"`
	}
	if err := json.Unmarshal(post(t, f, "/admingateway/apps/create", map[string]string{"app_key": f.Key, "app_name": "Sequence isolated"}, auth.Authorization), &app); err != nil || app.Secret == "" {
		t.Fatal("fixture tenant create failed")
	}
	f.Secret = app.Secret
	write := func() {
		data, err := json.Marshal(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("/src/.local/f1-sequence/runtime-private.json", data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	alice, bob := connect(t, f, "seq-alice", "Web"), connect(t, f, "seq-bob", "Web")
	connect(t, f, "seq-bob", "PC")
	post(t, f, "/users/register", map[string]string{"user_id": "seq-carol"}, "")
	post(t, f, "/groups/add", map[string]any{"group_id": f.Group, "group_name": "Sequence test", "member_ids": []string{"seq-alice", "seq-bob", "seq-carol"}}, "")
	f.IDs["private"] = sendAndVerify(t, f, alice, bob, false, 1)
	f.IDs["group"] = sendAndVerify(t, f, alice, bob, true, 1)
	write()
	db := testDB(t)
	if err := db.Exec("INSERT INTO conversation_seen (app_key,user_id,target_id,channel_type,sub_channel,seen_seq) VALUES (?,?,?,?,?,1),(?,?,?,?,?,1)", f.Key, "seq-bob", "seq-alice", 1, "", f.Key, "seq-bob", f.Group, 2, "").Error; err != nil {
		t.Fatal(err)
	}
	if err := (hisdbs.PrivateHisMsgDao{}).DelMsgsBaseTime(f.Key, math.MaxInt64); err != nil {
		t.Fatal(err)
	}
	if err := (hisdbs.GroupHisMsgDao{}).DelMsgsBaseTime(f.Key, math.MaxInt64); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"p_hismsgs", "g_hismsgs"} {
		var count int64
		if err := db.Raw("SELECT COUNT(*) FROM "+table+" WHERE app_key=?", f.Key).Scan(&count).Error; err != nil || count != 0 {
			t.Fatal("owned tenant history purge incomplete")
		}
	}
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM message_sequence WHERE app_key=? AND seq=1", f.Key).Scan(&count).Error; err != nil || count != 2 {
		t.Fatal("watermarks not durable before restart")
	}
	t.Log("private/group ACK, two-device receipt, history verified; real retention removed histories and retained both watermarks/seen=1")
}

func TestSequenceAfterRestart(t *testing.T) {
	guard(t)
	f := &fixture{}
	data, err := os.ReadFile("/src/.local/f1-sequence/runtime-private.json")
	if err != nil || json.Unmarshal(data, f) != nil || len(f.IDs) != 2 {
		t.Fatal("owned restart fixture missing")
	}
	alice, bob := connect(t, f, "seq-alice", "Web"), connect(t, f, "seq-bob", "Web")
	connect(t, f, "seq-bob", "PC")
	var currentIDs []string
	for _, group := range []bool{false, true} {
		id := sendAndVerify(t, f, alice, bob, group, 2)
		currentIDs = append(currentIDs, id)
		if id == f.IDs["private"] || id == f.IDs["group"] {
			t.Fatal("message ID reused")
		}
	}
	db := testDB(t)
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM message_sequence WHERE app_key=? AND seq=2", f.Key).Scan(&count).Error; err != nil || count != 2 {
		t.Fatal("wrong post-restart watermark")
	}
	if err := db.Raw("SELECT COUNT(*) FROM conversation_seen WHERE app_key=? AND seen_seq=1", f.Key).Scan(&count).Error; err != nil || count != 2 {
		t.Fatal("seen was reset to hide sequence reuse")
	}
	for _, table := range []string{"sendboxmsgs", "inboxmsgs"} {
		eventually(t, func() bool {
			var n int64
			err := db.Raw("SELECT COUNT(DISTINCT msg_id) FROM "+table+" WHERE app_key=? AND msg_id IN (?,?)", f.Key, currentIDs[0], currentIDs[1]).Scan(&n).Error
			return err == nil && n == 2
		})
	}
	time.Sleep(250 * time.Millisecond)
	receipts := func() int { n := 0; f.Received.Range(func(_, _ any) bool { n++; return true }); return n }
	beforeReceipts := receipts()
	boxIDs := map[string]int64{}
	for _, table := range []string{"sendboxmsgs", "inboxmsgs"} {
		var last int64
		if err := db.Raw("SELECT COALESCE(MAX(id),0) FROM "+table+" WHERE app_key=?", f.Key).Scan(&last).Error; err != nil {
			t.Fatal(err)
		}
		boxIDs[table] = last
	}
	// An owned temporary table fault must reject both send paths before storing another message.
	if err := db.Exec("RENAME TABLE message_sequence TO message_sequence_fault").Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Exec("RENAME TABLE message_sequence_fault TO message_sequence").Error; err != nil {
			t.Error(err)
		}
	}()
	for i, send := range []func(string, *pbobjs.UpMsg) (utils.ClientErrorCode, *codec.PublishAckMsgBody){alice.SendPrivateMsg, alice.SendGroupMsg} {
		target := "seq-bob"
		if i == 1 {
			target = f.Group
		}
		_, ack := send(target, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"must reject"}`), Flags: 8})
		if ack == nil || ack.Code != 10009 || ack.MsgId != "" || ack.MsgSeqNo != 0 {
			t.Fatal("sequence storage fault became successful publish")
		}
	}
	for _, table := range []string{"p_hismsgs", "g_hismsgs"} {
		if err := db.Raw("SELECT COUNT(*) FROM "+table+" WHERE app_key=?", f.Key).Scan(&count).Error; err != nil || count != 1 {
			t.Fatal("rejected sequence allocation stored another message")
		}
	}
	time.Sleep(time.Second)
	if receipts() != beforeReceipts {
		t.Fatal("failed allocation produced a client delivery")
	}
	for table, last := range boxIDs {
		if err := db.Raw("SELECT COUNT(*) FROM "+table+" WHERE app_key=? AND id>?", f.Key, last).Scan(&count).Error; err != nil || count != 0 {
			t.Fatal("failed allocation created inbox/sendbox data")
		}
	}
	t.Log("IM-only restart: private/group seq=2>seen=1, actual two-device receipt/history; table fault rejected both sends, no new history/inbox/sendbox or receipt observed in 1s")
}
