package mindlink

import "encoding/json"

const (
	OperationPrincipalEnsure  = "principals.ensure"
	OperationPrincipalDisable = "principals.disable"
	PrincipalDisablePath      = "/apigateway/mindlink/principals/disable"
	// ReceiptTimeFormat records a UTC observation inside the SQL transaction.
	// It is not the wall-clock instant at which MySQL completed its commit.
	ReceiptTimeFormat = "2006-01-02T15:04:05.000Z"
)

// OperationResult is an immutable receipt, emitted only after its transaction
// commits. A later current-state observation must not overwrite this receipt.
// This is the N1 output shape, not an arbitrary-result parser. Producers must
// validate the operation-specific state; route activation remains a separate gate.
type OperationResult struct {
	OperationID               string            `json:"operation_id"`
	OperationType             string            `json:"operation_type"`
	RequestID                 string            `json:"request_id"`
	RequestDigest             string            `json:"request_digest"`
	Environment               string            `json:"environment"`
	TenantID                  string            `json:"tenant_id"`
	AppKey                    string            `json:"appkey"`
	OwnerID                   string            `json:"owner_id"`
	TargetID                  string            `json:"target_id"`
	ActorKind                 string            `json:"actor_kind"`
	ActorID                   string            `json:"actor_id"`
	AvatarGeneration          int64             `json:"avatar_generation"`
	ExpectedPolicyVersion     int64             `json:"expected_policy_version"`
	DesiredPolicyVersion      int64             `json:"desired_policy_version"`
	DesiredState              json.RawMessage   `json:"desired_state"`
	State                     string            `json:"state"`
	EffectivePolicyVersion    *int64            `json:"effective_policy_version"`
	EffectiveAvatarGeneration *int64            `json:"effective_avatar_generation"`
	CommitVersion             *string           `json:"commit_version"`
	AcceptedAt                string            `json:"accepted_at"`
	CommittedAt               *string           `json:"committed_at"`
	Failure                   *OperationFailure `json:"failure"`
}

type OperationFailure struct {
	Reason    string `json:"reason"`
	Retryable bool   `json:"retryable"`
}

type PrincipalDisableRequest struct {
	OperationID            string `json:"operation_id"`
	RequestID              string `json:"request_id"`
	RequestDigest          string `json:"request_digest"`
	AvatarID               string `json:"avatar_id"`
	PrincipalID            string `json:"principal_id"`
	TargetAvatarGeneration int64  `json:"target_avatar_generation"`
	ExpectedPolicyVersion  int64  `json:"expected_policy_version"`
	DesiredPolicyVersion   int64  `json:"desired_policy_version"`
}

type PrincipalDisabledState struct {
	State                  string `json:"state"`
	AvatarID               string `json:"avatar_id"`
	PrincipalID            string `json:"principal_id"`
	TargetAvatarGeneration int64  `json:"target_avatar_generation"`
}

type PrincipalActiveState struct {
	State                  string `json:"state"`
	AvatarID               string `json:"avatar_id"`
	PrincipalID            string `json:"principal_id"`
	TargetAvatarGeneration int64  `json:"target_avatar_generation"`
	DisplayName            string `json:"display_name"`
}
