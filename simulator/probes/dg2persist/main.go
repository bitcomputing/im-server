package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/connectmanager/server/codec"
	"im-server/simulator/utils"
	"im-server/simulator/wsclients"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

const sourceCommit = "4ecec887fb38d2d81af4bf2773b05d304e8ced97"

type params struct {
	Source        string              `json:"source_commit"`
	Address       string              `json:"ws_address"`
	AppKey        string              `json:"app_key"`
	Sender        string              `json:"sender_id"`
	Receiver      string              `json:"receiver_id"`
	G1            string              `json:"group_1"`
	G2            string              `json:"group_2"`
	RunID         string              `json:"run_id"`
	ApprovalRef   string              `json:"approved_phase_ref"`
	DurabilityRef string              `json:"protected_durability_ref"`
	MentionSQLRef string              `json:"zero_mention_sql_ref"`
	Deadline      string              `json:"new_action_cutoff_utc"`
	BaselineIDs   map[string][]string `json:"baseline_history_ids"`
}

func (p params) validate(phase string, execute bool, now time.Time) error {
	if phase != "baseline" && phase != "g1" && phase != "g2" && phase != "verify" {
		return errors.New("unknown phase")
	}
	if p.Source != sourceCommit || p.Address != "ws://127.0.0.1:19003" || p.AppKey != "dg2c3350172f150" {
		return errors.New("fixed source, loopback address or tenant mismatch")
	}
	for _, s := range []string{p.Sender, p.Receiver, p.G1, p.G2, p.RunID} {
		if s == "" || len(s) > 128 || !utf8.ValidString(s) || strings.TrimSpace(s) != s || strings.IndexFunc(s, unicode.IsControl) >= 0 {
			return errors.New("missing or invalid identity parameter; obtain actual business return values")
		}
	}
	if p.Sender == p.Receiver || p.G1 == p.G2 {
		return errors.New("distinct existing actors and groups required")
	}
	for g, ids := range p.BaselineIDs {
		if (g != p.G1 && g != p.G2) || len(ids) == 0 {
			return errors.New("baseline history outside scope")
		}
		for _, id := range ids {
			if id == "" {
				return errors.New("empty baseline message ID")
			}
		}
	}
	if execute {
		end, err := time.Parse(time.RFC3339, p.Deadline)
		if err != nil || !now.Add(30*time.Second).Before(end) || p.ApprovalRef == "" || p.DurabilityRef == "" {
			return errors.New("phase references or at least 30 seconds before new-action cutoff required; these do not grant authorization")
		}
		if phase == "g2" && p.MentionSQLRef == "" {
			return errors.New("global clear requires separately verified zero-mention SQL evidence reference")
		}
		if phase == "baseline" && (len(p.BaselineIDs[p.G1]) == 0 || len(p.BaselineIDs[p.G2]) == 0) {
			return errors.New("baseline requires actual existing history IDs for both groups")
		}
	}
	return nil
}

func (p params) scope() string {
	b, _ := json.Marshal([]string{p.Source, p.Address, p.AppKey, p.Sender, p.Receiver, p.G1, p.G2, p.RunID})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func checkedQuery(code utils.ClientErrorCode, ack *codec.QueryAckMsgBody, dst proto.Message) error {
	if code != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 {
		return errors.New("native query rejected or result unknown; no mutation retry")
	}
	if dst != nil {
		if err := proto.Unmarshal(ack.Data, dst); err != nil {
			return errors.New("invalid native query response")
		}
	}
	return nil
}

type peer interface {
	Query(string, string, []byte) (utils.ClientErrorCode, *codec.QueryAckMsgBody)
	Publish(string, string, []byte) (utils.ClientErrorCode, *codec.PublishAckMsgBody)
	seen(string) bool
}

type native struct {
	*wsclients.WsImClient
	received       sync.Map
	group1, group2 string
}

func (n *native) seen(id string) bool { _, ok := n.received.Load(id); return ok }

type view struct {
	LatestID    string `json:"latest_id"`
	ReadID      string `json:"read_id"`
	ReadIndex   int64  `json:"read_index,string"`
	UnreadIndex int64  `json:"unread_index,string"`
	Unread      int64  `json:"unread,string"`
}
type expectation struct {
	G1IDs       []string `json:"group_1_ids"`
	ReadID      string   `json:"read_id"`
	ReadIndex   int64    `json:"read_index,string"`
	G2ID        string   `json:"group_2_id,omitempty"`
	G2ReadIndex int64    `json:"group_2_read_index,string"`
}
type record struct {
	Event    string       `json:"event"`
	Phase    string       `json:"phase"`
	Scope    string       `json:"scope_sha256"`
	Step     string       `json:"step,omitempty"`
	ID       string       `json:"message_id,omitempty"`
	At       string       `json:"observed_utc"`
	Expected *expectation `json:"expected,omitempty"`
	Views    []view       `json:"views,omitempty"`
}

func strictJSON(r io.Reader, dst any) error {
	b, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return errors.New("oversized or unreadable JSON input")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return errors.New("invalid JSON input")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("trailing or oversized JSON input")
	}
	return nil
}

func readExpected(path, scope string) (*expectation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("expected journal unavailable")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return nil, errors.New("oversized or unreadable expected journal")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	var last record
	for {
		var r record
		err = d.Decode(&r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("incomplete expected journal; do not replay")
		}
		last = r
	}
	if last.Event != "native_observed_DB_restart_unverified" || last.Scope != scope || (last.Phase != "g1" && last.Phase != "g2") || last.Expected == nil || len(last.Expected.G1IDs) != 3 || last.Expected.ReadID == "" || last.Expected.ReadIndex <= 0 {
		return nil, errors.New("expected journal lacks a matching successful native observation")
	}
	unique := map[string]bool{}
	for _, id := range last.Expected.G1IDs {
		if id == "" || unique[id] {
			return nil, errors.New("missing original message ID")
		}
		unique[id] = true
	}
	if last.Expected.ReadID != last.Expected.G1IDs[0] || (last.Expected.G2ID != "" && (last.Expected.G2ReadIndex <= 0 || unique[last.Expected.G2ID])) {
		return nil, errors.New("inconsistent original expectation")
	}
	return last.Expected, nil
}

type engine struct {
	p         params
	phase     string
	sender    peer
	receivers [2]peer
	log       func(record) error
	now       func() time.Time
}

func (e *engine) before() error { return e.p.validate(e.phase, true, e.now()) }
func (e *engine) emit(r record) error {
	r.Scope = e.p.scope()
	r.Phase = e.phase
	r.At = e.now().UTC().Format(time.RFC3339Nano)
	return e.log(r)
}
func (e *engine) query(c peer, topic, target string, in, out proto.Message) error {
	if err := e.before(); err != nil {
		return err
	}
	b, err := proto.Marshal(in)
	if err != nil {
		return errors.New("cannot encode query")
	}
	code, ack := c.Query(topic, target, b)
	return checkedQuery(code, ack, out)
}
func noMention(m *pbobjs.Mentions) bool {
	return m == nil || (!m.IsMentioned && m.MentionMsgCount == 0 && len(m.Senders) == 0 && len(m.MentionMsgs) == 0)
}
func (e *engine) views(group string) ([2]view, error) {
	var views [2]view
	for i, c := range e.receivers {
		var ordinary, inner pbobjs.Conversation
		if err := e.query(c, "qry_conver", e.p.Receiver, &pbobjs.QryConverReq{TargetId: group, ChannelType: pbobjs.ChannelType_Group}, &ordinary); err != nil {
			return views, err
		}
		if err := e.query(c, "qry_conver", e.p.Receiver, &pbobjs.QryConverReq{TargetId: group, ChannelType: pbobjs.ChannelType_Group, IsInner: true}, &inner); err != nil {
			return views, err
		}
		if ordinary.TargetId != group || inner.TargetId != group || ordinary.ChannelType != pbobjs.ChannelType_Group || inner.ChannelType != pbobjs.ChannelType_Group || ordinary.SubChannel != "" || inner.SubChannel != "" || !noMention(ordinary.Mentions) || ordinary.LatestReadIndex != inner.LatestReadIndex || ordinary.LatestUnreadIndex != inner.LatestUnreadIndex {
			return views, errors.New("conversation identity, mention or metadata mismatch")
		}
		views[i] = view{LatestID: ordinary.GetMsg().GetMsgId(), ReadID: inner.LatestReadMsgId, ReadIndex: inner.LatestReadIndex, UnreadIndex: inner.LatestUnreadIndex, Unread: ordinary.UnreadCount}
	}
	return views, e.emit(record{Event: "conversation_snapshot", Step: group, Views: views[:]})
}
func (e *engine) allowedConversations(pendingGroup string) error {
	for _, c := range e.receivers {
		var resp pbobjs.QryConversationsResp
		if err := e.query(c, "qry_convers", e.p.Receiver, &pbobjs.QryConversationsReq{StartTime: math.MaxInt64, Count: 100}, &resp); err != nil {
			return err
		}
		if !resp.IsFinished {
			return errors.New("conversation set not exhaustively observed; stop global clear")
		}
		seen := map[string]bool{}
		for _, v := range resp.Conversations {
			if v == nil || (v.TargetId != e.p.G1 && v.TargetId != e.p.G2) || v.ChannelType != pbobjs.ChannelType_Group || v.SubChannel != "" || seen[v.TargetId] || !noMention(v.Mentions) {
				return errors.New("unapproved conversation or mention state")
			}
			seen[v.TargetId] = true
			want := int64(0)
			if v.TargetId == pendingGroup {
				want = 1
			}
			if v.UnreadCount != want || v.UnreadTag != 0 {
				return errors.New("unexpected unread state before mutation")
			}
		}
		if !seen[e.p.G1] || !seen[e.p.G2] {
			return errors.New("both existing approved conversations must actually be observed; do not create missing objects")
		}
	}
	return nil
}
func (e *engine) history(c peer, group string, ids []string) error {
	var resp pbobjs.DownMsgSet
	if err := e.query(c, "qry_hismsg_by_ids", group, &pbobjs.QryHisMsgByIdsReq{TargetId: group, ChannelType: pbobjs.ChannelType_Group, MsgIds: ids}, &resp); err != nil {
		return err
	}
	got := map[string]int{}
	for _, m := range resp.Msgs {
		if m == nil || m.ChannelType != pbobjs.ChannelType_Group || m.TargetId != group {
			return errors.New("history identity mismatch")
		}
		got[m.MsgId]++
	}
	if len(got) != len(ids) {
		return errors.New("history set mismatch")
	}
	for _, id := range ids {
		if got[id] != 1 {
			return errors.New("original history ID missing or duplicated")
		}
	}
	return nil
}
func (e *engine) wait(check func() (bool, error)) error {
	end := e.now().Add(60 * time.Second)
	for {
		ok, err := check()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if err = e.before(); err != nil {
			return err
		}
		if !e.now().Before(end) {
			return errors.New("native observation timeout; stop new writes")
		}
		time.Sleep(200 * time.Millisecond)
	}
}
func (e *engine) send(group, step string, flags int32) (string, error) {
	if err := e.before(); err != nil {
		return "", err
	}
	content, _ := json.Marshal(map[string]string{"content": "DG2-" + e.p.RunID + "-" + step})
	b, err := proto.Marshal(&pbobjs.UpMsg{MsgType: "jg:text", MsgContent: content, Flags: flags})
	if err != nil {
		return "", err
	}
	if err = e.emit(record{Event: "intent_before_native_mutation", Step: step}); err != nil {
		return "", err
	}
	code, ack := e.sender.Publish("g_msg", group, b)
	if code != 0 || ack == nil || ack.Code != 0 || ack.MsgId == "" {
		return "", errors.New("publish rejected or unknown; do not resend")
	}
	if err = e.emit(record{Event: "publish_ack_not_DB_proof", Step: step, ID: ack.MsgId}); err != nil {
		return "", err
	}
	return ack.MsgId, e.wait(func() (bool, error) {
		for _, r := range e.receivers {
			if !r.seen(ack.MsgId) {
				return false, nil
			}
			if err := e.history(r, group, []string{ack.MsgId}); err != nil {
				return false, err
			}
		}
		v, err := e.views(group)
		return v[0].LatestID == ack.MsgId && v[1].LatestID == ack.MsgId, err
	})
}
func (e *engine) clear(topic, step string, req proto.Message) error {
	if err := e.before(); err != nil {
		return err
	}
	if err := e.emit(record{Event: "intent_before_native_mutation", Step: step}); err != nil {
		return err
	}
	if err := e.query(e.receivers[0], topic, e.p.Receiver, req, nil); err != nil {
		return err
	}
	return e.emit(record{Event: "clear_ack_not_DB_proof", Step: step})
}
func (e *engine) verify(w *expectation) error {
	if w == nil {
		return errors.New("original native expectation required")
	}
	v, err := e.views(e.p.G1)
	if err != nil {
		return err
	}
	for i, r := range e.receivers {
		if err = e.history(r, e.p.G1, w.G1IDs); err != nil {
			return err
		}
		if v[i].LatestID != w.G1IDs[2] || v[i].ReadID != w.ReadID || v[i].ReadIndex != w.ReadIndex || v[i].UnreadIndex != w.ReadIndex || v[i].Unread != 0 {
			return errors.New("group 1 original state not restored")
		}
	}
	if w.G2ID != "" {
		v, err = e.views(e.p.G2)
		if err != nil {
			return err
		}
		for i, r := range e.receivers {
			if err = e.history(r, e.p.G2, []string{w.G2ID}); err != nil {
				return err
			}
			if v[i].LatestID != w.G2ID || v[i].ReadIndex != w.G2ReadIndex || v[i].UnreadIndex != w.G2ReadIndex || v[i].Unread != 0 {
				return errors.New("global-clear original state not restored")
			}
		}
	}
	return nil
}
func (e *engine) run(w *expectation) (*expectation, error) {
	if err := e.before(); err != nil {
		return nil, err
	}
	if e.phase == "verify" {
		return w, e.verify(w)
	}
	if err := e.allowedConversations(""); err != nil {
		return nil, err
	}
	if e.phase == "baseline" {
		for _, g := range []string{e.p.G1, e.p.G2} {
			if _, err := e.views(g); err != nil {
				return nil, err
			}
			for _, r := range e.receivers {
				if ids := e.p.BaselineIDs[g]; len(ids) > 0 {
					if err := e.history(r, g, ids); err != nil {
						return nil, err
					}
				}
			}
		}
		return nil, nil
	}
	if e.phase == "g2" {
		if err := e.verify(w); err != nil {
			return nil, err
		}
		id, err := e.send(e.p.G2, "M4", 10)
		if err != nil {
			return nil, err
		}
		v, err := e.views(e.p.G2)
		if err != nil {
			return nil, err
		}
		if v[0].Unread != 1 || v[1].Unread != 1 || v[0].UnreadIndex <= v[0].ReadIndex || v[1].UnreadIndex != v[0].UnreadIndex {
			return nil, errors.New("global-clear counted baseline not observed")
		}
		if err = e.allowedConversations(e.p.G2); err != nil {
			return nil, err
		}
		if err = e.clear("clear_total_unread", "global-clear", &pbobjs.QryTotalUnreadCountReq{}); err != nil {
			return nil, err
		}
		w.G2ID = id
		w.G2ReadIndex = v[0].UnreadIndex
		return w, e.wait(func() (bool, error) { err := e.verify(w); return err == nil, err })
	}
	v, err := e.views(e.p.G1)
	if err != nil {
		return nil, err
	}
	before := v[0].UnreadIndex
	id, err := e.send(e.p.G1, "M1", 10)
	if err != nil {
		return nil, err
	}
	v, err = e.views(e.p.G1)
	if err != nil {
		return nil, err
	}
	if v[0].Unread != 1 || v[1].Unread != 1 || v[0].UnreadIndex <= before || v[1].UnreadIndex != v[0].UnreadIndex {
		return nil, errors.New("counted baseline not observed")
	}
	var set pbobjs.DownMsgSet
	if err = e.query(e.receivers[0], "qry_hismsg_by_ids", e.p.G1, &pbobjs.QryHisMsgByIdsReq{TargetId: e.p.G1, ChannelType: pbobjs.ChannelType_Group, MsgIds: []string{id}}, &set); err != nil {
		return nil, err
	}
	if len(set.Msgs) != 1 || set.Msgs[0] == nil || set.Msgs[0].MsgId != id || set.Msgs[0].MsgTime <= 0 {
		return nil, errors.New("counted read message/time missing")
	}
	w = &expectation{G1IDs: []string{id}, ReadID: id, ReadIndex: v[0].UnreadIndex}
	if err = e.clear("clear_unread", "G1-read", &pbobjs.ClearUnreadReq{Conversations: []*pbobjs.Conversation{{TargetId: e.p.G1, ChannelType: pbobjs.ChannelType_Group, LatestReadIndex: w.ReadIndex, LatestReadMsgId: id, LatestReadMsgTime: set.Msgs[0].MsgTime}}}); err != nil {
		return nil, err
	}
	if err = e.wait(func() (bool, error) {
		v, err := e.views(e.p.G1)
		return v[0].ReadID == id && v[1].ReadID == id && v[0].ReadIndex == w.ReadIndex && v[1].ReadIndex == w.ReadIndex && v[0].Unread == 0 && v[1].Unread == 0, err
	}); err != nil {
		return nil, err
	}
	for i, flags := range []int32{8, 72} {
		id, err = e.send(e.p.G1, fmt.Sprintf("M%d", i+2), flags)
		if err != nil {
			return nil, err
		}
		w.G1IDs = append(w.G1IDs, id)
		v, err = e.views(e.p.G1)
		if err != nil {
			return nil, err
		}
		for _, state := range v {
			if state.ReadID != w.ReadID || state.ReadIndex != w.ReadIndex || state.UnreadIndex != w.ReadIndex || state.Unread != 0 {
				return nil, errors.New("non-counted message changed legal read state")
			}
		}
	}
	return w, e.verify(w)
}

func connect(p params, phase string, tokens struct{ Sender, Receiver string }) (peer, [2]peer, func(), error) {
	var sender peer
	var receivers [2]peer
	var opened []*native
	closeAll := func() {
		for _, c := range opened {
			c.Disconnect()
		}
	}
	newClient := func(user, token, platform, device string) (peer, error) {
		if err := p.validate(phase, true, time.Now()); err != nil {
			return nil, err
		}
		if token == "" {
			return nil, errors.New("stdin token missing")
		}
		n := &native{group1: p.G1, group2: p.G2}
		n.WsImClient = wsclients.NewWsImClient(p.Address, p.AppKey, token, func(m *pbobjs.DownMsg) {
			if m != nil && m.ChannelType == pbobjs.ChannelType_Group && (m.TargetId == p.G1 || m.TargetId == p.G2) {
				n.received.Store(m.MsgId, true)
			}
		}, nil, nil)
		n.Verbose = false
		n.Platform = platform
		n.DeviceId = p.RunID + "-" + device
		opened = append(opened, n)
		code, ack := n.Connect("", "")
		if code != 0 || ack == nil || ack.Code != 0 || ack.UserId != user || n.UserId != user {
			return nil, errors.New("CONNECT failed or authoritative user mismatch")
		}
		return n, nil
	}
	for i, platform := range []string{"Web", "PC"} {
		var err error
		receivers[i], err = newClient(p.Receiver, tokens.Receiver, platform, fmt.Sprintf("R%d", i))
		if err != nil {
			closeAll()
			return nil, receivers, closeAll, err
		}
	}
	if phase == "g1" || phase == "g2" {
		var err error
		sender, err = newClient(p.Sender, tokens.Sender, "Web", "S")
		if err != nil {
			closeAll()
			return nil, receivers, closeAll, err
		}
	}
	return sender, receivers, closeAll, nil
}

func command(args []string, stdin io.Reader, stdout io.Writer) error {
	f := flag.NewFlagSet("dg2persist", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	paramFile := f.String("params", "", "non-secret parameter JSON")
	phase := f.String("phase", "baseline", "baseline|g1|g2|verify")
	execute := f.Bool("execute", false, "connect only under separately granted phase authorization")
	out := f.String("journal", "", "new exclusive local JSONL record")
	prior := f.String("expected", "", "original g1/g2 journal")
	if err := f.Parse(args); err != nil || f.NArg() != 0 {
		return errors.New("invalid flags; use -params FILE -phase baseline|g1|g2|verify; default performs validation only")
	}
	pf, err := os.Open(*paramFile)
	if err != nil {
		return errors.New("non-secret parameters unavailable")
	}
	var p params
	err = strictJSON(pf, &p)
	pf.Close()
	if err != nil {
		return err
	}
	if err = p.validate(*phase, *execute, time.Now()); err != nil {
		return err
	}
	if !*execute {
		return json.NewEncoder(stdout).Encode(map[string]any{"source_commit": sourceCommit, "phase": *phase, "parameters_valid": true, "connected": false, "execution_authorized": false, "scope_sha256": p.scope()})
	}
	var expected *expectation
	if *phase == "verify" || *phase == "g2" {
		expected, err = readExpected(*prior, p.scope())
		if err != nil {
			return err
		}
		if *phase == "g2" && expected.G2ID != "" {
			return errors.New("original global-clear operation already recorded; do not replay")
		}
	}
	var tokens struct{ Sender, Receiver string }
	if err = strictJSON(stdin, &tokens); err != nil {
		return err
	}
	// Exclusive journal is retained even on partial/unknown results; never reuse a new path to resend.
	j, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("journal exists or cannot be created; inspect original result, do not replay")
	}
	defer j.Close()
	log := func(r record) error {
		if err := json.NewEncoder(j).Encode(r); err != nil {
			return err
		}
		return j.Sync()
	}
	e := &engine{p: p, phase: *phase, log: log, now: time.Now}
	if err = e.emit(record{Event: "phase_intent_before_connect"}); err != nil {
		return err
	}
	websocket.DefaultDialer.HandshakeTimeout = 10 * time.Second
	// The fixed simulator prints unknown inbound topics even with Verbose=false. Discard its raw stdout.
	raw, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	os.Stdout = raw
	// Do not restore raw stdout: a late listener callback may still print after disconnect.
	defer raw.Close()
	s, r, closeAll, err := connect(p, *phase, tokens)
	if err != nil {
		_ = e.emit(record{Event: "UNKNOWN_stop_new_writes"})
		return err
	}
	defer closeAll()
	e.sender = s
	e.receivers = r
	expected, err = e.run(expected)
	if err != nil {
		_ = e.emit(record{Event: "UNKNOWN_or_failed_stop_new_writes"})
		return err
	}
	event := "native_observed_DB_restart_unverified"
	if *phase == "verify" {
		event = "native_reobservation_DB_runtime_identity_unverified"
	}
	if err = e.emit(record{Event: event, Expected: expected}); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(map[string]any{"native_phase_observed": *phase, "DB_verified": false, "restart_verified": false, "scope_sha256": p.scope()})
}
func main() {
	if err := command(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
