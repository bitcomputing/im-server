//go:build integration

package mindlink

import (
	"context"
	"database/sql"
	"math"
	"sync"
	"testing"

	"im-server/commons/messagesequence"
)

func TestMySQLMessageSequenceJoinsHistoryTransaction(t *testing.T) {
	db := nativeFixture(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO conversation_seen(app_key,user_id,target_id,channel_type,sub_channel,seen_seq) VALUES('sequence-app','owner','peer',1,'',100)`); err != nil {
		t.Fatal(err)
	}
	reserve := func(tx *sql.Tx) (messagesequence.Watermark, error) {
		return messagesequence.Reserve(ctx, tx, "sequence-app", "owner:peer", "", 1, messagesequence.Watermark{}, 10, func() (messagesequence.Watermark, error) {
			return messagesequence.SQLFloor(ctx, tx, "sequence-app", "owner:peer", "", 1)
		})
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := reserve(tx)
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if first.Seq != 101 || first.SendTime != 10 {
		tx.Rollback()
		t.Fatalf("lost durable read floor: %+v", first)
	}
	if _, err = tx.Exec(`INSERT INTO p_hismsgs(app_key,conver_id,msg_id,msg_seq_no,send_time) VALUES('sequence-app','owner:peer','rolled-back',?,?)`, first.Seq, first.SendTime); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"message_sequence", "p_hismsgs"} {
		var count int
		if err = db.QueryRow("SELECT COUNT(*) FROM " + table + " WHERE app_key='sequence-app'").Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s escaped rollback: %d %v", table, count, err)
		}
	}
	var wg sync.WaitGroup
	results := make(chan messagesequence.Watermark, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Error(err)
				return
			}
			defer tx.Rollback()
			got, err := reserve(tx)
			if err != nil {
				t.Error(err)
				return
			}
			if err = tx.Commit(); err != nil {
				t.Error(err)
				return
			}
			results <- got
		}()
	}
	wg.Wait()
	close(results)
	seqs, times := map[int64]bool{}, map[int64]bool{}
	for got := range results {
		if seqs[got.Seq] || times[got.SendTime] {
			t.Fatal("concurrent allocation collided")
		}
		seqs[got.Seq] = true
		times[got.SendTime] = true
	}
	if len(seqs) != 12 || !seqs[101] || !seqs[112] {
		t.Fatalf("lost sequence allocations: %v", seqs)
	}
	if _, err = db.Exec(`UPDATE message_sequence SET seq=? WHERE app_key='sequence-app'`, int64(math.MaxInt64)); err != nil {
		t.Fatal(err)
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	got, err := reserve(tx)
	if err == nil || got != (messagesequence.Watermark{}) {
		t.Fatalf("overflow became publishable: %+v %v", got, err)
	}
}
