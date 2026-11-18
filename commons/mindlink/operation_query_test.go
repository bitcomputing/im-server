package mindlink

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func queryFixture(t *testing.T) (*Verifier, ed25519.PrivateKey, Claims, []byte) {
	t.Helper()
	trust, key := fixtureTrust()
	trust.QueryOperations = []string{"operations.query"}
	trust.MutationOperations = []string{OperationPrincipalDisable}
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	c := fixtureClaims(string(fixtureJSON(t, disableBody())))
	c["operation_type"], c["path"], c["target_id"] = OperationPrincipalDisable, PrincipalDisablePath, disablePrincipal
	c["request_id"], c["request_digest"] = "fixture-disable-1", disableDigest
	c["iat"], c["exp"], c["jti"] = int64(0), int64(0), ""
	var original Claims
	if err := json.Unmarshal(fixtureJSON(t, c), &original); err != nil {
		t.Fatal(err)
	}
	version, generation, token := int64(8), int64(3), strings.Repeat("c", 64)
	committed := "2026-11-09T03:00:00.124Z"
	r := OperationResult{
		OperationID: original.OperationID, OperationType: original.OperationType, RequestID: original.RequestID, RequestDigest: original.RequestDigest,
		Environment: original.Environment, TenantID: original.TenantID, AppKey: original.AppKey, OwnerID: original.OwnerID, TargetID: original.TargetID,
		ActorKind: original.ActorKind, ActorID: original.ActorID, AvatarGeneration: original.AvatarGeneration,
		ExpectedPolicyVersion: original.ExpectedPolicyVersion, DesiredPolicyVersion: original.DesiredPolicyVersion,
		DesiredState: fixtureJSON(t, PrincipalDisabledState{"disabled", "avatar-A", disablePrincipal, 3}), State: "ready",
		EffectivePolicyVersion: &version, EffectiveAvatarGeneration: &generation, CommitVersion: &token,
		AcceptedAt: "2026-11-09T03:00:00.123Z", CommittedAt: &committed,
	}
	return v, key, original, fixtureJSON(t, r)
}

func queryProof(t *testing.T, v *Verifier, key ed25519.PrivateKey, original Claims, change func(*Claims)) *Authorization {
	t.Helper()
	c := original
	c.OperationType, c.Method, c.Path = "operations.query", "GET", "/apigateway/mindlink/operations/"+c.OperationID
	c.ExpectedPolicyVersion, c.DesiredPolicyVersion = 8, 8
	c.BodySHA256 = fmt.Sprintf("%x", sha256.Sum256(nil))
	c.IssuedAt, c.ExpiresAt, c.JTI = time.Now().Unix(), time.Now().Unix()+60, strings.Repeat("d", 64)
	if change != nil {
		change(&c)
	}
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
	a, err := v.Verify(fixtureRequest(t, token, c.Method, c.Path, ""), "operations.query", time.Now())
	if err != nil {
		t.Fatalf("query fixture failed verification: %v", err)
	}
	return a
}

func TestOperationQueryProof(t *testing.T) {
	v, key, original, _ := queryFixture(t)
	if err := v.parseOperationQuery(queryProof(t, v, key, original, nil)); err != nil {
		t.Fatalf("valid query rejected: %v", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*Claims)
		want   error
	}{
		{"wrong path", func(c *Claims) { c.Path += "/extra" }, ErrInvalidRequest},
		{"nonpositive read version", func(c *Claims) { c.ExpectedPolicyVersion = 0; c.DesiredPolicyVersion = 0 }, ErrInvalidRequest},
		{"mutation versions", func(c *Claims) { c.ExpectedPolicyVersion = 7 }, ErrInvalidRequest},
		{"noncanonical owner", func(c *Claims) { c.OwnerID = "042"; c.ActorID = "042" }, ErrInvalidRequest},
		{"avatar actor", func(c *Claims) { c.ActorKind = "avatar"; c.ActorID = "avatar-A"; c.AvatarGeneration = 3 }, ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := v.parseOperationQuery(queryProof(t, v, key, original, tc.change)); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
	other, _, _, _ := queryFixture(t)
	for _, a := range []*Authorization{nil, {}, queryProof(t, other, key, original, nil)} {
		if err := v.parseOperationQuery(a); !errors.Is(err, ErrForbidden) {
			t.Fatalf("unverified proof accepted: %v", err)
		}
	}
}

func TestDisableOriginalResultValidation(t *testing.T) {
	v, _, original, raw := queryFixture(t)
	if err := v.validateDisableResult(raw, original); err != nil {
		t.Fatalf("valid N1 ready rejected: %v", err)
	}
	var base map[string]json.RawMessage
	if err := json.Unmarshal(raw, &base); err != nil {
		t.Fatal(err)
	}
	for field := range base {
		t.Run("missing "+field, func(t *testing.T) {
			var r map[string]json.RawMessage
			json.Unmarshal(raw, &r)
			delete(r, field)
			if err := v.validateDisableResult(fixtureJSON(t, r), original); !errors.Is(err, ErrUnknownResult) {
				t.Fatalf("incomplete original accepted: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		field string
		value any
	}{
		{"operation_id", strings.Repeat("e", 64)}, {"operation_type", "principals.ensure"}, {"request_id", "other"}, {"request_digest", strings.Repeat("e", 64)},
		{"environment", "online"}, {"tenant_id", "other"}, {"appkey", "other"}, {"owner_id", "43"}, {"target_id", "ma222222222222222222222222222222"},
		{"actor_kind", "system"}, {"actor_id", "other"}, {"avatar_generation", 3}, {"expected_policy_version", 8}, {"desired_policy_version", 9},
		{"state", "persisted"}, {"state", "observed"}, {"state", nil}, {"effective_policy_version", nil}, {"effective_policy_version", 9},
		{"effective_avatar_generation", 2}, {"effective_avatar_generation", nil}, {"commit_version", "bad"}, {"committed_at", nil}, {"committed_at", "2026-11-09T03:00:00.124000Z"},
		{"accepted_at", "2026-11-09T03:00:00Z"}, {"accepted_at", nil}, {"failure", map[string]any{"reason": "NOT_READY", "retryable": false}},
		{"desired_state", map[string]any{"state": "disabled", "avatar_id": "other", "principal_id": disablePrincipal, "target_avatar_generation": 3}},
		{"desired_state", map[string]any{"state": "disabled", "avatar_id": "avatar-A", "principal_id": disablePrincipal, "target_avatar_generation": 2}},
		{"desired_state", nil}, {"extra", true},
	} {
		t.Run(tc.field+fmt.Sprint(tc.value), func(t *testing.T) {
			var r map[string]json.RawMessage
			json.Unmarshal(raw, &r)
			r[tc.field] = fixtureJSON(t, tc.value)
			if err := v.validateDisableResult(fixtureJSON(t, r), original); !errors.Is(err, ErrUnknownResult) {
				t.Fatalf("forged original accepted: %v", err)
			}
		})
	}
	for _, bad := range [][]byte{[]byte(`{}`), append(append([]byte(nil), raw...), []byte(` {}`)...), []byte(strings.Replace(string(raw), `"state":"ready"`, `"state":"ready","state":"ready"`, 1))} {
		if err := v.validateDisableResult(bad, original); !errors.Is(err, ErrUnknownResult) {
			t.Fatalf("bad result accepted: %v", err)
		}
	}
}

func TestDisableOriginalNonEffectStates(t *testing.T) {
	v, _, original, raw := queryFixture(t)
	for _, state := range []string{"accepted", "failed", "unknown"} {
		t.Run(state, func(t *testing.T) {
			var r map[string]any
			json.Unmarshal(raw, &r)
			r["state"] = state
			for _, field := range []string{"effective_policy_version", "effective_avatar_generation", "commit_version", "committed_at"} {
				r[field] = nil
			}
			if state == "failed" {
				r["failure"] = map[string]any{"reason": "NOT_READY", "retryable": false}
			}
			if state == "unknown" {
				r["failure"] = map[string]any{"reason": "UNKNOWN_RESULT", "retryable": true}
			}
			if err := v.validateDisableResult(fixtureJSON(t, r), original); err != nil {
				t.Fatalf("valid stored %s rejected: %v", state, err)
			}
			if state != "accepted" {
				r["failure"] = map[string]any{"reason": "UNKNOWN_RESULT", "retryable": false}
				if err := v.validateDisableResult(fixtureJSON(t, r), original); !errors.Is(err, ErrUnknownResult) {
					t.Fatalf("invalid failure accepted: %v", err)
				}
			}
		})
	}
}
