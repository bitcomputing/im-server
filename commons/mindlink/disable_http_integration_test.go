//go:build integration

package mindlink

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMySQLPublicDisableRequiresSourceAndCommitsOriginal(t *testing.T) {
	db := nativeFixture(t)
	for _, q := range []string{"DELETE FROM mindlink_tickets", "DELETE FROM mindlink_operations", "UPDATE mindlink_owner_barriers SET write_enabled=TRUE"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	v, auth := authorityProof(t, time.Now(), "system", false)
	source, _ := sourceChild(t, string(fixtureJSON(t, authoritySnapshot(auth))), 0, 0)
	h, err := NewPolicyHTTPWithDisable(context.Background(), v, db, source)
	if err != nil {
		t.Fatal(err)
	}
	_, key := fixtureTrust()
	request := func() *httptest.ResponseRecorder {
		token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, auth.claims))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, fixtureRequest(t, token, "POST", PrincipalDisablePath, string(auth.body)))
		return rec
	}
	for _, tc := range []struct {
		name, output       string
		exit, status, code int
	}{
		{"not-ready", "", 3, 409, 19005},
		{"denied", "", 4, 403, 19008},
		{"source-failure", "", 5, 503, 19007},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refusal, _ := sourceChild(t, tc.output, tc.exit, 0)
			h, err = NewPolicyHTTPWithDisable(context.Background(), v, db, refusal)
			if err != nil {
				t.Fatal(err)
			}
			assertPolicyError(t, request(), tc.status, tc.code)
			assertDisableProjection(t, db, "active", 2, 7, 0, 0)
		})
	}
	h, err = NewPolicyHTTPWithDisable(context.Background(), v, db, source)
	if err != nil {
		t.Fatal(err)
	}
	first := request()
	if first.Code != 200 {
		t.Fatalf("disable HTTP status=%d body=%s", first.Code, first.Body.String())
	}
	assertDisableProjection(t, db, "disabled", 3, 8, 1, 1)
	var result struct {
		Data struct {
			Operation json.RawMessage `json:"operation"`
			Replayed  bool            `json:"replayed"`
		} `json:"data"`
	}
	if json.Unmarshal(first.Body.Bytes(), &result) != nil || result.Data.Replayed || v.validateDisableResult(result.Data.Operation, auth.claims) != nil {
		t.Fatal("invalid original receipt")
	}
	again := request()
	var replay struct {
		Data struct {
			Operation json.RawMessage `json:"operation"`
			Replayed  bool            `json:"replayed"`
		} `json:"data"`
	}
	if again.Code != 200 || json.Unmarshal(again.Body.Bytes(), &replay) != nil || !replay.Data.Replayed || string(replay.Data.Operation) != string(result.Data.Operation) {
		t.Fatal("retry did not recover original")
	}
	assertDisableProjection(t, db, "disabled", 3, 8, 1, 1)
	denied, _ := sourceChild(t, "", 4, 0)
	h, err = NewPolicyHTTPWithDisable(context.Background(), v, db, denied)
	if err != nil {
		t.Fatal(err)
	}
	assertPolicyError(t, request(), 403, 19008)
	assertDisableProjection(t, db, "disabled", 3, 8, 1, 1)
}
