//go:build integration

package mindlink

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	"im-server/commons/configures"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/commons/tools"
	"im-server/simulator/utils"
	"im-server/simulator/wsclients"
)

func TestNativePolicyRebuiltHTTPWS(t *testing.T) {
	if os.Getenv("MINDLINK_TEST_NATIVE_BINARY") == "" {
		t.Skip("requires rebuilt owned fixture image; not process/WS coverage")
	}
	if os.Getenv("MINDLINK_TEST_NATIVE_BINARY") != "/opt/imserver" || os.Getenv("MINDLINK_TEST_NATIVE_ARTIFACT_DIR") != "/evidence" {
		t.Fatal("refusing unrecognized runtime fixture")
	}
	bin, err := os.Open("/opt/imserver")
	if err != nil {
		t.Fatal("open rebuilt fixture binary")
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, bin)
	bin.Close()
	if copyErr != nil || hex.EncodeToString(hash.Sum(nil)) != os.Getenv("MINDLINK_TEST_NATIVE_SHA256") {
		t.Fatal("rebuilt fixture binary hash mismatch")
	}
	db := nativeBoundaryDB(t)
	if configures.Config.Mysql.Address != "127.0.0.1:3306" {
		t.Fatal("process test requires owned MySQL network namespace")
	}
	legacy := "ma333333333333333333333333333333"
	for _, q := range []string{
		"INSERT INTO users(app_key,user_id,nickname) VALUES('fixture-app','43','ordinary receiver'),('fixture-app','ma333333333333333333333333333333','legacy fixture')",
		"INSERT INTO groupinfos(app_key,group_id,group_name) VALUES('fixture-app','ma111111111111111111111111111111','ordinary namespace collision')",
		"INSERT INTO groupmembers(app_key,group_id,member_id) VALUES('fixture-app','ordinary-group','43'),('fixture-app','ma111111111111111111111111111111','42'),('fixture-app','ma111111111111111111111111111111','43')",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	privateDir := t.TempDir()
	conf := map[string]any{
		"nodeName": "native-066519ee", "nodeHost": "127.0.0.1", "msgStoreEngine": "mysql",
		"log": map[string]any{"logPath": privateDir, "logName": "native-process"}, "kvdb": map[string]any{"isOpen": false},
		"mysql":      map[string]any{"user": configures.Config.Mysql.User, "password": configures.Config.Mysql.Password, "address": configures.Config.Mysql.Address, "name": configures.Config.Mysql.DbName, "debug": false},
		"apiGateway": map[string]any{"httpPort": 9001}, "connectManager": map[string]any{"wsPort": 9003}, "navGateway": map[string]any{"httpPort": 9002}, "adminGateway": map[string]any{"httpPort": 8090},
		"adminSecret": "isolated-native-fixture", "mindlink": map[string]any{"enabled": false, "trusts": []any{}},
	}
	raw, err := yaml.Marshal(conf)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(privateDir, "config.yml")
	if os.WriteFile(configPath, raw, 0600) != nil {
		t.Fatal("write private runtime fixture config")
	}
	log, err := os.OpenFile("/evidence/native-process-private.log", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("open private process evidence")
	}
	t.Cleanup(func() { log.Close() })
	procCtx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(procCtx, "/opt/imserver", "-config", configPath)
	cmd.Stdout, cmd.Stderr = log, log
	if cmd.Start() != nil {
		t.Fatal("start rebuilt owned IM process")
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
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
	health := http.Client{Timeout: time.Second}
	ready := false
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
				ready = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatal("rebuilt IM did not become healthy")
	}
	var received sync.Map
	var illegalReceived atomic.Int32
	connect := func(id string, allowed bool) *wsclients.WsImClient {
		c := wsclients.NewWsImClient("ws://127.0.0.1:9003", nativeApp, nativeToken(t, id), func(msg *pbobjs.DownMsg) {
			received.Store(id+":"+msg.MsgId, true)
			if msg.SenderId == nativePrincipal || msg.SenderId == legacy || msg.ChannelType == pbobjs.ChannelType_Group && msg.TargetId == "protected-group" || msg.ChannelType == pbobjs.ChannelType_Private && (msg.TargetId == nativePrincipal || msg.TargetId == legacy) {
				illegalReceived.Add(1)
			}
		}, nil, nil)
		c.Verbose, c.Platform, c.DeviceId = false, "Web", "native-"+id
		code, ack := c.Connect("", "")
		t.Cleanup(c.Disconnect)
		if allowed {
			if code != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 || c.UserId != id {
				t.Fatalf("ordinary fixture WS login failed code=%d", code)
			}
		} else if ack == nil || ack.Code != 19008 {
			t.Fatal("internal fixture WS login was not refused")
		}
		return c
	}
	owner := connect("42", true)
	_ = connect("43", true)
	oldSession := connect(legacy, true)
	_ = connect(nativePrincipal, false)
	if _, err := db.Exec("UPDATE mindlink_principals SET state='disabled' WHERE principal_id=?", nativePrincipal); err != nil {
		t.Fatal(err)
	}
	_ = connect(nativePrincipal, false)
	if _, err := db.Exec("INSERT INTO mindlink_principals(environment,tenant_id,appkey,owner_id,principal_id,avatar_id,avatar_generation,policy_version,state) VALUES('dev','fixture-tenant','fixture-app','42',?,'avatar-C',1,1,'active')", legacy); err != nil {
		t.Fatal(err)
	}
	_, staleAck := oldSession.SendPrivateMsg("42", &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"rejected fixture"}`), Flags: 10})
	if staleAck == nil || staleAck.Code != 19008 || staleAck.MsgId != "" {
		t.Fatal("existing now-projected WS sender escaped refusal")
	}
	for _, kind := range []string{"jg:text", "jg:image", "jg:voice", "jg:video", "jg:file"} {
		_, ack := owner.SendGroupMsg("protected-group", &pbobjs.UpMsg{MsgType: kind, MsgContent: []byte(`{"content":"rejected fixture"}`), Flags: 10})
		if ack == nil || ack.Code != 19008 || ack.MsgId != "" {
			t.Fatal("managed group WS send escaped refusal", kind)
		}
		r := nativeProcessPost(t, "/messages/group/send", map[string]any{"sender_id": "42", "target_id": "protected-group", "msg_type": kind, "msg_content": `{"content":"rejected fixture"}`})
		if r.Code != 19008 || len(r.Data) > 0 && string(r.Data) != "null" {
			t.Fatal("managed group signed HTTP returned admission success", kind, r.Code)
		}
	}
	for _, tc := range []struct {
		path string
		body string
	}{
		{"/users/register", `{"user_id":"ma111111111111111111111111111111","nickname":"overwrite"}`},
		{"/users/update", `{"user_id":"ma111111111111111111111111111111","nickname":"overwrite"}`},
		{"/messages/private/send", `{"sender_id":"42","target_id":"ma111111111111111111111111111111","msg_type":"jg:text","msg_content":"fixture"}`},
		{"/messages/system/send", `{"sender_id":"42","target_id":"ma111111111111111111111111111111","msg_type":"jg:text","msg_content":"fixture"}`},
		{"/users/onlinestatus/query", `{"user_ids":["42","ma111111111111111111111111111111"]}`},
		{"/convers/add", `{"user_id":"42","target_id":"protected-group","channel_type":2}`},
		{"/convers/del", `{"user_id":"42","items":[{"user_id":"42","target_id":"protected-group","channel_type":2}]}`},
		{"/convers/undisturb", `{"user_id":"42","items":[{"target_id":"protected-group","channel_type":2,"undisturb_type":1}]}`},
		{"/convers/top", `{"user_id":"42","items":[{"target_id":"protected-group","channel_type":2,"is_top":true}]}`},
	} {
		r := nativeProcessPost(t, tc.path, json.RawMessage(tc.body))
		if r.Code != 19008 {
			t.Fatal("signed HTTP internal operation escaped", tc.path, r.Code)
		}
	}
	for _, channel := range []pbobjs.ChannelType{pbobjs.ChannelType_Private, pbobjs.ChannelType_Group} {
		target := nativePrincipal
		if channel == pbobjs.ChannelType_Group {
			target = "protected-group"
		}
		data, _ := tools.PbMarshal(&pbobjs.QryHisMsgsReq{TargetId: target, ChannelType: channel, Count: 10})
		_, ack := owner.Query("qry_hismsgs", target, data)
		if ack == nil || ack.Code != 19008 || len(ack.Data) != 0 {
			t.Fatal("protected WS history did not refuse")
		}
	}
	r := nativeProcessPost(t, "/users/onlinestatus/query", map[string]any{"user_ids": []string{"42", "43"}})
	var statuses struct {
		Items []json.RawMessage `json:"items"`
	}
	if r.Code != 0 || json.Unmarshal(r.Data, &statuses) != nil || len(statuses.Items) != 2 {
		t.Fatal("ordinary online status query failed", r.Code)
	}
	r = nativeProcessPost(t, "/users/update", map[string]string{"user_id": "42", "nickname": "ordinary update"})
	if r.Code != 0 {
		t.Fatal("ordinary async HTTP update was refused", r.Code)
	}
	if !eventually(func() bool {
		var nickname string
		return db.QueryRow("SELECT nickname FROM users WHERE app_key=? AND user_id='42'", nativeApp).Scan(&nickname) == nil && nickname == "ordinary update"
	}) {
		t.Fatal("ordinary async ServerPub update did not persist")
	}
	r = nativeProcessPost(t, "/messages/private/send", map[string]string{"sender_id": "42", "target_id": "43", "msg_type": "jg:text", "msg_content": `{"content":"ordinary HTTP fixture"}`})
	var sent []struct {
		ID string `json:"msg_id"`
	}
	if r.Code != 0 || json.Unmarshal(r.Data, &sent) != nil || len(sent) != 1 || sent[0].ID == "" {
		t.Fatal("ordinary HTTP private send returned no message ID")
	}
	if !eventually(func() bool {
		var n int
		_, seen := received.Load("43:" + sent[0].ID)
		return seen && db.QueryRow("SELECT COUNT(*) FROM p_hismsgs WHERE app_key=? AND msg_id=?", nativeApp, sent[0].ID).Scan(&n) == nil && n == 1
	}) {
		t.Fatal("ordinary HTTP message did not reach SDK and one history row")
	}
	_, ack := owner.SendGroupMsg(nativePrincipal, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"ordinary namespace fixture"}`), Flags: 10})
	if ack == nil || ack.Code != 0 || ack.MsgId == "" || ack.MsgSeqNo <= 0 {
		t.Fatal("ordinary group/user namespace collision was blocked")
	}
	if !eventually(func() bool {
		var n int
		_, seen := received.Load("43:" + ack.MsgId)
		return seen && db.QueryRow("SELECT COUNT(*) FROM g_hismsgs WHERE app_key=? AND msg_id=?", nativeApp, ack.MsgId).Scan(&n) == nil && n == 1
	}) {
		t.Fatal("ordinary namespace group did not reach SDK and history")
	}
	for _, q := range []string{
		"SELECT COUNT(*) FROM p_hismsgs WHERE app_key='fixture-app' AND (sender_id IN ('ma111111111111111111111111111111','ma333333333333333333333333333333') OR receiver_id IN ('ma111111111111111111111111111111','ma333333333333333333333333333333'))",
		"SELECT COUNT(*) FROM g_hismsgs WHERE app_key='fixture-app' AND conver_id='protected-group'",
		"SELECT COUNT(*) FROM inboxmsgs WHERE app_key='fixture-app' AND user_id IN ('ma111111111111111111111111111111','ma333333333333333333333333333333')",
	} {
		var n int
		if db.QueryRow(q).Scan(&n) != nil || n != 0 {
			t.Fatal("illegal native runtime message rows")
		}
	}
	if illegalReceived.Load() != 0 {
		t.Fatal("an SDK receiver observed a rejected native message")
	}
	t.Log("rebuilt image: active/disabled and established WS refused; signed HTTP and WS managed-group/history refused; ordinary HTTP/WS SDK receipt and exact history IDs verified; public API disabled throughout")
}

func nativeProcessPost(t *testing.T, path string, payload any) response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:9001/apigateway"+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	nonce, timestamp := token(), strconv.FormatInt(time.Now().UnixMilli(), 10)
	signature := sha1.Sum([]byte("fixture-secret" + nonce + timestamp))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("appkey", nativeApp)
	req.Header.Set("nonce", nonce)
	req.Header.Set("timestamp", timestamp)
	req.Header.Set("signature", hex.EncodeToString(signature[:]))
	client := http.Client{Timeout: 5 * time.Second}
	r, err := client.Do(req)
	if err != nil {
		t.Fatal("owned signed HTTP request failed", path)
	}
	defer r.Body.Close()
	var result response
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&result) != nil {
		t.Fatal("invalid owned signed HTTP response", path, r.StatusCode)
	}
	result.HTTP = r.StatusCode
	return result
}
