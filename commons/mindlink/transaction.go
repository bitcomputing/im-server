package mindlink

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

var (
	ErrRequestConflict   = errors.New("mindlink original request conflict")
	ErrStalePolicy       = errors.New("mindlink stale authority")
	ErrPrincipalDisabled = errors.New("mindlink principal disabled")
	ErrUnknownResult     = errors.New("mindlink result unknown")
)

type MutationResult struct {
	Result   json.RawMessage
	Replayed bool
}

// Transact binds a mutation to its original request and ticket. authorize must
// lock current authority: read permission for an original result, submission
// permission/version for a new effect. effect must only use
// this transaction. External delivery belongs in a transactional outbox.
func (v *Verifier) Transact(ctx context.Context, db *sql.DB, auth *Authorization, authorize func(*sql.Tx, Claims, bool) error, effect func(*sql.Tx) (json.RawMessage, error)) (*MutationResult, error) {
	if v == nil || len(v.bindings) == 0 || db == nil || authorize == nil || effect == nil {
		return nil, ErrNotReady
	}
	if auth == nil || auth.verifier != v || auth.claims.Method == http.MethodGet {
		return nil, ErrForbidden
	}
	if ctx == nil || len(auth.claims.Issuer) > 1024 {
		return nil, ErrInvalidRequest
	}
	c := auth.claims
	if !validAt(c, time.Now()) {
		return nil, ErrForbidden
	}
	requestKeyBytes, err := json.Marshal([]string{c.Issuer, c.Environment, c.TenantID, c.AppKey, c.OwnerID, c.ActorKind, c.ActorID, c.RequestID})
	if err != nil {
		return nil, ErrInvalidRequest
	}
	requestKey := sha256.Sum256(requestKeyBytes)
	frozen := c
	frozen.IssuedAt, frozen.ExpiresAt, frozen.JTI = 0, 0, ""
	metadata, err := json.Marshal(frozen)
	if err != nil {
		return nil, ErrInvalidRequest
	}
	bindingHash := sha256.Sum256(metadata)
	var reservation [32]byte
	if _, err := rand.Read(reservation[:]); err != nil {
		return nil, ErrUnknownResult
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, ErrUnknownResult
	}
	defer tx.Rollback()
	// DATETIME does not carry a zone; store the first observation explicitly in UTC.
	_, err = tx.ExecContext(ctx, `INSERT INTO mindlink_operations (issuer,operation_id,request_key,binding_hash,binding_json,reservation_id,created_time) VALUES(?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE operation_id=operation_id`, c.Issuer, c.OperationID, requestKey[:], bindingHash[:], metadata, reservation[:], time.Now().UTC().Format("2006-01-02 15:04:05.000000"))
	if err != nil {
		return nil, ErrUnknownResult
	}
	var storedBinding, storedReservation, original []byte
	err = tx.QueryRowContext(ctx, `SELECT binding_hash,reservation_id,result_json FROM mindlink_operations WHERE issuer=? AND operation_id=? FOR UPDATE`, c.Issuer, c.OperationID).Scan(&storedBinding, &storedReservation, &original)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRequestConflict
	}
	if err != nil {
		return nil, ErrUnknownResult
	}
	if !bytes.Equal(storedBinding, bindingHash[:]) {
		return nil, ErrRequestConflict
	}
	replayed := !bytes.Equal(storedReservation, reservation[:])
	if replayed && !validResult(original) {
		return nil, ErrUnknownResult
	}
	if err := authorize(tx, c, replayed); err != nil {
		return nil, mutationError(err)
	}
	// A request may have waited behind operation or authority locks beyond its TTL.
	if !validAt(c, time.Now()) {
		return nil, ErrForbidden
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO mindlink_tickets (issuer,jti,ticket_hash,operation_id) VALUES(?,?,?,?) ON DUPLICATE KEY UPDATE jti=jti`, c.Issuer, c.JTI, auth.ticketHash[:], c.OperationID)
	if err != nil {
		return nil, ErrUnknownResult
	}
	var ticketHash []byte
	var operationID string
	if err := tx.QueryRowContext(ctx, `SELECT ticket_hash,operation_id FROM mindlink_tickets WHERE issuer=? AND jti=? FOR UPDATE`, c.Issuer, c.JTI).Scan(&ticketHash, &operationID); err != nil {
		return nil, ErrUnknownResult
	}
	if !bytes.Equal(ticketHash, auth.ticketHash[:]) || operationID != c.OperationID {
		return nil, ErrRequestConflict
	}
	if !validAt(c, time.Now()) {
		return nil, ErrForbidden
	}
	if !replayed {
		original, err = effect(tx)
		if err != nil {
			return nil, mutationError(err)
		}
		if !validResult(original) {
			return nil, ErrUnknownResult
		}
		updated, err := tx.ExecContext(ctx, `UPDATE mindlink_operations SET result_json=? WHERE issuer=? AND operation_id=? AND reservation_id=? AND result_json IS NULL`, []byte(original), c.Issuer, c.OperationID, reservation[:])
		if err != nil {
			return nil, ErrUnknownResult
		}
		rows, err := updated.RowsAffected()
		if err != nil || rows != 1 {
			return nil, ErrUnknownResult
		}
	}
	if !validAt(c, time.Now()) {
		return nil, ErrForbidden
	}
	if err := tx.Commit(); err != nil {
		return nil, ErrUnknownResult
	}
	return &MutationResult{Result: append(json.RawMessage(nil), original...), Replayed: replayed}, nil
}

func validResult(result []byte) bool {
	trimmed := bytes.TrimSpace(result)
	return len(result) <= MaxBodyBytes && len(trimmed) > 0 && trimmed[0] == '{' && json.Valid(trimmed)
}

func mutationError(err error) error {
	for _, known := range []error{ErrInvalidRequest, ErrForbidden, ErrScopeMismatch, ErrNotReady, ErrRequestConflict, ErrStalePolicy, ErrPrincipalDisabled, ErrUnknownResult} {
		if errors.Is(err, known) {
			return known
		}
	}
	return ErrUnknownResult
}
