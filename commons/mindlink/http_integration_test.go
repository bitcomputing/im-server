//go:build integration

package mindlink

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"gopkg.in/yaml.v3"
)

func prepareHTTPProjection(t *testing.T) (*sql.DB, *Verifier, ed25519.PrivateKey, Claims, []byte) {
	t.Helper()
	db, v, key, original, raw := seedQueryOperation(t)
	migration, err := os.ReadFile("../dbcommons/sqls/20261109.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"DELETE FROM mindlink_principals", "DELETE FROM mindlink_owner_barriers",
		"INSERT INTO mindlink_owner_barriers(environment,tenant_id,appkey,owner_id,read_enabled) VALUES('dev','fixture-tenant','fixture-app','42',TRUE)",
		"INSERT INTO mindlink_principals(environment,tenant_id,appkey,owner_id,principal_id,avatar_id,avatar_generation,policy_version,state) VALUES('dev','fixture-tenant','fixture-app','42','ma111111111111111111111111111111','avatar-A',3,8,'disabled')",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return db, v, key, original, raw
}

func callQueryHTTP(t *testing.T, serverURL string, key ed25519.PrivateKey, a *Authorization) (int, map[string]json.RawMessage) {
	t.Helper()
	c := a.claims
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
	r, err := http.NewRequest("GET", serverURL+c.Path, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(r)
	if err != nil {
		t.Fatal("owned loopback HTTP request failed")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil {
		t.Fatalf("invalid HTTP envelope, status=%d", resp.StatusCode)
	}
	return resp.StatusCode, envelope
}

func TestMySQLPolicyHTTPReadsOriginalAndHonorsCurrentReadRevocation(t *testing.T) {
	db, v, key, original, raw := prepareHTTPProjection(t)
	s := httptest.NewServer(NewPolicyHTTP(v, db))
	defer s.Close()
	a := queryProof(t, v, key, original, nil)
	status, envelope := callQueryHTTP(t, s.URL, key, a)
	if status != 200 || string(envelope["code"]) != "0" {
		t.Fatalf("valid HTTP query rejected: status=%d code=%s", status, envelope["code"])
	}
	var data struct {
		Operation json.RawMessage `json:"operation"`
		Replayed  bool            `json:"replayed"`
	}
	if json.Unmarshal(envelope["data"], &data) != nil || !data.Replayed || !bytes.Equal(data.Operation, raw) {
		t.Fatal("HTTP did not return the original immutable receipt")
	}
	assertQueryOriginalUnchanged(t, db, original, raw, 2)
	if _, err := db.Exec("UPDATE mindlink_owner_barriers SET read_enabled=FALSE WHERE owner_id='42'"); err != nil {
		t.Fatal(err)
	}
	status, envelope = callQueryHTTP(t, s.URL, key, a)
	if status != 403 || string(envelope["code"]) != "19008" || string(envelope["data"]) != "null" {
		t.Fatalf("revoked original query replay accepted: status=%d code=%s", status, envelope["code"])
	}
	assertQueryOriginalUnchanged(t, db, original, raw, 2)
}

func TestMySQLPolicyHTTPRejectsMissingStaleOrForeignProjection(t *testing.T) {
	for _, tc := range []struct {
		name, statement string
		status, code    int
	}{
		{"default read grant", "UPDATE mindlink_owner_barriers SET read_enabled=DEFAULT", 403, 19008},
		{"missing barrier", "DELETE FROM mindlink_owner_barriers", 409, 19005},
		{"missing principal", "DELETE FROM mindlink_principals", 409, 19005},
		{"stale policy", "UPDATE mindlink_principals SET policy_version=9", 409, 19004},
		{"foreign owner", "UPDATE mindlink_principals SET owner_id='43'", 403, 19002},
		{"foreign tenant", "UPDATE mindlink_principals SET tenant_id='other'", 403, 19002},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, v, key, original, raw := prepareHTTPProjection(t)
			if _, err := db.Exec(tc.statement); err != nil {
				t.Fatal(err)
			}
			s := httptest.NewServer(NewPolicyHTTP(v, db))
			defer s.Close()
			status, envelope := callQueryHTTP(t, s.URL, key, queryProof(t, v, key, original, nil))
			if status != tc.status || string(envelope["code"]) != jsonNumber(tc.code) || string(envelope["data"]) != "null" {
				t.Fatalf("bad current projection accepted: status=%d code=%s", status, envelope["code"])
			}
			assertQueryOriginalUnchanged(t, db, original, raw, 1)
		})
	}
}

func TestMySQLPolicyHTTPDisableValidatesButDoesNotInventReady(t *testing.T) {
	db, v, key, original, raw := prepareHTTPProjection(t)
	s := httptest.NewServer(NewPolicyHTTP(v, db))
	defer s.Close()
	for _, tc := range []struct {
		name         string
		change       func(map[string]any)
		status, code int
	}{
		{"valid closed", func(map[string]any) {}, 409, 19005},
		{"unknown body field", func(b map[string]any) { b["unexpected"] = true }, 400, 19001},
		{"wrong digest", func(b map[string]any) { b["request_digest"] = strings.Repeat("f", 64) }, 409, 19003},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := disableBody()
			body["operation_id"] = strings.Repeat("e", 64)
			body["request_id"] = "disable-still-closed"
			tc.change(body)
			encoded := fixtureJSON(t, body)
			c := original
			c.OperationID = body["operation_id"].(string)
			c.RequestID = body["request_id"].(string)
			c.RequestDigest = body["request_digest"].(string)
			c.IssuedAt = time.Now().Unix()
			c.ExpiresAt = c.IssuedAt + 60
			c.JTI = strings.Repeat("f", 64)
			c.BodySHA256 = fmt.Sprintf("%x", sha256.Sum256(encoded))
			token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
			r, err := http.NewRequest("POST", s.URL+PrincipalDisablePath, bytes.NewReader(encoded))
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Content-Type", "application/json")
			client := http.Client{Timeout: 5 * time.Second}
			response, err := client.Do(r)
			if err != nil {
				t.Fatal("owned loopback disable request failed")
			}
			defer response.Body.Close()
			var envelope map[string]json.RawMessage
			if json.NewDecoder(response.Body).Decode(&envelope) != nil || response.StatusCode != tc.status || string(envelope["code"]) != jsonNumber(tc.code) || string(envelope["data"]) != "null" {
				t.Fatalf("incorrect closed/invalid disable result: status=%d code=%s", response.StatusCode, envelope["code"])
			}
			assertQueryOriginalUnchanged(t, db, original, raw, 1)
		})
	}
}

// Run prepare on the host, then initial/recover inside the owned MySQL network
// namespace. The operator restarts only the labeled fixture gateway between runs.
func TestMySQLPolicyHTTPGatewayProcessRecovery(t *testing.T) {
	phase := os.Getenv("MINDLINK_TEST_GATEWAY_PHASE")
	if phase == "" {
		t.Skip("requires separately built owned gateway; not runtime coverage")
	}
	if phase == "prepare" {
		db, _, _, _, _ := prepareHTTPProjection(t)
		initial, err := os.ReadFile("../../sql/imserver.sql")
		if err != nil {
			t.Fatal(err)
		}
		// Reuse the owned fixture without changing production initialization SQL.
		initial = bytes.ReplaceAll(initial, []byte("CREATE TABLE `"), []byte("CREATE TABLE IF NOT EXISTS `"))
		if _, err := db.Exec(string(initial)); err != nil {
			t.Fatal(err)
		}
		var private struct {
			DSN string `json:"dsn"`
		}
		b, err := os.ReadFile(os.Getenv("MINDLINK_TEST_DSN_FILE"))
		if err != nil || json.Unmarshal(b, &private) != nil {
			t.Fatal("read owned private fixture")
		}
		cfg, err := mysql.ParseDSN(private.DSN)
		if err != nil {
			t.Fatal("invalid owned DSN")
		}
		// The query fixture may predate the fresh schema's current version marker.
		if _, err := db.Exec("UPDATE globalconfs SET conf_value='20261109' WHERE conf_key='jimdb_version'"); err != nil {
			t.Fatal(err)
		}
		upgradeNativeFixture(t, db, cfg, "20261114")
		cfg.Addr = "127.0.0.1:3306"
		trust, _ := fixtureTrust()
		runtime := map[string]any{
			"nodeName": "policy-query-066519ee", "nodeHost": "127.0.0.1", "msgStoreEngine": "mysql",
			"log": map[string]any{"logPath": "/opt/logs", "logName": "query-fixture"}, "kvdb": map[string]any{"isOpen": false},
			"mysql":      map[string]any{"user": cfg.User, "password": cfg.Passwd, "address": cfg.Addr, "name": cfg.DBName, "debug": false},
			"apiGateway": map[string]any{"httpPort": 9001}, "connectManager": map[string]any{"wsPort": 9003}, "navGateway": map[string]any{"httpPort": 9002}, "adminGateway": map[string]any{"httpPort": 8090},
			"mindlink": map[string]any{"enabled": true, "trusts": []any{map[string]any{
				"issuer": trust.Issuer, "kid": trust.KeyID, "audience": trust.Audience, "environment": trust.Environment, "tenant_id": trust.TenantID, "appkey": trust.AppKey, "system_actor_id": trust.SystemActorID,
				"public_key_ed25519_base64": base64.StdEncoding.EncodeToString(trust.PublicKey), "query_operations": []string{"operations.query"}, "mutation_operations": []string{OperationPrincipalDisable},
			}}},
		}
		conf, err := yaml.Marshal(runtime)
		if err != nil {
			t.Fatal(err)
		}
		folder := filepath.Join("..", "..", ".local", "mindlink-policy")
		if err := os.WriteFile(filepath.Join(folder, "gateway-fixture-private.yml"), conf, 0600); err != nil {
			t.Fatal("write private fixture config")
		}
		linux, err := json.Marshal(map[string]string{"dsn": cfg.FormatDSN()})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "fixture-gateway-linux-private.json"), linux, 0600); err != nil {
			t.Fatal("write private Linux fixture DSN")
		}
		t.Log("prepared owned fixture only; no HTTP/restart acceptance in prepare phase")
		return
	}
	if phase != "initial" && phase != "recover" {
		t.Fatal("unknown fixture phase")
	}
	endpoint := os.Getenv("MINDLINK_TEST_GATEWAY_URL")
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "" || u.RawQuery != "" || u.User != nil {
		t.Fatal("refusing non-owned loopback gateway")
	}
	db := ownedDB(t)
	v, key, original, raw := queryFixture(t)
	a := queryProof(t, v, key, original, func(c *Claims) {
		if phase == "recover" {
			c.JTI = strings.Repeat("e", 64)
		}
	})
	if phase == "initial" {
		observed := make(chan error, 1)
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req := r.Clone(r.Context())
			req.URL = u.ResolveReference(r.URL)
			req.Host = u.Host
			req.RequestURI = ""
			response, err := http.DefaultTransport.RoundTrip(req)
			if err == nil {
				var envelope struct {
					Code int `json:"code"`
					Data struct {
						Operation json.RawMessage `json:"operation"`
					} `json:"data"`
				}
				if json.NewDecoder(io.LimitReader(response.Body, MaxBodyBytes)).Decode(&envelope) != nil || response.StatusCode != 200 || envelope.Code != 0 || !bytes.Equal(envelope.Data.Operation, raw) {
					err = fmt.Errorf("upstream fixture did not return the original")
				}
				response.Body.Close()
			}
			conn, _, hijackErr := w.(http.Hijacker).Hijack()
			if hijackErr == nil {
				conn.Close()
			} else {
				err = hijackErr
			}
			observed <- err
		}))
		defer proxy.Close()
		token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, a.claims))
		r, err := http.NewRequest("GET", proxy.URL+a.claims.Path, nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		client := http.Client{Timeout: 5 * time.Second}
		response, err := client.Do(r)
		if response != nil {
			response.Body.Close()
		}
		if err == nil {
			t.Fatal("response-loss proxy returned an HTTP response")
		}
		select {
		case err := <-observed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no completed upstream response observed")
		}
		assertQueryOriginalUnchanged(t, db, original, raw, 2)
	}
	status, envelope := callQueryHTTP(t, endpoint, key, a)
	var data struct {
		Operation json.RawMessage `json:"operation"`
		Replayed  bool            `json:"replayed"`
	}
	if status != 200 || string(envelope["code"]) != "0" || json.Unmarshal(envelope["data"], &data) != nil || !data.Replayed || !bytes.Equal(data.Operation, raw) {
		t.Fatalf("gateway original recovery failed: status=%d code=%s", status, envelope["code"])
	}
	tickets := 2
	if phase == "recover" {
		tickets = 3
	}
	assertQueryOriginalUnchanged(t, db, original, raw, tickets)
}
