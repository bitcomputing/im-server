//go:build integration

package mindlink

import (
	"database/sql"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"
)

func waitNativeReadUI(t *testing.T, db *sql.DB, received *sync.Map) {
	t.Helper()
	if os.Getenv("MINDLINK_TEST_NATIVE_UI_WAIT") != "1" {
		t.Log("management page not exercised in this native-only run")
		return
	}
	type scope struct {
		table, conver string
		channel       int
	}
	scopes := []scope{{"p_hismsgs", "42:43", 1}, {"g_hismsgs", nativePrincipal, 2}}
	ids := func(s scope) map[string]int64 {
		rows, err := db.Query("SELECT msg_id,msg_seq_no FROM "+s.table+" WHERE app_key=? AND conver_id=? AND sub_channel=''", nativeApp, s.conver)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		result := map[string]int64{}
		for rows.Next() {
			var id string
			var seq int64
			if rows.Scan(&id, &seq) != nil {
				t.Fatal("read scoped UI baseline IDs")
			}
			if _, duplicate := result[id]; duplicate {
				t.Fatal("duplicate history ID in owned UI scope")
			}
			result[id] = seq
		}
		if rows.Err() != nil {
			t.Fatal(rows.Err())
		}
		return result
	}
	seen := func() [4]int64 {
		var state [4]int64
		if err := db.QueryRow("SELECT COUNT(*),COALESCE(MAX(seen_seq),0) FROM conversation_seen WHERE app_key=? AND user_id='43' AND target_id='42' AND channel_type=1 AND sub_channel=''", nativeApp).Scan(&state[0], &state[1]); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow("SELECT COUNT(*),COALESCE(MAX(seen_seq),0) FROM conversation_seen WHERE app_key=? AND user_id='42' AND target_id=? AND channel_type=2 AND sub_channel=''", nativeApp, nativePrincipal).Scan(&state[2], &state[3]); err != nil {
			t.Fatal(err)
		}
		return state
	}
	baseline := []map[string]int64{ids(scopes[0]), ids(scopes[1])}
	seenBefore := seen()
	ready, err := json.Marshal(map[string]string{"appkey": nativeApp, "sender": "42", "private_target": "43", "group_target": nativePrincipal})
	if err != nil || os.WriteFile("/evidence/ui-ready.json", ready, 0600) != nil {
		t.Fatal("write owned UI readiness marker")
	}
	t.Log("owned management page fixture ready; waiting for the browser's two new send/history IDs")
	for deadline := time.Now().Add(30 * time.Minute); time.Now().Before(deadline); {
		raw, err := os.ReadFile("/evidence/ui-done.json")
		if os.IsNotExist(err) {
			time.Sleep(250 * time.Millisecond)
			continue
		}
		var done struct {
			Private string `json:"private_message_id"`
			Group   string `json:"group_message_id"`
		}
		if err != nil || json.Unmarshal(raw, &done) != nil || done.Private == "" || done.Group == "" || done.Private == done.Group {
			t.Fatal("invalid owned browser result IDs")
		}
		for i, id := range []string{done.Private, done.Group} {
			s := scopes[i]
			current := ids(s)
			seq, present := current[id]
			if _, old := baseline[i][id]; old || !present || seq <= 0 || len(current) != len(baseline[i])+1 {
				t.Fatal("browser result was not exactly one new history ID in its scope")
			}
			waitNativeHistory(t, db, received, s.table, id, seq)
			var watermark int64
			if err := db.QueryRow("SELECT seq FROM message_sequence WHERE app_key=? AND conver_id=? AND channel_type=? AND sub_channel=''", nativeApp, s.conver, s.channel).Scan(&watermark); err != nil || watermark != seq {
				t.Fatal("browser send did not match durable sequence watermark", err)
			}
			t.Logf("actual browser send/history result: table=%s id=%s seq=%d unique=1 SDK-recipient43=true watermark=%d", s.table, id, seq, watermark)
		}
		if seen() != seenBefore {
			t.Fatal("ordinary management send/history unexpectedly advanced independent seen")
		}
		t.Log("actual management page private/group send+history matched new unique DB rows and SDK receipt; independent seen unchanged")
		return
	}
	t.Fatal("owned management page check did not finish")
}
