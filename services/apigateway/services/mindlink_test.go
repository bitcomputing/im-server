package services

import (
	"bytes"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
	"im-server/commons/configures"
	"im-server/commons/mindlink"
)

func TestMindlinkHTTPSTADeploymentExample(t *testing.T) {
	raw, err := os.ReadFile("../../../deploy/sta/mindlink.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg configures.ImConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	h, err := NewMindlinkHTTP(cfg.Mindlink, nil)
	if err != nil || cfg.Mindlink.Enabled || cfg.Mindlink.DisableAuthoritySource.Enabled {
		t.Fatal("deployment example must remain disabled", err)
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("POST", "/apigateway/mindlink/principals/ensure", nil))
	if r.Code != 409 {
		t.Fatalf("disabled example is not closed: status=%d", r.Code)
	}
	valid := mindlinkConfigFixture()
	cfg.Mindlink.Enabled = true
	// Test the complete operation list and STA binding without a live authority or DB.
	cfg.Mindlink.Trusts[0].AppKey = "fixture-sta-app"
	cfg.Mindlink.Trusts[0].PublicKey = valid.Trusts[0].PublicKey
	h, err = NewMindlinkHTTP(cfg.Mindlink, &sql.DB{})
	if err != nil {
		t.Fatal("STA trust with the deployment operation list rejected", err)
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/apigateway/mindlink/operations/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil))
	if r.Code != 403 {
		t.Fatalf("STA handler did not enforce actor authorization: status=%d", r.Code)
	}
}

func mindlinkConfigFixture() configures.MindlinkConfig {
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	return configures.MindlinkConfig{Enabled: true, Trusts: []configures.MindlinkTrustConfig{{
		Issuer: "fixture-biz", KeyID: "fixture-key", Audience: "fixture-im", Environment: "dev", TenantID: "fixture-tenant", AppKey: "fixture-app", SystemActorID: "fixture-system",
		PublicKey: base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), QueryOperations: []string{"operations.query"}, MutationOperations: []string{"principals.disable"},
	}}}
}

func TestMindlinkStationStopMutationIsExplicitlyConfigurable(t *testing.T) {
	cfg := mindlinkConfigFixture()
	cfg.Trusts[0].MutationOperations = []string{"stations.stopped"}
	if _, err := NewMindlinkHTTP(cfg, &sql.DB{}); err != nil {
		t.Fatalf("restricted station operation rejected: %v", err)
	}
}

func TestMindlinkHTTPConfigFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*configures.MindlinkConfig)
	}{
		{"bad base64", func(c *configures.MindlinkConfig) { c.Trusts[0].PublicKey = "not a public key" }},
		{"missing padding", func(c *configures.MindlinkConfig) { c.Trusts[0].PublicKey = c.Trusts[0].PublicKey[:43] }},
		{"short key", func(c *configures.MindlinkConfig) {
			c.Trusts[0].PublicKey = base64.StdEncoding.EncodeToString([]byte{1})
		}},
		{"duplicate binding", func(c *configures.MindlinkConfig) { c.Trusts = append(c.Trusts, c.Trusts[0]) }},
		{"unknown operation", func(c *configures.MindlinkConfig) { c.Trusts[0].QueryOperations = []string{"messages.unsupported"} }},
		{"cross operation", func(c *configures.MindlinkConfig) {
			c.Trusts[0].MutationOperations = append(c.Trusts[0].MutationOperations, "operations.query")
		}},
		{"empty trust", func(c *configures.MindlinkConfig) { c.Trusts = nil }},
		{"invalid authority source", func(c *configures.MindlinkConfig) {
			c.DisableAuthoritySource = configures.MindlinkAuthoritySourceConfig{Enabled: true, Program: "relative", ConfigPath: "relative"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mindlinkConfigFixture()
			tc.change(&cfg)
			h, err := NewMindlinkHTTP(cfg, &sql.DB{})
			if !errors.Is(err, mindlink.ErrInvalidRequest) {
				t.Fatalf("invalid trust accepted: %v", err)
			}
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest("GET", "/apigateway/mindlink/operations/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil))
			if r.Code != 409 {
				t.Fatalf("invalid config did not produce a closed handler: %d", r.Code)
			}
		})
	}
}

func TestMindlinkHTTPYAMLDefaultAndEnabledTrust(t *testing.T) {
	var cfg configures.ImConfig
	if err := yaml.Unmarshal([]byte("apiGateway:\n  httpPort: 9001\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	h, err := NewMindlinkHTTP(cfg.Mindlink, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("POST", "/apigateway/mindlink/principals/disable", nil))
	if r.Code != 409 {
		t.Fatal("omitted configuration is not closed")
	}
	valid := mindlinkConfigFixture()
	raw := fmt.Sprintf("mindlink:\n  enabled: true\n  trusts:\n    - issuer: fixture-biz\n      kid: fixture-key\n      audience: fixture-im\n      environment: dev\n      tenant_id: fixture-tenant\n      appkey: fixture-app\n      system_actor_id: fixture-system\n      public_key_ed25519_base64: %s\n      query_operations: [operations.query]\n      mutation_operations: [principals.disable]\n", valid.Trusts[0].PublicKey)
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	closed, missingDB := NewMindlinkHTTP(cfg.Mindlink, nil)
	if !errors.Is(missingDB, mindlink.ErrNotReady) || closed == nil {
		t.Fatal("enabled configuration without database did not fail closed")
	}
	h, err = NewMindlinkHTTP(cfg.Mindlink, &sql.DB{})
	if err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/apigateway/mindlink/operations/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil))
	if r.Code != 403 {
		t.Fatalf("valid enabled trust not used: status=%d", r.Code)
	}
}
