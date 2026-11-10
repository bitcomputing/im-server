package mindlink

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func messageNativeBinding() MessageNativeBinding {
	return MessageNativeBinding{ChannelType: 1, ConversationID: "43:" + disablePrincipal, SenderPrincipalID: "43", TargetPrincipalID: disablePrincipal}
}

func messageReceipt(a *Authorization) MessageSendResult {
	c, b := a.claims, messageNativeBinding()
	return MessageSendResult{Version: MessageWireVersion, OperationID: c.OperationID, OperationType: OperationMessageSend, RequestID: c.RequestID, RequestDigest: c.RequestDigest,
		BodySHA256: c.BodySHA256, ParentDigest: strings.Repeat("d", 64), DeliverySnapshotSHA256: strings.Repeat("b", 64), Environment: c.Environment, TenantID: c.TenantID, AppKey: c.AppKey,
		OwnerID: c.OwnerID, ActorKind: c.ActorKind, ActorID: c.ActorID, AvatarGeneration: c.AvatarGeneration, ConversationID: c.TargetID, PolicyVersion: c.ExpectedPolicyVersion,
		State: "persisted", MessageID: "pb2aa22bb33cc44dd55e", MsgSeqNo: "2147483648", MsgTime: "1700000000123", ChannelType: b.ChannelType, NativeConversationID: b.ConversationID,
		SenderPrincipalID: b.SenderPrincipalID, TargetPrincipalID: b.TargetPrincipalID, SubChannel: "", OutboxID: messageOutboxID(c), CommitToken: strings.Repeat("f", 64),
		AcceptedAt: "2026-11-10T01:00:00.123Z", CommittedAt: "2026-11-10T01:00:00.124Z"}
}

func TestMessageSendReceiptStrictBinding(t *testing.T) {
	v, a := messageProof(t, string(fixtureJSON(t, messageRequest())), OperationMessageSend, "POST", MessageSendPath, nil)
	r := messageReceipt(a)
	base := fixtureJSON(t, r)
	got, err := v.ValidateMessageSendResult(a, base, messageNativeBinding())
	if err != nil || got == nil || got.MsgSeqNo != "2147483648" {
		t.Fatalf("valid result rejected or lossy: %v", err)
	}
	group := MessageNativeBinding{ChannelType: 2, ConversationID: "mg" + strings.Repeat("1", 30), SenderPrincipalID: "43", TargetPrincipalID: "mg" + strings.Repeat("1", 30)}
	groupResult := r
	groupResult.ChannelType, groupResult.NativeConversationID, groupResult.SenderPrincipalID, groupResult.TargetPrincipalID = group.ChannelType, group.ConversationID, group.SenderPrincipalID, group.TargetPrincipalID
	if result, err := v.ValidateMessageSendResult(a, fixtureJSON(t, groupResult), group); result == nil || err != nil {
		t.Fatalf("valid group result rejected: %v", err)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(base, &fields)
	if len(fields) != 30 {
		t.Fatalf("unexpected result field count: %d", len(fields))
	}
	for field := range fields {
		for _, mode := range []string{"missing", "null", "changed"} {
			t.Run(field+"/"+mode, func(t *testing.T) {
				var m map[string]json.RawMessage
				_ = json.Unmarshal(base, &m)
				switch mode {
				case "missing":
					delete(m, field)
				case "null":
					m[field] = json.RawMessage("null")
				default:
					m[field] = json.RawMessage(`"wrong"`)
					if field == "message_id" {
						// This value is assigned by IM, not known before the first send.
						m[field] = json.RawMessage(`"message-id-exceeds-native-20"`)
					}
				}
				if result, err := v.ValidateMessageSendResult(a, fixtureJSON(t, m), messageNativeBinding()); result != nil || err == nil {
					t.Fatal("unbound receipt accepted")
				}
			})
		}
	}
	for _, bad := range []string{"0", "01", "+1", " 1", "1e3", "9223372036854775808", ""} {
		r.MsgSeqNo = bad
		if got, err := v.ValidateMessageSendResult(a, fixtureJSON(t, r), messageNativeBinding()); got != nil || err == nil {
			t.Fatalf("invalid sequence accepted: %q", bad)
		}
	}
	r.MsgSeqNo = "9223372036854775807"
	if got, err := v.ValidateMessageSendResult(a, fixtureJSON(t, r), messageNativeBinding()); got == nil || err != nil {
		t.Fatalf("MaxInt64 sequence rejected: %v", err)
	}
	for _, state := range []string{"accepted", "ready", "unknown", "failed", "observed"} {
		r = messageReceipt(a)
		r.State = state
		if got, err := v.ValidateMessageSendResult(a, fixtureJSON(t, r), messageNativeBinding()); got != nil || err == nil {
			t.Fatalf("non-persisted success accepted: %s", state)
		}
	}
	for n, raw := range []string{string(base) + `{}`, strings.TrimSuffix(string(base), "}") + `,"state":"persisted"}`,
		strings.Replace(string(base), `"msg_seq_no":"2147483648"`, `"msg_seq_no":2147483648`, 1),
		strings.Replace(string(base), `"policy_version":7`, `"policy_version":7.0`, 1),
		strings.Replace(string(base), `"committed_at":"2026-11-10T01:00:00.124Z"`, `"committed_at":"2026-11-10T01:00:00.124+00:00"`, 1)} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			if got, err := v.ValidateMessageSendResult(a, []byte(raw), messageNativeBinding()); got != nil || err == nil {
				t.Fatal("non-exact receipt accepted")
			}
		})
	}
	for _, b := range []MessageNativeBinding{{}, {ChannelType: 1, ConversationID: "wrong", SenderPrincipalID: "43", TargetPrincipalID: disablePrincipal},
		{ChannelType: 2, ConversationID: disablePrincipal, SenderPrincipalID: "43", TargetPrincipalID: disablePrincipal},
		{ChannelType: 1, ConversationID: "43:" + disablePrincipal, SenderPrincipalID: "43", TargetPrincipalID: disablePrincipal, SubChannel: "other"}} {
		if got, err := v.ValidateMessageSendResult(a, base, b); got != nil || err == nil {
			t.Fatal("unconfirmed/changed native binding accepted")
		}
	}
}
