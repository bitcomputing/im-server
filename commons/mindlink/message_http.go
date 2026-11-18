package mindlink

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type MessageHTTP struct {
	verifier *Verifier
	store    *MessageStore
}

// NewMessageHTTP is the private producer adapter. Production routing supplies
// nil dependencies until projection writers, native fences and delivery pass.
func NewMessageHTTP(v *Verifier, store *MessageStore) *MessageHTTP {
	return &MessageHTTP{v, store}
}

func (h *MessageHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	operation := ""
	if r.Method == http.MethodPost && r.URL.Path == MessageSendPath {
		operation = OperationMessageSend
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, MessageResultPathPrefix) {
		operation = OperationMessageQuery
	}
	if operation == "" {
		http.NotFound(w, r)
		return
	}
	if h == nil || h.verifier == nil || h.store == nil {
		writePolicyError(w, ErrNotReady)
		return
	}
	auth, err := h.verifier.Verify(r, operation, time.Now())
	if err != nil {
		writePolicyError(w, err)
		return
	}
	var result *MutationResult
	if operation == OperationMessageSend {
		result, err = h.store.Send(r.Context(), h.verifier, auth)
	} else {
		result, err = h.store.Query(r.Context(), h.verifier, auth)
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
