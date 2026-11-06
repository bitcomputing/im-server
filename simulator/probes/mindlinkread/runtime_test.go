package mindlinkread

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"im-server/commons/pbdefines/pbobjs"
	"im-server/simulator/utils"
	"im-server/simulator/wsclients"

	"google.golang.org/protobuf/proto"
)

const host = "im-f1-read-20261106-im-server-1"

type fixture struct {
	t      *testing.T
	Key    string   `json:"app_key"`
	Secret string   `json:"secret"`
	Token  string   `json:"token"`
	Group  string   `json:"group"`
	Seq    int64    `json:"seq"`
	IDs    []string `json:"ids"`
	Seen   sync.Map `json:"-"`
}

func guard(t *testing.T) {
	t.Helper()
	if os.Getenv("JIM_F1_READ_ISOLATED") != "im-f1-read-20261106" {
		t.Skip("requires the owned F1-READ stack and private evidence mount")
	}
}

func (f *fixture) post(path string, payload any, auth string) json.RawMessage {
	f.t.Helper()
	data, _ := json.Marshal(payload)
	url := "http://" + host + ":9001/apigateway" + path
	if auth != "" || path == "/admingateway/login" {
		url = "http://" + host + ":8090" + path
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		f.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if f.Key != "" {
		nonce, stamp := "read", fmt.Sprint(time.Now().UnixMilli())
		sig := sha1.Sum([]byte(f.Secret + nonce + stamp))
		req.Header.Set("appkey", f.Key)
		req.Header.Set("nonce", nonce)
		req.Header.Set("timestamp", stamp)
		req.Header.Set("signature", hex.EncodeToString(sig[:]))
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	var result struct {
		Code int
		Data json.RawMessage
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || resp.StatusCode != 200 || result.Code != 0 {
		f.t.Fatalf("fixture API %s failed: status=%d code=%d err=%v", path, resp.StatusCode, result.Code, err)
	}
	return result.Data
}

func (f *fixture) connect(user, platform string) *wsclients.WsImClient {
	f.t.Helper()
	var out struct{ Token string }
	if err := json.Unmarshal(f.post("/users/register", map[string]string{"user_id": user}, ""), &out); err != nil || out.Token == "" {
		f.t.Fatal("fixture registration failed")
	}
	if user == "read-owner" {
		f.Token = out.Token
	}
	c := wsclients.NewWsImClient("ws://"+host+":9003", f.Key, out.Token, func(msg *pbobjs.DownMsg) { f.Seen.Store(user+"/"+platform+"/"+msg.MsgId, true) }, nil, nil)
	c.Verbose = false
	c.Platform = platform
	c.DeviceId = "read-" + platform
	code, ack := c.Connect("", "")
	if code != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 {
		f.t.Fatal("fixture WS login failed")
	}
	f.t.Cleanup(c.Disconnect)
	return c
}

func seen(t *testing.T, c *wsclients.WsImClient, group string, seq int64) (int32, *pbobjs.ConverSeenState) {
	t.Helper()
	method := "qry_conver_seen"
	var req proto.Message = &pbobjs.QryConverSeenReq{TargetId: group, ChannelType: pbobjs.ChannelType_Group}
	if seq >= 0 {
		method = "set_conver_seen"
		req = &pbobjs.SetConverSeenReq{TargetId: group, ChannelType: pbobjs.ChannelType_Group, SeenSeq: seq}
	}
	data, _ := proto.Marshal(req)
	code, ack := c.Query(method, c.UserId, data)
	if ack == nil {
		t.Fatalf("seen transport failed: %v", code)
	}
	state := &pbobjs.ConverSeenState{}
	if ack.Code == 0 {
		if err := proto.Unmarshal(ack.Data, state); err != nil {
			t.Fatal(err)
		}
	}
	return ack.Code, state
}

func eventually(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("expected runtime state did not appear")
}

func TestSeenBeforeRestart(t *testing.T) {
	guard(t)
	f := &fixture{t: t}
	var auth struct{ Authorization string }
	if err := json.Unmarshal(f.post("/admingateway/login", map[string]string{"account": "admin", "password": "123456"}, ""), &auth); err != nil || auth.Authorization == "" {
		t.Fatal("fixture admin login failed")
	}
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	f.Key = "sr" + hex.EncodeToString(random)
	var app struct {
		Secret string `json:"app_secret"`
	}
	if err := json.Unmarshal(f.post("/admingateway/apps/create", map[string]string{"app_key": f.Key, "app_name": "F1 READ isolated"}, auth.Authorization), &app); err != nil || app.Secret == "" {
		t.Fatal("fixture app create failed")
	}
	f.Secret = app.Secret
	owner := f.connect("read-owner", "Web")
	pc := f.connect("read-owner", "PC")
	sender := f.connect("read-sender", "Web")
	peer := f.connect("read-peer", "Web")
	f.Group = "read-" + hex.EncodeToString(random)
	f.post("/groups/add", map[string]any{"group_id": f.Group, "group_name": "F1 READ", "member_ids": []string{owner.UserId, sender.UserId, peer.UserId}}, "")
	if code, _ := seen(t, owner, f.Group, -1); code != 0 {
		t.Fatalf("independent reading query unavailable: code=%d", code)
	}
	for _, flags := range []int32{10, 8, 72} {
		code, ack := sender.SendGroupMsg(f.Group, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"READ isolated"}`), Flags: flags})
		if code != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 {
			t.Fatal("publish failed")
		}
		f.IDs = append(f.IDs, ack.MsgId)
		eventually(t, func() bool {
			_, web := f.Seen.Load("read-owner/Web/" + ack.MsgId)
			_, desktop := f.Seen.Load("read-owner/PC/" + ack.MsgId)
			return web && desktop
		})
		eventually(t, func() bool {
			code, state := seen(t, owner, f.Group, -1)
			if code != 0 {
				t.Fatalf("query failed %d", code)
			}
			return state.LatestSeq == int64(len(f.IDs)) && state.HasNew
		})
	}
	for _, c := range []*wsclients.WsImClient{owner, pc} {
		code, history := c.QryHistoryMsgs(&pbobjs.QryHisMsgsReq{TargetId: f.Group, ChannelType: pbobjs.ChannelType_Group, Count: 20})
		if code != utils.ClientErrorCode_Success || history == nil || len(history.Msgs) != 3 {
			t.Fatal("history unavailable or missing no-count messages")
		}
		_, conver := c.QryConversation(&pbobjs.QryConverReq{TargetId: f.Group, ChannelType: pbobjs.ChannelType_Group})
		if conver == nil || conver.UnreadCount != 1 || conver.GetMsg().GetMsgId() != f.IDs[2] {
			t.Fatal("no-count broke ordinary unread or summary")
		}
	}
	data, _ := proto.Marshal(&pbobjs.QryTotalUnreadCountReq{})
	_, clear := owner.Query("clear_total_unread", owner.UserId, data)
	if clear == nil || clear.Code != 0 {
		t.Fatal("legacy unread clear failed")
	}
	if code, state := seen(t, pc, f.Group, -1); code != 0 || state.SeenSeq != 0 || !state.HasNew {
		t.Fatal("homepage unread clear marked the independent entry seen")
	}
	for _, c := range []*wsclients.WsImClient{owner, pc} {
		_, conver := c.QryConversation(&pbobjs.QryConverReq{TargetId: f.Group, ChannelType: pbobjs.ChannelType_Group})
		if conver == nil || conver.UnreadCount != 0 {
			t.Fatal("legacy unread did not clear on both devices")
		}
	}
	if code, state := seen(t, owner, f.Group, 2); code != 0 || state.SeenSeq != 2 || state.LatestSeq != 3 || !state.HasNew {
		t.Fatal("partial display confirmation failed")
	}
	for _, seq := range []int64{1, 2, 3, 2, 3} {
		if code, state := seen(t, pc, f.Group, seq); code != 0 || state.SeenSeq < 2 {
			t.Fatal("older/duplicate device read failed")
		}
	}
	if code, state := seen(t, owner, f.Group, -1); code != 0 || state.SeenSeq != 3 || state.HasNew {
		t.Fatal("device reads did not merge")
	}
	if code, _ := seen(t, owner, f.Group, 4); code == 0 {
		t.Fatal("forged future reading position accepted")
	}
	outsider := f.connect("read-outsider", "Web")
	if code, _ := seen(t, outsider, f.Group, -1); code != 13002 {
		t.Fatalf("nonmember read group metadata: code=%d", code)
	}
	if code, _ := seen(t, outsider, f.Group, 3); code != 13002 {
		t.Fatalf("nonmember advanced group metadata: code=%d", code)
	}
	spoof, _ := proto.Marshal(&pbobjs.QryConverSeenReq{TargetId: f.Group, ChannelType: pbobjs.ChannelType_Group})
	_, spoofAck := owner.Query("qry_conver_seen", sender.UserId, spoof)
	var spoofState pbobjs.ConverSeenState
	if spoofAck == nil || spoofAck.Code != 0 || proto.Unmarshal(spoofAck.Data, &spoofState) != nil || spoofState.SeenSeq != 3 {
		t.Fatal("RPC routing target overrode authenticated requester")
	}
	f.Seq = 3
	data, _ = json.Marshal(f)
	if err := os.WriteFile("/evidence/runtime-private.json", data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("app=%s group=%s ids=%v seen=3 latest=3", f.Key, f.Group, f.IDs)
}

func TestSeenAfterRestart(t *testing.T) {
	guard(t)
	f := &fixture{t: t}
	data, err := os.ReadFile("/evidence/runtime-private.json")
	if err != nil || json.Unmarshal(data, f) != nil || f.Seq != 3 {
		t.Fatal("restart fixture missing")
	}
	c := f.connect("read-owner", "Web")
	if code, state := seen(t, c, f.Group, -1); code != 0 || state.SeenSeq != 3 || state.LatestSeq != 3 || state.HasNew {
		t.Fatal("reading position did not survive IM restart")
	}
	code, history := c.QryHistoryMsgs(&pbobjs.QryHisMsgsReq{TargetId: f.Group, ChannelType: pbobjs.ChannelType_Group, Count: 20})
	if code != utils.ClientErrorCode_Success || history == nil || len(history.Msgs) != 3 {
		t.Fatal("stored no-count history did not survive restart")
	}
	for i, msg := range history.Msgs {
		if msg.MsgId != f.IDs[i] || msg.MsgSeqNo != int64(i+1) {
			t.Fatal("restored history IDs or sequence changed")
		}
	}
	_, conver := c.QryConversation(&pbobjs.QryConverReq{TargetId: f.Group, ChannelType: pbobjs.ChannelType_Group})
	if conver == nil || conver.GetMsg().GetMsgId() != f.IDs[2] || conver.UnreadCount != 0 {
		t.Fatal("summary or ordinary unread did not survive restart")
	}
	t.Logf("restored app=%s group=%s seen=3 latest=3", f.Key, f.Group)
}
