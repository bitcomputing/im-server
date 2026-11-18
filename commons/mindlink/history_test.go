package mindlink

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestHistoryQueryBindsOwnerPage(t *testing.T) {
	trust, key := fixtureTrust()
	trust.QueryOperations = []string{OperationHistoryRead}
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	fields := fixtureClaims("")
	fields["operation_type"], fields["method"] = OperationHistoryRead, http.MethodGet
	fields["target_id"] = strings.Repeat("c", 64)
	fields["expected_policy_version"], fields["desired_policy_version"] = int64(1), int64(1)
	path := HistoryPathPrefix + strings.Repeat("c", 64) + "/0/20"
	fields["path"] = path
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(path)))
	fields["request_digest"] = digest
	verify := func() *Authorization {
		token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, fields))
		a, e := v.Verify(fixtureRequest(t, token, "GET", path, ""), OperationHistoryRead, fixtureNow)
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	a := verify()
	before, limit, err := v.parseHistory(a)
	if err != nil || before != 0 || limit != 20 {
		t.Fatalf("page %d %d: %v", before, limit, err)
	}
	for _, suffix := range []string{"/00/20", "/-1/20", "/0/101", "/0/0", "/0/20/extra"} {
		path = HistoryPathPrefix + strings.Repeat("c", 64) + suffix
		fields["path"] = path
		fields["request_digest"] = fmt.Sprintf("%x", sha256.Sum256([]byte(path)))
		if _, _, e := v.parseHistory(verify()); e == nil {
			t.Fatalf("accepted %s", suffix)
		}
	}
	path = HistoryPathPrefix + strings.Repeat("c", 64) + "/0/20"
	fields["path"] = path
	fields["request_digest"] = digest
	fields["actor_kind"], fields["actor_id"], fields["avatar_generation"] = "avatar", "A", int64(1)
	if _, _, e := v.parseHistory(verify()); e == nil {
		t.Fatal("inform recipient gained history")
	}
}
