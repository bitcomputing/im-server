package mindlink

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"im-server/commons/errs"
)

type PolicyHTTP struct {
	verifier      *Verifier
	db            *sql.DB
	disableSource *DisableAuthoritySource
}

func NewPolicyHTTP(v *Verifier, db *sql.DB) *PolicyHTTP { return &PolicyHTTP{verifier: v, db: db} }

// Enabling the restrictive lifecycle operation requires a server-owned source and native fences.
func NewPolicyHTTPWithDisable(ctx context.Context, v *Verifier, db *sql.DB, source *DisableAuthoritySource) (*PolicyHTTP, error) {
	if v == nil || source == nil || !source.enabled {
		return nil, ErrNotReady
	}
	if _, err := NewNativeFence(ctx, db); err != nil {
		return nil, err
	}
	return &PolicyHTTP{verifier: v, db: db, disableSource: source}, nil
}

type policyEnvelope struct {
	Code errs.IMErrorCode `json:"code"`
	Msg  string           `json:"msg"`
	Data any              `json:"data"`
}

func (h *PolicyHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	operation := ""
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/apigateway/mindlink/operations/") {
		operation = "operations.query"
	}
	if r.Method == http.MethodPost && r.URL.Path == PrincipalDisablePath {
		operation = OperationPrincipalDisable
	}
	if r.Method == http.MethodPost && r.URL.Path == PrincipalEnsurePath {
		operation = OperationPrincipalEnsure
	}
	if r.Method == http.MethodPost && r.URL.Path == ConversationEnsurePath {
		operation = OperationConversationEnsure
	}
	if operation == "" {
		http.NotFound(w, r)
		return
	}
	if h == nil || h.verifier == nil || h.db == nil {
		writePolicyError(w, ErrNotReady)
		return
	}
	auth, err := h.verifier.Verify(r, operation, time.Now())
	if err != nil {
		writePolicyError(w, err)
		return
	}
	var result *MutationResult
	if operation == OperationPrincipalDisable {
		if _, err := h.verifier.ParsePrincipalDisable(auth); err != nil {
			writePolicyError(w, err)
			return
		}
		if h.disableSource == nil {
			writePolicyError(w, ErrNotReady)
			return
		}
		if err = h.disableSource.Check(r.Context(), h.verifier, auth); err != nil {
			writePolicyError(w, err)
			return
		}
		result, err = h.verifier.transactPrincipalDisable(r.Context(), h.db, auth)
	} else if operation == OperationPrincipalEnsure {
		result, err = h.verifier.transactPrincipalEnsure(r.Context(), h.db, auth, h.disableSource)
	} else if operation == OperationConversationEnsure {
		result, err = h.verifier.transactConversationEnsure(r.Context(), h.db, auth, h.disableSource)
	} else {
		result, err = h.verifier.QueryOperation(r.Context(), h.db, auth, func(tx *sql.Tx, query, original Claims) error {
			return authorizeOperationRead(r.Context(), tx, query, original)
		})
	}
	if err != nil {
		writePolicyError(w, err)
		return
	}
	data := struct {
		Operation json.RawMessage `json:"operation"`
		Replayed  bool            `json:"replayed"`
	}{result.Result, result.Replayed}
	writePolicyJSON(w, http.StatusOK, policyEnvelope{0, "success", data})
}

func writePolicyError(w http.ResponseWriter, err error) {
	code := errs.IMErrorCode_MINDLINK_UNKNOWN_RESULT
	for _, item := range []struct {
		err  error
		code errs.IMErrorCode
	}{
		{ErrInvalidRequest, errs.IMErrorCode_MINDLINK_INVALID_REQUEST},
		{ErrScopeMismatch, errs.IMErrorCode_MINDLINK_SCOPE_MISMATCH},
		{ErrRequestConflict, errs.IMErrorCode_MINDLINK_REQUEST_CONFLICT},
		{ErrStalePolicy, errs.IMErrorCode_MINDLINK_STALE_POLICY},
		{ErrNotReady, errs.IMErrorCode_MINDLINK_NOT_READY},
		{ErrPrincipalDisabled, errs.IMErrorCode_MINDLINK_PRINCIPAL_DISABLED},
		{ErrForbidden, errs.IMErrorCode_MINDLINK_FORBIDDEN},
	} {
		if errors.Is(err, item.err) {
			code = item.code
			break
		}
	}
	api := errs.GetApiErrorByCode(code)
	writePolicyJSON(w, api.HttpCode, policyEnvelope{code, api.Msg, nil})
}

func writePolicyJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
