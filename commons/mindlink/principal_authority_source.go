package mindlink

import (
	"context"
	"encoding/json"
	"time"
)

// A separate fixed invocation reads current positive BIZ facts; the retirement
// protocol cannot authorize creation even when the wire signature is valid.
func (s *DisableAuthoritySource) CheckPrincipal(ctx context.Context, v *Verifier, auth *Authorization) error {
	if s == nil || !s.enabled {
		return ErrNotReady
	}
	if ctx == nil {
		return ErrInvalidRequest
	}
	body, err := v.ParsePrincipalEnsure(auth)
	if err != nil {
		return err
	}
	c := auth.claims
	if c.ActorKind != "system" || !validAt(c, time.Now()) {
		return ErrForbidden
	}
	if !isLowerHex(c.RequestID, 64) {
		return ErrInvalidRequest
	}
	raw, err := s.invoke(ctx, c.RequestID, "--principal")
	if err != nil {
		return err
	}
	if err = checkPrincipalAuthoritySnapshot(raw, c, body); err != nil {
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

func checkPrincipalAuthoritySnapshot(raw []byte, c Claims, body *PrincipalEnsureRequest) error {
	var fields map[string]json.RawMessage
	required := append(append([]string(nil), authoritySourceFields...), "display_name")
	if body == nil || DecodeStrictObject(raw, &fields, required) != nil || len(fields) != len(required) {
		return ErrUnknownResult
	}
	var name string
	if json.Unmarshal(fields["display_name"], &name) != nil || name != body.DisplayName {
		return ErrRequestConflict
	}
	delete(fields, "display_name")
	base, err := json.Marshal(fields)
	if err != nil {
		return ErrUnknownResult
	}
	var snapshot disableAuthoritySnapshot
	if DecodeStrictObject(base, &snapshot, authoritySourceFields) != nil {
		return ErrUnknownResult
	}
	if snapshot.Version != "mindlink-principal-authority-v1" || snapshot.OperationType != OperationPrincipalEnsure || snapshot.LifecycleState != "active" || snapshot.RetiredAtUnixSeconds != 0 || !isLowerHex(snapshot.ParentDigest, 64) {
		return ErrUnknownResult
	}
	return checkAuthorityBinding(snapshot, c, &PrincipalDisableRequest{OperationID: body.OperationID, RequestID: body.RequestID, RequestDigest: body.RequestDigest, PrincipalID: body.PrincipalID, AvatarID: body.AvatarID, TargetAvatarGeneration: body.TargetAvatarGeneration, ExpectedPolicyVersion: body.ExpectedPolicyVersion, DesiredPolicyVersion: body.DesiredPolicyVersion})
}
