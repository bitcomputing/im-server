package mindlink

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// MessageNativeBinding must come from the confirmed immutable projection, never
// from a client, opaque conversation ID or the received receipt itself.
type MessageNativeBinding struct {
	ChannelType       int32  `json:"channel_type"`
	ConversationID    string `json:"native_conversation_id"`
	SenderPrincipalID string `json:"sender_principal_id"`
	TargetPrincipalID string `json:"target_principal_id"`
	SubChannel        string `json:"sub_channel"`
}

// MessageSendResult is the original committed message receipt. The only success
// state in this version is persisted; it does not prove delivery or execution.
type MessageSendResult struct {
	Version                string `json:"version"`
	OperationID            string `json:"operation_id"`
	OperationType          string `json:"operation_type"`
	RequestID              string `json:"request_id"`
	RequestDigest          string `json:"request_digest"`
	BodySHA256             string `json:"body_sha256"`
	ParentDigest           string `json:"parent_digest"`
	DeliverySnapshotSHA256 string `json:"delivery_snapshot_sha256"`
	Environment            string `json:"environment"`
	TenantID               string `json:"tenant_id"`
	AppKey                 string `json:"appkey"`
	OwnerID                string `json:"owner_id"`
	ActorKind              string `json:"actor_kind"`
	ActorID                string `json:"actor_id"`
	AvatarGeneration       int64  `json:"avatar_generation"`
	ConversationID         string `json:"conversation_id"`
	PolicyVersion          int64  `json:"policy_version"`
	State                  string `json:"state"`
	MessageID              string `json:"message_id"`
	MsgSeqNo               string `json:"msg_seq_no"`
	MsgTime                string `json:"msg_time"`
	ChannelType            int32  `json:"channel_type"`
	NativeConversationID   string `json:"native_conversation_id"`
	SenderPrincipalID      string `json:"sender_principal_id"`
	TargetPrincipalID      string `json:"target_principal_id"`
	SubChannel             string `json:"sub_channel"`
	OutboxID               string `json:"outbox_id"`
	CommitToken            string `json:"commit_token"`
	AcceptedAt             string `json:"accepted_at"`
	CommittedAt            string `json:"committed_at"`
}

var messageResultFields = strings.Fields("version operation_id operation_type request_id request_digest body_sha256 parent_digest delivery_snapshot_sha256 environment tenant_id appkey owner_id actor_kind actor_id avatar_generation conversation_id policy_version state message_id msg_seq_no msg_time channel_type native_conversation_id sender_principal_id target_principal_id sub_channel outbox_id commit_token accepted_at committed_at")

// ValidateMessageSendResult checks a trusted IM response's shape and binding;
// parsing caller-posted JSON does not establish its source or a real DB commit.
func (v *Verifier) ValidateMessageSendResult(auth *Authorization, raw []byte, native MessageNativeBinding) (*MessageSendResult, error) {
	body, err := v.ParseMessageSend(auth)
	if err != nil {
		return nil, err
	}
	if !validMessageNativeBinding(native) {
		return nil, ErrNotReady
	}
	c := auth.claims
	var result MessageSendResult
	if DecodeStrictObject(raw, &result, messageResultFields) != nil {
		return nil, ErrUnknownResult
	}
	if result.Version != MessageWireVersion || result.OperationType != OperationMessageSend || result.State != "persisted" || result.OperationID != c.OperationID || result.RequestID != c.RequestID || result.RequestDigest != c.RequestDigest || result.BodySHA256 != c.BodySHA256 || result.ParentDigest != body.ParentDigest || result.DeliverySnapshotSHA256 != body.DeliverySnapshotSHA256 || result.Environment != c.Environment || result.TenantID != c.TenantID || result.AppKey != c.AppKey || result.OwnerID != c.OwnerID || result.ActorKind != c.ActorKind || result.ActorID != c.ActorID || result.AvatarGeneration != c.AvatarGeneration || result.ConversationID != body.ConversationID || result.PolicyVersion != body.PolicyVersion {
		return nil, ErrUnknownResult
	}
	if result.ChannelType != native.ChannelType || result.NativeConversationID != native.ConversationID || result.SenderPrincipalID != native.SenderPrincipalID || result.TargetPrincipalID != native.TargetPrincipalID || result.SubChannel != native.SubChannel || result.OutboxID != messageOutboxID(c) || !isLowerHex(result.CommitToken, 64) || !validNativeMessageID(result.MessageID) || !positiveDecimal(result.MsgSeqNo) || !positiveDecimal(result.MsgTime) {
		return nil, ErrUnknownResult
	}
	accepted, aerr := time.Parse(ReceiptTimeFormat, result.AcceptedAt)
	committed, cerr := time.Parse(ReceiptTimeFormat, result.CommittedAt)
	if aerr != nil || cerr != nil || accepted.Format(ReceiptTimeFormat) != result.AcceptedAt || committed.Format(ReceiptTimeFormat) != result.CommittedAt || committed.Before(accepted) {
		return nil, ErrUnknownResult
	}
	return &result, nil
}

func messageOutboxID(c Claims) string {
	raw, _ := json.Marshal([]string{"mindlink-message-outbox-v1", c.Issuer, c.OperationID})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func positiveDecimal(s string) bool {
	n, err := strconv.ParseInt(s, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == s
}

func validMessageNativeBinding(b MessageNativeBinding) bool {
	for _, id := range []string{b.SenderPrincipalID, b.TargetPrincipalID} {
		if len(id) < 1 || len(id) > 32 || !utf8.ValidString(id) || strings.Contains(id, ":") {
			return false
		}
	}
	if b.SubChannel != "" {
		return false
	}
	switch b.ChannelType {
	case 1:
		return b.SenderPrincipalID != b.TargetPrincipalID && b.ConversationID == min(b.SenderPrincipalID, b.TargetPrincipalID)+":"+max(b.SenderPrincipalID, b.TargetPrincipalID)
	case 2:
		return b.ConversationID == b.TargetPrincipalID
	}
	return false
}
