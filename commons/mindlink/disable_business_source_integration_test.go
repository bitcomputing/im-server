//go:build integration

package mindlink

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMySQLPublicDisableRealBusinessSource(t *testing.T) {
	program, config := os.Getenv("MINDLINK_REAL_SOURCE_PROGRAM"), os.Getenv("MINDLINK_REAL_SOURCE_CONFIG")
	if program == "" || config == "" {
		t.Skip("real business source executable and owned configuration not supplied")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(config), "real-source-snapshot.json"))
	var snapshot disableAuthoritySnapshot
	if err != nil || DecodeStrictObject(raw, &snapshot, authoritySourceFields) != nil {
		t.Fatal("invalid real source snapshot")
	}
	if snapshot.Environment != "dev" || snapshot.TenantID != "isolated-tenant" || snapshot.AppKey != "isolated-app" {
		t.Fatal("refusing non-fixture source scope")
	}
	db := nativeFixture(t)
	if _, err = db.Exec("DELETE FROM mindlink_operations; DELETE FROM mindlink_tickets; DELETE FROM mindlink_principals; DELETE FROM mindlink_owner_barriers"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO apps(app_key,app_secret,app_secure_key) VALUES(?, 'fixture-only', '0123456789abcdef')", snapshot.AppKey); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO users(app_key,user_id,nickname) VALUES(?,?,'fixture-principal')", snapshot.AppKey, snapshot.PrincipalID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO mindlink_owner_barriers(environment,tenant_id,appkey,owner_id,read_enabled,write_enabled) VALUES(?,?,?,?,TRUE,TRUE)", snapshot.Environment, snapshot.TenantID, snapshot.AppKey, snapshot.OwnerID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO mindlink_principals(environment,tenant_id,appkey,owner_id,principal_id,avatar_id,avatar_generation,policy_version,state) VALUES(?,?,?,?,?,?,?,?,'active')", snapshot.Environment, snapshot.TenantID, snapshot.AppKey, snapshot.OwnerID, snapshot.PrincipalID, snapshot.AvatarID, snapshot.TargetAvatarGeneration, snapshot.ExpectedPolicyVersion); err != nil {
		t.Fatal(err)
	}
	trust, key := fixtureTrust()
	trust.Environment, trust.TenantID, trust.AppKey = snapshot.Environment, snapshot.TenantID, snapshot.AppKey
	trust.Issuer, trust.Audience, trust.SystemActorID = snapshot.Issuer, snapshot.Audience, snapshot.SystemActorID
	trust.QueryOperations, trust.MutationOperations = []string{"operations.query"}, []string{OperationPrincipalDisable}
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	// Use the persisted business bytes, not a re-encoding in IM struct field order.
	payload, err := os.ReadFile(filepath.Join(filepath.Dir(config), "real-source-wire.json"))
	var body PrincipalDisableRequest
	if err != nil || json.Unmarshal(payload, &body) != nil {
		t.Fatal("original business wire unavailable")
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != snapshot.WireBodySHA256 {
		t.Fatal("wire bytes differ between business and IM")
	}
	claims := Claims{Issuer: trust.Issuer, Audience: trust.Audience, Environment: trust.Environment, TenantID: trust.TenantID, AppKey: trust.AppKey, OwnerID: snapshot.OwnerID, OperationType: OperationPrincipalDisable, OperationID: body.OperationID, RequestID: body.RequestID, TargetID: body.PrincipalID, ActorKind: "system", ActorID: trust.SystemActorID, ExpectedPolicyVersion: body.ExpectedPolicyVersion, DesiredPolicyVersion: body.DesiredPolicyVersion, RequestDigest: body.RequestDigest, Method: "POST", Path: PrincipalDisablePath, BodySHA256: snapshot.WireBodySHA256, IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Unix() + 60, JTI: strings.Repeat("9", 64)}
	source, err := NewDisableAuthoritySource(DisableAuthoritySourceConfig{Enabled: true, Program: program, ConfigPath: config})
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewPolicyHTTPWithDisable(context.Background(), v, db, source)
	if err != nil {
		t.Fatal(err)
	}
	var original string
	for i := 0; i < 2; i++ {
		token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, claims))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, fixtureRequest(t, token, "POST", PrincipalDisablePath, string(payload)))
		var response struct {
			Data struct {
				Operation json.RawMessage `json:"operation"`
				Replayed  bool            `json:"replayed"`
			} `json:"data"`
		}
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &response) != nil || response.Data.Replayed != (i == 1) || v.validateDisableResult(response.Data.Operation, claims) != nil {
			t.Fatalf("real source HTTP result invalid, status=%d", rec.Code)
		}
		if i == 0 {
			original = string(response.Data.Operation)
		} else if original != string(response.Data.Operation) {
			t.Fatal("original result changed")
		}
	}
	var state string
	var generation, policy int64
	if err = db.QueryRow("SELECT state,avatar_generation,policy_version FROM mindlink_principals WHERE appkey=? AND principal_id=?", snapshot.AppKey, snapshot.PrincipalID).Scan(&state, &generation, &policy); err != nil || state != "disabled" || generation != snapshot.TargetAvatarGeneration || policy != snapshot.DesiredPolicyVersion {
		t.Fatal("actual IM projection mismatch")
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM mindlink_operations WHERE issuer=? AND operation_id=?", snapshot.Issuer, body.OperationID).Scan(&count); err != nil || count != 1 {
		t.Fatal("not exactly one original operation")
	}
	t.Log("real business executable -> verified source -> signed IM HTTP -> committed projection and original replay passed")
}
