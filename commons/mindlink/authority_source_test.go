package mindlink

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The child uses the source's real argv/stdout/exit protocol, without a database.
func TestMain(m *testing.M) {
	if os.Getenv("IM_AUTHORITY_TEST_CHILD") == "1" {
		var cfg struct {
			Output, Marker string
			Exit, DelayMS  int
			Principal      bool
			Conversation   bool
		}
		args := os.Args[1:]
		principal := len(args) > 0 && args[0] == "--principal"
		conversation := len(args) > 0 && args[0] == "--conversation"
		if principal || conversation {
			args = args[1:]
		}
		if len(args) != 4 || args[0] != "--config" || args[2] != "--operation-id" || args[3] != strings.Repeat("e", 64) {
			os.Exit(2)
		}
		raw, err := os.ReadFile(args[1])
		if err != nil || json.Unmarshal(raw, &cfg) != nil || cfg.Principal != principal || cfg.Conversation != conversation {
			os.Exit(2)
		}
		if cfg.Marker != "" {
			_ = os.WriteFile(cfg.Marker, []byte("called"), 0600)
		}
		time.Sleep(time.Duration(cfg.DelayMS) * time.Millisecond)
		fmt.Fprint(os.Stderr, "private-DSN-JWS-body-must-not-leak")
		fmt.Fprint(os.Stdout, cfg.Output)
		os.Exit(cfg.Exit)
	}
	os.Exit(m.Run())
}

func authorityProof(t *testing.T, at time.Time, actor string, wrongDigest bool) (*Verifier, *Authorization) {
	t.Helper()
	trust, key := fixtureTrust()
	trust.MutationOperations = []string{OperationPrincipalDisable}
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	actorID := trust.SystemActorID
	if actor == "owner" {
		actorID = "42"
	}
	semantic := fixtureJSON(t, []any{"mindlink-policy-n1", OperationPrincipalDisable, "dev", "fixture-tenant", "fixture-app", "42", actor, actorID, int64(0), disablePrincipal, "avatar-A", int64(3), int64(7), int64(8), "disabled"})
	hash := sha256.Sum256(semantic)
	b := disableBody()
	b["request_id"], b["request_digest"] = strings.Repeat("e", 64), hex.EncodeToString(hash[:])
	if wrongDigest {
		b["request_digest"] = strings.Repeat("f", 64)
	}
	raw := string(fixtureJSON(t, b))
	c := fixtureClaims(raw)
	c["operation_type"], c["path"], c["target_id"] = OperationPrincipalDisable, disablePath, disablePrincipal
	c["request_id"], c["request_digest"] = b["request_id"], b["request_digest"]
	c["actor_kind"], c["actor_id"] = actor, actorID
	c["iat"], c["exp"] = at.Unix(), at.Unix()+60
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
	a, err := v.Verify(fixtureRequest(t, token, "POST", disablePath, raw), OperationPrincipalDisable, at)
	if err != nil {
		t.Fatal(err)
	}
	return v, a
}

// Exact21-field 75739e949 source protocol. No test fixture creates an IM grant.
func authoritySnapshot(a *Authorization) map[string]any {
	c := a.claims
	return map[string]any{
		"version": "mindlink-disable-authority-v1", "environment": c.Environment, "tenant_id": c.TenantID,
		"appkey": c.AppKey, "issuer": c.Issuer, "audience": c.Audience, "system_actor_id": c.ActorID,
		"owner_id": c.OwnerID, "operation_type": OperationPrincipalDisable, "parent_operation_id": c.RequestID,
		"parent_digest": strings.Repeat("d", 64), "wire_operation_id": c.OperationID, "wire_request_digest": c.RequestDigest,
		"wire_body_sha256": c.BodySHA256, "principal_id": c.TargetID, "avatar_id": "avatar-A",
		"target_avatar_generation": int64(3), "expected_policy_version": int64(7), "desired_policy_version": int64(8),
		"lifecycle_state": "retired", "retired_at_unix_seconds": int64(1700000000),
	}
}

func sourceChild(t *testing.T, raw string, exit, delayMS int) (*DisableAuthoritySource, string) {
	t.Helper()
	t.Setenv("IM_AUTHORITY_TEST_CHILD", "1")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "called")
	path := filepath.Join(dir, "source.json")
	if err := os.WriteFile(path, fixtureJSON(t, map[string]any{"Output": raw, "Exit": exit, "DelayMS": delayMS, "Marker": marker}), 0600); err != nil {
		t.Fatal(err)
	}
	source, err := NewDisableAuthoritySource(DisableAuthoritySourceConfig{Enabled: true, Program: program, ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	return source, marker
}

func TestDisableAuthoritySourceStrictSnapshot(t *testing.T) {
	v, a := authorityProof(t, time.Now(), "system", false)
	b, err := v.ParsePrincipalDisable(a)
	if err != nil {
		t.Fatal(err)
	}
	good := string(fixtureJSON(t, authoritySnapshot(a)))
	if err := checkDisableAuthoritySnapshot([]byte(good), a.claims, b); err != nil {
		t.Fatalf("fixed source protocol rejected: %v", err)
	}
	for field, value := range authoritySnapshot(a) {
		for _, change := range []string{"missing", "null", "different"} {
			t.Run(field+"/"+change, func(t *testing.T) {
				m := authoritySnapshot(a)
				switch change {
				case "missing":
					delete(m, field)
				case "null":
					m[field] = nil
				default:
					if n, ok := value.(int64); ok {
						m[field] = n + 1
						if field == "retired_at_unix_seconds" {
							m[field] = int64(0)
						}
					} else {
						m[field] = "different"
					}
				}
				if err := checkDisableAuthoritySnapshot(fixtureJSON(t, m), a.claims, b); err == nil {
					t.Fatal("invalid or unbound source accepted")
				}
			})
		}
	}
	for _, raw := range []string{
		"{}", "null", good + " {}", strings.TrimSuffix(good, "}") + `,"version":"mindlink-disable-authority-v1"}`,
		strings.TrimSuffix(good, "}") + `,"\u0076ersion":"mindlink-disable-authority-v1"}`,
		strings.Replace(good, `"owner_id"`, `"Owner_id"`, 1),
		strings.Replace(good, `"target_avatar_generation":3`, `"target_avatar_generation":3e0`, 1),
		strings.Replace(good, `"target_avatar_generation":3`, `"target_avatar_generation":9223372036854775808`, 1),
		strings.Replace(good, `"avatar-A"`, `"\ud800"`, 1), good + string([]byte{0xff}),
	} {
		if err := checkDisableAuthoritySnapshot([]byte(raw), a.claims, b); err == nil {
			t.Fatal("malformed source accepted")
		}
	}
}

func TestDisableAuthoritySourceProcess(t *testing.T) {
	v, a := authorityProof(t, time.Now(), "system", false)
	good := string(fixtureJSON(t, authoritySnapshot(a)))
	for _, tc := range []struct {
		name, raw string
		exit      int
		want      error
	}{
		{"success", good, 0, nil}, {"empty", "", 0, ErrUnknownResult},
		{"valid_json_nonzero", good, 5, ErrUnknownResult}, {"denied", good, 4, ErrForbidden},
		{"disabled", good, 3, ErrNotReady}, {"invalid_config", good, 2, ErrNotReady},
		{"trailing", good + " {}", 0, ErrUnknownResult}, {"flood", strings.Repeat("x", 33000), 0, ErrUnknownResult},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := sourceChild(t, tc.raw, tc.exit, 0)
			err := s.Check(context.Background(), v, a)
			if !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
				t.Fatalf("result=%v want=%v", err, tc.want)
			}
			if strings.Contains(fmt.Sprint(err), "private-DSN") {
				t.Fatal("source diagnostics leaked")
			}
		})
	}
	// Re-read the same fixed source on every invocation, never cache an allow.
	s, marker := sourceChild(t, good, 0, 0)
	if err := s.Check(context.Background(), v, a); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("child did not execute")
	}
	if err := os.WriteFile(s.configPath, fixtureJSON(t, map[string]any{"Exit": 4}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(context.Background(), v, a); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cached allow survived refusal: %v", err)
	}
}

func TestDisableAuthoritySourceRefusesBeforeExecution(t *testing.T) {
	for _, mode := range []string{"wrong_digest", "owner", "expired", "foreign_verifier", "nil_context", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			at, actor := time.Now(), "system"
			if mode == "expired" {
				at = at.Add(-time.Hour)
			}
			if mode == "owner" {
				actor = "owner"
			}
			v, a := authorityProof(t, at, actor, mode == "wrong_digest")
			if mode == "foreign_verifier" {
				v, _ = authorityProof(t, time.Now(), "system", false)
			}
			s, marker := sourceChild(t, string(fixtureJSON(t, authoritySnapshot(a))), 0, 0)
			ctx := context.Background()
			if mode == "nil_context" {
				ctx = nil
			}
			if mode == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if err := s.Check(ctx, v, a); err == nil {
				t.Fatal("unverified source invocation allowed")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("invalid request invoked child")
			}
		})
	}
}

func TestDisableAuthoritySourceDeadlineAndConfiguration(t *testing.T) {
	v, a := authorityProof(t, time.Now(), "system", false)
	s, _ := sourceChild(t, string(fixtureJSON(t, authoritySnapshot(a))), 0, 10000)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := s.Check(ctx, v, a); !errors.Is(err, ErrUnknownResult) || time.Since(start) > 3*time.Second {
		t.Fatalf("earlier deadline not honored: %v", err)
	}
	closed, err := NewDisableAuthoritySource(DisableAuthoritySourceConfig{})
	if err != nil || !errors.Is(closed.Check(context.Background(), v, a), ErrNotReady) || !errors.Is((*DisableAuthoritySource)(nil).Check(context.Background(), v, a), ErrNotReady) {
		t.Fatal("default source must be closed")
	}
	for _, cfg := range []DisableAuthoritySourceConfig{{Enabled: true}, {Enabled: true, Program: "relative", ConfigPath: s.configPath}, {Enabled: true, Program: s.program, ConfigPath: "relative"}} {
		if source, err := NewDisableAuthoritySource(cfg); err == nil || source != nil {
			t.Fatal("non-fixed source configuration accepted")
		}
	}
}

func TestDisableAuthoritySourceRealBIZDisabled(t *testing.T) {
	program, path := os.Getenv("IM_AUTHORITY_BIZ_TEST_PROGRAM"), os.Getenv("IM_AUTHORITY_BIZ_TEST_CONFIG")
	if program == "" || path == "" {
		t.Skip("fixed BIZ executable and isolated disabled fixture not supplied")
	}
	raw, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(raw)) != `{"enabled":false}` {
		t.Fatal("real process check requires the isolated disabled fixture")
	}
	v, a := authorityProof(t, time.Now(), "system", false)
	source, err := NewDisableAuthoritySource(DisableAuthoritySourceConfig{true, program, path})
	if err != nil || !errors.Is(source.Check(context.Background(), v, a), ErrNotReady) {
		t.Fatalf("real closed source accepted: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, "--config", path, "--operation-id", a.claims.RequestID)
	configureAuthorityProcess(cmd)
	stdout, err := cmd.Output()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 || len(stdout) != 0 {
		t.Fatal("fixed source did not report SOURCE_DISABLED without a snapshot")
	}
}

func TestDisableAuthoritySourceTimeBounds(t *testing.T) {
	t.Run("standalone_cap", func(t *testing.T) {
		v, a := authorityProof(t, time.Now(), "system", false)
		s, marker := sourceChild(t, string(fixtureJSON(t, authoritySnapshot(a))), 0, 30000)
		start := time.Now()
		err := s.Check(context.Background(), v, a)
		elapsed := time.Since(start)
		if !errors.Is(err, ErrUnknownResult) || elapsed < 9*time.Second || elapsed > 13*time.Second {
			t.Fatalf("standalone source cap: elapsed=%s err=%v", elapsed, err)
		}
		if _, err := os.Stat(marker); err != nil {
			t.Fatal("capped child did not execute")
		}
	})
	t.Run("ticket_expires_during_call", func(t *testing.T) {
		// N1 permits five seconds of skew. Start within that grace, finish after it.
		at := time.Now().Truncate(time.Second).Add(-62 * time.Second)
		v, a := authorityProof(t, at, "system", false)
		s, marker := sourceChild(t, string(fixtureJSON(t, authoritySnapshot(a))), 0, 3400)
		if err := s.Check(context.Background(), v, a); !errors.Is(err, ErrForbidden) {
			t.Fatalf("ticket expiry ignored after source returned: %v", err)
		}
		if _, err := os.Stat(marker); err != nil {
			t.Fatal("ticket was rejected before the source ran")
		}
	})
}
