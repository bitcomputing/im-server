package mindlinkread

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/pbdefines/pbobjs"
	hisdbs "im-server/services/historymsg/storages/dbs"
	"im-server/simulator/utils"
)

func retentionFixture(t *testing.T) *fixture {
	t.Helper()
	guard(t)
	f := &fixture{t: t}
	data, err := os.ReadFile("/evidence/runtime-private.json")
	if err != nil || json.Unmarshal(data, f) != nil || f.Seq != 3 || len(f.IDs) != 3 {
		t.Fatal("owned retention fixture unavailable")
	}
	return f
}

// Run after the ordinary restart probe; remove only this run's disposable tenant history.
func TestPurgeSeenHistoryMySQL(t *testing.T) {
	f := retentionFixture(t)
	password := os.Getenv("MYSQL_ROOT_PASSWORD")
	if password == "" {
		t.Fatal("isolated password missing")
	}
	configures.Config.Mysql.User = "root"
	configures.Config.Mysql.Password = password
	configures.Config.Mysql.Address = "im-f1-read-20261106-mysql-1:3306"
	configures.Config.Mysql.DbName = "jim_db"
	if err := dbcommons.InitMysql(); err != nil {
		t.Fatal(err)
	}
	if err := (hisdbs.GroupHisMsgDao{}).DelMsgsBaseTime(f.Key, time.Now().UnixMilli()+1); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := dbcommons.GetDb().Raw("SELECT COUNT(*) FROM g_hismsgs WHERE app_key=?", f.Key).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("owned history purge incomplete")
	}
	var seq int64
	if err := dbcommons.GetDb().Raw("SELECT seen_seq FROM conversation_seen WHERE app_key=? AND user_id='read-owner' AND target_id=?", f.Key, f.Group).Scan(&seq).Error; err != nil || seq != 3 {
		t.Fatal("history purge changed monotonic reading position")
	}
	t.Log("real retention DAO removed history; durable seen remains 3; restart only owned IM before the next probe")
}

func TestSeenAfterRetention(t *testing.T) {
	f := retentionFixture(t)
	owner := f.connect("read-owner", "Web")
	sender := f.connect("read-sender", "PC")
	code, ack := sender.SendGroupMsg(f.Group, &pbobjs.UpMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"after owned retention"}`), Flags: 8})
	if code != utils.ClientErrorCode_Success || ack == nil || ack.Code != 0 {
		t.Fatal("post-retention publish failed")
	}
	eventually(t, func() bool {
		code, state := seen(t, owner, f.Group, -1)
		if code != 0 {
			t.Fatalf("query failed %d", code)
		}
		if state.LatestSeq > 0 {
			t.Logf("post-retention latest=%d seen=%d has_new=%v", state.LatestSeq, state.SeenSeq, state.HasNew)
			if !state.HasNew || state.LatestSeq <= state.SeenSeq {
				t.Fatal("new stored message was hidden by a reused reading sequence")
			}
			return true
		}
		return false
	})
}
