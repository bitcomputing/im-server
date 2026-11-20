package mindlink

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConversationAuthorityBindsFrozenWire(t *testing.T) {
	for _, environment := range []string{"dev", "sta"} {
		t.Run(environment, func(t *testing.T) {
			c := Claims{Issuer: "biz", Audience: "im", Environment: environment, TenantID: "tenant", AppKey: "app", OwnerID: "42", ActorKind: "system", ActorID: "system", OperationType: OperationConversationEnsure, OperationID: strings.Repeat("a", 64), RequestID: strings.Repeat("b", 64), RequestDigest: strings.Repeat("c", 64), BodySHA256: strings.Repeat("d", 64), TargetID: strings.Repeat("e", 64)}
			snapshot := map[string]string{
				"version": "mindlink-conversation-authority-v1", "environment": c.Environment, "tenant_id": c.TenantID, "appkey": c.AppKey,
				"issuer": c.Issuer, "audience": c.Audience, "system_actor_id": c.ActorID, "owner_id": c.OwnerID,
				"parent_operation_id": c.RequestID, "parent_digest": strings.Repeat("f", 64), "wire_operation_id": c.OperationID,
				"wire_request_digest": c.RequestDigest, "wire_body_sha256": c.BodySHA256, "conversation_id": c.TargetID, "lifecycle_state": "active",
			}
			raw, _ := json.Marshal(snapshot)
			if err := checkConversationAuthoritySnapshot(raw, c); err != nil {
				t.Fatal(err)
			}
			for field, value := range snapshot {
				delete(snapshot, field)
				bad, _ := json.Marshal(snapshot)
				if checkConversationAuthoritySnapshot(bad, c) == nil {
					t.Fatalf("missing %s accepted", field)
				}
				snapshot[field] = "different"
				bad, _ = json.Marshal(snapshot)
				if checkConversationAuthoritySnapshot(bad, c) == nil {
					t.Fatalf("changed %s accepted", field)
				}
				snapshot[field] = value
			}
			snapshot["extra"] = "ignored"
			raw, _ = json.Marshal(snapshot)
			if checkConversationAuthoritySnapshot(raw, c) == nil {
				t.Fatal("unknown field accepted")
			}
		})
	}
}
