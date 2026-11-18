//go:build integration

package mindlink

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"im-server/commons/bases"
	"im-server/commons/errs"
	api "im-server/services/apigateway/apis"
)

func TestNativePolicyLegacyHTTPRejectsBeforeEnqueue(t *testing.T) {
	_ = nativeBoundaryDB(t)
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name    string
		handler gin.HandlerFunc
		body    string
	}{
		{"private", api.SendPrivateMsg, `{"sender_id":"42","target_id":"ma111111111111111111111111111111","msg_type":"jg:text","msg_content":"fixture"}`},
		{"system", api.SendSystemMsg, `{"sender_id":"42","target_id":"ma111111111111111111111111111111","msg_type":"jg:text","msg_content":"fixture"}`},
		{"group", api.SendGroupMsg, `{"sender_id":"42","target_id":"protected-group","msg_type":"jg:text","msg_content":"fixture"}`},
		{"stream", api.SendPrivateStreamMsg, `{"from_id":"42","target_id":"ma111111111111111111111111111111","partial_content":"fixture","seq":1}`},
		{"profile", api.UpdateUser, `{"user_id":"ma111111111111111111111111111111","nickname":"must-not-update"}`},
		{"group_settings", api.SetGroupSettings, `{"group_id":"protected-group","settings":{"hide_grp_msg":1}}`},
		{"member_settings", api.SetGroupMemberSettings, `{"group_id":"protected-group","member_id":"42","settings":{"hide_grp_msg":1}}`},
		{"bot", api.UpdateBot, `{"bot_id":"ma111111111111111111111111111111","nickname":"must-not-update"}`},
		{"import", api.ImportHisMsg, `{"sender_id":"42","target_id":"ma111111111111111111111111111111","channel_type":1,"msg_type":"jg:text","msg_content":"fixture"}`},
		{"mark_read", api.MarkRead, `{"user_id":"42","target_id":"protected-group","channel_type":2,"msg_ids":["fixture"]}`},
		{"clear_unread", api.ClearConverUnread, `{"user_id":"42","items":[{"target_id":"protected-group","channel_type":2}]}`},
		{"private_global_mute", api.AddPrivateGlobalMuteMembers, `{"user_ids":["42","ma111111111111111111111111111111"]}`},
		{"group_global_mute", api.AddGroupGlobalMuteMembers, `{"user_ids":["42","ma111111111111111111111111111111"]}`},
		{"ban", api.UserBan, `{"items":[{"user_id":"ma111111111111111111111111111111"}]}`},
		{"block", api.BlockUser, `{"user_id":"42","block_user_ids":["ma111111111111111111111111111111"]}`},
		{"online_status", api.QryUserOnlineStatus, `{"user_ids":["42","ma111111111111111111111111111111"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			ctx.Set(string(bases.CtxKey_AppKey), nativeApp)
			ctx.Request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/native-fixture", bytes.NewBufferString(tc.body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			callNativeStorage(t, func() { tc.handler(ctx) })
			var result response
			if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Code != int(errs.IMErrorCode_MINDLINK_FORBIDDEN) || len(result.Data) > 0 && string(result.Data) != "null" {
				t.Errorf("native HTTP did not reject before enqueue: status=%d code=%d", w.Code, result.Code)
			}
		})
	}
}

func TestNativePolicyHTTPRejectsNullAddressItems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name    string
		handler gin.HandlerFunc
		body    string
	}{
		{"groupcast", api.SendGroupCastMsg, `{"sender_id":"42","msg_type":"jg:text","msg_content":"fixture","target_convers":[null]}`},
		{"mention", api.SendPrivateMsg, `{"sender_id":"42","target_id":"43","msg_type":"jg:text","msg_content":"fixture","mention_info":{"mention_type":"mention_someone","target_users":[null]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			ctx.Set(string(bases.CtxKey_AppKey), nativeApp)
			ctx.Request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/native-fixture", bytes.NewBufferString(tc.body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			callNativeStorage(t, func() { tc.handler(ctx) })
			var result response
			if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Code != int(errs.IMErrorCode_API_REQ_BODY_ILLEGAL) {
				t.Error("null address item did not return body error")
			}
		})
	}
}
