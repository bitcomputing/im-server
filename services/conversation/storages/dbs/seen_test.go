package dbs

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"im-server/commons/configures"
	"im-server/commons/dbcommons"
)

func seenTestDB(t *testing.T) {
	t.Helper()
	if os.Getenv("JIM_F1_READ_ISOLATED") != "im-f1-read-20261106" {
		t.Skip("requires the independently owned F1-READ MySQL fixture")
	}
	password := os.Getenv("MYSQL_ROOT_PASSWORD")
	if password == "" {
		t.Fatal("isolated MySQL password missing")
	}
	configures.Config.Mysql.User = "root"
	configures.Config.Mysql.Password = password
	configures.Config.Mysql.Address = "im-f1-read-20261106-mysql-1:3306"
	configures.Config.Mysql.DbName = "jim_db"
	if err := dbcommons.InitMysql(); err != nil {
		t.Fatal(err)
	}
}

func TestSeenSchemaMySQL(t *testing.T) {
	seenTestDB(t)
	var count int64
	if err := dbcommons.GetDb().Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='conversation_seen'").Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("durable independent conversation reading state is missing")
	}
}

func TestSeenMergeMySQL(t *testing.T) {
	seenTestDB(t)
	app := fmt.Sprintf("sr%x", os.Getpid())
	db := dbcommons.GetDb()
	t.Cleanup(func() { db.Exec("DELETE FROM conversation_seen WHERE app_key IN (?,?)", app, app+"b") })
	dao := SeenDao{}
	query := func(tenant, user, target, sub string) int64 {
		t.Helper()
		seq, err := dao.Find(tenant, user, target, sub, 2)
		if err != nil {
			t.Fatal(err)
		}
		return seq
	}
	if query(app, "owner", "group", "") != 0 {
		t.Fatal("new conversation is already seen")
	}
	for _, seq := range []int64{2, 1, 2, 3} {
		if err := dao.Advance(app, "owner", "group", "", 2, seq); err != nil {
			t.Fatal(err)
		}
	}
	if query(app, "owner", "group", "") != 3 {
		t.Fatal("duplicate or older read regressed durable state")
	}
	var wg sync.WaitGroup
	for seq := int64(4); seq <= 24; seq++ {
		wg.Add(1)
		go func(seq int64) {
			defer wg.Done()
			if err := dao.Advance(app, "owner", "group", "", 2, seq); err != nil {
				t.Error(err)
			}
		}(seq)
	}
	wg.Wait()
	if query(app, "owner", "group", "") != 24 {
		t.Fatal("concurrent devices lost the maximum read position")
	}
	for _, key := range [][4]string{{app + "b", "owner", "group", ""}, {app, "other", "group", ""}, {app, "owner", "other", ""}, {app, "owner", "group", "thread"}} {
		if query(key[0], key[1], key[2], key[3]) != 0 {
			t.Fatal("read state leaked across its identity scope")
		}
	}
	if seq, err := (SeenDao{}).Find(app, "owner", "group", "", 1); err != nil || seq != 0 {
		t.Fatal("read state leaked across channel type")
	}
}
