package mindlink

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"im-server/commons/pbdefines/pbobjs"
	"im-server/commons/tools"
	"im-server/simulator/utils"
	"im-server/simulator/wsclients"
)

// Only the separately provisioned F0 stack is supported; there is no target override.
const apiURL = "http://127.0.0.1:19501/apigateway"
const wsURL = "ws://127.0.0.1:19503"

type result struct {
	Name    string         `json:"name"`
	Status  string         `json:"status"`
	Details map[string]any `json:"details"`
}

type response struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	HTTP int             `json:"http_status"`
}

type fixture struct {
	t           *testing.T
	key, secret string
	owner, a, b *wsclients.WsImClient
	ownerPC     *wsclients.WsImClient
	seenWeb     sync.Map
	seenPC      sync.Map
	results     []result
	group       string
}

func token() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (f *fixture) post(path string, payload any, authorization string) response {
	f.t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		f.t.Fatal(err)
	}
	url := apiURL + path
	if strings.HasPrefix(path, "/admingateway/") {
		url = "http://127.0.0.1:18590" + path
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		f.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if f.key != "" {
		nonce, timestamp := token(), strconv.FormatInt(time.Now().UnixMilli(), 10)
		sig := sha1.Sum([]byte(f.secret + nonce + timestamp))
		req.Header.Set("appkey", f.key)
		req.Header.Set("nonce", nonce)
		req.Header.Set("timestamp", timestamp)
		req.Header.Set("signature", hex.EncodeToString(sig[:]))
	}
	client := &http.Client{Timeout: 10 * time.Second}
	r, err := client.Do(req)
	if err != nil {
		f.t.Fatalf("request %s failed: %v", path, err)
	}
	defer r.Body.Close()
	out := response{HTTP: r.StatusCode}
	if r.StatusCode == http.StatusNotFound {
		return out
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&out); err != nil {
		f.t.Fatalf("decode %s HTTP %d: %v", path, r.StatusCode, err)
	}
	return out
}

func (f *fixture) mustPost(path string, payload any) response {
	f.t.Helper()
	r := f.post(path, payload, "")
	if r.HTTP != 200 || r.Code != 0 {
		f.t.Fatalf("control request %s failed: HTTP %d code %d", path, r.HTTP, r.Code)
	}
	return r
}

func (f *fixture) record(t *testing.T, name string, ok bool, details map[string]any) {
	t.Helper()
	status := "passed"
	if !ok {
		status = "failed"
		t.Errorf("product capability failed: %s", name)
	}
	f.results = append(f.results, result{name, status, details})
}

func (f *fixture) connect(id, platform string, seen *sync.Map) *wsclients.WsImClient {
	f.t.Helper()
	r := f.mustPost("/users/register", map[string]any{"user_id": id, "nickname": "isolated F0"})
	var data struct{ Token string }
	if err := json.Unmarshal(r.Data, &data); err != nil || data.Token == "" {
		f.t.Fatal("registration returned no token")
	}
	c := wsclients.NewWsImClient(wsURL, f.key, data.Token, func(msg *pbobjs.DownMsg) {
		if seen != nil {
			seen.Store(msg.MsgId, msg.Flags)
		}
	}, nil, nil)
	c.Verbose, c.Platform, c.DeviceId = false, platform, "f0-"+platform
	code, ack := c.Connect("", "")
	if code != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 {
		f.t.Fatal("isolated WS connection failed")
	}
	f.t.Cleanup(c.Disconnect)
	return c
}

func (f *fixture) groupWith(id string) string {
	f.mustPost("/groups/add", map[string]any{"group_id": id, "group_name": "isolated F0 triple", "member_ids": []string{f.owner.UserId, f.a.UserId, f.b.UserId}})
	return id
}

func (f *fixture) mute(on int) {
	f.mustPost("/groups/groupmembermute/set", map[string]any{"group_id": f.group, "member_ids": []string{f.owner.UserId}, "is_mute": on})
}

func (f *fixture) history(c *wsclients.WsImClient, group, id string) bool {
	code, set := c.QryHistoryMsgs(&pbobjs.QryHisMsgsReq{TargetId: group, ChannelType: pbobjs.ChannelType_Group, Count: 100})
	if code != utils.ClientErrorCode_Success || set == nil {
		f.t.Fatal("history query failed; absence cannot be treated as refusal")
	}
	for _, msg := range set.Msgs {
		if msg.MsgId == id {
			return true
		}
	}
	return false
}

func queryReadMetadata(t *testing.T, c *wsclients.WsImClient, group string) *pbobjs.Conversation {
	t.Helper()
	code, value := c.QryConversation(&pbobjs.QryConverReq{TargetId: group, ChannelType: pbobjs.ChannelType_Group, IsInner: true})
	if code != utils.ClientErrorCode_Success || value == nil {
		t.Fatal("conversation read metadata query failed")
	}
	return value
}

func eventually(check func() bool) bool {
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func TestMindlinkNativeProbes(t *testing.T) {
	if os.Getenv("JIM_F0_ISOLATED") != "im-f0-20261105" {
		t.Skip("not_covered: provision and inspect the isolated F0 stack before opting in")
	}
	output := os.Getenv("JIM_F0_REPORT")
	if !filepath.IsAbs(output) || !strings.Contains(filepath.ToSlash(output), "/.local/f0-im/") {
		t.Fatal("evidence must be an absolute path inside .local/f0-im")
	}
	f := &fixture{t: t}
	t.Cleanup(func() {
		data, err := json.MarshalIndent(map[string]any{"task": "DG-20261104-003-F0-IM", "timestamp": time.Now().UTC(), "results": f.results}, "", "  ")
		if err != nil || os.WriteFile(output, data, 0600) != nil {
			t.Error("failed to save probe evidence")
		}
	})
	login := f.post("/admingateway/login", map[string]string{"account": "admin", "password": "123456"}, "")
	var auth struct{ Authorization string }
	if login.Code != 0 || json.Unmarshal(login.Data, &auth) != nil || auth.Authorization == "" {
		t.Fatal("isolated administrator login failed")
	}
	f.key = "f0" + token()
	app := f.post("/admingateway/apps/create", map[string]string{"app_key": f.key, "app_name": "F0 isolated probes"}, auth.Authorization)
	var appData struct {
		Secret string `json:"app_secret"`
	}
	if app.Code != 0 || json.Unmarshal(app.Data, &appData) != nil || appData.Secret == "" {
		t.Fatal("isolated tenant creation failed")
	}
	f.secret = appData.Secret
	f.owner = f.connect("f0-owner", "Web", &f.seenWeb)
	f.ownerPC = f.connect("f0-owner", "PC", &f.seenPC)
	f.a, f.b = f.connect("f0-a", "Web", nil), f.connect("f0-b", "Web", nil)
	f.group = f.groupWith("f0-mute-" + token())
	private, _ := json.Marshal(map[string]string{"app_key": f.key, "mute_group": f.group, "owner_token": f.owner.Token})
	if os.WriteFile(filepath.Join(filepath.Dir(output), "fixture.private.json"), private, 0600) != nil {
		t.Fatal("fixture evidence save failed")
	}

	t.Run("non_count", func(t *testing.T) {
		group := f.groupWith("f0-count-" + token())
		code, control := f.a.SendGroupMsg(group, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"F0 counted control"}`), Flags: 10})
		if code != utils.ClientErrorCode_Success || control == nil || control.Code != 0 {
			t.Fatal("counted control publish failed")
		}
		conver := func(c *wsclients.WsImClient) *pbobjs.Conversation {
			code, v := c.QryConversation(&pbobjs.QryConverReq{TargetId: group, ChannelType: pbobjs.ChannelType_Group})
			if code != utils.ClientErrorCode_Success || v == nil {
				t.Fatal("conversation query failed")
			}
			return v
		}
		f.record(t, "counted_control_two_sdk_sessions", eventually(func() bool { return conver(f.owner).UnreadCount == 1 && conver(f.ownerPC).UnreadCount == 1 }), nil)
		if f.owner.ClearUnread(&pbobjs.ClearUnreadReq{Conversations: []*pbobjs.Conversation{{TargetId: group, ChannelType: pbobjs.ChannelType_Group}}}) != utils.ClientErrorCode_Success {
			t.Fatal("control clear unread failed")
		}
		f.record(t, "clear_unread_two_sdk_sessions", eventually(func() bool { return conver(f.owner).UnreadCount == 0 && conver(f.ownerPC).UnreadCount == 0 }), nil)
		var nonCountIDs []string
		for _, flags := range []int32{8, 72} {
			_, ack := f.a.SendGroupMsg(group, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"F0 non-counted"}`), Flags: flags})
			if ack == nil || ack.Code != 0 {
				t.Fatal("non-counted publish failed")
			}
			nonCountIDs = append(nonCountIDs, ack.MsgId)
			delivered := eventually(func() bool { _, a := f.seenWeb.Load(ack.MsgId); _, b := f.seenPC.Load(ack.MsgId); return a && b })
			history := eventually(func() bool { return f.history(f.owner, group, ack.MsgId) && f.history(f.ownerPC, group, ack.MsgId) })
			updated := eventually(func() bool {
				return conver(f.owner).GetMsg().GetMsgId() == ack.MsgId && conver(f.ownerPC).GetMsg().GetMsgId() == ack.MsgId
			})
			web, pc := conver(f.owner), conver(f.ownerPC)
			_, unread := f.owner.QryTotalUnreadCount(&pbobjs.QryTotalUnreadCountReq{})
			downFlags, _ := f.seenWeb.Load(ack.MsgId)
			f.record(t, "non_count_history_summary_sdk", delivered && history && updated && web.UnreadCount == 0 && pc.UnreadCount == 0 && unread != nil && unread.TotalCount == 0, map[string]any{"flags": flags, "delivered_flags": downFlags, "message_id": ack.MsgId, "group_id": group, "delivered_both": delivered, "history_both": history, "summary_both": updated, "web_unread": web.UnreadCount, "pc_unread": pc.UnreadCount, "web_latest_read_id": web.LatestReadMsgId, "pc_latest_read_id": pc.LatestReadMsgId})
		}
		r := f.mustPost("/messages/group/send", map[string]any{"sender_id": f.a.UserId, "target_id": group, "msg_type": "jg:text", "msg_content": `{"content":"F0 HTTP non-counted"}`, "is_count": false, "is_storage": true})
		var sent []struct {
			ID string `json:"msg_id"`
		}
		if json.Unmarshal(r.Data, &sent) != nil || len(sent) != 1 || sent[0].ID == "" {
			t.Fatal("HTTP non-counted publish returned no ID")
		}
		id := sent[0].ID
		nonCountIDs = append(nonCountIDs, id)
		ok := eventually(func() bool {
			return f.history(f.owner, group, id) && conver(f.owner).GetMsg().GetMsgId() == id && conver(f.ownerPC).GetMsg().GetMsgId() == id
		})
		f.record(t, "http_non_count_history_summary_sdk", ok && conver(f.owner).UnreadCount == 0 && conver(f.ownerPC).UnreadCount == 0, map[string]any{"message_id": id, "group_id": group})
		var startTime int64
		var pageIDs []string
		complete := true
		for i := len(nonCountIDs) - 1; i >= 0; i-- {
			code, page := f.owner.QryHistoryMsgs(&pbobjs.QryHisMsgsReq{TargetId: group, ChannelType: pbobjs.ChannelType_Group, Count: 1, StartTime: startTime})
			if code != utils.ClientErrorCode_Success || page == nil || len(page.Msgs) != 1 {
				t.Fatal("history page unavailable")
			}
			msg := page.Msgs[0]
			pageIDs = append(pageIDs, msg.MsgId)
			complete = complete && msg.MsgId == nonCountIDs[i] && msg.Flags&2 == 0
			startTime = msg.MsgTime // Storage already uses a strict less-than cursor.
		}
		f.record(t, "non_count_history_pagination", complete, map[string]any{"expected_send_order": nonCountIDs, "actual_page_order": pageIDs})
		latest := conver(f.owner)
		clear := f.owner.ClearUnread(&pbobjs.ClearUnreadReq{Conversations: []*pbobjs.Conversation{{TargetId: group, ChannelType: pbobjs.ChannelType_Group, LatestReadIndex: latest.LatestUnreadIndex, LatestReadMsgId: id, LatestReadMsgTime: latest.GetMsg().GetMsgTime()}}})
		seen := eventually(func() bool {
			return queryReadMetadata(t, f.owner, group).LatestReadMsgId == id && queryReadMetadata(t, f.ownerPC, group).LatestReadMsgId == id
		})
		f.record(t, "non_count_seen_cursor_via_clear_unread", clear == utils.ClientErrorCode_Success && seen, map[string]any{"clear_code": clear, "requested_message_id": id, "web_read_id": queryReadMetadata(t, f.owner, group).LatestReadMsgId, "pc_read_id": queryReadMetadata(t, f.ownerPC, group).LatestReadMsgId, "read_metadata_source": "qry_conver IsInner=true; ordinary fillConvers omits read message ID"})
	})

	t.Run("muted_owner_paths", func(t *testing.T) {
		f.mute(1)
		for _, kind := range []string{"jg:text", "jg:image", "jg:voice", "jg:video", "jg:file"} {
			_, ack := f.owner.SendGroupMsg(f.group, &pbobjs.UpMsg{MsgType: kind, MsgContent: []byte(`{"content":"F0 muted attempt"}`), Flags: 10})
			if ack == nil {
				t.Fatal("WS mutation returned no ACK")
			}
			f.record(t, "muted_ws_"+kind, ack.Code == 13004 && !f.history(f.a, f.group, ack.MsgId), map[string]any{"ack_code": ack.Code, "message_id": ack.MsgId})
			r := f.post("/messages/group/send", map[string]any{"sender_id": f.owner.UserId, "target_id": f.group, "msg_type": kind, "msg_content": `{"content":"F0 signed HTTP muted attempt"}`}, "")
			if r.Code == 13004 {
				f.record(t, "muted_http_"+kind, true, map[string]any{"http_code": r.Code})
				continue
			}
			var sent []struct {
				ID string `json:"msg_id"`
			}
			if json.Unmarshal(r.Data, &sent) != nil || len(sent) != 1 || sent[0].ID == "" {
				t.Fatal("HTTP mutation returned no message ID")
			}
			persisted := eventually(func() bool { return f.history(f.a, f.group, sent[0].ID) })
			f.record(t, "muted_http_"+kind, false, map[string]any{"http_code": r.Code, "message_id": sent[0].ID, "persisted": persisted})
		}
		_, stream := f.owner.SendGroupMsg(f.group, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"F0 stream flag"}`), Flags: 1034})
		if stream == nil {
			t.Fatal("stream-flag mutation returned no ACK")
		}
		f.record(t, "muted_ws_stream_flag", stream.Code == 13004 && !f.history(f.a, f.group, stream.MsgId), map[string]any{"ack_code": stream.Code, "message_id": stream.MsgId})
		f.mustPost("/groups/groupmemberallow/set", map[string]any{"group_id": f.group, "member_ids": []string{f.owner.UserId}, "is_allow": 1})
		_, allowed := f.owner.SendGroupMsg(f.group, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"F0 allowlisted muted attempt"}`), Flags: 10})
		if allowed == nil {
			t.Fatal("allowlist mutation returned no ACK")
		}
		f.record(t, "mute_precedes_allowlist", allowed.Code == 13004 && !f.history(f.a, f.group, allowed.MsgId), map[string]any{"ack_code": allowed.Code, "message_id": allowed.MsgId, "note": "allowlist is not a native owner/admin role"})
		data, _ := tools.PbMarshal(&pbobjs.GroupMemberMuteReq{GroupId: f.group, MemberIds: []string{f.owner.UserId}, IsMute: 0})
		_, unmute := f.owner.Query("group_member_mute", f.group, data)
		if unmute == nil {
			t.Fatal("self-unmute query returned no ACK")
		}
		_, after := f.owner.SendGroupMsg(f.group, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"F0 after self unmute"}`), Flags: 10})
		if after == nil {
			t.Fatal("post-unmute publish returned no ACK")
		}
		persisted := eventually(func() bool { return f.history(f.a, f.group, after.MsgId) })
		f.record(t, "owner_cannot_self_unmute", unmute.Code != 0 && after.Code == 13004 && !persisted, map[string]any{"unmute_ack": unmute.Code, "send_ack": after.Code, "persisted": persisted, "message_id": after.MsgId})
		f.mute(1)
		f.mute(0)
		_, control := f.owner.SendGroupMsg(f.group, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"F0 authorized unmute control"}`), Flags: 10})
		if control == nil {
			t.Fatal("authorized unmute control returned no ACK")
		}
		f.record(t, "http_unmute_restores_ws_send", control.Code == 0 && eventually(func() bool { return f.history(f.a, f.group, control.MsgId) }), map[string]any{"ack_code": control.Code, "message_id": control.MsgId})
	})
	for _, name := range []string{"real_web_app_ui", "phone_badge_sound_push", "offline_provider_push", "entry_red_dot_and_cross_device_seen_cursor", "native_group_owner_admin_roles", "group_stream_http_and_released_sdk", "business_F0_policy_projection"} {
		f.results = append(f.results, result{name, "not_covered", nil})
	}
}

// This runs separately after an operator restarts only the inspected F0 IM container.
func TestRestartedConversationSummary(t *testing.T) {
	if os.Getenv("JIM_F0_RESTARTED") != "im-f0-20261105" {
		t.Skip("not_covered: restart the owned isolated F0 container before opting in")
	}
	output := os.Getenv("JIM_F0_REPORT")
	if !filepath.IsAbs(output) || !strings.Contains(filepath.ToSlash(output), "/.local/f0-im/") {
		t.Fatal("evidence must be inside .local/f0-im")
	}
	var private map[string]string
	data, err := os.ReadFile(filepath.Join(filepath.Dir(output), "fixture.private.json"))
	if err != nil || json.Unmarshal(data, &private) != nil || private["app_key"] == "" || private["owner_token"] == "" {
		t.Fatal("isolated fixture unavailable")
	}
	var report struct {
		Results   []result
		Timestamp time.Time
	}
	data, err = os.ReadFile(output)
	if err != nil || json.Unmarshal(data, &report) != nil {
		t.Fatal("baseline report unavailable")
	}
	if report.Timestamp.IsZero() || time.Since(report.Timestamp) < 70*time.Second {
		t.Fatal("blocked: allow more than the 60-second persistence period before the isolated restart")
	}
	var group, id string
	for _, r := range report.Results {
		if r.Name == "http_non_count_history_summary_sdk" {
			group, _ = r.Details["group_id"].(string)
			id, _ = r.Details["message_id"].(string)
		}
	}
	if group == "" || id == "" {
		t.Fatal("baseline message unavailable")
	}
	f := &fixture{t: t, key: private["app_key"]}
	c := wsclients.NewWsImClient(wsURL, f.key, private["owner_token"], func(*pbobjs.DownMsg) {}, nil, nil)
	c.Verbose = false
	code, ack := c.Connect("", "")
	if code != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 {
		t.Fatal("post-restart WS login failed")
	}
	defer c.Disconnect()
	f.record(t, "history_survives_restart", f.history(c, group, id), nil)
	code, conver := c.QryConversation(&pbobjs.QryConverReq{TargetId: group, ChannelType: pbobjs.ChannelType_Group})
	f.record(t, "summary_survives_restart", code == utils.ClientErrorCode_Success && conver != nil && conver.GetMsg().GetMsgId() == id, map[string]any{"query_code": code, "summary_found": conver != nil && conver.Msg != nil})
	data, err = json.MarshalIndent(f.results, "", "  ")
	if err != nil || os.WriteFile(filepath.Join(filepath.Dir(output), "restart-report.json"), data, 0600) != nil {
		t.Error("restart evidence save failed")
	}
}
