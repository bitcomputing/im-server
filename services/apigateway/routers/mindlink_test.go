package routers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMindlinkRoutesAreRegisteredAndDoNotUseLegacySignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	Route(e, "apigateway", nil)
	for _, tc := range []struct {
		method, path string
		status, code int
	}{
		{"POST", "/apigateway/mindlink/principals/disable", 409, 19005},
		{"POST", "/apigateway/mindlink/principals/ensure", 409, 19005},
		{"GET", "/apigateway/mindlink/operations/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 409, 19005},
		{"POST", "/apigateway/users/register", 400, 10001},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRecorder()
			e.ServeHTTP(r, httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`)))
			var body struct {
				Code int `json:"code"`
			}
			if json.Unmarshal(r.Body.Bytes(), &body) != nil || r.Code != tc.status || body.Code != tc.code {
				t.Fatalf("wrong registered auth boundary: status=%d code=%d", r.Code, body.Code)
			}
		})
	}
}
