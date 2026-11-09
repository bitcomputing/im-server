//go:build integration

package mindlink

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"im-server/commons/bases"
	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/errs"
	"im-server/commons/gmicro/actorsystem"
	"im-server/commons/logs"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/commons/tools"
	"im-server/services/commonservices/tokens"
	connectactors "im-server/services/connectmanager/actors"
	"im-server/services/connectmanager/server/codec"
	"im-server/services/connectmanager/server/imcontext"
	"im-server/services/connectmanager/server/imhttpmsghandlers"
	connections "im-server/services/connectmanager/services"
	converactors "im-server/services/conversation/actors"
	friendactors "im-server/services/friendmanager/actors"
	groupactors "im-server/services/group/actors"
	historyactors "im-server/services/historymsg/actors"
	useractors "im-server/services/usermanager/actors"

	"google.golang.org/protobuf/proto"
)

const nativePrincipal = "ma111111111111111111111111111111"
const nativeApp = "fixture-app"
const nativeSecureKey = "0123456789abcdef"

func nativeBoundaryDB(t *testing.T) *sql.DB {
	t.Helper()
	path := os.Getenv("MINDLINK_TEST_DSN_FILE")
	if path == "" {
		t.Skip("requires owned MySQL; not native boundary coverage")
	}
	raw, err := os.ReadFile(path)
	var private struct {
		DSN string `json:"dsn"`
	}
	if err != nil || json.Unmarshal(raw, &private) != nil {
		t.Fatal("read owned native fixture")
	}
	cfg, err := mysql.ParseDSN(private.DSN)
	if err != nil {
		t.Fatal("invalid owned DSN")
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil || host != "127.0.0.1" || cfg.Net != "tcp" || cfg.DBName != "mindlink_policy_c82" {
		t.Fatal("refusing non-owned database")
	}
	cfg.MultiStatements = true
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open owned DB")
	}
	t.Cleanup(func() { root.Close() })
	name := fmt.Sprintf("mindlink_policy_c82_boundary_%x", time.Now().UnixNano())
	if !strings.HasPrefix(name, "mindlink_policy_c82_boundary_") || len(name) > 64 {
		t.Fatal("invalid scratch name")
	}
	t.Log("owned native boundary scratch:", name)
	if _, err := root.Exec("CREATE DATABASE `" + name + "`"); err != nil {
		t.Fatal("create owned scratch")
	}
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open owned scratch")
	}
	saved := configures.Config
	t.Cleanup(func() {
		_ = bases.InitNativePolicy(context.Background(), nil)
		dbcommons.CloseDB()
		db.Close()
		configures.Config = saved
		if _, err := root.Exec("DROP DATABASE `" + name + "`"); err != nil {
			t.Error("owned native scratch cleanup failed")
		}
	})
	initial, err := os.ReadFile("../../../sql/imserver.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(initial)); err != nil {
		t.Fatal("initialize owned native schema", err)
	}
	for _, q := range []string{
		"INSERT INTO apps(app_key,app_secret,app_secure_key) VALUES('fixture-app','fixture-secret','0123456789abcdef')",
		"INSERT INTO users(app_key,user_id,nickname) VALUES('fixture-app','42','ordinary owner'),('fixture-app','ma111111111111111111111111111111','internal fixture')",
		"INSERT INTO groupinfos(app_key,group_id,group_name) VALUES('fixture-app','protected-group','fixture'),('fixture-app','ordinary-group','ordinary')",
		"INSERT INTO groupmembers(app_key,group_id,member_id,is_mute,is_allow) VALUES('fixture-app','protected-group','ma111111111111111111111111111111',1,0),('fixture-app','protected-group','42',0,1),('fixture-app','ordinary-group','42',0,0)",
		"INSERT INTO mindlink_owner_barriers(environment,tenant_id,appkey,owner_id,read_enabled) VALUES('dev','fixture-tenant','fixture-app','42',TRUE)",
		"INSERT INTO mindlink_principals(environment,tenant_id,appkey,owner_id,principal_id,avatar_id,avatar_generation,policy_version,state) VALUES('dev','fixture-tenant','fixture-app','42','ma111111111111111111111111111111','avatar-A',2,7,'active')",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	configures.Config.Mysql.User, configures.Config.Mysql.Password = cfg.User, cfg.Passwd
	configures.Config.Mysql.Address, configures.Config.Mysql.DbName = cfg.Addr, cfg.DBName
	configures.Config.Mysql.Debug = false
	configures.Config.Mindlink.Enabled = false
	configures.Config.Log.LogPath, configures.Config.Log.LogName = t.TempDir(), "native-boundary"
	logs.InitLogs()
	if dbcommons.InitMysql() != nil {
		t.Fatal("initialize actual native storage on owned scratch")
	}
	if bases.InitNativePolicy(context.Background(), db) != nil {
		t.Fatal("initialize actual native fence")
	}
	return db
}

type nativeLoginContext struct{ attachment imcontext.Attachment }

func (c *nativeLoginContext) Write(any)                            {}
func (c *nativeLoginContext) Close(error)                          {}
func (c *nativeLoginContext) Attachment() imcontext.Attachment     { return c.attachment }
func (c *nativeLoginContext) SetAttachment(a imcontext.Attachment) { c.attachment = a }
func (c *nativeLoginContext) IsActive() bool                       { return true }
func (c *nativeLoginContext) RemoteAddr() string                   { return "127.0.0.1:10000" }

func nativeToken(t *testing.T, id string) string {
	t.Helper()
	tok := tokens.ImToken{AppKey: nativeApp, UserId: id, TokenTime: time.Now().UnixMilli()}
	encoded, err := tok.ToTokenString([]byte(nativeSecureKey))
	if err != nil {
		t.Fatal("create isolated native token")
	}
	return encoded
}

func TestNativePolicyActualTokenAndDecodedActorBoundaries(t *testing.T) {
	db := nativeBoundaryDB(t)
	for _, state := range []string{"active", "disabled"} {
		if _, err := db.Exec("UPDATE mindlink_principals SET state=?", state); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{nativePrincipal, strings.ToUpper(nativePrincipal), "má" + nativePrincipal[2:]} {
			for _, bot := range []bool{false, true} {
				ctx := &nativeLoginContext{}
				msg := &codec.ConnectMsgBody{Appkey: nativeApp, Token: nativeToken(t, id), Platform: "Web"}
				var code int32
				if bot {
					code = int32(connections.CheckBotLogin(ctx, msg))
				} else {
					code, _ = connections.CheckLogin(ctx, msg)
				}
				if code != int32(errs.IMErrorCode_MINDLINK_FORBIDDEN) || imcontext.GetUserId(ctx) != "" {
					t.Errorf("internal native login allowed state=%s bot=%t code=%d", state, bot, code)
				}
			}
		}
	}
	ctx := &nativeLoginContext{}
	code, _ := connections.CheckLogin(ctx, &codec.ConnectMsgBody{Appkey: nativeApp, Token: nativeToken(t, "42"), Platform: "Web"})
	if code != 0 || imcontext.GetUserId(ctx) != "42" {
		t.Fatal("ordinary owner login changed", code)
	}
	frame, err := tools.PbMarshal(codec.NewUserPublishMessage(&codec.PublishMsgBody{Index: 91, Topic: "p_msg", TargetId: "42"}).ToImWebsocketMsg())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/im/publish", bytes.NewReader(frame))
	request.Header.Set("x-appkey", nativeApp)
	request.Header.Set("x-token", nativeToken(t, nativePrincipal))
	response := httptest.NewRecorder()
	imhttpmsghandlers.ImHttpPubHandler(response, request)
	var ack codec.ImWebsocketMsg
	if response.Code != 200 || tools.PbUnMarshal(response.Body.Bytes(), &ack) != nil || ack.GetPubAckMsgBody() == nil || ack.GetPubAckMsgBody().Code != int32(errs.IMErrorCode_MINDLINK_FORBIDDEN) || ack.GetPubAckMsgBody().Index != 91 || ack.GetPubAckMsgBody().MsgId != "" {
		t.Fatal("native HTTP publish token escaped denial")
	}
	for _, tc := range []struct {
		method, service, route string
		actor                  actorsystem.IUntypedActor
		msg                    proto.Message
	}{
		{"reg_user", "usermanager", nativePrincipal, &useractors.UserRegistActor{}, &pbobjs.UserInfo{UserId: strings.ToUpper(nativePrincipal), Nickname: "overwrite"}},
		{"group_member_mute", "group", "protected-group", &groupactors.GroupMemberMuteActor{}, &pbobjs.GroupMemberMuteReq{GroupId: "protected-group", MemberIds: []string{nativePrincipal}, IsMute: 0}},
		{"add_friends", "friendmanager", "42", &friendactors.AddFriendActor{}, &pbobjs.FriendMembersReq{FriendMembers: []*pbobjs.FriendMember{{FriendId: nativePrincipal}}}},
		{"add_favorite_msgs", "historymsg", "42", &historyactors.AddFavoriteMsgsActor{}, &pbobjs.FavoriteMsgIds{Items: []*pbobjs.FavoriteMsgIdItem{{SenderId: "42", ReceiverId: "protected-group", ChannelType: pbobjs.ChannelType_Group, MsgId: "fixture-message"}}}},
		{"qry_g_conver_conf", "conversation", "protected-group", &converactors.QryGlobalConverConfActor{}, &pbobjs.ConverConfReq{ConverId: "protected-group", ChannelType: pbobjs.ChannelType_Group}},
	} {
		code, data := nativeActorCall(tc.actor, tc.service, tc.method, tc.route, tc.msg)
		if code != int32(errs.IMErrorCode_MINDLINK_FORBIDDEN) || len(data) != 0 {
			t.Errorf("actual native actor allowed %s code=%d", tc.method, code)
		}
	}
	var nickname string
	if db.QueryRow("SELECT nickname FROM users WHERE app_key=? AND user_id=?", nativeApp, nativePrincipal).Scan(&nickname) != nil || nickname != "internal fixture" {
		t.Fatal("rejected registration changed user")
	}
	var mute int
	if db.QueryRow("SELECT is_mute FROM groupmembers WHERE app_key=? AND group_id='protected-group' AND member_id=?", nativeApp, nativePrincipal).Scan(&mute) != nil || mute != 1 {
		t.Fatal("rejected self-unmute changed roster")
	}
	deviceCode, _ := nativeActorCall(&connectactors.AddBindDeviceActor{}, "connectmanager", "add_bind_device", nativePrincipal, &pbobjs.BindDevice{DeviceId: "rejected-device", Platform: "Web"}, "")
	if deviceCode != int32(errs.IMErrorCode_MINDLINK_FORBIDDEN) {
		t.Errorf("route-only device binding escaped fence code=%d", deviceCode)
	}
	var devices int
	if err := db.QueryRow("SELECT COUNT(*) FROM binddevices WHERE app_key=? AND user_id=?", nativeApp, nativePrincipal).Scan(&devices); err != nil || devices != 0 {
		t.Fatal("rejected device binding left rows")
	}
	var friends int
	if err := db.QueryRow("SELECT COUNT(*) FROM friendrels WHERE app_key=?", nativeApp).Scan(&friends); err != nil || friends != 0 {
		t.Fatal("rejected friend mutation left rows")
	}
}

type nativeBoundarySender struct {
	actorsystem.DeadLetterActorRef
	reply *pbobjs.RpcMessageWraper
}

func (s *nativeBoundarySender) Tell(msg proto.Message, _ actorsystem.ActorRef) {
	s.reply, _ = msg.(*pbobjs.RpcMessageWraper)
}
func nativeActorCall(actor actorsystem.IUntypedActor, service, method, target string, msg proto.Message, requesters ...string) (int32, []byte) {
	sender := &nativeBoundarySender{}
	wrapped := bases.BaseProcessActor(actor, service)
	wrapped.(actorsystem.ISenderHandler).SetSender(sender)
	data, _ := tools.PbMarshal(msg)
	requester := "42"
	if len(requesters) > 0 {
		requester = requesters[0]
	}
	wrapped.(actorsystem.IReceiveHandler).OnReceive(context.Background(), &pbobjs.RpcMessageWraper{RpcMsgType: pbobjs.RpcMsgType_QueryMsg, AppKey: nativeApp, RequesterId: requester, TargetId: target, Method: method, IsFromApi: true, AppDataBytes: data})
	if sender.reply == nil {
		return -1, nil
	}
	return sender.reply.ResultCode, sender.reply.AppDataBytes
}
