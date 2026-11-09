package mindlink

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DisableAuthoritySourceConfig is server-owned, never selected by a request.
type DisableAuthoritySourceConfig struct {
	Enabled             bool
	Program, ConfigPath string
}

// DisableAuthoritySource checks a business retirement observation, not an IM
// grant or a cross-database revocation fence. It does not activate PolicyHTTP.
type DisableAuthoritySource struct {
	enabled             bool
	program, configPath string
}

func NewDisableAuthoritySource(cfg DisableAuthoritySourceConfig) (*DisableAuthoritySource, error) {
	if !cfg.Enabled {
		return &DisableAuthoritySource{}, nil
	}
	if !filepath.IsAbs(cfg.Program) || !filepath.IsAbs(cfg.ConfigPath) {
		return nil, ErrInvalidRequest
	}
	info, err := os.Lstat(cfg.Program)
	if err != nil || !info.Mode().IsRegular() {
		return nil, ErrNotReady
	}
	return &DisableAuthoritySource{true, cfg.Program, cfg.ConfigPath}, nil
}

// Check independently verifies N1, invokes the fixed source once, and checks its
// exact original binding. A nil result grants no permission to commit an effect.
func (s *DisableAuthoritySource) Check(ctx context.Context, v *Verifier, auth *Authorization) error {
	if s == nil || !s.enabled {
		return ErrNotReady
	}
	if ctx == nil {
		return ErrInvalidRequest
	}
	body, err := v.ParsePrincipalDisable(auth)
	if err != nil {
		return err
	}
	c := auth.claims
	// BIZ source-v1 is the existing system wire producer, not an owner wire.
	if c.ActorKind != "system" || !validAt(c, time.Now()) {
		return ErrForbidden
	}
	if !isLowerHex(c.RequestID, 64) {
		return ErrInvalidRequest
	}
	if ctx.Err() != nil {
		return ErrUnknownResult
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.program, "--config", s.configPath, "--operation-id", c.RequestID)
	configureAuthorityProcess(cmd)
	cmd.WaitDelay = time.Second
	var output authorityOutput
	cmd.Stdout, cmd.Stderr = &output, io.Discard
	err = cmd.Run()
	if ctx.Err() != nil || output.overflow {
		return ErrUnknownResult
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			switch exit.ExitCode() {
			case 2, 3:
				return ErrNotReady
			case 4:
				return ErrForbidden
			}
		}
		return ErrUnknownResult
	}
	if err := checkDisableAuthoritySnapshot(output.data, c, body); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ErrUnknownResult
	}
	if !validAt(c, time.Now()) {
		return ErrForbidden
	}
	return nil
}

type authorityOutput struct {
	data     []byte
	overflow bool
}

func (w *authorityOutput) Write(p []byte) (int, error) {
	if len(p) > 32*1024-len(w.data) {
		w.overflow = true
		return 0, io.ErrShortBuffer
	}
	w.data = append(w.data, p...)
	return len(p), nil
}

// Exact version75739e949 protocol. Its parent digest is supplied by the trusted
// source; it is distinct from the independently verified N1 wire digest.
type disableAuthoritySnapshot struct {
	Version                string `json:"version"`
	Environment            string `json:"environment"`
	TenantID               string `json:"tenant_id"`
	AppKey                 string `json:"appkey"`
	Issuer                 string `json:"issuer"`
	Audience               string `json:"audience"`
	SystemActorID          string `json:"system_actor_id"`
	OwnerID                string `json:"owner_id"`
	OperationType          string `json:"operation_type"`
	ParentOperationID      string `json:"parent_operation_id"`
	ParentDigest           string `json:"parent_digest"`
	WireOperationID        string `json:"wire_operation_id"`
	WireRequestDigest      string `json:"wire_request_digest"`
	WireBodySHA256         string `json:"wire_body_sha256"`
	PrincipalID            string `json:"principal_id"`
	AvatarID               string `json:"avatar_id"`
	TargetAvatarGeneration int64  `json:"target_avatar_generation"`
	ExpectedPolicyVersion  int64  `json:"expected_policy_version"`
	DesiredPolicyVersion   int64  `json:"desired_policy_version"`
	LifecycleState         string `json:"lifecycle_state"`
	RetiredAtUnixSeconds   int64  `json:"retired_at_unix_seconds"`
}

var authoritySourceFields = strings.Fields("version environment tenant_id appkey issuer audience system_actor_id owner_id operation_type parent_operation_id parent_digest wire_operation_id wire_request_digest wire_body_sha256 principal_id avatar_id target_avatar_generation expected_policy_version desired_policy_version lifecycle_state retired_at_unix_seconds")

func checkDisableAuthoritySnapshot(raw []byte, c Claims, body *PrincipalDisableRequest) error {
	var snapshot disableAuthoritySnapshot
	if body == nil || DecodeStrictObject(raw, &snapshot, authoritySourceFields) != nil {
		return ErrUnknownResult
	}
	if snapshot.Version != "mindlink-disable-authority-v1" || snapshot.OperationType != OperationPrincipalDisable || snapshot.LifecycleState != "retired" || snapshot.RetiredAtUnixSeconds <= 0 || !isLowerHex(snapshot.ParentDigest, 64) {
		return ErrUnknownResult
	}
	if c.ActorKind != "system" || snapshot.SystemActorID != c.ActorID {
		return ErrForbidden
	}
	if snapshot.Environment != c.Environment || snapshot.TenantID != c.TenantID || snapshot.AppKey != c.AppKey || snapshot.Issuer != c.Issuer || snapshot.Audience != c.Audience || snapshot.OwnerID != c.OwnerID {
		return ErrScopeMismatch
	}
	if snapshot.ParentOperationID != c.RequestID || snapshot.WireOperationID != c.OperationID || snapshot.WireRequestDigest != c.RequestDigest || snapshot.WireBodySHA256 != c.BodySHA256 || snapshot.PrincipalID != body.PrincipalID || snapshot.AvatarID != body.AvatarID || snapshot.TargetAvatarGeneration != body.TargetAvatarGeneration || snapshot.ExpectedPolicyVersion != body.ExpectedPolicyVersion || snapshot.DesiredPolicyVersion != body.DesiredPolicyVersion {
		return ErrRequestConflict
	}
	return nil
}
