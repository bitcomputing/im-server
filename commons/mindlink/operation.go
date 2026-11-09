package mindlink

import "encoding/json"

const (
	OperationPrincipalEnsure  = "principal.ensure"
	OperationPrincipalDisable = "principal.disable"
	// ReceiptTimeFormat records a UTC observation inside the SQL transaction.
	// It is not the wall-clock instant at which MySQL completed its commit.
	ReceiptTimeFormat = "2006-01-02T15:04:05.000000Z"
)

// OperationResult is an immutable receipt, emitted only after its transaction
// commits. A later current-state observation must not overwrite this receipt.
// Route activation and operation-specific validation remain separate gates.
type OperationResult struct {
	Issuer                 string          `json:"iss"`
	Audience               string          `json:"aud"`
	Environment            string          `json:"environment"`
	TenantID               string          `json:"tenant_id"`
	AppKey                 string          `json:"appkey"`
	OwnerID                string          `json:"owner_id"`
	OperationType          string          `json:"operation_type"`
	OperationID            string          `json:"operation_id"`
	RequestID              string          `json:"request_id"`
	TargetID               string          `json:"target_id"`
	ActorKind              string          `json:"actor_kind"`
	ActorID                string          `json:"actor_id"`
	AvatarGeneration       int64           `json:"avatar_generation"`
	ExpectedPolicyVersion  int64           `json:"expected_policy_version"`
	DesiredPolicyVersion   int64           `json:"desired_policy_version"`
	RequestDigest          string          `json:"request_digest"`
	Method                 string          `json:"method"`
	Path                   string          `json:"path"`
	BodySHA256             string          `json:"body_sha256"`
	CommittedPolicyVersion int64           `json:"committed_policy_version"`
	DesiredState           json.RawMessage `json:"desired_state"`
	State                  string          `json:"state"`
	RecordedAt             string          `json:"recorded_at"`
}

type PrincipalEnsureRequest struct {
	OperationID               string `json:"operation_id"`
	RequestID                 string `json:"request_id"`
	RequestDigest             string `json:"request_digest"`
	Environment               string `json:"environment"`
	TenantID                  string `json:"tenant_id"`
	AppKey                    string `json:"appkey"`
	OwnerID                   string `json:"owner_id"`
	AvatarID                  string `json:"avatar_id"`
	PrincipalID               string `json:"principal_id"`
	PrincipalGeneration       int64  `json:"principal_generation"`
	ExpectedProjectionVersion int64  `json:"expected_projection_version"`
	DesiredProjectionVersion  int64  `json:"desired_projection_version"`
	DisplayName               string `json:"display_name"`
}

type PrincipalDisableRequest struct {
	OperationID               string `json:"operation_id"`
	RequestID                 string `json:"request_id"`
	RequestDigest             string `json:"request_digest"`
	Environment               string `json:"environment"`
	TenantID                  string `json:"tenant_id"`
	AppKey                    string `json:"appkey"`
	OwnerID                   string `json:"owner_id"`
	AvatarID                  string `json:"avatar_id"`
	PrincipalID               string `json:"principal_id"`
	PrincipalGeneration       int64  `json:"principal_generation"`
	ExpectedProjectionVersion int64  `json:"expected_projection_version"`
	DesiredProjectionVersion  int64  `json:"desired_projection_version"`
}
