package mindlinkread

import (
	"bytes"
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

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type observationFixture struct {
	Fixture *fixture `json:"fixture"`
	DMId    string   `json:"dm_id"`
}

func observationRuntimeDB(t *testing.T) *gorm.DB {
	t.Helper()
	password := os.Getenv("MYSQL_ROOT_PASSWORD")
	if password == "" {
		t.Fatal("owned MySQL password missing")
	}
	configures.Config.Mysql.User, configures.Config.Mysql.Password = "root", password
	configures.Config.Mysql.Address, configures.Config.Mysql.DbName = "im-f1-read-20261106-mysql-1:3306", "jim_db"
	if err := dbcommons.InitMysql(); err != nil {
		t.Fatal(err)
	}
	db := dbcommons.GetDb()
	db.Config.Logger = logger.Default.LogMode(logger.Silent)
	return db
}

func saveObservationFixture(t *testing.T, f *observationFixture) {
	t.Helper()
	data, err := json.Marshal(f)
	if err != nil || os.WriteFile("/evidence/observation-runtime-private.json", data, 0600) != nil {
		t.Fatal("cannot save private owned runtime coordinates")
	}
}

func assertObservationMessages(t *testing.T, f *observationFixture, owner, peer *wsclients.WsImClient) []int64 {
	t.Helper()
	code, history := owner.QryHistoryMsgs(&pbobjs.QryHisMsgsReq{TargetId: f.Fixture.Group, ChannelType: pbobjs.ChannelType_Group, Count: 20})
	if code != utils.ClientErrorCode_Success || history == nil || len(history.Msgs) != 2 {
		t.Fatal("no-count history missing")
	}
	positions := make([]int64, 0, len(history.Msgs))
	for i, msg := range history.Msgs {
		if msg.MsgId != f.Fixture.IDs[i] || msg.MsgSeqNo != int64(i+1) {
			t.Fatal("received/ACK/history ID or durable sequence differs")
		}
		positions = append(positions, msg.MsgSeqNo)
	}
	_, summary := owner.QryConversation(&pbobjs.QryConverReq{TargetId: f.Fixture.Group, ChannelType: pbobjs.ChannelType_Group})
	if summary == nil || summary.GetMsg().GetMsgId() != f.Fixture.IDs[1] || summary.UnreadCount != 0 {
		t.Fatal("no-count summary/unread changed")
	}
	code, dm := peer.QryHistoryMsgs(&pbobjs.QryHisMsgsReq{TargetId: owner.UserId, ChannelType: pbobjs.ChannelType_Private, Count: 20})
	if code != utils.ClientErrorCode_Success || dm == nil || len(dm.Msgs) != 1 || dm.Msgs[0].MsgId != f.DMId || dm.Msgs[0].MsgSeqNo != 1 {
		t.Fatal("ordinary DM history regressed")
	}
	_, ordinary := peer.QryConversation(&pbobjs.QryConverReq{TargetId: owner.UserId, ChannelType: pbobjs.ChannelType_Private})
	if ordinary == nil || ordinary.GetMsg().GetMsgId() != f.DMId || ordinary.UnreadCount != 1 {
		t.Fatal("ordinary DM unread/summary regressed")
	}
	return positions
}

func TestObservationBeforeRestart(t *testing.T) {
	guard(t)
	f := &observationFixture{Fixture: &fixture{t: t}}
	var auth struct{ Authorization string }
	if err := json.Unmarshal(f.Fixture.post("/admingateway/login", map[string]string{"account": "admin", "password": "123456"}, ""), &auth); err != nil || auth.Authorization == "" {
		t.Fatal("owned admin login failed")
	}
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	f.Fixture.Key, f.Fixture.Group = "so"+hex.EncodeToString(random), "observe-"+hex.EncodeToString(random)
	saveObservationFixture(t, f)
	var app struct {
		Secret string `json:"app_secret"`
	}
	if err := json.Unmarshal(f.Fixture.post("/admingateway/apps/create", map[string]string{"app_key": f.Fixture.Key, "app_name": "READ consistent observation"}, auth.Authorization), &app); err != nil || app.Secret == "" {
		t.Fatal("owned app create failed")
	}
	f.Fixture.Secret = app.Secret
	saveObservationFixture(t, f)
	owner, pc := f.Fixture.connect("read-owner", "Web"), f.Fixture.connect("read-owner", "PC")
	sender, peer := f.Fixture.connect("read-sender", "Web"), f.Fixture.connect("read-peer", "Web")
	f.Fixture.post("/groups/add", map[string]any{"group_id": f.Fixture.Group, "group_name": "READ observation", "member_ids": []string{owner.UserId, sender.UserId, peer.UserId}}, "")
	for range 2 {
		code, ack := sender.SendGroupMsg(f.Fixture.Group, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"owned no-count observation"}`), Flags: 8})
		if code != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 {
			t.Fatal("owned no-count publish failed")
		}
		f.Fixture.IDs = append(f.Fixture.IDs, ack.MsgId)
		saveObservationFixture(t, f)
		eventually(t, func() bool {
			_, web := f.Fixture.Seen.Load("read-owner/Web/" + ack.MsgId)
			_, desktop := f.Fixture.Seen.Load("read-owner/PC/" + ack.MsgId)
			return web && desktop
		})
	}
	code, dmAck := owner.SendPrivateMsg(peer.UserId, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"ordinary DM regression"}`), Flags: 10})
	if code != utils.ClientErrorCode_Success || dmAck == nil || dmAck.Code != 0 {
		t.Fatal("ordinary DM publish failed")
	}
	f.DMId = dmAck.MsgId
	saveObservationFixture(t, f)
	eventually(t, func() bool { _, received := f.Fixture.Seen.Load("read-peer/Web/" + f.DMId); return received })
	eventually(t, func() bool {
		code, state := seen(t, owner, f.Fixture.Group, -1)
		return code == 0 && state.LatestSeq == 2
	})
	positions := assertObservationMessages(t, f, owner, peer)
	f.Fixture.Seq = positions[len(positions)-1]
	saveObservationFixture(t, f)
	db := observationRuntimeDB(t)
	ctx := bases.CreateRpcCtx(f.Fixture.Key, owner.UserId)
	req := &pbobjs.QryConverSeenReq{TargetId: f.Fixture.Group, ChannelType: pbobjs.ChannelType_Group}
	tx := db.Begin()
	defer tx.Rollback()
	codeSeen, original := services.QryConverSeenObservationWithTx(ctx, tx, req)
	if codeSeen != 0 || original == nil || original.State.SeenSeq != "0" || original.State.LatestSeq != "2" || !original.State.HasNew || original.Coordinates.TargetId != f.Fixture.Group {
		t.Fatal("typed query did not observe actual displayed history coordinates")
	}
	originalJSON, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if code, state := services.SetConverSeenWithTx(ctx, tx, &pbobjs.SetConverSeenReq{TargetId: f.Fixture.Group, ChannelType: 2, SeenSeq: f.Fixture.Seq}); code != 0 || state == nil || state.SeenSeq != f.Fixture.Seq {
		t.Fatal("staged seen did not advance")
	}
	for _, c := range []*wsclients.WsImClient{owner, pc} {
		if code, state := seen(t, c, f.Fixture.Group, -1); code != 0 || state.SeenSeq != 0 || state.LatestSeq != 2 {
			t.Fatal("uncommitted seen escaped to another device")
		}
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	tx = db.Begin()
	defer tx.Rollback()
	if code, state := services.SetConverSeenWithTx(ctx, tx, &pbobjs.SetConverSeenReq{TargetId: f.Fixture.Group, ChannelType: 2, SeenSeq: positions[0]}); code != 0 || state == nil || state.SeenSeq != positions[0] {
		t.Fatal("partial display advance failed")
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	for _, position := range []int64{f.Fixture.Seq, positions[0], f.Fixture.Seq, positions[0]} {
		if code, state := seen(t, pc, f.Fixture.Group, position); code != 0 || state.SeenSeq != 2 || state.HasNew {
			t.Fatal("out-of-order device regressed seen")
		}
	}
	tx = db.Begin()
	defer tx.Rollback()
	if code, latest := services.QryConverSeenObservationWithTx(ctx, tx, req); code != 0 || latest == nil || latest.State.SeenSeq != "2" || latest.State.LatestSeq != "2" || latest.State.HasNew {
		t.Fatal("new observation did not read committed seen")
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	unchanged, _ := json.Marshal(original)
	if !bytes.Equal(originalJSON, unchanged) {
		t.Fatal("later device read overwrote original observation/time")
	}
	assertObservationMessages(t, f, owner, peer)
	deadline := time.Now().Add(90 * time.Second)
	for {
		var groupRows, dmRows int64
		if err := db.Raw("SELECT COUNT(*) FROM conversations WHERE app_key=? AND user_id='read-owner' AND target_id=? AND latest_msg_id=? AND latest_unread_msg_index=latest_read_msg_index", f.Fixture.Key, f.Fixture.Group, f.Fixture.IDs[1]).Scan(&groupRows).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Raw("SELECT COUNT(*) FROM conversations WHERE app_key=? AND user_id='read-peer' AND target_id='read-owner' AND latest_msg_id=? AND latest_unread_msg_index=latest_read_msg_index+1", f.Fixture.Key, f.DMId).Scan(&dmRows).Error; err != nil {
			t.Fatal(err)
		}
		if groupRows == 1 && dmRows == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("group/ordinary DM summaries did not persist before restart")
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Logf("app=%s group=%s group_ids=%v dm_id=%s both_devices_received=true history_sequences=1,2 typed_original=0/2/true stable=true rollback=0 committed_then_older_devices=2 no_count_unread=0 ordinary_dm_unread=1 summaries_persisted=true", f.Fixture.Key, f.Fixture.Group, f.Fixture.IDs, f.DMId)
}

func TestObservationAfterRestart(t *testing.T) {
	guard(t)
	f := &observationFixture{Fixture: &fixture{t: t}}
	data, err := os.ReadFile("/evidence/observation-runtime-private.json")
	if err != nil || json.Unmarshal(data, f) != nil || f.Fixture.Seq != 2 || len(f.Fixture.IDs) != 2 || f.DMId == "" {
		t.Fatal("owned restart fixture missing")
	}
	owner, pc := f.Fixture.connect("read-owner", "Web"), f.Fixture.connect("read-owner", "PC")
	peer := f.Fixture.connect("read-peer", "Web")
	for _, c := range []*wsclients.WsImClient{owner, pc} {
		if code, state := seen(t, c, f.Fixture.Group, -1); code != 0 || state.SeenSeq != 2 || state.LatestSeq != 2 || state.HasNew {
			t.Fatal("committed seen did not survive IM restart")
		}
	}
	assertObservationMessages(t, f, owner, peer)
	db := observationRuntimeDB(t)
	tx := db.Begin()
	defer tx.Rollback()
	code, observation := services.QryConverSeenObservationWithTx(bases.CreateRpcCtx(f.Fixture.Key, owner.UserId), tx, &pbobjs.QryConverSeenReq{TargetId: f.Fixture.Group, ChannelType: 2})
	if code != 0 || observation == nil || observation.State.SeenSeq != "2" || observation.State.LatestSeq != "2" || observation.State.HasNew || observation.Coordinates.TargetId != f.Fixture.Group {
		t.Fatal("restarted typed native observation differs")
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	t.Logf("restored app=%s group=%s group_ids=%v dm_id=%s seen=2 latest=2 no_count_unread=0 ordinary_dm_unread=1", f.Fixture.Key, f.Fixture.Group, f.Fixture.IDs, f.DMId)
}
