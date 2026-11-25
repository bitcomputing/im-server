package mindlink

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func stationStopProof(t *testing.T, change func(*StationStopRequest, *Claims)) (*Verifier, *Authorization) {
	t.Helper()
	trust, key := fixtureTrust()
	trust.MutationOperations = []string{OperationStationStopped, OperationMessageSend}
	trust.QueryOperations = []string{"operations.query", OperationHistoryRead}
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	var c Claims
	if json.Unmarshal(fixtureJSON(t, fixtureClaims("")), &c) != nil {
		t.Fatal("claims")
	}
	c.OperationType, c.Method, c.Path = OperationStationStopped, "POST", StationStoppedPath
	c.ActorKind, c.ActorID, c.AvatarGeneration = "system", trust.SystemActorID, 0
	c.TargetID, c.ExpectedPolicyVersion, c.DesiredPolicyVersion = strings.Repeat("c", 64), 1, 1
	c.IssuedAt, c.ExpiresAt = time.Now().Unix(), time.Now().Unix()+60
	c.JTI = strings.Repeat("1", 64)
	b := StationStopRequest{Version: StationStopVersion, ConversationID: c.TargetID, PolicyVersion: 1, EventID: strings.Repeat("d", 64), AccessID: "access-1", InstanceID: "instance-1", Epoch: 7, Reason: "tls_failed", Text: "基站已停止"}
	b.OperationID = stationStopOperationID(c, b.EventID)
	b.RequestID = b.OperationID
	c.OperationID, c.RequestID = b.OperationID, b.RequestID
	if change != nil {
		change(&b, &c)
	}
	b.RequestDigest, err = StationStopDigest(c, &b)
	if err != nil {
		t.Fatal(err)
	}
	c.RequestDigest = b.RequestDigest
	raw := fixtureJSON(t, b)
	c.BodySHA256 = fmt.Sprintf("%x", sha256.Sum256(raw))
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
	a, err := v.Verify(fixtureRequest(t, token, c.Method, c.Path, string(raw)), c.OperationType, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return v, a
}

func TestStationStoppedStrictSystemIntent(t *testing.T) {
	v, a := stationStopProof(t, nil)
	if _, err := v.ParseStationStopped(a); err != nil {
		t.Fatal(err)
	}
	if a.claims.RequestDigest != "0ebba6577e56acc0ddf5bd63537242c6f13636d1eda6b1dc81e73c1edf31889c" || a.claims.OperationID != "07ebea5931118478f6d5126b2ea39231664fc51d1d0ff05f90ecff71d7088f28" {
		t.Fatal("cross-language reference vector changed")
	}
	for _, reason := range []string{"auth_failed", "account_unavailable", "server_rejected", "tls_failed"} {
		v, a := stationStopProof(t, func(b *StationStopRequest, _ *Claims) { b.Reason = reason })
		if _, err := v.ParseStationStopped(a); err != nil {
			t.Fatal("permanent reason rejected", err)
		}
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(a.body, &fields) != nil {
		t.Fatal("body")
	}
	for name := range fields {
		for _, mode := range []string{"missing", "null"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				var copyFields map[string]json.RawMessage
				json.Unmarshal(a.body, &copyFields)
				if mode == "missing" {
					delete(copyFields, name)
				} else {
					copyFields[name] = json.RawMessage("null")
				}
				copyAuth := *a
				copyAuth.body = fixtureJSON(t, copyFields)
				copyAuth.claims.BodySHA256 = fmt.Sprintf("%x", sha256.Sum256(copyAuth.body))
				if _, err := v.ParseStationStopped(&copyAuth); err == nil {
					t.Fatal("non-exact request accepted")
				}
			})
		}
	}
	for _, change := range []func(*StationStopRequest, *Claims){
		func(b *StationStopRequest, c *Claims) { c.ActorKind, c.ActorID = "owner", c.OwnerID },
		func(b *StationStopRequest, c *Claims) { c.ActorKind, c.ActorID, c.AvatarGeneration = "avatar", "A", 1 },
		func(b *StationStopRequest, c *Claims) { b.Reason = "retry_later" },
		func(b *StationStopRequest, c *Claims) { b.Epoch = 0 },
		func(b *StationStopRequest, c *Claims) { b.Text = "" },
		func(b *StationStopRequest, c *Claims) { b.AccessID = "" },
		func(b *StationStopRequest, c *Claims) { b.InstanceID = "" },
		func(b *StationStopRequest, c *Claims) {
			b.OperationID = strings.Repeat("e", 64)
			b.RequestID = b.OperationID
			c.OperationID = b.OperationID
			c.RequestID = b.RequestID
		},
		func(b *StationStopRequest, c *Claims) { b.PolicyVersion = 2 },
	} {
		v, a := stationStopProof(t, change)
		if _, err := v.ParseStationStopped(a); err == nil {
			t.Fatal("invalid system intent accepted")
		}
	}
	copyAuth := *a
	copyAuth.body = append(append([]byte(nil), a.body...), ' ')
	if _, err := v.ParseStationStopped(&copyAuth); err == nil {
		t.Fatal("wire mutation accepted")
	}
	copyAuth = *a
	copyAuth.body = []byte(strings.Replace(string(a.body), "基站已停止", "changed", 1))
	copyAuth.claims.BodySHA256 = fmt.Sprintf("%x", sha256.Sum256(copyAuth.body))
	if _, err := v.ParseStationStopped(&copyAuth); err == nil {
		t.Fatal("opaque semantic digest accepted")
	}
}

func TestStationStopReceiptIsExactOriginal(t *testing.T) {
	v, a := stationStopProof(t, nil)
	c := a.claims
	n := MessageNativeBinding{ChannelType: 2, ConversationID: "native-group", SenderPrincipalID: "native-owner", TargetPrincipalID: "native-group"}
	r := StationStopResult{Version: StationStopVersion, OperationType: OperationStationStopped, OperationID: c.OperationID, RequestID: c.RequestID, RequestDigest: c.RequestDigest, BodySHA256: c.BodySHA256, Environment: c.Environment, TenantID: c.TenantID, AppKey: c.AppKey, OwnerID: c.OwnerID, ActorKind: c.ActorKind, ActorID: c.ActorID, ConversationID: c.TargetID, PolicyVersion: 1, EventID: strings.Repeat("d", 64), State: "persisted", MessageID: "message-1", MsgSeqNo: "1", MsgTime: "1000", NativeConversationID: n.ConversationID, SenderPrincipalID: n.SenderPrincipalID}
	raw := fixtureJSON(t, r)
	if _, err := v.validateStationStopResult(a, raw, n); err != nil {
		t.Fatal("exact receipt rejected", err)
	}
	var fields map[string]json.RawMessage
	json.Unmarshal(raw, &fields)
	for field := range fields {
		var changed map[string]json.RawMessage
		json.Unmarshal(raw, &changed)
		delete(changed, field)
		if _, err := v.validateStationStopResult(a, fixtureJSON(t, changed), n); err == nil {
			t.Fatal("missing receipt field accepted", field)
		}
	}
	for _, change := range []func(*StationStopResult){
		func(r *StationStopResult) { r.State = "delivered" },
		func(r *StationStopResult) { r.EventID = strings.Repeat("e", 64) },
		func(r *StationStopResult) { r.SenderPrincipalID = "another-owner" },
		func(r *StationStopResult) { r.NativeConversationID = "another-group" },
		func(r *StationStopResult) { r.MsgSeqNo = "01" },
		func(r *StationStopResult) { r.MsgTime = "0" },
		func(r *StationStopResult) { r.BodySHA256 = strings.Repeat("f", 64) },
	} {
		copyResult := r
		change(&copyResult)
		if _, err := v.validateStationStopResult(a, fixtureJSON(t, copyResult), n); err == nil {
			t.Fatal("changed original receipt accepted")
		}
	}
}

func TestStationStoppedDoesNotGrantGeneralSystemSend(t *testing.T) {
	v, a := messageProof(t, string(fixtureJSON(t, messageRequest())), OperationMessageSend, "POST", MessageSendPath, func(c map[string]any) { c["actor_kind"], c["actor_id"] = "system", "fixture-system" })
	if _, err := v.ParseMessageSend(a); err == nil {
		t.Fatal("system gained general messages.send")
	}
}

func TestStationHistoryV3PathUsesHistoryRead(t *testing.T) {
	v, a := stationStopProof(t, nil)
	c := a.claims
	c.OperationType, c.ActorKind, c.ActorID = OperationHistoryRead, "owner", c.OwnerID
	c.Method, c.Path = "GET", HistoryV3PathPrefix+c.TargetID+"/0/20"
	c.RequestDigest = fmt.Sprintf("%x", sha256.Sum256([]byte(c.Path)))
	a = &Authorization{verifier: v, claims: c}
	before, limit, err := v.parseHistory(a)
	if err != nil || before != 0 || limit != 20 {
		t.Fatalf("v3 history binding rejected: %v", err)
	}
	c.RequestDigest = fmt.Sprintf("%x", sha256.Sum256([]byte(HistoryPathPrefix+c.TargetID+"/0/20")))
	a.claims = c
	if _, _, err = v.parseHistory(a); err == nil {
		t.Fatal("old path digest authorized v3")
	}
}

func TestStationStoppedTrustDoesNotAcceptForgedSystemOrScope(t *testing.T) {
	v, a := stationStopProof(t, nil)
	_, key := fixtureTrust()
	for _, change := range []func(*Claims){
		func(c *Claims) { c.ActorID = "forged-system" },
		func(c *Claims) { c.AvatarGeneration = 1 },
		func(c *Claims) { c.Environment = "sta" },
		func(c *Claims) { c.TenantID = "another-tenant" },
		func(c *Claims) { c.AppKey = "another-app" },
	} {
		c := a.claims
		change(&c)
		token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
		if auth, err := v.Verify(fixtureRequest(t, token, "POST", StationStoppedPath, string(a.body)), OperationStationStopped, time.Now()); err == nil || auth != nil {
			t.Fatal("forged scope/system accepted")
		}
	}
	trust, _ := fixtureTrust()
	trust.MutationOperations = []string{OperationMessageSend}
	ungranted, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, a.claims))
	if _, err := ungranted.Verify(fixtureRequest(t, token, "POST", StationStoppedPath, string(a.body)), OperationStationStopped, time.Now()); err == nil {
		t.Fatal("unconfigured operation accepted")
	}
}

func TestStationHistoryCategoryNeedsTrustedPayloadFact(t *testing.T) {
	payload := []byte(`{"content":"ordinary","extra":{"category":"station_stopped"}}`)
	if got, err := stationHistoryCategory("mindlink_group", "owner", "owner", payload, nil); err != nil || got != "message" {
		t.Fatal("extra upgraded category")
	}
	hash := sha256.Sum256(payload)
	if got, err := stationHistoryCategory("mindlink_group", "owner", "owner", payload, hash[:]); err != nil || got != "station_stopped" {
		t.Fatal("trusted fact lost")
	}
	for _, tc := range []struct {
		kind, owner, sender string
		hash                []byte
	}{
		{"owner_avatar_dm", "owner", "owner", hash[:]},
		{"avatar_pair_group", "owner", "owner", hash[:]},
		{"mindlink_group", "owner", "avatar", hash[:]},
		{"mindlink_group", "owner", "owner", []byte("forged")},
	} {
		if _, err := stationHistoryCategory(tc.kind, tc.owner, tc.sender, payload, tc.hash); err == nil {
			t.Fatal("invalid trusted fact accepted")
		}
	}
	legacy, _ := json.Marshal(HistoryMessage{ID: "ordinary", Text: "hello"})
	if strings.Contains(string(legacy), "category") {
		t.Fatal("old history shape changed")
	}
}

func TestStationStopMigrationMatchesFreshSchema(t *testing.T) {
	migration, err := os.ReadFile("../dbcommons/sqls/20261122.sql")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := os.ReadFile("../../sql/imserver.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ReplaceAll(string(initial), "\r\n", "\n"), strings.ReplaceAll(string(migration), "\r\n", "\n")) || !strings.Contains(string(initial), "VALUES('jimdb_version','20261125')") {
		t.Fatal("fresh schema or migration registration differs")
	}
}
