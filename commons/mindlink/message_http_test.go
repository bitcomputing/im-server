package mindlink

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMessageHTTPClosed(t *testing.T) {
	for _, tc := range []struct{ method, path string }{
		{"POST", MessageSendPath}, {"GET", MessageResultPathPrefix + strings.Repeat("a", 64)},
	} {
		t.Run(tc.method, func(t *testing.T) {
			w := httptest.NewRecorder()
			NewMessageHTTP(nil, nil).ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			var response struct {
				Code int             `json:"code"`
				Data json.RawMessage `json:"data"`
			}
			if json.Unmarshal(w.Body.Bytes(), &response) != nil || w.Code != 409 || response.Code != 19005 || string(response.Data) != "null" {
				t.Fatal("closed capability exposed a success result")
			}
		})
	}
}
