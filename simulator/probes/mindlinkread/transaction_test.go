package mindlinkread

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"im-server/commons/bases"
	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/conversation/services"
	"im-server/simulator/utils"
	"im-server/simulator/wsclients"

	"gorm.io/gorm/logger"
)

func assertTransactionRuntime(t *testing.T, f *fixture, owner *wsclients.WsImClient) {
	t.Helper()
	if code, state := seen(t, owner, f.Group, -1); code != 0 || state.SeenSeq != f.Seq || state.LatestSeq != f.Seq || state.HasNew {
		t.Fatal("native query differs from committed adapter result")
	}
	code, history := owner.QryHistoryMsgs(&pbobjs.QryHisMsgsReq{TargetId: f.Group, ChannelType: pbobjs.ChannelType_Group, Count: 20})
	if code != utils.ClientErrorCode_Success || history == nil || len(history.Msgs) != 1 || history.Msgs[0].MsgId != f.IDs[0] || history.Msgs[0].MsgSeqNo != f.Seq {
		t.Fatal("actual received/history ID or displayed sequence changed")
	}
	_, conver := owner.QryConversation(&pbobjs.QryConverReq{TargetId: f.Group, ChannelType: pbobjs.ChannelType_Group})
	if conver == nil || conver.GetMsg().GetMsgId() != f.IDs[0] || conver.UnreadCount != 0 {
		t.Fatal("adapter changed native no-count summary/unread")
	}
}

func TestTransactionBeforeRestart(t *testing.T) {
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
	f.Key, f.Group = "st"+hex.EncodeToString(random), "tx-"+hex.EncodeToString(random)
	var app struct {
		Secret string `json:"app_secret"`
	}
	if err := json.Unmarshal(f.post("/admingateway/apps/create", map[string]string{"app_key": f.Key, "app_name": "READ transaction probe"}, auth.Authorization), &app); err != nil || app.Secret == "" {
		t.Fatal("fixture app create failed")
	}
	f.Secret = app.Secret
	owner, sender := f.connect("read-owner", "Web"), f.connect("read-sender", "PC")
	f.post("/groups/add", map[string]any{"group_id": f.Group, "group_name": "READ transaction", "member_ids": []string{owner.UserId, sender.UserId}}, "")
	code, ack := sender.SendGroupMsg(f.Group, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"transaction probe"}`), Flags: 8})
	if code != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 {
		t.Fatal("publish failed")
	}
	f.IDs = []string{ack.MsgId}
	eventually(t, func() bool {
		_, received := f.Seen.Load("read-owner/Web/" + ack.MsgId)
		return received
	})
	eventually(t, func() bool {
		_, history := owner.QryHistoryMsgs(&pbobjs.QryHisMsgsReq{TargetId: f.Group, ChannelType: pbobjs.ChannelType_Group, Count: 20})
		if history == nil || len(history.Msgs) != 1 || history.Msgs[0].MsgId != ack.MsgId {
			return false
		}
		f.Seq = history.Msgs[0].MsgSeqNo
		return f.Seq == 1
	})
	password := os.Getenv("MYSQL_ROOT_PASSWORD")
	if password == "" {
		t.Fatal("isolated password missing")
	}
	configures.Config.Mysql.User, configures.Config.Mysql.Password = "root", password
	configures.Config.Mysql.Address, configures.Config.Mysql.DbName = "im-f1-read-20261106-mysql-1:3306", "jim_db"
	if err := dbcommons.InitMysql(); err != nil {
		t.Fatal(err)
	}
	db := dbcommons.GetDb()
	db.Config.Logger = logger.Default.LogMode(logger.Silent)
	// This is a trusted in-process adapter fixture; public authentication/policy/result handling belongs to its caller.
	ctx := bases.CreateRpcCtx(f.Key, owner.UserId)
	req := &pbobjs.SetConverSeenReq{TargetId: f.Group, ChannelType: pbobjs.ChannelType_Group, SeenSeq: f.Seq}
	for _, commit := range []bool{false, true} {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		defer tx.Rollback()
		if code, state := services.SetConverSeenWithTx(ctx, tx, req); code != 0 || state == nil || state.SeenSeq != f.Seq || state.LatestSeq != f.Seq || state.HasNew {
			t.Fatal("adapter rejected actual visible history sequence")
		}
		if code, state := seen(t, owner, f.Group, -1); code != 0 || state.SeenSeq != 0 || state.LatestSeq != f.Seq || !state.HasNew {
			t.Fatal("uncommitted adapter write escaped to native RPC")
		}
		if commit {
			if err := tx.Commit().Error; err != nil {
				t.Fatal(err)
			}
		} else {
			if err := tx.Rollback().Error; err != nil {
				t.Fatal(err)
			}
			if code, state := seen(t, owner, f.Group, -1); code != 0 || state.SeenSeq != 0 {
				t.Fatal("caller rollback did not preserve native seen=0")
			}
		}
	}
	assertTransactionRuntime(t, f, owner)
	// Wait for the existing summary flusher before restarting only IM; inspect the actual row rather than sleeping blindly.
	deadline := time.Now().Add(90 * time.Second)
	for {
		var count int64
		if err := db.Raw("SELECT COUNT(*) FROM conversations WHERE app_key=? AND user_id=? AND target_id=? AND latest_msg_id=? AND latest_unread_msg_index=latest_read_msg_index", f.Key, owner.UserId, f.Group, f.IDs[0]).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("native summary did not persist before restart")
		}
		time.Sleep(200 * time.Millisecond)
	}
	data, err := json.Marshal(f)
	if err != nil || os.WriteFile("/evidence/transaction-private.json", data, 0600) != nil {
		t.Fatal("cannot preserve private restart fixture")
	}
	t.Logf("app=%s group=%s message_id=%s actual_history_seq=%d received=true rollback_seen=0 committed_seen=1 latest=1 no_count_unread=0 summary_persisted=true", f.Key, f.Group, f.IDs[0], f.Seq)
}

func TestTransactionAfterRestart(t *testing.T) {
	guard(t)
	f := &fixture{t: t}
	data, err := os.ReadFile("/evidence/transaction-private.json")
	if err != nil || json.Unmarshal(data, f) != nil || f.Seq != 1 || len(f.IDs) != 1 {
		t.Fatal("owned transaction restart fixture missing")
	}
	assertTransactionRuntime(t, f, f.connect("read-owner", "Web"))
	t.Logf("restored app=%s group=%s message_id=%s seen=1 latest=1 no_count_unread=0", f.Key, f.Group, f.IDs[0])
}
