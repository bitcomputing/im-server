package mindlink

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPrincipalAuthoritySnapshotRequiresEveryCurrentBinding(t *testing.T) {
	c := Claims{Issuer: "biz", Audience: "im", Environment: "dev", TenantID: "tenant", AppKey: "app", OwnerID: "42", ActorKind: "system", ActorID: "system", OperationType: OperationPrincipalEnsure, OperationID: strings.Repeat("a", 64), RequestID: strings.Repeat("b", 64), RequestDigest: strings.Repeat("c", 64), BodySHA256: strings.Repeat("d", 64), TargetID: "ma" + strings.Repeat("1", 30), DesiredPolicyVersion: 1}
	b := &PrincipalEnsureRequest{OperationID: c.OperationID, RequestID: c.RequestID, RequestDigest: c.RequestDigest, PrincipalID: c.TargetID, AvatarID: "A", TargetAvatarGeneration: 1, DesiredPolicyVersion: 1, DisplayName: "name"}
	snapshot := authoritySnapshot(&Authorization{claims: c})
	snapshot["version"], snapshot["operation_type"], snapshot["lifecycle_state"], snapshot["retired_at_unix_seconds"] = "mindlink-principal-authority-v1", OperationPrincipalEnsure, "active", int64(0)
	snapshot["avatar_id"], snapshot["target_avatar_generation"], snapshot["expected_policy_version"], snapshot["desired_policy_version"], snapshot["display_name"] = "A", int64(1), int64(0), int64(1), "name"
	raw, _ := json.Marshal(snapshot)
	if err := checkPrincipalAuthoritySnapshot(raw, c, b); err != nil {
		t.Fatal(err)
	}
	for field, value := range snapshot {
		delete(snapshot, field)
		bad, _ := json.Marshal(snapshot)
		if checkPrincipalAuthoritySnapshot(bad, c, b) == nil {
			t.Fatalf("missing %s accepted", field)
		}
		snapshot[field] = "different"
		bad, _ = json.Marshal(snapshot)
		if checkPrincipalAuthoritySnapshot(bad, c, b) == nil {
			t.Fatalf("changed %s accepted", field)
		}
		snapshot[field] = value
	}
	duplicate := append(append([]byte(nil), raw[:len(raw)-1]...), []byte(`,"display_name":"name"}`)...)
	if checkPrincipalAuthoritySnapshot(duplicate, c, b) == nil {
		t.Fatal("duplicate name accepted")
	}
	snapshot["extra"] = true
	bad, _ := json.Marshal(snapshot)
	if checkPrincipalAuthoritySnapshot(bad, c, b) == nil {
		t.Fatal("extra field accepted")
	}
}
