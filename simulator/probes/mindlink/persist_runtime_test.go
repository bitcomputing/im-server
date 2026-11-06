package mindlink

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"im-server/commons/pbdefines/pbobjs"
	"im-server/commons/tools"
	"im-server/simulator/utils"
	"im-server/simulator/wsclients"
)

type persistenceExpectation struct {
	Timestamp        time.Time `json:"timestamp"`
	AppKey           string    `json:"app_key"`
	UserID           string    `json:"user_id"`
	GroupID          string    `json:"group_id"`
	MessageIDs       []string  `json:"message_ids"`
	LatestID         string    `json:"latest_id"`
	ReadID           string    `json:"read_id"`
	ReadIndex        int64     `json:"read_index"`
	AllReadGroupID   string    `json:"all_read_group_id"`
	AllReadMessageID string    `json:"all_read_message_id"`
	AllReadIndex     int64     `json:"all_read_index"`
}

func persistenceRuntimeDirectory(t *testing.T) string {
	t.Helper()
	if os.Getenv("JIM_F1_RUNTIME_ISOLATED") != "im-f0-20261105" {
		t.Skip("requires inspected local F0 stack running the new F1 binary")
	}
	dir := os.Getenv("JIM_F1_RUNTIME_EVIDENCE")
	if !filepath.IsAbs(dir) || !strings.Contains(filepath.ToSlash(dir), "/.local/f1-persist/") {
		t.Fatal("F1 evidence must be inside .local/f1-persist")
	}
	return dir
}

func writePersistenceEvidence(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil || os.WriteFile(path, data, 0600) != nil {
		t.Fatal("persistence evidence could not be saved")
	}
}

func TestConversationStateBeforeRestart(t *testing.T) {
	dir := persistenceRuntimeDirectory(t)
	f := &fixture{t: t}
	login := f.post("/admingateway/login", map[string]string{"account": "admin", "password": "123456"}, "")
	var auth struct{ Authorization string }
	if login.Code != 0 || json.Unmarshal(login.Data, &auth) != nil || auth.Authorization == "" {
		t.Fatal("isolated administrator login failed")
	}
	f.key = "f1" + token()
	group := "f1-state-" + token()
	allGroup := "f1-allread-" + token()
	t.Logf("isolated runtime tenant=%s group=%s all_read_group=%s", f.key, group, allGroup)
	writePersistenceEvidence(t, filepath.Join(dir, "runtime-attempt-"+f.key+".json"), map[string]any{"app_key": f.key, "group_id": group, "all_read_group_id": allGroup, "users": []string{"f1-owner", "f1-a", "f1-b"}, "stage": "intent_before_external_creation", "timestamp": time.Now().UTC()})
	app := f.post("/admingateway/apps/create", map[string]string{"app_key": f.key, "app_name": "F1 persistence isolated"}, auth.Authorization)
	var created struct {
		Secret string `json:"app_secret"`
	}
	if app.Code != 0 || json.Unmarshal(app.Data, &created) != nil || created.Secret == "" {
		t.Fatal("isolated persistence tenant creation failed")
	}
	f.secret = created.Secret
	f.owner = f.connect("f1-owner", "Web", &f.seenWeb)
	f.ownerPC = f.connect("f1-owner", "PC", &f.seenPC)
	f.a, f.b = f.connect("f1-a", "Web", nil), f.connect("f1-b", "Web", nil)
	f.groupWith(group)
	query := func(c *wsclients.WsImClient) *pbobjs.Conversation {
		code, value := c.QryConversation(&pbobjs.QryConverReq{TargetId: group, ChannelType: pbobjs.ChannelType_Group})
		if code != utils.ClientErrorCode_Success || value == nil {
			t.Fatal("live conversation query failed")
		}
		return value
	}
	_, counted := f.a.SendGroupMsg(group, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"F1 counted"}`), Flags: 10})
	if counted == nil || counted.Code != 0 {
		t.Fatal("counted publish failed")
	}
	if !eventually(func() bool {
		return query(f.owner).UnreadCount == 1 && query(f.owner).GetMsg().GetMsgId() == counted.MsgId
	}) {
		t.Fatal("counted message was not received with unread state")
	}
	before := query(f.owner)
	if before.LatestUnreadIndex <= 0 {
		t.Fatal("counted message has no legal read index")
	}
	if f.owner.ClearUnread(&pbobjs.ClearUnreadReq{Conversations: []*pbobjs.Conversation{{TargetId: group, ChannelType: pbobjs.ChannelType_Group, LatestReadIndex: before.LatestUnreadIndex, LatestReadMsgId: counted.MsgId, LatestReadMsgTime: before.GetMsg().GetMsgTime()}}}) != utils.ClientErrorCode_Success {
		t.Fatal("counted read acknowledgement failed")
	}
	if !eventually(func() bool {
		return queryReadMetadata(t, f.owner, group).LatestReadMsgId == counted.MsgId && queryReadMetadata(t, f.ownerPC, group).LatestReadMsgId == counted.MsgId && query(f.owner).UnreadCount == 0
	}) {
		t.Fatal("legal counted read state did not synchronize")
	}
	ids := []string{counted.MsgId}
	for _, flags := range []int32{8, 72} {
		_, ack := f.a.SendGroupMsg(group, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"F1 non-counted"}`), Flags: flags})
		if ack == nil || ack.Code != 0 {
			t.Fatal("non-counted publish failed")
		}
		ids = append(ids, ack.MsgId)
		if !eventually(func() bool {
			_, web := f.seenWeb.Load(ack.MsgId)
			_, pc := f.seenPC.Load(ack.MsgId)
			return web && pc && f.history(f.owner, group, ack.MsgId) && f.history(f.ownerPC, group, ack.MsgId) && query(f.owner).GetMsg().GetMsgId() == ack.MsgId && query(f.ownerPC).GetMsg().GetMsgId() == ack.MsgId
		}) {
			t.Fatal("non-counted receipt/history/summary mismatch")
		}
	}
	latest := query(f.owner)
	if latest.UnreadCount != 0 || queryReadMetadata(t, f.owner, group).LatestReadMsgId != counted.MsgId {
		t.Fatal("non-counted messages changed legitimate read state")
	}
	expected := persistenceExpectation{Timestamp: time.Now().UTC(), AppKey: f.key, UserID: "f1-owner", GroupID: group, MessageIDs: ids, LatestID: ids[len(ids)-1], ReadID: counted.MsgId, ReadIndex: latest.LatestReadIndex}
	f.groupWith(allGroup)
	_, allAck := f.a.SendGroupMsg(allGroup, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"F1 all-read pending snapshot"}`), Flags: 10})
	if allAck == nil || allAck.Code != 0 {
		t.Fatal("clear-all control publish failed")
	}
	queryAll := func(c *wsclients.WsImClient) *pbobjs.Conversation {
		code, v := c.QryConversation(&pbobjs.QryConverReq{TargetId: allGroup, ChannelType: pbobjs.ChannelType_Group})
		if code != utils.ClientErrorCode_Success || v == nil {
			t.Fatal("all-read conversation query failed")
		}
		return v
	}
	if !eventually(func() bool { return queryAll(f.owner).UnreadCount == 1 && queryAll(f.ownerPC).UnreadCount == 1 }) {
		t.Fatal("all-read control unread was not received")
	}
	data, _ := tools.PbMarshal(&pbobjs.QryTotalUnreadCountReq{})
	clearCode, clearAck := f.owner.Query("clear_total_unread", f.owner.UserId, data)
	if clearCode != utils.ClientErrorCode_Success || clearAck == nil || clearAck.Code != 0 {
		t.Fatal("clear-total-unread RPC failed")
	}
	if !eventually(func() bool {
		return queryAll(f.owner).UnreadCount == 0 && queryAll(f.ownerPC).UnreadCount == 0 && queryAll(f.owner).LatestReadIndex > 0
	}) {
		t.Fatal("all-read state did not synchronize")
	}
	expected.AllReadGroupID = allGroup
	expected.AllReadMessageID = allAck.MsgId
	expected.AllReadIndex = queryAll(f.owner).LatestReadIndex
	expected.Timestamp = time.Now().UTC()
	writePersistenceEvidence(t, filepath.Join(dir, "runtime-private.json"), map[string]string{"app_key": f.key, "owner_token": f.owner.Token})
	writePersistenceEvidence(t, filepath.Join(dir, "runtime-before.json"), expected)
}

func TestConversationStateAfterRestart(t *testing.T) {
	dir := persistenceRuntimeDirectory(t)
	var expected persistenceExpectation
	data, err := os.ReadFile(filepath.Join(dir, "runtime-before.json"))
	if err != nil || json.Unmarshal(data, &expected) != nil || expected.LatestID == "" || expected.ReadIndex <= 0 {
		t.Fatal("valid persistence baseline unavailable")
	}
	if time.Since(expected.Timestamp) < 70*time.Second {
		t.Fatal("blocked: actual persistence interval must elapse before restart validation")
	}
	var private map[string]string
	data, err = os.ReadFile(filepath.Join(dir, "runtime-private.json"))
	if err != nil || json.Unmarshal(data, &private) != nil || private["owner_token"] == "" {
		t.Fatal("private persistence fixture unavailable")
	}
	c := wsclients.NewWsImClient(wsURL, expected.AppKey, private["owner_token"], func(*pbobjs.DownMsg) {}, nil, nil)
	c.Verbose = false
	code, ack := c.Connect("", "")
	if code != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 {
		t.Fatal("restored client login failed")
	}
	defer c.Disconnect()
	f := &fixture{t: t, key: expected.AppKey}
	for _, id := range expected.MessageIDs {
		if !f.history(c, expected.GroupID, id) {
			t.Fatal("expected counted/non-counted message absent from restored history")
		}
	}
	code, conver := c.QryConversation(&pbobjs.QryConverReq{TargetId: expected.GroupID, ChannelType: pbobjs.ChannelType_Group})
	readState := queryReadMetadata(t, c, expected.GroupID)
	ok := code == utils.ClientErrorCode_Success && conver != nil && conver.GetMsg().GetMsgId() == expected.LatestID && conver.LatestReadIndex == expected.ReadIndex && readState.LatestReadMsgId == expected.ReadID && readState.LatestReadIndex == expected.ReadIndex && conver.UnreadCount == 0
	if expected.AllReadGroupID == "" || expected.AllReadIndex <= 0 || !f.history(c, expected.AllReadGroupID, expected.AllReadMessageID) {
		t.Fatal("all-read persistence baseline/history unavailable")
	}
	allCode, allConver := c.QryConversation(&pbobjs.QryConverReq{TargetId: expected.AllReadGroupID, ChannelType: pbobjs.ChannelType_Group})
	allOK := allCode == utils.ClientErrorCode_Success && allConver != nil && allConver.GetMsg().GetMsgId() == expected.AllReadMessageID && allConver.LatestReadIndex == expected.AllReadIndex && allConver.UnreadCount == 0
	ok = ok && allOK
	writePersistenceEvidence(t, filepath.Join(dir, "runtime-after.json"), map[string]any{"passed": ok, "history_messages_verified": len(expected.MessageIDs) + 1, "summary_id": conver.GetMsg().GetMsgId(), "read_id": readState.GetLatestReadMsgId(), "read_index": readState.GetLatestReadIndex(), "unread": conver.GetUnreadCount(), "ordinary_query_read_id": conver.GetLatestReadMsgId(), "read_metadata_source": "qry_conver IsInner=true", "all_read_passed": allOK, "all_read_index": allConver.GetLatestReadIndex(), "all_read_unread": allConver.GetUnreadCount()})
	if !ok {
		t.Fatal("restored last message or legal read state mismatch")
	}
}
