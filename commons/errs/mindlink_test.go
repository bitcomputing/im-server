package errs

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestMindlinkHTTPErrorCodes(t *testing.T) {
	cases := []struct {
		code   IMErrorCode
		status int
		reason string
	}{
		{19001, 400, "INVALID_REQUEST"},
		{19002, 403, "SCOPE_MISMATCH"},
		{19003, 409, "REQUEST_CONFLICT"},
		{19004, 409, "STALE_POLICY"},
		{19005, 409, "NOT_READY"},
		{19006, 409, "PRINCIPAL_DISABLED"},
		{19007, 503, "UNKNOWN_RESULT"},
		{19008, 403, "FORBIDDEN"},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			got := GetApiErrorByCode(tc.code)
			if got.HttpCode != tc.status || got.Code != tc.code || got.Msg != tc.reason {
				t.Fatalf("error mapping = %+v, want HTTP %d/%d/%s", got, tc.status, tc.code, tc.reason)
			}
			body, err := json.Marshal(got)
			want := fmt.Sprintf(`{"code":%d,"msg":%q}`, tc.code, tc.reason)
			if err != nil || string(body) != want {
				t.Fatalf("wire body = %s (%v), want %s", body, err, want)
			}
		})
	}
}
