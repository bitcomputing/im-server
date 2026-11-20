//go:build integration

package mindlink

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
	"im-server/commons/configures"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/commonservices"
	"im-server/simulator/utils"
	"im-server/simulator/wsclients"
)

func TestPrivateReferenceAuthorHistory(t *testing.T) {
	if os.Getenv("DG003_QUOTE_OWNED") != "dg003-quote-author-1120" {
		t.Skip("requires the independently owned quote-author MySQL namespace")
	}
	if os.Getenv("MINDLINK_TEST_NATIVE_BINARY") != "/opt/imserver" || os.Getenv("MINDLINK_TEST_NATIVE_ARTIFACT_DIR") != "/evidence" {
		t.Fatal("unrecognized quote-author fixture")
	}
	bin, err := os.ReadFile("/opt/imserver")
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(bin)) != os.Getenv("MINDLINK_TEST_NATIVE_SHA256") {
		t.Fatal("fixed service binary hash mismatch")
	}
	db := nativeBoundaryDB(t)
	var uuid string
	if db.QueryRow("SELECT @@server_uuid").Scan(&uuid) != nil || uuid != os.Getenv("DG003_QUOTE_MYSQL_UUID") || configures.Config.Mysql.Address != "127.0.0.1:3306" {
		t.Fatal("refusing non-owned quote-author MySQL")
	}
	for _, query := range []string{
		"UPDATE users SET nickname='Self Current',user_portrait='fixture:self' WHERE app_key='fixture-app' AND user_id='42'",
		"INSERT INTO users(app_key,user_id,nickname,user_portrait) VALUES('fixture-app','43','Peer Current','fixture:peer'),('other-app','43','Foreign App Profile','fixture:foreign')",
	} {
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	type example struct {
		name, peer, sender, nickname                             string
		deleted, perUserDeleted, foreignConversation, foreignApp bool
	}
	cases := []example{
		{name: "self", peer: "43", sender: "42", nickname: "Self Current"},
		{name: "peer", peer: "43", sender: "43", nickname: "Peer Current"},
		{name: "missing_profile", peer: "57", sender: "57"},
		{name: "recalled_original", peer: "43", sender: "43", nickname: "Peer Current", deleted: true},
		{name: "owner_deleted_original", peer: "43", sender: "43", nickname: "Peer Current", perUserDeleted: true},
		{name: "foreign_conversation", peer: "43", sender: "43", foreignConversation: true},
		{name: "foreign_app", peer: "43", sender: "43", foreignApp: true},
	}
	stamp := time.Now().UnixMilli() - 10000
	originals, quotes := map[string]*pbobjs.DownMsg{}, map[string]*pbobjs.DownMsg{}
	insert := func(app, conver, receiver string, msg *pbobjs.DownMsg, deleted bool) {
		raw, err := proto.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO p_hismsgs(app_key,conver_id,sub_channel,sender_id,receiver_id,channel_type,msg_id,msg_seq_no,send_time,msg_type,msg_body,is_delete) VALUES(?,?,'',?,?,1,?,?,?,?,?,?)", app, conver, msg.SenderId, receiver, msg.MsgId, msg.MsgSeqNo, msg.MsgTime, msg.MsgType, raw, deleted); err != nil {
			t.Fatal(err)
		}
	}
	for i, tc := range cases {
		seq := int64(i*2 + 1)
		conver := commonservices.GetConversationId("42", tc.peer, pbobjs.ChannelType_Private)
		receiver := "42"
		if tc.sender == "42" {
			receiver = tc.peer
		}
		original := &pbobjs.DownMsg{MsgId: fmt.Sprintf("qorig%02d", i), ClientUid: "client-original-" + tc.name, SenderId: tc.sender, TargetId: receiver, ChannelType: pbobjs.ChannelType_Private, MsgType: "jg:text", MsgContent: []byte(`{"content":"trusted stored original"}`), MsgSeqNo: seq, MsgTime: stamp + seq, IsSend: true, TargetUserInfo: &pbobjs.UserInfo{UserId: "stale-target", Nickname: "untrusted stored target"}, SenderInfo: &pbobjs.UserInfo{UserId: "stale-sender", Nickname: "untrusted stored sender"}}
		app, sourceConver := nativeApp, conver
		if tc.foreignConversation {
			sourceConver = "42:unrelated"
		}
		if tc.foreignApp {
			app = "other-app"
		}
		insert(app, sourceConver, receiver, original, tc.deleted)
		ref := &pbobjs.DownMsg{MsgId: original.MsgId, MsgSeqNo: seq, MsgType: "jg:text", MsgContent: []byte(`{"content":"unresolved reference"}`)}
		if !tc.foreignConversation && !tc.foreignApp {
			ref.SenderId = "forged-client-sender"
			ref.TargetUserInfo = &pbobjs.UserInfo{UserId: "forged-client-sender", Nickname: "forged client profile"}
		}
		quote := &pbobjs.DownMsg{MsgId: fmt.Sprintf("qquote%02d", i), ClientUid: "client-quote-" + tc.name, SenderId: "42", TargetId: tc.peer, ChannelType: pbobjs.ChannelType_Private, MsgType: "jg:text", MsgContent: []byte(`{"content":"quote body"}`), MsgSeqNo: seq + 1, MsgTime: stamp + seq + 1, ReferMsg: ref}
		insert(nativeApp, conver, tc.peer, quote, false)
		if tc.perUserDeleted {
			if _, err = db.Exec("INSERT INTO p_delhismsgs(app_key,user_id,target_id,sub_channel,msg_id) VALUES('fixture-app','42',?,'',?)", tc.peer, original.MsgId); err != nil {
				t.Fatal(err)
			}
		}
		originals[tc.name], quotes[tc.name] = original, quote
	}
	before := quoteHistoryRows(t, db)
	private := t.TempDir()
	conf := map[string]any{"nodeName": "dg003-quote-author-1120", "nodeHost": "127.0.0.1", "msgStoreEngine": "mysql", "kvdb": map[string]any{"isOpen": false}, "log": map[string]any{"logPath": private, "logName": "quote-author"}, "mysql": map[string]any{"user": configures.Config.Mysql.User, "password": configures.Config.Mysql.Password, "address": configures.Config.Mysql.Address, "name": configures.Config.Mysql.DbName, "debug": false}, "apiGateway": map[string]any{"httpPort": 9001}, "connectManager": map[string]any{"wsPort": 9003}, "navGateway": map[string]any{"httpPort": 9002}, "adminGateway": map[string]any{"httpPort": 8090}, "adminSecret": "owned-quote-fixture", "mindlink": map[string]any{"enabled": false, "trusts": []any{}}}
	raw, err := yaml.Marshal(conf)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(private, "config.yml")
	if os.WriteFile(path, raw, 0600) != nil || os.Mkdir(filepath.Join(private, "conf"), 0700) != nil || os.WriteFile(filepath.Join(private, "conf/config.yml"), raw, 0600) != nil {
		t.Fatal("write private service config")
	}
	log, err := os.OpenFile("/evidence/quote-author-process-private.log", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	startNativeReadProcess(t, ctx, path, log)
	client := wsclients.NewWsImClient("ws://127.0.0.1:9003", nativeApp, nativeToken(t, "42"), nil, nil, nil)
	client.Verbose, client.Platform, client.DeviceId = false, "Web", "quote-author-1120"
	code, ack := client.Connect("", "")
	if code != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 || client.UserId != "42" {
		t.Fatal("ordinary fixture WS login failed")
	}
	t.Cleanup(client.Disconnect)
	var records []map[string]any
	t.Cleanup(func() {
		after := quoteHistoryRows(t, db)
		unchanged := reflect.DeepEqual(before, after)
		body, _ := json.MarshalIndent(map[string]any{"binary_sha256": os.Getenv("MINDLINK_TEST_NATIVE_SHA256"), "mysql_uuid": uuid, "cases": records, "history_before": before, "history_after": after, "history_rows_unchanged": unchanged}, "", "  ")
		if os.WriteFile("/evidence/quote-author.json", body, 0600) != nil {
			t.Error("save quote evidence")
		}
		if !unchanged {
			t.Error("history retrieval rewrote persisted original/quote/delete rows")
		}
	})
	// Profiles are available through the same real RPC chain before testing quotes.
	for _, id := range []string{"42", "43"} {
		raw, _ := proto.Marshal(&pbobjs.UserIdReq{UserId: id})
		transport, a := client.Query("qry_user_info", id, raw)
		profile := &pbobjs.UserInfo{}
		if transport != utils.ClientErrorCode_Success || a == nil || a.Code != 0 || proto.Unmarshal(a.Data, profile) != nil || profile.UserId != id || profile.Nickname == "" {
			t.Fatal("normal profile RPC control failed")
		}
		records = append(records, map[string]any{"control": "user-profile", "id": id, "profile": profile, "code": a.Code})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for reload := 0; reload < 2; reload++ {
				req := &pbobjs.QryHisMsgsReq{TargetId: tc.peer, ChannelType: pbobjs.ChannelType_Private, Count: 50}
				raw, _ := proto.Marshal(req)
				// Respect the real WS limiter (100/s, burst 10); a rate refusal is
				// neither author restoration nor the Native authorization result.
				time.Sleep(25 * time.Millisecond)
				transport, a := client.Query("qry_hismsgs", tc.peer, raw)
				ackCode := int32(-1)
				if a != nil {
					ackCode = a.Code
				}
				records = append(records, map[string]any{"case": tc.name, "reload": reload, "phase": "transport", "transport": transport, "code": ackCode, "ack_missing": a == nil, "utc": time.Now().UTC().Format(time.RFC3339Nano)})
				page := &pbobjs.DownMsgSet{}
				if transport != utils.ClientErrorCode_Success || a == nil || a.Code != 0 || proto.Unmarshal(a.Data, page) != nil {
					t.Fatalf("normal authenticated history query failed: transport=%d ack=%d", transport, ackCode)
				}
				records = append(records, map[string]any{"case": tc.name, "reload": reload, "method": "qry_hismsgs", "request": req, "code": a.Code, "response": page})
				var got *pbobjs.DownMsg
				for _, msg := range page.Msgs {
					if msg.MsgId == quotes[tc.name].MsgId {
						got = msg.ReferMsg
					}
				}
				if got == nil {
					t.Fatal("stored quote/reference missing")
				}
				if tc.foreignConversation || tc.foreignApp {
					if !proto.Equal(got, quotes[tc.name].ReferMsg) {
						t.Fatal("out-of-scope original resolved or acquired a profile")
					}
					continue
				}
				original := originals[tc.name]
				if got.MsgId != original.MsgId || got.SenderId != tc.sender || got.MsgSeqNo != original.MsgSeqNo || string(got.MsgContent) != string(original.MsgContent) || got.IsDelete != (tc.deleted || tc.perUserDeleted) {
					t.Fatal("reference identity/body/deletion semantics changed")
				}
				if got.TargetUserInfo == nil || got.TargetUserInfo.UserId != tc.sender || got.TargetUserInfo.Nickname != tc.nickname || got.SenderInfo == nil || !proto.Equal(got.TargetUserInfo, got.SenderInfo) {
					t.Fatalf("trusted reference author missing or wrong: sender=%s target=%v sender_info=%v", tc.sender, got.TargetUserInfo, got.SenderInfo)
				}
				if got.IsSend != (tc.sender == "42") {
					t.Fatal("reference self/peer direction guessed incorrectly")
				}
			}
		})
	}
	t.Run("native_fence", func(t *testing.T) {
		req := &pbobjs.QryHisMsgsReq{TargetId: nativePrincipal, ChannelType: pbobjs.ChannelType_Private, Count: 20}
		raw, _ := proto.Marshal(req)
		time.Sleep(25 * time.Millisecond)
		transport, a := client.Query("qry_hismsgs", nativePrincipal, raw)
		ackCode := int32(-1)
		if a != nil {
			ackCode = a.Code
		}
		records = append(records, map[string]any{"case": "native_fence", "phase": "transport", "transport": transport, "code": ackCode, "ack_missing": a == nil, "utc": time.Now().UTC().Format(time.RFC3339Nano)})
		if a == nil || a.Code != 19008 || len(a.Data) != 0 {
			t.Fatalf("Native history fence did not return its expected refusal: transport=%d ack=%d", transport, ackCode)
		}
		records = append(records, map[string]any{"case": "native_fence", "method": "qry_hismsgs", "request": req, "code": a.Code, "response_bytes": len(a.Data)})
	})
}

func quoteHistoryRows(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	result := map[string][]string{}
	for _, table := range []string{"p_hismsgs", "p_delhismsgs"} {
		rows, err := db.Query("SELECT * FROM " + table)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]any, len(cols))
			pointers := make([]any, len(cols))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err = rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			row := map[string]any{}
			for i, col := range cols {
				row[col] = values[i]
			}
			raw, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			result[table] = append(result[table], string(raw))
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		sort.Strings(result[table])
	}
	return result
}
