package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/connectmanager/server/codec"
	"im-server/simulator/utils"
)

func validParams() params {
	return params{Source: sourceCommit, Address: "ws://127.0.0.1:19003", AppKey: "dg2c3350172f150", Sender: "synthetic-sender", Receiver: "synthetic-receiver", G1: "synthetic-group-1", G2: "synthetic-group-2", RunID: "synthetic-run", ApprovalRef: "synthetic-approval", DurabilityRef: "synthetic-durability", MentionSQLRef: "synthetic-sql-zero", Deadline: "2026-11-10T00:00:00Z"}
}

func TestOfflineStrictInputsAndNoReplayJournal(t *testing.T) {
	var p params
	if strictJSON(strings.NewReader(`{"source_commit":"x","unknown":1}`), &p) == nil {
		t.Fatal("unknown config field accepted")
	}
	if strictJSON(strings.NewReader(`{} {}`), &p) == nil {
		t.Fatal("multiple JSON inputs accepted")
	}
	if strictJSON(strings.NewReader(`{}`+strings.Repeat(" ", 1<<20)), &p) == nil {
		t.Fatal("oversized input accepted")
	}
	p = validParams()
	p.Deadline = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	dir := t.TempDir()
	config := filepath.Join(dir, "params.json")
	journal := filepath.Join(dir, "g1.jsonl")
	b, _ := json.Marshal(p)
	if err := os.WriteFile(config, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journal, []byte("original-intent"), 0600); err != nil {
		t.Fatal(err)
	}
	err := command([]string{"-params", config, "-phase", "g1", "-execute", "-journal", journal}, strings.NewReader(`{"sender":"synthetic-token-s","receiver":"synthetic-token-r"}`), io.Discard)
	if err == nil {
		t.Fatal("existing intent journal did not block execution")
	}
	after, _ := os.ReadFile(journal)
	if string(after) != "original-intent" {
		t.Fatal("original intent overwritten")
	}
	var out bytes.Buffer
	if err = command([]string{"-params", config, "-phase", "g1"}, nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"connected":false`) || strings.Contains(out.String(), "token") {
		t.Fatal("validation mode connected or exposed credentials")
	}
}

type fakeServer struct {
	p          params
	convers    map[string]*pbobjs.Conversation
	readIDs    map[string]string
	history    map[string]*pbobjs.DownMsg
	flags      []int32
	clears     int
	loseACK    bool
	queryCode  utils.ClientErrorCode
	unfinished bool
	clearCode  int32
	noReceipt  bool
}
type fakePeer struct{ server *fakeServer }

func (f *fakePeer) seen(id string) bool { return !f.server.noReceipt && f.server.history[id] != nil }
func (f *fakePeer) Query(topic, target string, data []byte) (utils.ClientErrorCode, *codec.QueryAckMsgBody) {
	s := f.server
	if s.queryCode != 0 {
		return s.queryCode, nil
	}
	var result proto.Message
	switch topic {
	case "qry_convers":
		r := &pbobjs.QryConversationsResp{IsFinished: !s.unfinished}
		for _, v := range s.convers {
			r.Conversations = append(r.Conversations, proto.Clone(v).(*pbobjs.Conversation))
		}
		result = r
	case "qry_conver":
		var q pbobjs.QryConverReq
		if proto.Unmarshal(data, &q) != nil {
			panic("bad driver protobuf")
		}
		c := s.convers[q.TargetId]
		if c == nil {
			// Fixed4ece returns a successful empty shell for a missing conversation.
			c = &pbobjs.Conversation{TargetId: q.TargetId, ChannelType: q.ChannelType}
		}
		r := proto.Clone(c).(*pbobjs.Conversation)
		if q.IsInner {
			r.LatestReadMsgId = s.readIDs[q.TargetId]
		}
		result = r
	case "qry_hismsg_by_ids":
		var q pbobjs.QryHisMsgByIdsReq
		_ = proto.Unmarshal(data, &q)
		r := &pbobjs.DownMsgSet{}
		for _, id := range q.MsgIds {
			if m := s.history[id]; m != nil {
				r.Msgs = append(r.Msgs, m)
			}
		}
		result = r
	case "clear_unread":
		var q pbobjs.ClearUnreadReq
		_ = proto.Unmarshal(data, &q)
		s.clears++
		if len(q.Conversations) != 1 {
			panic("clear expanded scope")
		}
		v := q.Conversations[0]
		if v.TargetId != s.p.G1 || v.LatestReadMsgId != "synthetic-m1" || v.LatestReadIndex != 1 || v.LatestReadMsgTime != 1234 {
			panic("wrong legal read payload")
		}
		c := s.convers[v.TargetId]
		c.LatestReadIndex = v.LatestReadIndex
		c.UnreadCount = 0
		s.readIDs[v.TargetId] = v.LatestReadMsgId
	case "clear_total_unread":
		s.clears++
		for _, c := range s.convers {
			c.LatestReadIndex = c.LatestUnreadIndex
			c.UnreadCount = 0
			c.UnreadTag = 0
			c.Mentions = nil
		}
	default:
		panic("unapproved native topic " + topic)
	}
	var b []byte
	if result != nil {
		b, _ = proto.Marshal(result)
	}
	code := int32(0)
	if topic == "clear_unread" || topic == "clear_total_unread" {
		code = s.clearCode
	}
	return 0, &codec.QueryAckMsgBody{Data: b, Code: code}
}
func (f *fakePeer) Publish(topic, target string, data []byte) (utils.ClientErrorCode, *codec.PublishAckMsgBody) {
	if topic != "g_msg" {
		panic("unapproved publish")
	}
	s := f.server
	var up pbobjs.UpMsg
	_ = proto.Unmarshal(data, &up)
	if up.MentionInfo != nil || up.MsgType != "jg:text" {
		panic("unexpected message content/type")
	}
	s.flags = append(s.flags, up.Flags)
	id := fmt.Sprintf("synthetic-m%d", len(s.flags))
	m := &pbobjs.DownMsg{MsgId: id, TargetId: target, ChannelType: pbobjs.ChannelType_Group, MsgTime: 1234}
	s.history[id] = m
	c := s.convers[target]
	if c == nil {
		c = &pbobjs.Conversation{TargetId: target, ChannelType: pbobjs.ChannelType_Group}
		s.convers[target] = c
	}
	c.Msg = m
	if up.Flags == 10 {
		c.LatestUnreadIndex++
		c.UnreadCount++
	}
	if s.loseACK {
		return utils.ClientErrorCode_SendTimeout, nil
	}
	return 0, &codec.PublishAckMsgBody{MsgId: id}
}
func fakeEngine(phase string) (*engine, *fakeServer, *[]record) {
	p := validParams()
	s := &fakeServer{p: p, convers: map[string]*pbobjs.Conversation{}, readIDs: map[string]string{}, history: map[string]*pbobjs.DownMsg{}}
	for _, g := range []string{p.G1, p.G2} {
		s.convers[g] = &pbobjs.Conversation{TargetId: g, ChannelType: pbobjs.ChannelType_Group}
	}
	fp := &fakePeer{server: s}
	records := []record{}
	e := &engine{p: p, phase: phase, sender: fp, receivers: [2]peer{fp, fp}, now: func() time.Time { return time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC) }, log: func(r record) error { records = append(records, r); return nil }}
	return e, s, &records
}

func TestOfflineNativeScenarioAndOriginalIDRecovery(t *testing.T) {
	e, s, records := fakeEngine("g1")
	w, err := e.run(nil)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(s.flags) != "[10 8 72]" || s.clears != 1 || w.ReadID != "synthetic-m1" || w.ReadIndex != 1 || len(w.G1IDs) != 3 {
		t.Fatal("native scenario diverged")
	}
	intents := 0
	for _, r := range *records {
		if r.Event == "intent_before_native_mutation" {
			intents++
		}
	}
	if intents != 4 {
		t.Fatalf("expected four durable intent boundaries, got %d", intents)
	}
	e.phase = "verify"
	if _, err = e.run(w); err != nil {
		t.Fatal(err)
	}
	if len(s.flags) != 3 || s.clears != 1 {
		t.Fatal("recovery verification replayed a mutation")
	}
	e.phase = "g2"
	w, err = e.run(w)
	if err != nil {
		t.Fatal(err)
	}
	if w.G2ID != "synthetic-m4" || s.clears != 2 || len(s.flags) != 4 {
		t.Fatal("global-clear scenario diverged")
	}
	e.phase = "verify"
	if _, err = e.run(w); err != nil {
		t.Fatal(err)
	}
	s.history[w.G1IDs[1]] = nil
	if _, err = e.run(w); err == nil {
		t.Fatal("missing original history treated as restored")
	}
}

func TestOfflineUnknownPublishStopsEvenIfEffectOccurred(t *testing.T) {
	e, s, records := fakeEngine("g1")
	s.loseACK = true
	if _, err := e.run(nil); err == nil {
		t.Fatal("lost ACK accepted")
	}
	if len(s.flags) != 1 || s.clears != 0 || s.history["synthetic-m1"] == nil {
		t.Fatal("unknown result was replayed or misclassified as not executed")
	}
	intents := 0
	for _, r := range *records {
		if r.Event == "intent_before_native_mutation" {
			intents++
		}
	}
	if intents != 1 {
		t.Fatal("unknown publish generated another intent")
	}
}

func TestOfflineUnexpectedGlobalStateBlocksAllWrites(t *testing.T) {
	for _, kind := range []string{"mention", "other group", "unfinished", "query failure", "empty set", "missing group"} {
		t.Run(kind, func(t *testing.T) {
			e, s, _ := fakeEngine("g1")
			switch kind {
			case "mention":
				s.convers[e.p.G1].Mentions = &pbobjs.Mentions{MentionMsgCount: 1}
			case "other group":
				s.convers["unapproved"] = &pbobjs.Conversation{TargetId: "unapproved", ChannelType: pbobjs.ChannelType_Group}
			case "unfinished":
				s.unfinished = true
			case "query failure":
				s.queryCode = utils.ClientErrorCode_QueryTimeout
			case "empty set":
				s.convers = map[string]*pbobjs.Conversation{}
			case "missing group":
				delete(s.convers, e.p.G2)
			}
			if _, err := e.run(nil); err == nil {
				t.Fatal("unsafe global state accepted")
			}
			if len(s.flags) != 0 || s.clears != 0 {
				t.Fatal("mutated after failed gate")
			}
		})
	}
}

func TestOfflineBaselineRequiresOriginalHistoryForBothGroups(t *testing.T) {
	p := validParams()
	p.BaselineIDs = map[string][]string{p.G1: {"old-g1"}}
	if p.validate("baseline", true, time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)) == nil {
		t.Fatal("baseline accepted without group 2 original history")
	}
}

func TestOfflineFailedIntentOrClearStopsFollowingWrites(t *testing.T) {
	t.Run("intent journal sync failure", func(t *testing.T) {
		e, s, _ := fakeEngine("g1")
		e.log = func(r record) error {
			if r.Event == "intent_before_native_mutation" {
				return fmt.Errorf("synthetic disk failure")
			}
			return nil
		}
		if _, err := e.run(nil); err == nil || len(s.flags) != 0 || s.clears != 0 {
			t.Fatal("write proceeded without durable intent")
		}
	})
	t.Run("clear effect with rejected ack", func(t *testing.T) {
		e, s, _ := fakeEngine("g1")
		s.clearCode = 12007
		if _, err := e.run(nil); err == nil || len(s.flags) != 1 || s.clears != 1 || s.convers[e.p.G1].UnreadCount != 0 {
			t.Fatal("failed/unknown clear replayed or allowed next message")
		}
	})
	t.Run("receipt observation timeout", func(t *testing.T) {
		e, s, _ := fakeEngine("g1")
		s.noReceipt = true
		n := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
		e.now = func() time.Time { n = n.Add(2 * time.Minute); return n }
		if _, err := e.run(nil); err == nil || len(s.flags) != 1 || s.clears != 0 {
			t.Fatal("observation failure permitted next mutation")
		}
	})
}

func TestOfflineRejectUnsafeParameters(t *testing.T) {
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	for name, change := range map[string]func(*params){
		"wrong source":       func(p *params) { p.Source = "HEAD" },
		"external target":    func(p *params) { p.Address = "ws://example.invalid:19003" },
		"wrong tenant":       func(p *params) { p.AppKey = "other-app" },
		"missing receiver":   func(p *params) { p.Receiver = "" },
		"same actor":         func(p *params) { p.Receiver = p.Sender },
		"same group":         func(p *params) { p.G2 = p.G1 },
		"missing approval":   func(p *params) { p.ApprovalRef = "" },
		"missing durability": func(p *params) { p.DurabilityRef = "" },
		"expired":            func(p *params) { p.Deadline = "2026-11-08T00:00:00Z" },
	} {
		t.Run(name, func(t *testing.T) {
			p := validParams()
			change(&p)
			if p.validate("g1", true, now) == nil {
				t.Fatal("unsafe parameters accepted")
			}
		})
	}
	p := validParams()
	p.MentionSQLRef = ""
	if p.validate("g2", true, now) == nil {
		t.Fatal("global-clear without SQL evidence reference accepted")
	}
	if validParams().validate("unknown", false, now) == nil {
		t.Fatal("unknown phase accepted")
	}
	if err := validParams().validate("g1", true, now); err != nil {
		t.Fatal(err)
	}
}

func TestOfflineQueryNeverTreatsUnknownAsSuccess(t *testing.T) {
	for name, value := range map[string]struct {
		code utils.ClientErrorCode
		ack  *codec.QueryAckMsgBody
	}{
		"transport timeout": {utils.ClientErrorCode_QueryTimeout, &codec.QueryAckMsgBody{}},
		"nil ack":           {0, nil},
		"server rejection":  {0, &codec.QueryAckMsgBody{Code: 12007}},
		"bad protobuf":      {0, &codec.QueryAckMsgBody{Data: []byte{0xff}}},
	} {
		t.Run(name, func(t *testing.T) {
			if checkedQuery(value.code, value.ack, &pbobjs.Conversation{}) == nil {
				t.Fatal("failed/unknown query accepted")
			}
		})
	}
	if err := checkedQuery(0, &codec.QueryAckMsgBody{}, nil); err != nil {
		t.Fatal(err)
	}
}
