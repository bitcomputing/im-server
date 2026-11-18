package mindlink

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func assertPolicyError(t *testing.T, rec *httptest.ResponseRecorder, status, code int) {
	t.Helper()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid error envelope, status=%d", rec.Code)
	}
	if rec.Code != status || string(envelope["code"]) != jsonNumber(code) || string(envelope["data"]) != "null" || len(envelope) != 3 {
		t.Fatalf("wrong error envelope: status=%d code=%s fields=%d data=%s", rec.Code, envelope["code"], len(envelope), envelope["data"])
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatal("missing JSON content type")
	}
}

func jsonNumber(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestPolicyHTTPDefaultClosed(t *testing.T) {
	for _, tc := range []struct{ method, path string }{{"GET", "/apigateway/mindlink/operations/" + strings.Repeat("a", 64)}, {"POST", PrincipalDisablePath}, {"POST", PrincipalEnsurePath}, {"POST", MessageSendPath}, {"GET", MessageResultPathPrefix + strings.Repeat("e", 64)}} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		r.Header.Set("appkey", "legacy-app")
		r.Header.Set("signature", "legacy-signature")
		rec := httptest.NewRecorder()
		NewPolicyHTTP(nil, nil).ServeHTTP(rec, r)
		assertPolicyError(t, rec, 409, 19005)
	}
}

func TestPolicyHTTPRejectsUnverifiedRequestBeforeDatabase(t *testing.T) {
	v, _, _, _ := queryFixture(t)
	h := NewPolicyHTTP(v, &sql.DB{}) // no DB operation is allowed before verification.
	for _, tc := range []struct{ method, path string }{{"GET", "/apigateway/mindlink/operations/" + strings.Repeat("a", 64)}, {"POST", PrincipalDisablePath}, {"POST", PrincipalEnsurePath}, {"POST", MessageSendPath}, {"GET", MessageResultPathPrefix + strings.Repeat("e", 64)}} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		assertPolicyError(t, rec, 403, 19008)
	}
}
