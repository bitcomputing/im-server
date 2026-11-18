package mindlink

import (
	"context"
	"strings"
	"time"
)

func (s *DisableAuthoritySource) CheckConversation(ctx context.Context, v *Verifier, auth *Authorization) error {
	if s == nil || !s.enabled {
		return ErrNotReady
	}
	if ctx == nil {
		return ErrInvalidRequest
	}
	if _, err := v.ParseConversationEnsure(auth); err != nil {
		return err
	}
	c := auth.claims
	if !validAt(c, time.Now()) {
		return ErrForbidden
	}
	raw, err := s.invoke(ctx, c.RequestID, "--conversation")
	if err != nil {
		return err
	}
	if err = checkConversationAuthoritySnapshot(raw, c); err != nil {
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

// The digest binds the complete typed roster; the body hash binds its frozen bytes.
func checkConversationAuthoritySnapshot(raw []byte, c Claims) error {
	return checkConversationAuthoritySnapshotFor(raw, c, false)
}

func checkConversationAuthoritySnapshotFor(raw []byte, c Claims, cancel bool) error {
	if c.ActorKind != "system" || c.OperationType != OperationConversationEnsure {
		return ErrForbidden
	}
	var snapshot map[string]string
	fields := strings.Fields("version environment tenant_id appkey issuer audience system_actor_id owner_id parent_operation_id parent_digest wire_operation_id wire_request_digest wire_body_sha256 conversation_id lifecycle_state")
	if DecodeStrictObject(raw, &snapshot, fields) != nil || len(snapshot) != len(fields) || !isLowerHex(snapshot["parent_digest"], 64) {
		return ErrUnknownResult
	}
	version, state := "mindlink-conversation-authority-v1", "active"
	if cancel {
		version, state = "mindlink-conversation-cancellation-authority-v1", "obsolete"
	}
	if snapshot["version"] != version || snapshot["lifecycle_state"] != state {
		return ErrUnknownResult
	}
	for key, value := range map[string]string{"environment": c.Environment, "tenant_id": c.TenantID, "appkey": c.AppKey, "issuer": c.Issuer, "audience": c.Audience, "owner_id": c.OwnerID} {
		if snapshot[key] != value {
			return ErrScopeMismatch
		}
	}
	if snapshot["system_actor_id"] != c.ActorID {
		return ErrForbidden
	}
	for key, value := range map[string]string{"parent_operation_id": c.RequestID, "wire_operation_id": c.OperationID, "wire_request_digest": c.RequestDigest, "wire_body_sha256": c.BodySHA256, "conversation_id": c.TargetID} {
		if snapshot[key] != value {
			return ErrRequestConflict
		}
	}
	return nil
}
