package mindlink

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const PrincipalEnsurePath = "/apigateway/mindlink/principals/ensure"

type PrincipalEnsureRequest struct {
	OperationID            string `json:"operation_id"`
	RequestID              string `json:"request_id"`
	RequestDigest          string `json:"request_digest"`
	PrincipalID            string `json:"principal_id"`
	AvatarID               string `json:"avatar_id"`
	TargetAvatarGeneration int64  `json:"target_avatar_generation"`
	ExpectedPolicyVersion  int64  `json:"expected_policy_version"`
	DesiredPolicyVersion   int64  `json:"desired_policy_version"`
	DisplayName            string `json:"display_name"`
}

// P1 extends the restrictive N1 schema with the exact business-normalized name.
// A valid intent does not itself authorize creating a principal or owner grant.
func (v *Verifier) ParsePrincipalEnsure(auth *Authorization) (*PrincipalEnsureRequest, error) {
	if v == nil || auth == nil || auth.verifier != v {
		return nil, ErrForbidden
	}
	c := auth.claims
	if c.OperationType != OperationPrincipalEnsure || c.Method != http.MethodPost || c.Path != PrincipalEnsurePath || c.ActorKind != "owner" && c.ActorKind != "system" {
		return nil, ErrForbidden
	}
	owner, err := strconv.ParseUint(c.OwnerID, 10, 64)
	if err != nil || owner == 0 || strconv.FormatUint(owner, 10) != c.OwnerID || len(c.TenantID) > 128 || strings.TrimSpace(c.TenantID) == "" || !isASCII(c.TenantID) {
		return nil, ErrInvalidRequest
	}
	var body PrincipalEnsureRequest
	if err = DecodeStrictObject(auth.body, &body, strings.Fields("operation_id request_id request_digest principal_id avatar_id target_avatar_generation expected_policy_version desired_policy_version display_name")); err != nil {
		return nil, err
	}
	if !isLowerHex(body.OperationID, 64) || !isLowerHex(body.RequestDigest, 64) || len(body.RequestID) < 1 || len(body.RequestID) > 64 || !isASCII(body.RequestID) || len(body.PrincipalID) != 32 || !strings.HasPrefix(body.PrincipalID, "ma") || !isLowerHex(body.PrincipalID[2:], 30) || len(body.AvatarID) < 1 || len(body.AvatarID) > 100 || body.TargetAvatarGeneration <= 0 || body.ExpectedPolicyVersion < 0 || body.ExpectedPolicyVersion == math.MaxInt64 || body.DesiredPolicyVersion != body.ExpectedPolicyVersion+1 {
		return nil, ErrInvalidRequest
	}
	name := body.DisplayName
	if !utf8.ValidString(name) || len(name) == 0 || len(name) > 320 || utf8.RuneCountInString(name) > 80 || norm.NFC.String(strings.TrimSpace(name)) != name || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return nil, ErrInvalidRequest
	}
	if body.OperationID != c.OperationID || body.RequestID != c.RequestID || body.RequestDigest != c.RequestDigest || body.PrincipalID != c.TargetID || body.ExpectedPolicyVersion != c.ExpectedPolicyVersion || body.DesiredPolicyVersion != c.DesiredPolicyVersion {
		return nil, ErrRequestConflict
	}
	digest, err := PrincipalEnsureDigest(c, &body)
	if err != nil || digest != body.RequestDigest {
		return nil, ErrRequestConflict
	}
	return &body, nil
}

func PrincipalEnsureDigest(c Claims, body *PrincipalEnsureRequest) (string, error) {
	if body == nil {
		return "", ErrInvalidRequest
	}
	raw, err := json.Marshal([]any{"mindlink-policy-p1", OperationPrincipalEnsure, c.Environment, c.TenantID, c.AppKey, c.OwnerID, c.ActorKind, c.ActorID, c.AvatarGeneration, body.PrincipalID, body.AvatarID, body.TargetAvatarGeneration, body.ExpectedPolicyVersion, body.DesiredPolicyVersion, "active", body.DisplayName})
	if err != nil {
		return "", ErrInvalidRequest
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}
