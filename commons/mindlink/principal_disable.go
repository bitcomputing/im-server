package mindlink

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// ParsePrincipalDisable checks authenticated wire intent, not current authority.
// The final transaction must still lock the lifecycle snapshot and all gates.
func (v *Verifier) ParsePrincipalDisable(auth *Authorization) (*PrincipalDisableRequest, error) {
	if v == nil || auth == nil || auth.verifier != v {
		return nil, ErrForbidden
	}
	c := auth.claims
	if c.OperationType != OperationPrincipalDisable || c.Method != http.MethodPost || c.Path != PrincipalDisablePath || c.ActorKind != "owner" && c.ActorKind != "system" {
		return nil, ErrForbidden
	}
	owner, err := strconv.ParseUint(c.OwnerID, 10, 64)
	if err != nil || owner == 0 || strconv.FormatUint(owner, 10) != c.OwnerID || len(c.TenantID) > 128 || strings.TrimSpace(c.TenantID) == "" || !isASCII(c.TenantID) {
		return nil, ErrInvalidRequest
	}
	var body PrincipalDisableRequest
	if err := DecodeStrictObject(auth.body, &body, []string{"operation_id", "request_id", "request_digest", "principal_id", "avatar_id", "target_avatar_generation", "expected_policy_version", "desired_policy_version"}); err != nil {
		return nil, err
	}
	if !isLowerHex(body.OperationID, 64) || !isLowerHex(body.RequestDigest, 64) || len(body.RequestID) < 1 || len(body.RequestID) > 64 || !isASCII(body.RequestID) || len(body.PrincipalID) != 32 || !strings.HasPrefix(body.PrincipalID, "ma") || !isLowerHex(body.PrincipalID[2:], 30) || len(body.AvatarID) < 1 || len(body.AvatarID) > 100 || body.TargetAvatarGeneration <= 0 || body.ExpectedPolicyVersion <= 0 || body.ExpectedPolicyVersion == math.MaxInt64 || body.DesiredPolicyVersion != body.ExpectedPolicyVersion+1 {
		return nil, ErrInvalidRequest
	}
	if body.OperationID != c.OperationID || body.RequestID != c.RequestID || body.RequestDigest != c.RequestDigest || body.PrincipalID != c.TargetID || body.ExpectedPolicyVersion != c.ExpectedPolicyVersion || body.DesiredPolicyVersion != c.DesiredPolicyVersion {
		return nil, ErrRequestConflict
	}
	canonical, err := json.Marshal([]any{"mindlink-policy-n1", OperationPrincipalDisable, c.Environment, c.TenantID, c.AppKey, c.OwnerID, c.ActorKind, c.ActorID, c.AvatarGeneration, body.PrincipalID, body.AvatarID, body.TargetAvatarGeneration, body.ExpectedPolicyVersion, body.DesiredPolicyVersion, "disabled"})
	if err != nil {
		return nil, ErrInvalidRequest
	}
	digest := sha256.Sum256(canonical)
	if hex.EncodeToString(digest[:]) != body.RequestDigest {
		return nil, ErrRequestConflict
	}
	return &body, nil
}

func isLowerHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, ch := range []byte(value) {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}

func isASCII(value string) bool {
	for _, ch := range []byte(value) {
		if ch > 127 {
			return false
		}
	}
	return true
}
