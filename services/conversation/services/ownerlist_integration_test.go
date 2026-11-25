//go:build integration

package services

import (
	"google.golang.org/protobuf/proto"
	"im-server/commons/bases"
	"im-server/commons/dbcommons"
	"im-server/commons/pbdefines/pbobjs"
	"strings"
	"testing"
)

func TestOwnerListRequiresNegotiationReadAndMigration(t *testing.T) {
	db, ctx, _ := nativeReadFixture(t)
	id := strings.Repeat("b", 64)
	_, err := db.Exec("INSERT INTO mindlink_conversations(environment,tenant_id,appkey,owner_id,conversation_id,kind,channel_type,native_target_id,owner_im_id,policy_version,state,members_json) VALUES('dev','list-tenant','list-app','42',?,'owner_avatar_dm',1,?,'42',1,'active',?)", id, listAvatar, []byte(`[{"avatar_id":"list-avatar","principal_id":"ma111111111111111111111111111111"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO conversations(app_key,user_id,target_id,channel_type,sub_channel,sort_time,sync_time) VALUES('list-app','42',?,1,'',100,100)", listAvatar); err != nil {
		t.Fatal(err)
	}
	set := func(query string) {
		t.Helper()
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	check := func(negotiate, ready, visible bool) {
		t.Helper()
		code, page := QryConversationsV2(ctx, &pbobjs.QryConversationsReq{Count: 10, StartTime: 200, OwnerBusinessList: negotiate})
		if code != 0 || page == nil || page.OwnerBusinessList != ready {
			t.Fatalf("list code=%d response=%v", code, page)
		}
		found := false
		for _, r := range page.Conversations {
			if r.TargetId == listAvatar {
				if found || r.Business.GetType() != "owner_avatar_dm" || r.Business.GetBusinessId() != id || len(r.Business.AvatarIds) != 1 || r.Business.AvatarIds[0] != "list-avatar" {
					t.Fatal("invalid owner projection", r)
				}
				found = true
			}
		}
		if found != visible {
			t.Fatalf("owner row visible=%t wanted=%t", found, visible)
		}
	}
	// Reproduce both an old database and a restart after MySQL committed the ALTER.
	set("ALTER TABLE mindlink_owner_barriers DROP COLUMN list_projection_ready")
	for i := 0; i < 2; i++ {
		set("UPDATE globalconfs SET conf_value='20261122' WHERE conf_key='jimdb_version'")
		if err := dbcommons.Upgrade(); err != nil {
			t.Fatal(err)
		}
		var version string
		if err := db.QueryRow("SELECT conf_value FROM globalconfs WHERE conf_key='jimdb_version'").Scan(&version); err != nil || version != "20261125" {
			t.Fatal("migration version", version, err)
		}
		check(true, false, false)
	}
	var raw []byte
	if err := db.QueryRow("SELECT msg_body FROM p_hismsgs WHERE app_key='list-app' AND msg_id='list-msg-0'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var msg pbobjs.DownMsg
	if err := proto.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}
	msg.MsgSeqNo = 1
	raw, err = proto.Marshal(&msg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE p_hismsgs SET msg_body=? WHERE app_key='list-app' AND msg_id='list-msg-0'", raw); err != nil {
		t.Fatal(err)
	}
	set("UPDATE mindlink_owner_barriers SET list_projection_ready=TRUE")
	if _, _, err := ownerListRows(ctx, ""); err != nil {
		t.Fatal(err)
	}
	check(false, false, false)
	check(true, true, true)
	single := &pbobjs.QryConverReq{OwnerBusinessList: true, TargetId: listAvatar, ChannelType: pbobjs.ChannelType_Private}
	if code, row := QryConverV2(ctx, "42", single); code != 0 || row.GetBusiness().GetBusinessId() != id {
		t.Fatal("owner single projection", code, row)
	}
	if code, _ := QryConverV2(ctx, "ordinary-peer", single); code == 0 {
		t.Fatal("foreign single projection")
	}
	for _, enabled := range []bool{false, true} {
		if _, err := db.Exec("UPDATE mindlink_owner_barriers SET write_enabled=?", enabled); err != nil {
			t.Fatal(err)
		}
		rows, ready, err := ownerListRows(ctx, "")
		if err != nil || !ready || len(rows) != 1 || rows[0].Business.ReadOnly == enabled {
			t.Fatal("wrong paused owner projection", enabled, rows, err)
		}
	}
	set("UPDATE mindlink_principals SET state='disabled' WHERE avatar_id='list-avatar'")
	rows, ready, err := ownerListRows(ctx, listAvatar)
	if err != nil || !ready || len(rows) != 1 || !rows[0].Business.ReadOnly || rows[0].Msg == nil {
		t.Fatal("retired DM lost retained read-only history", rows, err)
	}
	set("UPDATE mindlink_principals SET state='active' WHERE avatar_id='list-avatar'")
	for i, kind := range []string{"mindlink_group", "avatar_pair_group"} {
		target := []string{"test-home", "test-pair"}[i]
		biz := strings.Repeat([]string{"a", "c"}[i], 64)
		members := []byte(`[{"avatar_id":"first"},{"avatar_id":"second"}]`)
		if _, err := db.Exec("INSERT INTO mindlink_conversations(environment,tenant_id,appkey,owner_id,conversation_id,kind,channel_type,native_target_id,owner_im_id,policy_version,state,members_json) VALUES('dev','list-tenant','list-app','42',?,?,2,?,'42',1,'active',?)", biz, kind, target, members); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO groupmembers(app_key,group_id,member_id) VALUES('list-app',?,'42')", target); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO conversations(app_key,user_id,target_id,channel_type,sub_channel,sort_time,sync_time) VALUES('list-app','42',?,2,'',100,100)", target); err != nil {
			t.Fatal(err)
		}
	}
	request := &pbobjs.QryConversationsReq{Count: 1, OwnerBusinessList: true}
	got := map[string]bool{}
	for pages := 0; ; pages++ {
		if pages > 8 {
			t.Fatal("pagination did not finish")
		}
		code, page := QryConversationsV2(ctx, request)
		if code != 0 || page == nil || len(page.Conversations) != 1 {
			t.Fatal("incomplete owner page", code, page)
		}
		r := page.Conversations[0]
		if got[r.TargetId] {
			t.Fatal("repeated target", r.TargetId)
		}
		got[r.TargetId] = true
		if r.TargetId == "test-pair" && (r.Business.GetType() != "avatar_pair_group" || !r.Business.ReadOnly || r.Msg != nil || r.Business.HasNew || r.UnreadCount != 0) {
			t.Fatal("invalid empty pair", r)
		}
		if page.IsFinished {
			break
		}
		if page.OwnerListCursor == "" {
			t.Fatal("missing cursor")
		}
		request.OwnerListCursor = page.OwnerListCursor
	}
	if !got["test-home"] || !got["test-pair"] || !got[listAvatar] || !got["ordinary-peer"] {
		t.Fatal("missing owner targets", got)
	}
	var foreign = bases.CreateRpcCtx("list-app", "ordinary-peer")
	if rows, ready, e := ownerListRows(foreign, ""); e != nil || ready || len(rows) > 0 {
		t.Fatal("foreign owner read", ready, e)
	}
	set("UPDATE conversations SET is_deleted=1 WHERE app_key='list-app'")
	check(true, true, false)
	if code, _ := QryConverV2(ctx, "42", single); code == 0 {
		t.Fatal("hidden direct projection")
	}
	set("UPDATE mindlink_owner_barriers SET read_enabled=FALSE")
	check(true, false, false)
}
