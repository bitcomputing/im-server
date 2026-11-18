package mindlink

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
)

const ConversationEnsurePath = "/apigateway/mindlink/conversations/ensure"
const OperationConversationEnsure = "conversation.ensure"

type ConversationMember struct {
	AvatarID      string `json:"avatar_id"`
	PrincipalID   string `json:"principal_id"`
	Generation    int64  `json:"avatar_generation"`
	PolicyVersion int64  `json:"principal_policy_version"`
}

type ConversationEnsureRequest struct {
	OperationID           string               `json:"operation_id"`
	RequestID             string               `json:"request_id"`
	RequestDigest         string               `json:"request_digest"`
	ConversationID        string               `json:"conversation_id"`
	Kind                  string               `json:"kind"`
	NativeTargetID        string               `json:"native_target_id"`
	OwnerIMID             string               `json:"owner_im_id"`
	ExpectedPolicyVersion int64                `json:"expected_policy_version"`
	DesiredPolicyVersion  int64                `json:"desired_policy_version"`
	Members               []ConversationMember `json:"members"`
}

type ConversationActiveState struct {
	State          string               `json:"state"`
	ConversationID string               `json:"conversation_id"`
	Kind           string               `json:"kind"`
	NativeTargetID string               `json:"native_target_id"`
	OwnerIMID      string               `json:"owner_im_id"`
	Members        []ConversationMember `json:"members"`
}

// C1 is a complete current membership snapshot, not an additive client request.
func (v *Verifier) ParseConversationEnsure(auth *Authorization) (*ConversationEnsureRequest, error) {
	if v == nil || auth == nil || auth.verifier != v {
		return nil, ErrForbidden
	}
	c := auth.claims
	if c.OperationType != OperationConversationEnsure || c.Method != "POST" || c.Path != ConversationEnsurePath || c.ActorKind != "system" {
		return nil, ErrForbidden
	}
	owner, err := strconv.ParseUint(c.OwnerID, 10, 64)
	if err != nil || owner == 0 || strconv.FormatUint(owner, 10) != c.OwnerID || len(c.TenantID) > 128 || strings.TrimSpace(c.TenantID) == "" || !isASCII(c.TenantID) {
		return nil, ErrInvalidRequest
	}
	var b ConversationEnsureRequest
	if err := DecodeStrictObject(auth.body, &b, strings.Fields("operation_id request_id request_digest conversation_id kind native_target_id owner_im_id expected_policy_version desired_policy_version members")); err != nil {
		return nil, err
	}
	if !isLowerHex(b.OperationID, 64) || !isLowerHex(b.RequestID, 64) || !isLowerHex(b.RequestDigest, 64) || !isLowerHex(b.ConversationID, 64) || len(b.OwnerIMID) < 1 || len(b.OwnerIMID) > 32 || strings.IndexFunc(b.OwnerIMID, unicode.IsControl) >= 0 || b.ExpectedPolicyVersion < 0 || b.DesiredPolicyVersion <= b.ExpectedPolicyVersion || len(b.Members) > 1000 || b.Members == nil {
		return nil, ErrInvalidRequest
	}
	switch b.Kind {
	case "owner_avatar_dm":
		if len(b.Members) != 1 || b.NativeTargetID != b.Members[0].PrincipalID {
			return nil, ErrInvalidRequest
		}
	case "avatar_pair_group":
		if len(b.Members) != 2 {
			return nil, ErrInvalidRequest
		}
		fallthrough
	case "mindlink_group":
		if len(b.NativeTargetID) != 32 || !strings.HasPrefix(b.NativeTargetID, "mg") || !isLowerHex(b.NativeTargetID[2:], 30) {
			return nil, ErrInvalidRequest
		}
	default:
		return nil, ErrInvalidRequest
	}
	seen := map[string]bool{}
	for i, m := range b.Members {
		if len(m.AvatarID) < 1 || len(m.AvatarID) > 100 || i > 0 && b.Members[i-1].AvatarID >= m.AvatarID || len(m.PrincipalID) != 32 || !strings.HasPrefix(m.PrincipalID, "ma") || !isLowerHex(m.PrincipalID[2:], 30) || m.Generation <= 0 || m.PolicyVersion <= 0 || seen[m.PrincipalID] || m.PrincipalID == b.OwnerIMID {
			return nil, ErrInvalidRequest
		}
		seen[m.PrincipalID] = true
	}
	if b.OperationID != c.OperationID || b.RequestID != c.RequestID || b.RequestDigest != c.RequestDigest || b.ConversationID != c.TargetID || b.ExpectedPolicyVersion != c.ExpectedPolicyVersion || b.DesiredPolicyVersion != c.DesiredPolicyVersion {
		return nil, ErrRequestConflict
	}
	hash, err := ConversationEnsureDigest(c, &b)
	if err != nil || hash != b.RequestDigest {
		return nil, ErrRequestConflict
	}
	return &b, nil
}

func ConversationEnsureDigest(c Claims, b *ConversationEnsureRequest) (string, error) {
	if b == nil {
		return "", ErrInvalidRequest
	}
	members := make([]any, 0, len(b.Members))
	for _, m := range b.Members {
		members = append(members, []any{m.AvatarID, m.PrincipalID, m.Generation, m.PolicyVersion})
	}
	raw, err := json.Marshal([]any{"mindlink-conversation-c1", OperationConversationEnsure, c.Environment, c.TenantID, c.AppKey, c.OwnerID, c.ActorKind, c.ActorID, c.AvatarGeneration, b.ConversationID, b.Kind, b.NativeTargetID, b.OwnerIMID, b.ExpectedPolicyVersion, b.DesiredPolicyVersion, members})
	if err != nil {
		return "", ErrInvalidRequest
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
