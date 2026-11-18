package mindlink

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

const disablePath = "/apigateway/mindlink/principals/disable"
const disablePrincipal = "ma111111111111111111111111111111"
const disableDigest = "d4c8be1d15ec03cebfa23441d69bf64496c0f1c2ba5bc9da2dfdae0dee048079"

func disableBody() map[string]any {
	return map[string]any{
		"operation_id": strings.Repeat("a", 64), "request_id": "fixture-disable-1", "request_digest": disableDigest,
		"principal_id": disablePrincipal, "avatar_id": "avatar-A", "target_avatar_generation": int64(3),
		"expected_policy_version": int64(7), "desired_policy_version": int64(8),
	}
}

func disableAuthorization(t *testing.T, body string, change func(map[string]any)) (*Verifier, *Authorization) {
	t.Helper()
	trust, key := fixtureTrust()
	trust.MutationOperations = []string{"principals.disable", "fixture.mutate"}
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	c := fixtureClaims(body)
	c["operation_type"], c["path"], c["target_id"] = "principals.disable", disablePath, disablePrincipal
	c["request_id"], c["request_digest"] = "fixture-disable-1", disableDigest
	if change != nil {
		change(c)
	}
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
	a, err := v.Verify(fixtureRequest(t, token, c["method"].(string), c["path"].(string), body), c["operation_type"].(string), fixtureNow)
	if err != nil {
		t.Fatalf("signed fixture rejected before operation parser: %v", err)
	}
	return v, a
}

func TestPrincipalDisablePublishedDigestVectors(t *testing.T) {
	for _, tc := range []struct {
		name, avatar, digest          string
		generation, expected, desired int64
	}{
		{"ascii", "avatar-A", disableDigest, 3, 7, 8},
		{"max_int64", "avatar-A", "ec7a7ffaded218e8d05d2ad5265d64ba2950f0a7d97445e285f594af52ac8324", math.MaxInt64, math.MaxInt64 - 1, math.MaxInt64},
		{"unicode_html", "分身<&>\u2028", "2c6e9f88b080ad6cfe2484721e33d3ee2cce08ca2e9db1541c7df8d983a3fe71", 3, 7, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := disableBody()
			b["avatar_id"], b["request_digest"], b["target_avatar_generation"] = tc.avatar, tc.digest, tc.generation
			b["expected_policy_version"], b["desired_policy_version"] = tc.expected, tc.desired
			v, a := disableAuthorization(t, string(fixtureJSON(t, b)), func(c map[string]any) {
				c["request_digest"], c["expected_policy_version"], c["desired_policy_version"] = tc.digest, tc.expected, tc.desired
			})
			parsed, err := v.ParsePrincipalDisable(a)
			if err != nil || parsed == nil {
				t.Fatalf("published valid vector rejected: %v", err)
			}
			decode := func(raw []byte) map[string]any {
				var value map[string]any
				d := json.NewDecoder(strings.NewReader(string(raw)))
				d.UseNumber()
				if err := d.Decode(&value); err != nil {
					t.Fatal(err)
				}
				return value
			}
			if !reflect.DeepEqual(decode(fixtureJSON(t, parsed)), decode(fixtureJSON(t, b))) {
				t.Fatalf("wire value lost: %s", fixtureJSON(t, parsed))
			}
		})
	}
}

func TestPrincipalDisableRejectsSignedBodyAttacks(t *testing.T) {
	base := string(fixtureJSON(t, disableBody()))
	cases := []struct {
		name, raw string
		want      error
	}{
		{"null body", "null", ErrInvalidRequest}, {"array", "[]", ErrInvalidRequest},
		{"duplicate", strings.Replace(base, `"avatar_id":"avatar-A"`, `"avatar_id":"avatar-A","avatar_id":"other"`, 1), ErrInvalidRequest},
		{"escaped duplicate", strings.Replace(base, `"avatar_id":"avatar-A"`, `"avatar_id":"avatar-A","avatar_\u0069d":"other"`, 1), ErrInvalidRequest},
		{"case alias", strings.Replace(base, `"avatar_id"`, `"Avatar_id"`, 1), ErrInvalidRequest},
		{"second JSON", base + ` {}`, ErrInvalidRequest},
		{"surrogate", strings.Replace(base, `"avatar-A"`, `"\ud800"`, 1), ErrInvalidRequest},
		{"invalid UTF8", strings.Replace(base, "avatar-A", "avatar-\xff", 1), ErrInvalidRequest},
	}
	for _, field := range []string{"operation_id", "request_id", "request_digest", "principal_id", "avatar_id", "target_avatar_generation", "expected_policy_version", "desired_policy_version"} {
		b := disableBody()
		delete(b, field)
		cases = append(cases, struct {
			name, raw string
			want      error
		}{"missing " + field, string(fixtureJSON(t, b)), ErrInvalidRequest})
		b = disableBody()
		b[field] = nil
		cases = append(cases, struct {
			name, raw string
			want      error
		}{"null " + field, string(fixtureJSON(t, b)), ErrInvalidRequest})
	}
	for _, tc := range []struct {
		field string
		value any
		want  error
	}{
		{"owner_id", "42", ErrInvalidRequest}, {"display_name", "name", ErrInvalidRequest}, {"avatar_generation", int64(0), ErrInvalidRequest},
		{"target_avatar_generation", int64(0), ErrInvalidRequest}, {"target_avatar_generation", int64(-1), ErrInvalidRequest},
		{"target_avatar_generation", true, ErrInvalidRequest}, {"target_avatar_generation", "3", ErrInvalidRequest},
		{"target_avatar_generation", json.Number("3.0"), ErrInvalidRequest}, {"target_avatar_generation", json.Number("3e0"), ErrInvalidRequest},
		{"target_avatar_generation", json.Number("9223372036854775808"), ErrInvalidRequest},
		{"expected_policy_version", int64(0), ErrInvalidRequest}, {"expected_policy_version", int64(math.MaxInt64), ErrInvalidRequest},
		{"desired_policy_version", int64(9), ErrInvalidRequest}, {"avatar_id", "", ErrInvalidRequest},
		{"avatar_id", strings.Repeat("界", 34), ErrInvalidRequest}, {"principal_id", "ma" + strings.Repeat("A", 30), ErrInvalidRequest},
		{"operation_id", "short", ErrInvalidRequest}, {"request_digest", strings.Repeat("C", 64), ErrInvalidRequest},
		{"request_id", strings.Repeat("r", 65), ErrInvalidRequest}, {"request_id", "请求", ErrInvalidRequest},
		{"avatar_id", "other-avatar", ErrRequestConflict},
	} {
		b := disableBody()
		b[tc.field] = tc.value
		cases = append(cases, struct {
			name, raw string
			want      error
		}{fmt.Sprintf("%s=%v", tc.field, tc.value), string(fixtureJSON(t, b)), tc.want})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, a := disableAuthorization(t, tc.raw, nil)
			got, err := v.ParsePrincipalDisable(a)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("attack accepted/misclassified: result=%v err=%v want=%v", got, err, tc.want)
			}
		})
	}
}

func TestPrincipalDisableBindsSignedClaimsAndActor(t *testing.T) {
	body := string(fixtureJSON(t, disableBody()))
	for _, tc := range []struct {
		field string
		value any
		want  error
	}{
		{"operation_type", "fixture.mutate", ErrForbidden}, {"path", fixturePath, ErrForbidden}, {"method", "PUT", ErrForbidden},
		{"actor_kind", "avatar", ErrForbidden}, {"operation_id", strings.Repeat("b", 64), ErrRequestConflict},
		{"request_id", "another-request", ErrRequestConflict}, {"request_digest", strings.Repeat("c", 64), ErrRequestConflict},
		{"target_id", "ma" + strings.Repeat("2", 30), ErrRequestConflict},
		{"expected_policy_version", int64(6), ErrRequestConflict}, {"desired_policy_version", int64(9), ErrRequestConflict},
		{"owner_id", "042", ErrInvalidRequest}, {"owner_id", "0", ErrInvalidRequest}, {"owner_id", "18446744073709551616", ErrInvalidRequest},
	} {
		t.Run(tc.field+fmt.Sprint(tc.value), func(t *testing.T) {
			v, a := disableAuthorization(t, body, func(c map[string]any) {
				c[tc.field] = tc.value
				if tc.field == "owner_id" {
					c["actor_id"] = tc.value
				}
			})
			got, err := v.ParsePrincipalDisable(a)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("binding attack result=%v err=%v want=%v", got, err, tc.want)
			}
		})
	}
	v, a := disableAuthorization(t, body, nil)
	other, _ := disableAuthorization(t, body, nil)
	for _, proof := range []*Authorization{nil, {}, a} {
		verifier := v
		if proof == a {
			verifier = other
		}
		if got, err := verifier.ParsePrincipalDisable(proof); got != nil || !errors.Is(err, ErrForbidden) {
			t.Fatalf("untrusted proof accepted: %v", err)
		}
	}
}

func TestN1OperationResultWireShape(t *testing.T) {
	// The external BIZ N1 example is the oracle; the old proposal must not leak
	// issuer/audience/method/path/body hash or invent zeroes for nullable fields.
	var result OperationResult
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(fixtureJSON(t, result), &fields); err != nil {
		t.Fatal(err)
	}
	names := strings.Fields("operation_id operation_type request_id request_digest environment tenant_id appkey owner_id target_id actor_kind actor_id avatar_generation expected_policy_version desired_policy_version desired_state state effective_policy_version effective_avatar_generation commit_version accepted_at committed_at failure")
	if len(fields) != len(names) {
		t.Fatalf("result has %d fields, want external N1 22: %s", len(fields), fixtureJSON(t, result))
	}
	for _, name := range names {
		if _, ok := fields[name]; !ok {
			t.Errorf("missing N1 field %s", name)
		}
	}
	for _, name := range []string{"effective_policy_version", "effective_avatar_generation", "commit_version", "committed_at", "failure"} {
		if string(fields[name]) != "null" {
			t.Errorf("unproven %s is not null: %s", name, fields[name])
		}
	}
	if ReceiptTimeFormat != "2006-01-02T15:04:05.000Z" {
		t.Fatalf("receipt format still uses a different contract: %q", ReceiptTimeFormat)
	}
	if OperationPrincipalDisable != "principals.disable" || OperationPrincipalEnsure != "principals.ensure" {
		t.Fatal("old operation spelling remains")
	}
}

func TestPrincipalDisablePreservesAuthenticatedBoundaryValues(t *testing.T) {
	for _, tc := range []struct{ name, kind, actor, owner, avatar, request string }{
		{"system", "system", "fixture-system", "42", "avatar-A", "fixture-disable-1"},
		{"max_owner_and_lengths", "owner", "18446744073709551615", "18446744073709551615", strings.Repeat("a", 100), strings.Repeat("r", 64)},
		{"exact_bytes", "owner", "42", "42", " A\u0301 ", " original request "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := disableBody()
			b["avatar_id"], b["request_id"] = tc.avatar, tc.request
			values := []any{"mindlink-policy-n1", "principals.disable", "dev", "fixture-tenant", "fixture-app", tc.owner, tc.kind, tc.actor, int64(0), disablePrincipal, tc.avatar, int64(3), int64(7), int64(8), "disabled"}
			digest := fmt.Sprintf("%x", sha256.Sum256(fixtureJSON(t, values)))
			b["request_digest"] = digest
			v, a := disableAuthorization(t, string(fixtureJSON(t, b)), func(c map[string]any) {
				c["actor_kind"], c["actor_id"], c["owner_id"], c["request_id"], c["request_digest"] = tc.kind, tc.actor, tc.owner, tc.request, digest
			})
			got, err := v.ParsePrincipalDisable(a)
			if err != nil || got == nil {
				t.Fatalf("valid boundary rejected: %v", err)
			}
			if got.AvatarID != tc.avatar || got.RequestID != tc.request || got.TargetAvatarGeneration != 3 || a.Claims().AvatarGeneration != 0 {
				t.Fatal("original bytes or actor/target generation lost")
			}
		})
	}
}
