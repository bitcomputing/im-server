package mindlink

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestReadOwnerQueryAndAdvanceAreDistinct(t *testing.T) {
	trust, key := fixtureTrust()
	trust.QueryOperations = []string{OperationSeenQuery}
	trust.MutationOperations = []string{OperationSeenAdvance}
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	fields := fixtureClaims("")
	fields["target_id"] = strings.Repeat("c", 64)
	path := "/apigateway/mindlink/conversations/" + strings.Repeat("c", 64) + "/seen"
	fields["operation_type"], fields["method"], fields["path"] = OperationSeenQuery, "GET", path
	fields["request_id"] = fields["operation_id"]
	fields["expected_policy_version"], fields["desired_policy_version"] = int64(1), int64(1)
	check := func(body string) (*ReadRequest, error) {
		fields["body_sha256"] = fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
		token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, fields))
		a, e := v.Verify(fixtureRequest(t, token, fields["method"].(string), path, body), fields["operation_type"].(string), fixtureNow)
		if e != nil {
			return nil, e
		}
		return v.ParseRead(a)
	}
	if r, e := check(""); e != nil || r.SeenSeq != nil {
		t.Fatalf("query: %+v %v", r, e)
	}
	fields["operation_type"], fields["method"] = OperationSeenAdvance, "PUT"
	body := map[string]any{"operation_id": fields["operation_id"], "request_id": fields["request_id"], "request_digest": fields["request_digest"], "seen_seq": "3"}
	raw, _ := json.Marshal(body)
	if r, e := check(string(raw)); e != nil || r.SeenSeq == nil || *r.SeenSeq != 3 {
		t.Fatalf("advance: %+v %v", r, e)
	}
	for _, value := range []any{"03", "-1", "9223372036854775808", 3, nil} {
		body["seen_seq"] = value
		raw, _ = json.Marshal(body)
		if _, e := check(string(raw)); e == nil {
			t.Fatalf("accepted %v", value)
		}
	}
	fields["actor_kind"], fields["actor_id"], fields["avatar_generation"] = "avatar", "A", int64(1)
	body["seen_seq"] = "3"
	raw, _ = json.Marshal(body)
	if _, e := check(string(raw)); e == nil {
		t.Fatal("avatar can advance owner position")
	}
}

func TestReadDigestFrozenMVectors(t *testing.T) {
	c := Claims{Environment: "dev", TenantID: "tenant-example", AppKey: "read-app", OwnerID: "42", TargetID: "conversation-example", ActorKind: "owner", ActorID: "42", ExpectedPolicyVersion: 7, DesiredPolicyVersion: 7, Path: "/apigateway/mindlink/conversations/conversation-example/seen"}
	b := ReadBinding{RequesterID: "test_42", TargetID: "mg000000000000000000000000000001", Kind: "pair", ChannelType: 2, Members: []string{"ma000000000000000000000000000001", "ma000000000000000000000000000002", "test_42"}}
	c.OperationType, c.Method = OperationSeenQuery, "GET"
	if got := ReadDigest(c, b, nil); got != "d77bb09590ad3224c7ea97c703566e27965f4fd753c7bc689d31033a61d1c5c9" {
		t.Fatal(got)
	}
	c.OperationType, c.Method = OperationSeenAdvance, "PUT"
	for n, want := range map[int64]string{0: "c42cfc005e857a1fe12548e0f6df43ba52953a97951ccedca11bcb2239d7b3d8", 3: "3524b5a3d9f4e3dfcff1eef5a3991739616773771b0b1f4dbd475fff227138dc", 9223372036854775807: "e373d47a707c3727c196d25da00ff2706a208b346b26132194b1cd5d1d3237e1"} {
		if got := ReadDigest(c, b, &n); got != want {
			t.Fatal(got)
		}
	}
}
