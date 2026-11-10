package mindlink

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConversationEnsureBindsExactRosterAndNativeKind(t *testing.T) {
	v := &Verifier{}
	c := Claims{OperationType: "conversation.ensure", OperationID: strings.Repeat("a", 64), RequestID: strings.Repeat("b", 64), TargetID: strings.Repeat("c", 64), Method: "POST", Path: "/apigateway/mindlink/conversations/ensure", ActorKind: "system", ActorID: "system", OwnerID: "42", Environment: "dev", TenantID: "tenant", AppKey: "app", DesiredPolicyVersion: 1}
	body := ConversationEnsureRequest{OperationID: c.OperationID, RequestID: c.RequestID, ConversationID: c.TargetID, Kind: "avatar_pair_group", NativeTargetID: "mg" + strings.Repeat("1", 30), OwnerIMID: "owner-native", DesiredPolicyVersion: 1, Members: []ConversationMember{{"A", "ma" + strings.Repeat("1", 30), 1, 1}, {"B", "ma" + strings.Repeat("2", 30), 1, 1}}}
	for _, tc := range []struct {
		name   string
		change func(*ConversationEnsureRequest)
		valid  bool
	}{
		{"pair", func(*ConversationEnsureRequest) {}, true},
		{"unsorted", func(b *ConversationEnsureRequest) { b.Members[0], b.Members[1] = b.Members[1], b.Members[0] }, false},
		{"same-principal", func(b *ConversationEnsureRequest) { b.Members[1].PrincipalID = b.Members[0].PrincipalID }, false},
		{"missing-peer", func(b *ConversationEnsureRequest) { b.Members = b.Members[:1] }, false},
		{"owner-as-avatar", func(b *ConversationEnsureRequest) { b.OwnerIMID = b.Members[0].PrincipalID }, false},
		{"owner-dm", func(b *ConversationEnsureRequest) {
			b.Kind = "owner_avatar_dm"
			b.Members = b.Members[:1]
			b.NativeTargetID = b.Members[0].PrincipalID
		}, true},
		{"empty-group", func(b *ConversationEnsureRequest) { b.Kind = "mindlink_group"; b.Members = []ConversationMember{} }, true},
		{"null-roster", func(b *ConversationEnsureRequest) { b.Kind = "mindlink_group"; b.Members = nil }, false},
		{"zero-generation", func(b *ConversationEnsureRequest) { b.Members[0].Generation = 0 }, false},
		{"zero-policy", func(b *ConversationEnsureRequest) { b.Members[0].PolicyVersion = 0 }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := body
			b.Members = append([]ConversationMember(nil), body.Members...)
			tc.change(&b)
			claims := c
			claims.RequestDigest, _ = ConversationEnsureDigest(claims, &b)
			b.RequestDigest = claims.RequestDigest
			raw, _ := json.Marshal(b)
			_, err := v.ParseConversationEnsure(&Authorization{claims: claims, body: raw, verifier: v})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t err=%v", tc.valid, err)
			}
		})
	}
}
