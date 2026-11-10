//go:build integration

package mindlink

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMySQLEmptyAndRetiredMindlinkGroupsRemainFenced(t *testing.T) {
	db := nativeFixture(t)
	group := "mg" + strings.Repeat("3", 30)
	if _, err := db.Exec(`INSERT INTO groupinfos(app_key,group_id,group_name) VALUES('fixture-app',?,'empty protected group')`, group); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO mindlink_conversations(environment,tenant_id,appkey,owner_id,conversation_id,kind,channel_type,native_target_id,owner_im_id,policy_version,state,members_json) VALUES('dev','fixture-tenant','fixture-app','42',?,'mindlink_group',2,?,'42',1,'active','[]')`, strings.Repeat("c", 64), group); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO mindlink_conversations(environment,tenant_id,appkey,owner_id,conversation_id,kind,channel_type,native_target_id,owner_im_id,policy_version,state,members_json) VALUES('dev','fixture-tenant','fixture-app','43',?,'avatar_pair_group',2,?,'43',1,'active','[]')`, strings.Repeat("d", 64), strings.ToUpper(group)); err == nil {
		t.Fatal("a second owner claimed the same native group")
	}
	for i, owner := range []string{"42", "43"} {
		id := strings.Repeat([]string{"e", "f"}[i], 64)
		if _, err := db.Exec(`INSERT INTO mindlink_conversations(environment,tenant_id,appkey,owner_id,conversation_id,kind,channel_type,native_target_id,owner_im_id,policy_version,state,members_json) VALUES('dev','fixture-tenant','fixture-app',?,?,'owner_avatar_dm',1,'dm-target',?,1,'active','[]')`, owner, id, owner); err != nil {
			t.Fatalf("independent owner DM mapping: %v", err)
		}
	}
	fence, err := NewNativeFence(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"active", "disabled"} {
		if _, err := db.Exec(`UPDATE mindlink_conversations SET state=?`, state); err != nil {
			t.Fatal(err)
		}
		if err := fence.Check(context.Background(), "fixture-app", nil, []string{strings.ToUpper(group)}); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s empty group escaped fence: %v", state, err)
		}
		visible, err := fence.Filter(context.Background(), "fixture-app", []NativeIdentities{{Groups: []string{group}}, {Groups: []string{"ordinary-group"}}})
		if err != nil || len(visible) != 2 || visible[0] || !visible[1] {
			t.Fatalf("%s filter: %v %v", state, visible, err)
		}
	}
}
