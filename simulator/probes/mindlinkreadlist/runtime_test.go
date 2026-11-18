//go:build integration

package mindlinkreadlist

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/simulator/utils"
	"im-server/simulator/wsclients"
)

const fixtureOwner = "im-f1-read-lists-20261109"
const avatar = "ma111111111111111111111111111111"
const lookalike = "ma222222222222222222222222222222"

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

type nativeFixture struct {
	t           *testing.T
	db          *sql.DB
	key, secret string
}

func (f nativeFixture) request(method, path string, payload any) (int, json.RawMessage) {
	f.t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, "http://127.0.0.1:9001/apigateway"+path, bytes.NewReader(body))
	if err != nil {
		f.t.Fatal(err)
	}
	nonce, stamp := randomHex(6), strconv.FormatInt(time.Now().UnixMilli(), 10)
	sig := sha1.Sum([]byte(f.secret + nonce + stamp))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("appkey", f.key)
	req.Header.Set("nonce", nonce)
	req.Header.Set("timestamp", stamp)
	req.Header.Set("signature", hex.EncodeToString(sig[:]))
	r, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		f.t.Fatal("owned native HTTP request failed", path, err)
	}
	defer r.Body.Close()
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&envelope); err != nil {
		f.t.Fatal("invalid native HTTP envelope", path, r.StatusCode)
	}
	return envelope.Code, envelope.Data
}

func (f nativeFixture) connect(id, platform string, received *sync.Map) *wsclients.WsImClient {
	f.t.Helper()
	code, data := f.request(http.MethodPost, "/users/register", map[string]any{"user_id": id, "nickname": "owned READ list fixture"})
	var token struct {
		Token string `json:"token"`
	}
	if code != 0 || json.Unmarshal(data, &token) != nil || token.Token == "" {
		f.t.Fatal("ordinary token registration failed", code)
	}
	c := wsclients.NewWsImClient("ws://127.0.0.1:9003", f.key, token.Token, func(m *pbobjs.DownMsg) {
		if received != nil {
			received.Store(m.MsgId, true)
		}
	}, nil, nil)
	c.Verbose, c.Platform, c.DeviceId = false, platform, "read-list-"+platform+"-"+id
	clientCode, ack := c.Connect("", "")
	if clientCode != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 || c.UserId != id {
		f.t.Fatal("ordinary WebSocket connection failed", id, clientCode)
	}
	f.t.Cleanup(c.Disconnect)
	return c
}

func query(t *testing.T, c *wsclients.WsImClient, topic string, in, out proto.Message) int32 {
	t.Helper()
	data, err := proto.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	code, ack := c.Query(topic, c.UserId, data)
	if ack == nil {
		t.Fatal("native query transport failed", topic, code)
	}
	if int32(code) != ack.Code {
		t.Fatal("native query client/ACK code mismatch", topic, code, ack.Code)
	}
	if ack.Code == 0 && out != nil && proto.Unmarshal(ack.Data, out) != nil {
		t.Fatal("invalid successful native protobuf", topic)
	}
	return int32(ack.Code)
}

func waitFor(t *testing.T, label string, timeout time.Duration, condition func() bool) {
	t.Helper()
	for until := time.Now().Add(timeout); time.Now().Before(until); {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("owned native fixture timed out", label)
}

func TestNativeConversationReadRuntime(t *testing.T) {
	if os.Getenv("JIM_READ_LIST_ISOLATED") != fixtureOwner || os.Getenv("JIM_READ_LIST_BINARY") != "/opt/imserver" {
		t.Skip("requires rebuilt independent READ-list runtime fixture")
	}
	if os.Getenv("JIM_READ_LIST_EVIDENCE") != "/evidence" {
		t.Fatal("unrecognized evidence mount")
	}
	bin, err := os.ReadFile("/opt/imserver")
	if err != nil {
		t.Fatal(err)
	}
	binaryHash := fmt.Sprintf("%x", sha256.Sum256(bin))
	if binaryHash != os.Getenv("JIM_READ_LIST_BINARY_SHA256") {
		t.Fatal("rebuilt native binary hash mismatch")
	}
	cfg, err := mysql.ParseDSN(os.Getenv("JIM_READ_LIST_DSN"))
	if err != nil || cfg.Net != "tcp" || cfg.Addr != fixtureOwner+"-mysql:3306" {
		t.Fatal("invalid owned runtime DB")
	}
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open owned runtime root")
	}
	t.Cleanup(func() { root.Close() })
	name := "read_list_runtime_" + randomHex(6)
	if _, err = root.Exec("CREATE DATABASE `" + name + "`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !strings.HasPrefix(name, "read_list_runtime_") || len(name) > 64 {
			t.Error("unsafe runtime scratch name")
			return
		}
		if _, err := root.Exec("DROP DATABASE `" + name + "`"); err != nil {
			t.Error("runtime scratch cleanup failed")
		}
	})
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open owned runtime scratch")
	}
	t.Cleanup(func() { db.Close() })
	initial, err := os.ReadFile("/fixture/imserver.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(initial)); err != nil {
		t.Fatal(err)
	}
	f := nativeFixture{t: t, db: db, key: "lr" + randomHex(6), secret: randomHex(16)}
	if _, err = db.Exec("INSERT INTO apps(app_key,app_secret,app_secure_key) VALUES(?,?,?)", f.key, f.secret, randomHex(8)); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"42", "43", avatar, lookalike} {
		if _, err = db.Exec("INSERT INTO users(app_key,user_id,nickname) VALUES(?,?,'owned fixture')", f.key, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec("INSERT INTO groupinfos(app_key,group_id,group_name) VALUES(?,'protected-group','owned protected group')", f.key); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"42", avatar} {
		if _, err = db.Exec("INSERT INTO groupmembers(app_key,group_id,member_id) VALUES(?,'protected-group',?)", f.key, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec("INSERT INTO mindlink_owner_barriers(environment,tenant_id,appkey,owner_id,read_enabled) VALUES('dev','runtime-tenant',?,'42',TRUE)", f.key); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO mindlink_principals(environment,tenant_id,appkey,owner_id,principal_id,avatar_id,avatar_generation,policy_version,state) VALUES('dev','runtime-tenant',?,'42',?,'runtime-avatar',1,1,'active')", f.key, avatar); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UnixMilli()
	for i, row := range []struct {
		target  string
		channel int
		unread  int
		offset  int64
	}{{avatar, 1, 100, 5000}, {"protected-group", 2, 100, 4000}, {lookalike, 1, 1, -5000}, {"43", 1, 0, -6000}} {
		msgID := ""
		var body []byte
		if row.target != "43" {
			msgID = fmt.Sprintf("legacy-list-%d", i)
			msg := &pbobjs.DownMsg{MsgId: msgID, MsgSeqNo: 1, SenderId: row.target, TargetId: "42", ChannelType: pbobjs.ChannelType(row.channel), TargetUserInfo: &pbobjs.UserInfo{UserId: row.target}}
			table, converID := "p_hismsgs", "42:"+row.target
			if row.channel == 2 {
				table, converID = "g_hismsgs", row.target
				msg.SenderId = "43"
				msg.TargetId = row.target
				msg.TargetUserInfo.UserId = "43"
				msg.GroupInfo = &pbobjs.GroupInfo{GroupId: row.target}
			}
			body, err = proto.Marshal(msg)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec("INSERT INTO "+table+"(app_key,conver_id,sub_channel,sender_id,receiver_id,channel_type,msg_id,msg_body,send_time,msg_seq_no) VALUES(?,?,'',?,'42',?,?,?,?,1)", f.key, converID, msg.SenderId, row.channel, msgID, body, stamp-2000); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = db.Exec("INSERT INTO conversations(app_key,user_id,target_id,channel_type,sub_channel,sort_time,sync_time,latest_msg_id,latest_msg,latest_unread_msg_index,latest_read_msg_index,is_top,top_updated_time) VALUES(?,'42',?,?,'',?,?,?,?,?,0,1,?)", f.key, row.target, row.channel, stamp+row.offset, stamp-1000, msgID, body, row.unread, stamp+row.offset); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range []struct {
		id, sender, target string
		channel            int
		stamp              int
	}{{"legacy-private", avatar, "42", 1, 103}, {"protected-group", "43", "protected-group", 2, 102}, {"42:43", "43", "42", 1, 101}} {
		if _, err = db.Exec("INSERT INTO globalconvers(app_key,conver_id,sender_id,target_id,channel_type,sub_channel,updated_time) VALUES(?,?,?,?,?,'',?)", f.key, r.id, r.sender, r.target, r.channel, r.stamp); err != nil {
			t.Fatal(err)
		}
	}
	private := t.TempDir()
	conf := map[string]any{"nodeName": "read-list-runtime", "nodeHost": "127.0.0.1", "msgStoreEngine": "mysql", "kvdb": map[string]any{"isOpen": false}, "log": map[string]any{"logPath": private, "logName": "read-list"}, "mysql": map[string]any{"user": cfg.User, "password": cfg.Passwd, "address": cfg.Addr, "name": cfg.DBName, "debug": false}, "apiGateway": map[string]any{"httpPort": 9001}, "navGateway": map[string]any{"httpPort": 9002}, "connectManager": map[string]any{"wsPort": 9003}, "adminGateway": map[string]any{"httpPort": 8090}, "adminSecret": randomHex(16), "mindlink": map[string]any{"enabled": false, "trusts": []any{}}}
	confBytes, err := yaml.Marshal(conf)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(private, "config.yml")
	if os.WriteFile(configPath, confBytes, 0600) != nil {
		t.Fatal("write private owned config")
	}
	log, err := os.OpenFile("/evidence/runtime-process-private.log", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	start := func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
		cmd := exec.CommandContext(ctx, "/opt/imserver", "-config", configPath)
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Start(); err != nil {
			cancel()
			t.Fatal("start rebuilt owned IM")
		}
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		var once sync.Once
		stop := func() {
			once.Do(func() {
				defer cancel()
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
			})
		}
		t.Cleanup(stop)
		waitFor(t, "IM health", 15*time.Second, func() bool {
			select {
			case <-done:
				t.Fatal("rebuilt IM exited; inspect private process log")
			default:
			}
			r, err := (&http.Client{Timeout: time.Second}).Get("http://127.0.0.1:9003/health")
			if err != nil {
				return false
			}
			defer r.Body.Close()
			return r.StatusCode == 200
		})
		live, err := os.ReadFile(fmt.Sprintf("/proc/%d/exe", cmd.Process.Pid))
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(live)) != binaryHash {
			t.Fatal("live process binary mismatch")
		}
		return stop
	}
	stop := start()
	var webReceived, pcReceived sync.Map
	web := f.connect("42", "Web", &webReceived)
	pc := f.connect("42", "PC", &pcReceived)
	peer := f.connect("43", "Web", nil)
	_ = f.connect(lookalike, "Web", nil)
	code, ack := peer.SendPrivateMsg("42", &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"owned READ-list ordinary message"}`), Flags: 10})
	if code != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 || ack.MsgId == "" {
		t.Fatal("ordinary native send failed", code)
	}
	ackID := ack.MsgId
	waitFor(t, "ordinary receipt/history/summary", 80*time.Second, func() bool {
		_, a := webReceived.Load(ackID)
		_, b := pcReceived.Load(ackID)
		var history, summary int
		err := db.QueryRow("SELECT (SELECT COUNT(*) FROM p_hismsgs WHERE app_key=? AND msg_id=?),(SELECT COUNT(*) FROM conversations WHERE app_key=? AND user_id='42' AND target_id='43' AND latest_msg_id=? AND latest_unread_msg_index-latest_read_msg_index=1)", f.key, ackID, f.key, ackID).Scan(&history, &summary)
		return err == nil && a && b && history == 1 && summary == 1
	})
	assertReads := func(c *wsclients.WsImClient, want int64, hiddenLookalike bool) {
		total := &pbobjs.QryTotalUnreadCountResp{}
		if q := query(t, c, "qry_total_unread_count", &pbobjs.QryTotalUnreadCountReq{}, total); q != 0 || total.TotalCount != want {
			t.Fatal("native total mismatch", q, total.TotalCount, want)
		}
		for _, topic := range []string{"qry_convers", "sync_convers", "qry_top_convers"} {
			var in proto.Message = &pbobjs.QryConversationsReq{Count: 100, StartTime: 1 << 62}
			if topic == "sync_convers" {
				in = &pbobjs.SyncConversationsReq{Count: 100}
			}
			if topic == "qry_top_convers" {
				in = &pbobjs.QryTopConversReq{StartTime: 1 << 62}
			}
			resp := &pbobjs.QryConversationsResp{}
			if q := query(t, c, topic, in, resp); q != 0 {
				t.Fatal("ordinary list failed", topic, q)
			}
			if len(resp.Conversations) != int(want) {
				t.Fatal("native list cardinality mismatch", topic, len(resp.Conversations), want)
			}
			for _, row := range resp.Conversations {
				if row.TargetId == avatar || row.TargetId == "protected-group" || hiddenLookalike && row.TargetId == lookalike {
					t.Fatal("native blind list leaked protected target", topic)
				}
			}
		}
		for _, path := range []string{"/convers/query?user_id=42&count=100&start=4611686018427387904", "/globalconvers/query?count=1"} {
			status, data := f.request(http.MethodGet, path, nil)
			if status != 0 || bytes.Contains(data, []byte(avatar)) || bytes.Contains(data, []byte("protected-group")) || hiddenLookalike && bytes.Contains(data, []byte(lookalike)) {
				t.Fatal("native signed HTTP blind list leaked/failed", path, status)
			}
			var listing struct {
				Items []json.RawMessage `json:"items"`
			}
			if json.Unmarshal(data, &listing) != nil {
				t.Fatal("invalid native HTTP list")
			}
			expected := int(want)
			if strings.HasPrefix(path, "/global") {
				expected = 1
			}
			if len(listing.Items) != expected {
				t.Fatal("native HTTP list cardinality mismatch", path, len(listing.Items), expected)
			}
		}
	}
	assertReads(web, 2, false)
	assertReads(pc, 2, false)
	if _, err = db.Exec("INSERT INTO mindlink_principals(environment,tenant_id,appkey,owner_id,principal_id,avatar_id,avatar_generation,policy_version,state) VALUES('dev','runtime-tenant',?,'42',?,'late-projection',1,1,'active')", f.key, lookalike); err != nil {
		t.Fatal(err)
	}
	assertReads(web, 1, true)
	if _, err = db.Exec("UPDATE mindlink_principals SET state='disabled' WHERE appkey=? AND principal_id=?", f.key, lookalike); err != nil {
		t.Fatal(err)
	}
	assertReads(pc, 1, true)
	// Failure injection is restricted to this randomly named, independently owned scratch DB.
	if _, err = db.Exec("RENAME TABLE mindlink_principals TO read_list_unavailable"); err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if !restored {
			if _, err := db.Exec("RENAME TABLE read_list_unavailable TO mindlink_principals"); err != nil {
				t.Error("owned authority fixture restoration failed")
				return
			}
			restored = true
		}
	}
	t.Cleanup(restore)
	for _, path := range []string{"/convers/query?user_id=42&count=100", "/globalconvers/query?count=1"} {
		status, data := f.request(http.MethodGet, path, nil)
		if status == 0 || len(data) > 0 && string(data) != "null" {
			t.Fatal("authority storage failure became successful native HTTP data", path, status)
		}
	}
	if q := query(t, web, "qry_total_unread_count", &pbobjs.QryTotalUnreadCountReq{}, nil); q == 0 {
		t.Fatal("authority storage failure became successful native ACK")
	}
	restore()
	if !restored {
		t.Fatal("authority fixture remained unavailable")
	}
	assertReads(web, 1, true)
	var protectedRows, protectedUnread int
	if err = db.QueryRow("SELECT COUNT(*),SUM(latest_unread_msg_index-latest_read_msg_index) FROM conversations WHERE app_key=? AND user_id='42' AND target_id IN (?, 'protected-group')", f.key, avatar).Scan(&protectedRows, &protectedUnread); err != nil || protectedRows != 2 || protectedUnread != 200 {
		t.Fatal("read filtering changed protected DB state", protectedRows, protectedUnread, err)
	}
	snapshot := func() string {
		rows, err := db.Query("SELECT target_id,channel_type,latest_msg_id,latest_unread_msg_index,latest_read_msg_index,is_top,is_deleted FROM conversations WHERE app_key=? AND user_id='42' ORDER BY target_id,channel_type", f.key)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var all []string
		for rows.Next() {
			var target, msg string
			var channel, unread, read, top, deleted int64
			if err := rows.Scan(&target, &channel, &msg, &unread, &read, &top, &deleted); err != nil {
				t.Fatal(err)
			}
			all = append(all, fmt.Sprintf("%s|%d|%s|%d|%d|%d|%d", target, channel, msg, unread, read, top, deleted))
		}
		if rows.Err() != nil {
			t.Fatal(rows.Err())
		}
		return strings.Join(all, "\n")
	}
	before := snapshot()
	web.Disconnect()
	pc.Disconnect()
	peer.Disconnect()
	stop()
	stop = start()
	web = f.connect("42", "Web", nil)
	assertReads(web, 1, true)
	if snapshot() != before {
		t.Fatal("ordinary/protected conversation DB metadata drifted across IM-only restart")
	}
	evidence := map[string]any{"binary_sha256": binaryHash, "source_sha256": os.Getenv("JIM_READ_LIST_SOURCE_SHA256"), "actual_ordinary_ack_id": ackID, "owner_devices_received": 2, "initial_native_list_count": 2, "initial_native_unread": 2, "after_live_projection_and_disable": 1, "after_IM_only_restart": 1, "protected_DB_rows": protectedRows, "protected_DB_unread": protectedUnread, "conversation_DB_snapshot_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(before))), "ordinary_history_and_summary_match_ACK": true, "live_binary_checked_before_after": true, "scope": "owned synthetic native HTTP/WS; no owner business API or real page"}
	raw, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil || os.WriteFile("/evidence/runtime-verification.json", append(raw, '\n'), 0600) != nil {
		t.Fatal("write sanitized runtime verification")
	}
	t.Log("actual native HTTP/WS lists and unread passed; warm projection/disabled filtering, ordinary ACK/history/summary and IM-only restart DB checks passed")
}
