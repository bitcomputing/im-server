package mindlink

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

var fixtureNow = time.Unix(1700000000, 0)

const fixturePath = "/apigateway/mindlink/fixture"

func TestVerifierEnvironmentNamespaceIsolation(t *testing.T) {
	environments := []string{"dev", "sta", "pre", "online"}
	for _, environment := range environments {
		t.Run(environment, func(t *testing.T) {
			trust, key := fixtureTrust()
			trust.Environment = environment
			v, err := NewVerifier([]Trust{trust})
			if err != nil {
				t.Fatal(err)
			}
			for _, claimed := range environments {
				payload := fixtureClaims(`{}`)
				payload["environment"] = claimed
				token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, payload))
				auth, err := v.Verify(fixtureRequest(t, token, "POST", fixturePath, `{}`), "fixture.mutate", fixtureNow)
				if claimed == environment {
					if err != nil || auth == nil || auth.Claims().Environment != environment {
						t.Fatalf("matching environment %q rejected: %v", claimed, err)
					}
				} else if auth != nil || !errors.Is(err, ErrScopeMismatch) {
					t.Fatalf("%q trust accepted %q environment: %v", environment, claimed, err)
				}
			}
		})
	}
	for _, environment := range []string{"", "STA", "staging", "prod", "sta "} {
		trust, _ := fixtureTrust()
		trust.Environment = environment
		if _, err := NewVerifier([]Trust{trust}); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("unsupported environment %q accepted: %v", environment, err)
		}
	}
}

func TestVerifySTAKeepsScopeActorAndOperationProtection(t *testing.T) {
	trust, key := fixtureTrust()
	trust.Environment, trust.TenantID, trust.AppKey = "sta", "fixture-sta-tenant", "fixture-sta-app"
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, field, value string
		want               error
	}{
		{"tenant", "tenant_id", "fixture-tenant", ErrScopeMismatch},
		{"app", "appkey", "fixture-app", ErrScopeMismatch},
		{"audience", "aud", "other-im", ErrScopeMismatch},
		{"issuer", "iss", "other-biz", ErrForbidden},
		{"owner", "owner_id", "43", ErrForbidden},
		{"system identity", "actor_id", "other-system", ErrForbidden},
		{"operation", "operation_type", "fixture.unlisted", ErrForbidden},
		{"body", "body_sha256", strings.Repeat("f", 64), ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := fixtureClaims(`{}`)
			payload["environment"], payload["tenant_id"], payload["appkey"] = trust.Environment, trust.TenantID, trust.AppKey
			if tc.name == "system identity" {
				payload["actor_kind"] = "system"
			}
			payload[tc.field] = tc.value
			token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, payload))
			operation := "fixture.mutate"
			if tc.field == "operation_type" {
				operation = tc.value
			}
			auth, err := v.Verify(fixtureRequest(t, token, "POST", fixturePath, `{}`), operation, fixtureNow)
			if auth != nil || !errors.Is(err, tc.want) {
				t.Fatalf("STA binding changed: identity=%t err=%v, want %v", auth != nil, err, tc.want)
			}
		})
	}
}

func fixtureTrust() (Trust, ed25519.PrivateKey) {
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	return Trust{
		Issuer: "fixture-biz", KeyID: "fixture-key", PublicKey: key.Public().(ed25519.PublicKey),
		Audience: "fixture-im", Environment: "dev", TenantID: "fixture-tenant", AppKey: "fixture-app",
		SystemActorID: "fixture-system", QueryOperations: []string{"fixture.query"}, MutationOperations: []string{"fixture.mutate"},
	}, key
}

func fixtureClaims(body string) map[string]any {
	return map[string]any{
		"iss": "fixture-biz", "aud": "fixture-im", "environment": "dev", "tenant_id": "fixture-tenant", "appkey": "fixture-app", "owner_id": "42",
		"operation_type": "fixture.mutate", "operation_id": strings.Repeat("a", 64), "request_id": "original-request", "target_id": "original-target",
		"actor_kind": "owner", "actor_id": "42", "avatar_generation": int64(0), "expected_policy_version": int64(7), "desired_policy_version": int64(8),
		"request_digest": strings.Repeat("c", 64), "method": "POST", "path": fixturePath, "body_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(body))),
		"iat": fixtureNow.Unix(), "exp": fixtureNow.Unix() + 60, "jti": strings.Repeat("b", 64),
	}
}

func fixtureToken(t *testing.T, key ed25519.PrivateKey, header, payload []byte) string {
	t.Helper()
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	return input + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, []byte(input)))
}

func fixtureJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fixtureRequest(t *testing.T, token, method, path, body string) *http.Request {
	t.Helper()
	r, err := http.NewRequest(method, "http://fixture.invalid"+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestVerifyBindsOriginalActorAndRequest(t *testing.T) {
	for _, actor := range []struct {
		kind, id   string
		generation int64
	}{{"owner", "42", 0}, {"avatar", "stable-avatar", 3}, {"system", "fixture-system", 0}} {
		t.Run(actor.kind, func(t *testing.T) {
			trust, key := fixtureTrust()
			v, err := NewVerifier([]Trust{trust})
			if err != nil {
				t.Fatal(err)
			}
			payload := fixtureClaims(`{"value":"original"}`)
			payload["actor_kind"], payload["actor_id"], payload["avatar_generation"] = actor.kind, actor.id, actor.generation
			token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, payload))
			auth, err := v.Verify(fixtureRequest(t, token, "POST", fixturePath, `{"value":"original"}`), "fixture.mutate", fixtureNow)
			if err != nil {
				t.Fatalf("valid original actor was rejected: %v", err)
			}
			if auth == nil {
				t.Fatal("valid request has no authenticated identity")
			}
			claims := auth.Claims()
			if claims.ActorKind != actor.kind || claims.ActorID != actor.id || claims.AvatarGeneration != actor.generation || claims.RequestID != "original-request" || claims.TargetID != "original-target" || claims.OwnerID != "42" {
				t.Fatal("original identity/request binding was lost")
			}
			body := auth.Body()
			body[0] = '!'
			if string(auth.Body()) != `{"value":"original"}` {
				t.Fatal("caller mutated authenticated body")
			}
		})
	}
}

func TestVerifyRejectsSignedClaimAttacks(t *testing.T) {
	cases := []struct {
		name, field string
		value       any
		omit        bool
		want        error
	}{
		{"missing actor", "actor_id", nil, true, ErrInvalidRequest},
		{"null generation", "avatar_generation", nil, false, ErrInvalidRequest},
		{"fractional generation", "avatar_generation", 0.5, false, ErrInvalidRequest},
		{"fractional time", "iat", 1700000000.5, false, ErrInvalidRequest},
		{"negative version", "expected_policy_version", -1, false, ErrInvalidRequest},
		{"unknown field", "extra", "value", false, ErrInvalidRequest},
		{"wrong issuer", "iss", "other-biz", false, ErrForbidden},
		{"wrong audience", "aud", "other-im", false, ErrScopeMismatch},
		{"wrong environment", "environment", "online", false, ErrScopeMismatch},
		{"wrong tenant", "tenant_id", "other-tenant", false, ErrScopeMismatch},
		{"wrong app", "appkey", "other-app", false, ErrScopeMismatch},
		{"owner impersonation", "actor_id", "43", false, ErrForbidden},
		{"owner posing as avatar", "avatar_generation", 3, false, ErrForbidden},
		{"unknown actor kind", "actor_kind", "admin", false, ErrForbidden},
		{"query used for write", "operation_type", "fixture.query", false, ErrForbidden},
		{"different operation", "operation_type", "unlisted.mutation", false, ErrForbidden},
		{"changed method", "method", "PUT", false, ErrForbidden},
		{"changed path", "path", "/other", false, ErrForbidden},
		{"changed body hash", "body_sha256", strings.Repeat("d", 64), false, ErrForbidden},
		{"uppercase digest", "request_digest", strings.Repeat("A", 64), false, ErrInvalidRequest},
		{"short jti", "jti", "short", false, ErrInvalidRequest},
		{"oversized request key", "request_id", strings.Repeat("r", 65), false, ErrInvalidRequest},
		{"non ASCII request key", "request_id", "请求", false, ErrInvalidRequest},
		{"expired", "exp", fixtureNow.Unix() - 6, false, ErrForbidden},
		{"future issued", "iat", fixtureNow.Unix() + 6, false, ErrForbidden},
		{"long lifetime", "exp", fixtureNow.Unix() + 61, false, ErrForbidden},
		{"zero lifetime", "exp", fixtureNow.Unix(), false, ErrForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trust, key := fixtureTrust()
			v, err := NewVerifier([]Trust{trust})
			if err != nil {
				t.Fatal(err)
			}
			payload := fixtureClaims(`{}`)
			if tc.omit {
				delete(payload, tc.field)
			} else {
				payload[tc.field] = tc.value
			}
			token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, payload))
			auth, err := v.Verify(fixtureRequest(t, token, "POST", fixturePath, `{}`), "fixture.mutate", fixtureNow)
			if auth != nil || !errors.Is(err, tc.want) {
				t.Fatalf("signed attack should be rejected with %v; got identity=%t err=%v", tc.want, auth != nil, err)
			}
		})
	}
}

func TestVerifyRejectsUntrustedWireAndDuplicateJSON(t *testing.T) {
	trust, key := fixtureTrust()
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	header := []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`)
	payload := fixtureJSON(t, fixtureClaims(`{}`))
	cases := []struct {
		name            string
		header, payload []byte
		key             ed25519.PrivateKey
		want            error
	}{
		{"wrong signature", header, payload, ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8}, 32)), ErrForbidden},
		{"wrong algorithm", []byte(`{"alg":"none","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), payload, key, ErrForbidden},
		{"wrong type", []byte(`{"alg":"EdDSA","typ":"JWT","kid":"fixture-key"}`), payload, key, ErrForbidden},
		{"unknown kid", []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"other-key"}`), payload, key, ErrForbidden},
		{"duplicate header", []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"bad","k\u0069d":"fixture-key"}`), payload, key, ErrInvalidRequest},
		{"duplicate payload", header, append([]byte(`{"owner_id":"evil",`), payload[1:]...), key, ErrInvalidRequest},
		{"null actor", header, bytes.Replace(payload, []byte(`"actor_id":"42"`), []byte(`"actor_id":null`), 1), key, ErrInvalidRequest},
		{"surrogate actor", header, bytes.Replace(payload, []byte(`"actor_id":"42"`), []byte(`"actor_id":"\ud800"`), 1), key, ErrInvalidRequest},
		{"invalid UTF8", header, bytes.Replace(payload, []byte(`"actor_id":"42"`), []byte{'"', 'a', 'c', 't', 'o', 'r', '_', 'i', 'd', '"', ':', '"', 0xff, '"'}, 1), key, ErrInvalidRequest},
		{"trailing value", header, append(append([]byte{}, payload...), []byte(` {}`)...), key, ErrInvalidRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token := fixtureToken(t, tc.key, tc.header, tc.payload)
			auth, err := v.Verify(fixtureRequest(t, token, "POST", fixturePath, `{}`), "fixture.mutate", fixtureNow)
			if auth != nil || !errors.Is(err, tc.want) {
				t.Fatalf("got identity=%t error=%v, want %v", auth != nil, err, tc.want)
			}
		})
	}
}

func TestVerifyBindsActualHTTPAndOperationClass(t *testing.T) {
	trust, key := fixtureTrust()
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	header := []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`)
	for _, tc := range []struct {
		name, method, path, body, contentType string
		want                                  error
	}{
		{"body changed", "POST", fixturePath, `{"changed":true}`, "application/json", ErrForbidden},
		{"method changed", "PUT", fixturePath, `{}`, "application/json", ErrForbidden},
		{"path changed", "POST", "/other", `{}`, "application/json", ErrForbidden},
		{"escaped path", "POST", "/apigateway/mindlink/%66ixture", `{}`, "application/json", ErrForbidden},
		{"query appended", "POST", fixturePath + "?owner_id=43", `{}`, "application/json", ErrInvalidRequest},
		{"wrong media", "POST", fixturePath, `{}`, "text/plain", ErrInvalidRequest},
		{"body too big", "POST", fixturePath, strings.Repeat("x", 1024*1024+1), "application/json", ErrInvalidRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := fixtureToken(t, key, header, fixtureJSON(t, fixtureClaims(`{}`)))
			r := fixtureRequest(t, token, tc.method, tc.path, tc.body)
			r.Header.Set("Content-Type", tc.contentType)
			auth, err := v.Verify(r, "fixture.mutate", fixtureNow)
			if auth != nil || !errors.Is(err, tc.want) {
				t.Fatalf("got identity=%t error=%v, want %v", auth != nil, err, tc.want)
			}
		})
	}
	query := fixtureClaims("")
	query["operation_type"], query["method"] = "fixture.query", "GET"
	token := fixtureToken(t, key, header, fixtureJSON(t, query))
	if _, err := v.Verify(fixtureRequest(t, token, "GET", fixturePath, ""), "fixture.query", fixtureNow); err != nil {
		t.Fatalf("valid independent query rejected: %v", err)
	}
	if auth, err := v.Verify(fixtureRequest(t, token, "GET", fixturePath, `{}`), "fixture.query", fixtureNow); auth != nil || !errors.Is(err, ErrInvalidRequest) {
		t.Fatal("GET body accepted")
	}
}

func TestVerifierCopiesTrustAndFailsClosed(t *testing.T) {
	trust, key := fixtureTrust()
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	trust.PublicKey[0] ^= 1
	trust.MutationOperations[0] = "other"
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, fixtureClaims(`{}`)))
	if _, err := v.Verify(fixtureRequest(t, token, "POST", fixturePath, `{}`), "fixture.mutate", fixtureNow); err != nil {
		t.Fatalf("caller changed configured trust: %v", err)
	}
	empty, err := NewVerifier(nil)
	if err != nil {
		t.Fatal(err)
	}
	if auth, err := empty.Verify(fixtureRequest(t, token, "POST", fixturePath, `{}`), "fixture.mutate", fixtureNow); auth != nil || !errors.Is(err, ErrNotReady) {
		t.Fatal("empty trust did not fail closed")
	}
	trust, _ = fixtureTrust()
	if _, err := NewVerifier([]Trust{trust, trust}); err == nil {
		t.Fatal("ambiguous trust accepted")
	}
	trust.PublicKey = nil
	if _, err := NewVerifier([]Trust{trust}); err == nil {
		t.Fatal("missing public key accepted")
	}
}

func TestVerifyRejectsCaseAliasesEvenWithValidSignature(t *testing.T) {
	trust, key := fixtureTrust()
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	payload := fixtureClaims(`{}`)
	payload["ACTOR_ID"] = "43"
	// A distinct, signed field must not silently override an exact schema field.
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, payload))
	if auth, err := v.Verify(fixtureRequest(t, token, "POST", fixturePath, `{}`), "fixture.mutate", fixtureNow); auth != nil || !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unknown case alias accepted or misclassified: identity=%t err=%v", auth != nil, err)
	}
}

func TestVerifyRequiresEveryClaimExplicitly(t *testing.T) {
	trust, key := fixtureTrust()
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	header := []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`)
	fields := []string{"iss", "aud", "environment", "tenant_id", "appkey", "owner_id", "operation_type", "operation_id", "request_id", "target_id", "actor_kind", "actor_id", "avatar_generation", "expected_policy_version", "desired_policy_version", "request_digest", "method", "path", "body_sha256", "iat", "exp", "jti"}
	for _, field := range fields {
		for _, missing := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/missing=%t", field, missing), func(t *testing.T) {
				payload := fixtureClaims(`{}`)
				if missing {
					delete(payload, field)
				} else {
					payload[field] = nil
				}
				token := fixtureToken(t, key, header, fixtureJSON(t, payload))
				if auth, err := v.Verify(fixtureRequest(t, token, "POST", fixturePath, `{}`), "fixture.mutate", fixtureNow); auth != nil || !errors.Is(err, ErrInvalidRequest) {
					t.Fatalf("missing/null %s accepted: %v", field, err)
				}
			})
		}
	}
}

func TestVerifyClockSkewBoundaries(t *testing.T) {
	trust, key := fixtureTrust()
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		iat, exp int64
		allowed  bool
	}{
		{"future five", 5, 65, true}, {"future six", 6, 66, false}, {"expired five", -65, -5, true}, {"expired six", -66, -6, false},
		{"max lifetime", 0, 60, true}, {"long lifetime", 0, 61, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := fixtureClaims(`{}`)
			p["iat"], p["exp"] = fixtureNow.Unix()+tc.iat, fixtureNow.Unix()+tc.exp
			token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, p))
			auth, err := v.Verify(fixtureRequest(t, token, "POST", fixturePath, `{}`), "fixture.mutate", fixtureNow)
			if tc.allowed && (err != nil || auth == nil) {
				t.Fatalf("valid skew rejected: %v", err)
			}
			if !tc.allowed && (auth != nil || !errors.Is(err, ErrForbidden)) {
				t.Fatalf("expired/future accepted: %v", err)
			}
		})
	}
}

func TestVerifyExpirySkewNeverExceedsFiveSeconds(t *testing.T) {
	trust, key := fixtureTrust()
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	p := fixtureClaims(`{}`)
	p["iat"], p["exp"] = fixtureNow.Unix()-60, fixtureNow.Unix()
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, p))
	for _, delta := range []time.Duration{5 * time.Second, 5*time.Second + time.Nanosecond, 6*time.Second - time.Nanosecond} {
		auth, err := v.Verify(fixtureRequest(t, token, "POST", fixturePath, `{}`), "fixture.mutate", fixtureNow.Add(delta))
		if delta == 5*time.Second {
			if err != nil || auth == nil {
				t.Fatalf("exact allowed skew rejected: %v", err)
			}
		} else if auth != nil || !errors.Is(err, ErrForbidden) {
			t.Fatalf("expiry skew %s accepted; maximum is five seconds", delta)
		}
	}
}

func TestVerifyChecksAllowlistAfterOperationMatches(t *testing.T) {
	trust, key := fixtureTrust()
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, op, method, body string }{
		{"query cannot mutate", "fixture.query", "POST", `{}`},
		{"mutation cannot query", "fixture.mutate", "GET", ""},
		{"unlisted operation", "fixture.unlisted", "POST", `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := fixtureClaims(tc.body)
			p["operation_type"], p["method"] = tc.op, tc.method
			token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, p))
			if auth, err := v.Verify(fixtureRequest(t, token, tc.method, fixturePath, tc.body), tc.op, fixtureNow); auth != nil || !errors.Is(err, ErrForbidden) {
				t.Fatalf("wrong operation class was authorized: identity=%t err=%v", auth != nil, err)
			}
		})
	}
}

func TestVerifyRejectsNoncompactAndNoncanonicalTokens(t *testing.T) {
	trust, key := fixtureTrust()
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, fixtureClaims(`{}`)))
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := strings.IndexByte(alphabet, token[len(token)-1])
	noncanonical := token[:len(token)-1] + string(alphabet[last+1])
	for _, value := range []string{token + "=", token[:8] + "\n" + token[8:], noncanonical, token + ".extra"} {
		if auth, err := v.Verify(fixtureRequest(t, value, "POST", fixturePath, `{}`), "fixture.mutate", fixtureNow); auth != nil || !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("noncompact/noncanonical token accepted: %v", err)
		}
	}
	r := fixtureRequest(t, token, "POST", fixturePath, `{}`)
	r.Header.Add("Authorization", "Bearer "+token)
	if auth, err := v.Verify(r, "fixture.mutate", fixtureNow); auth != nil || !errors.Is(err, ErrForbidden) {
		t.Fatal("multiple authorization headers accepted")
	}
}

func TestStrictJSONRejectsNestedDuplicatesWithoutLosingUnicode(t *testing.T) {
	var out struct {
		Value json.RawMessage `json:"value"`
	}
	for _, raw := range []string{`{"value":{"x":1,"\u0078":2}}`, `{"value":"\udc00"}`, `{"value":"\ud800x"}`, `{"value":` + strings.Repeat("[", 34) + `0` + strings.Repeat("]", 34) + `}`} {
		if err := DecodeStrictObject([]byte(raw), &out, []string{"value"}); !errors.Is(err, ErrInvalidRequest) {
			t.Fatal("ambiguous nested JSON accepted")
		}
	}
	for _, raw := range []string{`{"value":"\ud83d\ude00"}`, `{"value":"\\ud800"}`, `{"value":"é"}`, `{"value":"e\u0301"}`} {
		if err := DecodeStrictObject([]byte(raw), &out, []string{"value"}); err != nil {
			t.Fatalf("valid Unicode rejected: %v", err)
		}
	}
}
