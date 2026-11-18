//go:build integration

package mindlink

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The external harness verifies/stops/restarts only the explicitly owned MySQL
// process between these phases. A normal test invocation never stops a service.
func TestMySQLReliableMessageRestartProof(t *testing.T) {
	phase := os.Getenv("MINDLINK_RELIABLE_RESTART_PHASE")
	if phase != "seed" && phase != "verify" {
		t.Skip("requires explicit owned-process restart harness; not restart coverage")
	}
	_ = reliableConfig(t)
	path := filepath.Join(filepath.Dir(os.Getenv("MINDLINK_RELIABLE_TEST_DSN_FILE")), "restart-proof.json")
	var proof struct {
		Original Claims          `json:"original"`
		Result   json.RawMessage `json:"result"`
	}
	if phase == "seed" {
		db := reliableDB(t)
		reliableProjection(t, db, "avatar_owner_dm")
		store, err := NewMessageStore(context.Background(), db)
		if err != nil {
			t.Fatal(err)
		}
		v, auth := reliableProof(t, messageRequest(), "owner", 0)
		result, err := store.Send(context.Background(), v, auth)
		if err != nil {
			t.Fatal(err)
		}
		proof.Original, proof.Result = auth.claims, result.Result
		raw, err := json.Marshal(proof)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal("cannot store owned restart proof")
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &proof) != nil {
		t.Fatal("owned restart proof missing")
	}
	db := reliableConnection(t)
	store, err := NewMessageStore(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	v, query := reliableQueryProof(t, &Authorization{claims: proof.Original}, 7)
	got, err := store.Query(context.Background(), v, query)
	if err != nil || got == nil || !got.Replayed || string(got.Result) != string(proof.Result) {
		t.Fatalf("restart did not recover original receipt: %v", err)
	}
	for _, table := range []string{"p_hismsgs", "mindlink_message_outbox", "mindlink_operations", "message_sequence"} {
		if reliableRows(t, db, table) != 1 {
			t.Fatalf("restart duplicated/lost %s", table)
		}
	}
}
