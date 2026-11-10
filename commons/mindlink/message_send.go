package mindlink

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

const (
	MessageWireVersion      = "mindlink-reliable-v1"
	OperationMessageSend    = "messages.send"
	OperationMessageQuery   = "messages.query"
	MessageSendPath         = "/apigateway/mindlink/messages/send"
	MessageResultPathPrefix = "/apigateway/mindlink/messages/results/"
)

// MessageSendRequest is a separate wire intent, never the local F3 fingerprint
// or the disable DTO. Every field is explicit, with only reply_to nullable.
type MessageSendRequest struct {
	Version                string   `json:"version"`
	OperationID            string   `json:"operation_id"`
	RequestID              string   `json:"request_id"`
	RequestDigest          string   `json:"request_digest"`
	ParentDigest           string   `json:"parent_digest"`
	DeliverySnapshotSHA256 string   `json:"delivery_snapshot_sha256"`
	ConversationID         string   `json:"conversation_id"`
	PolicyVersion          int64    `json:"policy_version"`
	MessageType            string   `json:"message_type"`
	Text                   string   `json:"text"`
	Mentions               []string `json:"mentions"`
	ReplyTo                *string  `json:"reply_to"`
	AssetIDs               []string `json:"asset_ids"`
}

var messageSendFields = strings.Fields("version operation_id request_id request_digest parent_digest delivery_snapshot_sha256 conversation_id policy_version message_type text mentions reply_to asset_ids")

func (v *Verifier) ParseMessageSend(auth *Authorization) (*MessageSendRequest, error) {
	if v == nil || auth == nil || auth.verifier != v {
		return nil, ErrForbidden
	}
	c := auth.claims
	if c.OperationType != OperationMessageSend || c.Method != http.MethodPost || c.Path != MessageSendPath {
		return nil, ErrForbidden
	}
	if err := validateMessageActor(c); err != nil {
		return nil, err
	}
	var body MessageSendRequest
	required := strings.Fields("version operation_id request_id request_digest parent_digest delivery_snapshot_sha256 conversation_id policy_version message_type text mentions asset_ids")
	if err := DecodeStrictObject(auth.body, &body, required); err != nil {
		return nil, err
	}
	// A nullable field is still mandatory; DecodeStrictObject checks all others.
	var fields map[string]json.RawMessage
	if json.Unmarshal(auth.body, &fields) != nil || len(fields) != len(messageSendFields) {
		return nil, ErrInvalidRequest
	}
	if body.Version != MessageWireVersion || body.MessageType != "jg:text" || len(body.AssetIDs) != 0 {
		return nil, ErrNotReady
	}
	if !isLowerHex(body.OperationID, 64) || !isLowerHex(body.RequestID, 64) || !isLowerHex(body.RequestDigest, 64) || !isLowerHex(body.ParentDigest, 64) || !isLowerHex(body.DeliverySnapshotSHA256, 64) || !isLowerHex(body.ConversationID, 64) || body.PolicyVersion <= 0 || len(body.Text) == 0 || len(body.Text) > 65536 || len(body.Mentions) > 1000 {
		return nil, ErrInvalidRequest
	}
	for i, avatar := range body.Mentions {
		if len(avatar) < 1 || len(avatar) > 100 || i > 0 && body.Mentions[i-1] >= avatar {
			return nil, ErrInvalidRequest
		}
	}
	if body.ReplyTo != nil && !validNativeMessageID(*body.ReplyTo) {
		return nil, ErrInvalidRequest
	}
	hash := sha256.Sum256(auth.body)
	if c.BodySHA256 != hex.EncodeToString(hash[:]) || body.OperationID != c.OperationID || body.RequestID != c.RequestID || body.RequestDigest != c.RequestDigest || body.ConversationID != c.TargetID || body.PolicyVersion != c.ExpectedPolicyVersion || body.PolicyVersion != c.DesiredPolicyVersion {
		return nil, ErrRequestConflict
	}
	digest, err := MessageSendDigest(c, &body)
	if err != nil || digest != body.RequestDigest {
		return nil, ErrRequestConflict
	}
	return &body, nil
}

// MessageSendDigest encodes the exact eighteen-slot Go-JSON semantic array. It
// is an encoding helper, not authentication, a permission decision or a receipt.
func MessageSendDigest(c Claims, body *MessageSendRequest) (string, error) {
	if body == nil {
		return "", ErrInvalidRequest
	}
	var reply any
	if body.ReplyTo != nil {
		reply = *body.ReplyTo
	}
	raw, err := json.Marshal([]any{body.Version, OperationMessageSend, c.Environment, c.TenantID, c.AppKey, c.OwnerID, c.ActorKind, c.ActorID, c.AvatarGeneration,
		body.ConversationID, body.PolicyVersion, body.ParentDigest, body.DeliverySnapshotSHA256, body.MessageType, body.Text, body.Mentions, reply, body.AssetIDs})
	if err != nil {
		return "", ErrInvalidRequest
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

func (v *Verifier) ParseMessageResultQuery(auth *Authorization) error {
	if v == nil || auth == nil || auth.verifier != v {
		return ErrForbidden
	}
	c := auth.claims
	if c.OperationType != OperationMessageQuery || c.Method != http.MethodGet {
		return ErrForbidden
	}
	if err := validateMessageActor(c); err != nil {
		return err
	}
	if !isLowerHex(c.OperationID, 64) || !isLowerHex(c.RequestID, 64) || !isLowerHex(c.RequestDigest, 64) || !isLowerHex(c.TargetID, 64) || c.Path != MessageResultPathPrefix+c.RequestID || c.ExpectedPolicyVersion <= 0 || c.ExpectedPolicyVersion != c.DesiredPolicyVersion || len(auth.body) != 0 {
		return ErrInvalidRequest
	}
	return nil
}

func validateMessageActor(c Claims) error {
	owner, err := strconv.ParseUint(c.OwnerID, 10, 64)
	if err != nil || owner == 0 || strconv.FormatUint(owner, 10) != c.OwnerID || strings.TrimSpace(c.TenantID) == "" || !isASCII(c.TenantID) || len(c.TenantID) > 128 {
		return ErrInvalidRequest
	}
	if c.ActorKind == "owner" && c.ActorID == c.OwnerID && c.AvatarGeneration == 0 || c.ActorKind == "avatar" && len(c.ActorID) > 0 && len(c.ActorID) <= 100 && c.AvatarGeneration > 0 {
		return nil
	}
	return ErrForbidden
}

func validNativeMessageID(id string) bool {
	if len(id) < 1 || len(id) > 20 {
		return false
	}
	for _, b := range []byte(id) {
		if b < 33 || b > 126 {
			return false
		}
	}
	return true
}
