package tools

import (
	"fmt"
	"im-server/commons/errs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMindlinkErrorHTTPResponse(t *testing.T) {
	for code := errs.IMErrorCode(19001); code <= 19008; code++ {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			r := gin.New()
			r.GET("/rejected", func(c *gin.Context) { ErrorHttpResp(c, code) })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/rejected", nil))
			expected := errs.GetApiErrorByCode(code)
			if w.Code != expected.HttpCode || w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatalf("response HTTP %d, content type %s", w.Code, w.Header().Get("Content-Type"))
			}
			if want := fmt.Sprintf(`{"code":%d,"msg":%q}`, code, expected.Msg); w.Body.String() != want {
				t.Fatalf("response body = %s, want %s", w.Body, want)
			}
		})
	}
}
