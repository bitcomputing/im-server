package mindlink

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

const reliableASCIIDigest = "06a2f8529fab54e80451005f7ba11c7ff7bce6de9c8abf92aaf37fc350f4abed"

func messageRequest() MessageSendRequest {
	return MessageSendRequest{Version: "mindlink-reliable-v1", OperationID: strings.Repeat("a", 64), RequestID: strings.Repeat("e", 64), RequestDigest: reliableASCIIDigest,
		ParentDigest: strings.Repeat("d", 64), DeliverySnapshotSHA256: strings.Repeat("b", 64), ConversationID: strings.Repeat("c", 64), PolicyVersion: 7,
		MessageType: "jg:text", Text: "hello", Mentions: []string{}, AssetIDs: []string{}}
}

func messageProof(t *testing.T, raw, op, method, path string, change func(map[string]any)) (*Verifier, *Authorization) {
	t.Helper()
	trust, key := fixtureTrust()
	trust.MutationOperations, trust.QueryOperations = []string{"messages.send"}, []string{"messages.query"}
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	c := fixtureClaims(raw)
	c["operation_type"], c["method"], c["path"] = op, method, path
	c["request_id"], c["request_digest"], c["target_id"] = strings.Repeat("e", 64), reliableASCIIDigest, strings.Repeat("c", 64)
	c["expected_policy_version"], c["desired_policy_version"] = int64(7), int64(7)
	if change != nil {
		change(c)
	}
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
	a, err := v.Verify(fixtureRequest(t, token, method, path, raw), op, fixtureNow)
	if err != nil {
		t.Fatal(err)
	}
	return v, a
}

func TestMessageSendReferenceVectors(t *testing.T) {
	for _, tc := range []struct {
		name, text, actor, actorID, digest string
		generation, policy                 int64
		mentions                           []string
		reply                              *string
	}{
		{"ascii", "hello", "owner", "42", reliableASCIIDigest, 0, 7, []string{}, nil},
		{"unicode", "分身<&>\u2028😀", "avatar", "avatar-A", "c31b5de34a3bc904e5be1c954b6bd6836328be458ca1cac515e93b6cafbf6322", 3, 7, []string{"avatar-A", "avatar-B"}, stringPointer("pb2aa22bb33cc44dd55e")},
		{"max_policy", " ", "owner", "42", "8d951dea59551c55120b0e5fbeef319eee1e1cc8c87fb67e4539da98cd0c6645", 0, math.MaxInt64, []string{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := messageRequest()
			r.Text, r.Mentions, r.ReplyTo, r.PolicyVersion, r.RequestDigest = tc.text, tc.mentions, tc.reply, tc.policy, tc.digest
			v, a := messageProof(t, string(fixtureJSON(t, r)), "messages.send", "POST", MessageSendPath, func(c map[string]any) {
				c["actor_kind"], c["actor_id"], c["avatar_generation"] = tc.actor, tc.actorID, tc.generation
				c["expected_policy_version"], c["desired_policy_version"], c["request_digest"] = tc.policy, tc.policy, tc.digest
			})
			got, err := v.ParseMessageSend(a)
			if err != nil || got == nil {
				t.Fatalf("reference rejected: %v", err)
			}
			digest, err := MessageSendDigest(a.claims, got)
			if err != nil || digest != tc.digest {
				t.Fatalf("digest=%s err=%v", digest, err)
			}
		})
	}
}

func stringPointer(s string) *string { return &s }

func TestMessageSendStrictBody(t *testing.T) {
	base := string(fixtureJSON(t, messageRequest()))
	var object map[string]json.RawMessage
	_ = json.Unmarshal([]byte(base), &object)
	for field := range object {
		for _, mode := range []string{"missing", "null"} {
			if field == "reply_to" && mode == "null" {
				continue
			}
			t.Run(field+"/"+mode, func(t *testing.T) {
				var m map[string]json.RawMessage
				_ = json.Unmarshal([]byte(base), &m)
				if mode == "missing" {
					delete(m, field)
				} else {
					m[field] = json.RawMessage("null")
				}
				v, a := messageProof(t, string(fixtureJSON(t, m)), "messages.send", "POST", MessageSendPath, nil)
				if r, err := v.ParseMessageSend(a); r != nil || err == nil {
					t.Fatal("non-exact body accepted")
				}
			})
		}
	}
	cases := []string{base + `{}`, strings.TrimSuffix(base, "}") + `,"text":"hello"}`,
		strings.Replace(base, `"text"`, `"Text"`, 1), strings.Replace(base, `"hello"`, `"\ud800"`, 1),
		strings.Replace(base, `"policy_version":7`, `"policy_version":7e0`, 1),
		strings.Replace(base, `"policy_version":7`, `"policy_version":"7"`, 1),
		strings.Replace(base, `"mentions":[]`, `"mentions":["b","a"]`, 1),
		strings.Replace(base, `"mentions":[]`, `"mentions":["a","a"]`, 1),
		strings.Replace(base, `"hello"`, `"changed"`, 1),
		strings.Replace(base, `"asset_ids":[]`, `"asset_ids":["asset-1"]`, 1),
		strings.Replace(base, `"jg:text"`, `"jg:image"`, 1),
		strings.TrimSuffix(base, "}") + `,"read_enabled":true}`,
	}
	for n, raw := range cases {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			v, a := messageProof(t, raw, "messages.send", "POST", MessageSendPath, nil)
			if r, err := v.ParseMessageSend(a); r != nil || err == nil {
				t.Fatal("invalid/unsupported/conflicting body accepted")
			}
		})
	}
	// A valid signature and matching self-asserted digest do not replace semantics.
	r := messageRequest()
	r.RequestDigest = strings.Repeat("f", 64)
	v, a := messageProof(t, string(fixtureJSON(t, r)), "messages.send", "POST", MessageSendPath, func(c map[string]any) { c["request_digest"] = r.RequestDigest })
	if got, err := v.ParseMessageSend(a); got != nil || err == nil {
		t.Fatal("opaque digest accepted")
	}
	other, _ := messageProof(t, base, "messages.send", "POST", MessageSendPath, nil)
	if got, err := other.ParseMessageSend(a); got != nil || err == nil {
		t.Fatal("foreign Authorization accepted")
	}
	ownedV, ownedA := messageProof(t, base, "messages.send", "POST", MessageSendPath, nil)
	ownedA.body = append(ownedA.body, ' ')
	if got, err := ownedV.ParseMessageSend(ownedA); got != nil || err == nil {
		t.Fatal("stored wire body changed without updating its signed hash")
	}
}

func TestMessageSendQueryBoundary(t *testing.T) {
	path := MessageResultPathPrefix + strings.Repeat("e", 64)
	v, a := messageProof(t, "", "messages.query", "GET", path, nil)
	if err := v.ParseMessageResultQuery(a); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(map[string]any){
		func(c map[string]any) { c["request_id"] = strings.Repeat("f", 64) },
		func(c map[string]any) { c["desired_policy_version"] = int64(8) },
		func(c map[string]any) { c["actor_kind"], c["actor_id"] = "system", "fixture-system" },
	} {
		v, a := messageProof(t, "", "messages.query", "GET", path, change)
		if err := v.ParseMessageResultQuery(a); err == nil {
			t.Fatal("invalid query accepted")
		}
	}
}
