//go:build integration

package mindlink

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestNativeFilterOrdinalsAliasesAndCurrentMembers(t *testing.T) {
	db := nativeFixture(t)
	fence, err := NewNativeFence(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	rows := []NativeIdentities{
		{Users: []string{"42"}},
		{Users: []string{disablePrincipal, disablePrincipal}},
		{Users: []string{strings.ToUpper(disablePrincipal)}},
		{Users: []string{"m\u00e1" + strings.Repeat("1", 30)}},
		{Groups: []string{disablePrincipal}},
		{Groups: []string{"PROTECTED-GROUP"}},
		{Users: []string{"ma222222222222222222222222222222"}},
		{},
		{Users: []string{"42"}, Groups: []string{"ordinary-group"}},
	}
	want := []bool{true, false, false, false, true, false, true, true, true}
	check := func(app string, expected []bool) {
		t.Helper()
		got, err := fence.Filter(context.Background(), app, rows)
		if err != nil || !reflect.DeepEqual(got, expected) {
			t.Fatalf("ordinal/alias/native scope classification: got=%v want=%v err=%v", got, expected, err)
		}
	}
	check("fixture-app", want)
	check("FIXTURE-APP", want)
	if _, err := db.Exec("INSERT INTO groupinfos(app_key,group_id,group_name) VALUES('other-app','protected-group','ordinary other tenant')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO groupmembers(app_key,group_id,member_id) VALUES('other-app','protected-group',?)", disablePrincipal); err != nil {
		t.Fatal(err)
	}
	other := make([]bool, len(rows))
	for i := range other {
		other[i] = true
	}
	check("other-app", other)
	if _, err := db.Exec("UPDATE mindlink_principals SET state='disabled'"); err != nil {
		t.Fatal(err)
	}
	check("fixture-app", want)
	if _, err := db.Exec("DELETE FROM groupmembers WHERE app_key='fixture-app' AND group_id='protected-group' AND member_id=?", disablePrincipal); err != nil {
		t.Fatal(err)
	}
	want[5] = true
	check("fixture-app", want)
	if _, err := db.Exec("INSERT INTO groupmembers(app_key,group_id,member_id) VALUES('fixture-app','protected-group',?)", disablePrincipal); err != nil {
		t.Fatal(err)
	}
	want[5] = false
	check("fixture-app", want)
	var journal, tickets int
	if db.QueryRow("SELECT COUNT(*) FROM mindlink_operations").Scan(&journal) != nil || db.QueryRow("SELECT COUNT(*) FROM mindlink_tickets").Scan(&tickets) != nil || journal != 1 || tickets != 0 {
		t.Fatal("classification changed original operation/ticket state")
	}
}

func TestNativeFilterTenThousandCandidatesHaveBoundedQueries(t *testing.T) {
	db := nativeFixture(t)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fence, err := NewNativeFence(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]NativeIdentities, 10000)
	want := make([]bool, len(rows))
	for i := range rows {
		rows[i] = NativeIdentities{Users: []string{fmt.Sprintf("user-%05d", i), "42"}, Groups: []string{fmt.Sprintf("group-%05d", i)}}
		want[i] = true
	}
	for _, i := range []int{7, 511, 9999} {
		rows[i].Users = append(rows[i].Users, strings.ToUpper(disablePrincipal))
		want[i] = false
	}
	for _, i := range []int{0, 257, 8765} {
		rows[i].Groups = append(rows[i].Groups, "PROTECTED-GROUP")
		want[i] = false
	}
	before := nativeFilterSelects(t, db)
	got, err := fence.Filter(context.Background(), "fixture-app", rows)
	after := nativeFilterSelects(t, db)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("large mixed classification lost input ordinals: %v", err)
	}
	if queries := after - before; queries > 100 || queries == 0 {
		t.Fatalf("10k candidates used %d SELECTs, expected bounded batch queries", queries)
	} else {
		t.Logf("10000 candidates, 20003 distinct raw identities, actual SELECTs=%d", queries)
	}
}

func nativeFilterSelects(t *testing.T, db *sql.DB) uint64 {
	t.Helper()
	var name string
	var count uint64
	if err := db.QueryRow("SHOW SESSION STATUS LIKE 'Com_select'").Scan(&name, &count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestNativeFilterErrorsDiscardEveryPartialMask(t *testing.T) {
	db := nativeFixture(t)
	fence, err := NewNativeFence(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	rows := []NativeIdentities{{Users: []string{"42"}}, {Users: []string{disablePrincipal}, Groups: []string{"protected-group"}}}
	for _, tc := range []struct {
		name string
		ctx  context.Context
		app  string
		want error
	}{{"nil_context", nil, "fixture-app", ErrInvalidRequest}, {"empty_app", context.Background(), "", ErrInvalidRequest}} {
		t.Run(tc.name, func(t *testing.T) {
			mask, err := fence.Filter(tc.ctx, tc.app, rows)
			if mask != nil || !errors.Is(err, tc.want) {
				t.Fatalf("invalid input returned a mask: %v, %v", mask, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if mask, err := fence.Filter(ctx, "fixture-app", rows); mask != nil || err == nil {
		t.Fatal("canceled batch returned successful/partial classification")
	}
	if mask, err := fence.Filter(ctx, "fixture-app", nil); mask != nil || err == nil {
		t.Fatal("empty batch bypassed cancellation")
	}
	if _, err := db.Exec("RENAME TABLE groupmembers TO _filter_unavailable_members"); err != nil {
		t.Fatal(err)
	}
	if mask, err := fence.Filter(context.Background(), "fixture-app", rows); mask != nil || !errors.Is(err, ErrUnknownResult) {
		t.Fatal("group SQL failure exposed the prior user classification", mask, err)
	}
	if mask, err := fence.Filter(context.Background(), "fixture-app", nil); mask != nil || !errors.Is(err, ErrUnknownResult) {
		t.Fatal("empty batch hid unavailable classification schema", mask, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if mask, err := fence.Filter(context.Background(), "fixture-app", rows); mask != nil || !errors.Is(err, ErrUnknownResult) {
		t.Fatal("closed SQL pool became a successful/partial classification", mask, err)
	}
	if mask, err := fence.Filter(context.Background(), "fixture-app", nil); mask != nil || !errors.Is(err, ErrUnknownResult) {
		t.Fatal("empty batch hid closed SQL pool", mask, err)
	}
}
