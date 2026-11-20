package mindlink

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConversationCancellationBindsOriginalEnsure(t *testing.T) {
	for _, environment := range []string{"dev", "sta"} {
		t.Run(environment, func(t *testing.T) {
			v := &Verifier{}
			c := Claims{OperationType: OperationConversationCancel, OperationID: strings.Repeat("a", 64), RequestID: strings.Repeat("b", 64), TargetID: strings.Repeat("c", 64), Method: "POST", Path: ConversationCancelPath, ActorKind: "system", ActorID: "system", OwnerID: "42", Environment: environment, TenantID: "tenant", AppKey: "app", DesiredPolicyVersion: 2, ExpectedPolicyVersion: 1}
			b := ConversationEnsureRequest{OperationID: c.OperationID, RequestID: c.RequestID, ConversationID: c.TargetID, Kind: "mindlink_group", NativeTargetID: "mg" + strings.Repeat("1", 30), OwnerIMID: "owner-native", DesiredPolicyVersion: 2, ExpectedPolicyVersion: 1, Members: []ConversationMember{{"A", "ma" + strings.Repeat("1", 30), 1, 1}}}
			b.RequestDigest, _ = ConversationEnsureDigest(c, &b)
			c.RequestDigest = b.RequestDigest
			raw, _ := json.Marshal(b)
			auth := &Authorization{claims: c, body: raw, verifier: v}
			original, body, err := v.parseConversationCancel(auth)
			if err != nil || body.RequestDigest != c.RequestDigest || original.claims.OperationType != OperationConversationEnsure || original.claims.Path != ConversationEnsurePath || string(original.body) != string(raw) || auth.claims != c {
				t.Fatal("cancellation lost original binding or mutated input", err)
			}
			for _, change := range []func(*Authorization){
				func(a *Authorization) { a.claims.Method = "GET" },
				func(a *Authorization) { a.claims.Path = ConversationEnsurePath },
				func(a *Authorization) { a.claims.OperationType = OperationConversationEnsure },
				func(a *Authorization) { a.claims.ActorKind = "owner" },
				func(a *Authorization) { a.claims.OwnerID = "43" },
				func(a *Authorization) { a.claims.RequestDigest = strings.Repeat("f", 64) },
				func(a *Authorization) { a.verifier = &Verifier{} },
			} {
				bad := *auth
				change(&bad)
				if _, _, err := v.parseConversationCancel(&bad); err == nil {
					t.Fatal("changed cancel identity/route/body accepted")
				}
			}
			snapshot := map[string]string{"version": "mindlink-conversation-cancellation-authority-v1", "environment": c.Environment, "tenant_id": c.TenantID, "appkey": c.AppKey, "issuer": c.Issuer, "audience": c.Audience, "system_actor_id": c.ActorID, "owner_id": c.OwnerID, "parent_operation_id": c.RequestID, "parent_digest": strings.Repeat("d", 64), "wire_operation_id": c.OperationID, "wire_request_digest": c.RequestDigest, "wire_body_sha256": c.BodySHA256, "conversation_id": c.TargetID, "lifecycle_state": "obsolete"}
			raw, _ = json.Marshal(snapshot)
			if checkConversationAuthoritySnapshotFor(raw, original.claims, true) != nil || checkConversationAuthoritySnapshot(raw, original.claims) == nil {
				t.Fatal("negative authority confused with positive authority")
			}
			for field, value := range snapshot {
				delete(snapshot, field)
				raw, _ = json.Marshal(snapshot)
				if checkConversationAuthoritySnapshotFor(raw, original.claims, true) == nil {
					t.Fatalf("missing %s accepted", field)
				}
				snapshot[field] = value
			}
		})
	}
}
