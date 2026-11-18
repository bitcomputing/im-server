package mindlink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"time"
)

// Query returns the immutable send result under current read authority. It
// never recreates history, reserves a sequence or invokes the producer.
func (s *MessageStore) Query(ctx context.Context, v *Verifier, auth *Authorization) (*MutationResult, error) {
	if s == nil || s.db == nil {
		return nil, ErrNotReady
	}
	if err := v.ParseMessageResultQuery(auth); err != nil {
		return nil, err
	}
	if ctx == nil || len(auth.claims.Issuer) > 1024 {
		return nil, ErrInvalidRequest
	}
	c := auth.claims
	if !validAt(c, time.Now()) {
		return nil, ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, ErrUnknownResult
	}
	defer tx.Rollback()
	var bindingHash, metadata []byte
	if tx.QueryRowContext(ctx, "SELECT binding_hash,binding_json FROM mindlink_operations WHERE issuer=? AND operation_id=? FOR UPDATE", c.Issuer, c.OperationID).Scan(&bindingHash, &metadata) != nil {
		return nil, ErrUnknownResult
	}
	hash := sha256.Sum256(metadata)
	var original Claims
	if !bytes.Equal(hash[:], bindingHash) || DecodeStrictObject(metadata, &original, claimFields) != nil || original.IssuedAt != 0 || original.ExpiresAt != 0 || original.JTI != "" || original.Issuer != c.Issuer || original.OperationID != c.OperationID || original.OperationType != OperationMessageSend || original.Method != "POST" || original.Path != MessageSendPath {
		return nil, ErrUnknownResult
	}
	if original.Audience != c.Audience || original.Environment != c.Environment || original.TenantID != c.TenantID || original.AppKey != c.AppKey || original.OwnerID != c.OwnerID {
		return nil, ErrScopeMismatch
	}
	if original.RequestID != c.RequestID || original.RequestDigest != c.RequestDigest || original.TargetID != c.TargetID {
		return nil, ErrRequestConflict
	}
	p, err := lockMessageProjection(ctx, tx, c, false)
	if err != nil {
		return nil, err
	}
	if err := p.authorizeReader(ctx, tx, c); err != nil {
		return nil, err
	}
	raw, err := s.originalMessage(ctx, tx, v, original, p)
	if err != nil {
		return nil, err
	}
	if !validAt(c, time.Now()) {
		return nil, ErrForbidden
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO mindlink_tickets(issuer,jti,ticket_hash,operation_id) VALUES(?,?,?,?) ON DUPLICATE KEY UPDATE jti=jti", c.Issuer, c.JTI, auth.ticketHash[:], c.OperationID); err != nil {
		return nil, ErrUnknownResult
	}
	var ticketHash []byte
	var operationID string
	if tx.QueryRowContext(ctx, "SELECT ticket_hash,operation_id FROM mindlink_tickets WHERE issuer=? AND jti=? FOR UPDATE", c.Issuer, c.JTI).Scan(&ticketHash, &operationID) != nil {
		return nil, ErrUnknownResult
	}
	if !bytes.Equal(ticketHash, auth.ticketHash[:]) || operationID != c.OperationID {
		return nil, ErrRequestConflict
	}
	if !validAt(c, time.Now()) {
		return nil, ErrForbidden
	}
	if tx.Commit() != nil {
		return nil, ErrUnknownResult
	}
	return &MutationResult{Result: append(json.RawMessage(nil), raw...), Replayed: true}, nil
}
