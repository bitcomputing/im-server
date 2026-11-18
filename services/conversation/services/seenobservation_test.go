package services

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"im-server/commons/bases"
	"im-server/commons/errs"
	"im-server/commons/pbdefines/pbobjs"
)

func TestSeenStateJSON(t *testing.T) {
	state, ok := seenStateJSON(&pbobjs.ConverSeenState{LatestSeq: math.MaxInt64, HasNew: true})
	if !ok || state.LatestSeq != "9223372036854775807" || state.SeenSeq != "0" || !state.HasNew {
		t.Fatal("typed state lost int64 precision")
	}
	for _, invalid := range []*pbobjs.ConverSeenState{nil, {SeenSeq: -1}, {LatestSeq: -1}, {LatestSeq: 1, HasNew: false}} {
		if _, ok := seenStateJSON(invalid); ok {
			t.Fatal("invalid native state became a typed success")
		}
	}
}

func TestSeenObservationRequiresTransaction(t *testing.T) {
	if code, observation := QryConverSeenObservationWithTx(bases.CreateRpcCtx("app", "owner"), nil, &pbobjs.QryConverSeenReq{TargetId: "peer", ChannelType: pbobjs.ChannelType_Private}); code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || observation != nil {
		t.Fatal("query observation accepted a missing transaction")
	}
}

func TestSeenObservationMySQL(t *testing.T) {
	db, app, _ := consistentSeenFixture(t)
	ctx := bases.CreateRpcCtx(app, "owner")
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if code, observation := QryConverSeenObservationWithTx(ctx, tx, nil); code != errs.IMErrorCode_MSG_PARAM_ILLEGAL || observation != nil {
		t.Fatal("nil query produced an observation")
	}
	req := &pbobjs.QryConverSeenReq{TargetId: "peer", ChannelType: pbobjs.ChannelType_Private, SubChannel: "empty"}
	code, zero := QryConverSeenObservationWithTx(ctx, tx, req)
	if code != 0 || zero == nil {
		t.Fatal("valid empty scope unavailable")
	}
	encoded, err := json.Marshal(zero)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		State map[string]any `json:"state"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil || len(wire.State) != 3 || wire.State["seen_seq"] != "0" || wire.State["latest_seq"] != "0" || wire.State["has_new"] != false {
		t.Fatal("empty query omitted fields or used numeric zero")
	}
	var seenRows int64
	if err := tx.Raw("SELECT COUNT(*) FROM conversation_seen WHERE app_key=?", app).Scan(&seenRows).Error; err != nil || seenRows != 0 {
		t.Fatal("empty query wrote seen")
	}
	// A durable read position may exceed retained latest; seen is BIGINT even in the existing INT history schema.
	if err := tx.Exec("INSERT INTO conversation_seen(app_key,user_id,target_id,channel_type,sub_channel,seen_seq) VALUES(?,'owner','peer',1,'',?)", app, int64(math.MaxInt64)).Error; err != nil {
		t.Fatal(err)
	}
	req.SubChannel = ""
	before := time.Now().UTC().Truncate(time.Millisecond)
	code, observed := QryConverSeenObservationWithTx(ctx, tx, req)
	after := time.Now().UTC()
	if code != 0 || observed == nil || observed.State.SeenSeq != "9223372036854775807" || observed.State.LatestSeq != "3" || observed.State.HasNew {
		t.Fatal("observation lost int64 precision or retained seen/latest semantics")
	}
	if observed.Coordinates.AppKey != app || observed.Coordinates.RequesterId != "owner" || observed.Coordinates.TargetId != "peer" || observed.Coordinates.ChannelType != pbobjs.ChannelType_Private || observed.Coordinates.SubChannel != "" {
		t.Fatal("observation is not bound to actual original native coordinates")
	}
	when, err := time.Parse("2006-01-02T15:04:05.000Z", observed.StateObservedAt)
	if err != nil || len(observed.StateObservedAt) != 24 || when.Before(before) || when.After(after) {
		t.Fatal("observation time is not its original UTC millisecond read time")
	}
	original, err := json.Marshal(observed)
	if err != nil || strings.Contains(string(original), app) || strings.Contains(string(original), "requester") || strings.Contains(string(original), "coordinates") {
		t.Fatal("native internal coordinates leaked into public state JSON")
	}
	req.TargetId, req.ChannelType = "group", pbobjs.ChannelType_Group
	if code, newer := QryConverSeenObservationWithTx(ctx, tx, req); code != 0 || newer == nil || newer.Coordinates.TargetId != "group" || newer.State.LatestSeq != "3" {
		t.Fatal("independent query did not produce its own state/coordinates")
	}
	unchanged, _ := json.Marshal(observed)
	if !bytes.Equal(original, unchanged) || observed.Coordinates.TargetId != "peer" {
		t.Fatal("later query/request mutation overwrote original observation")
	}
	if err := tx.Raw("SELECT COUNT(*) FROM conversation_seen WHERE app_key=?", app).Scan(&seenRows).Error; err != nil || seenRows != 1 {
		t.Fatal("query observation created or removed a seen row")
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if code, observation := QryConverSeenObservationWithTx(ctx, tx, req); code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || observation != nil {
		t.Fatal("finished transaction produced a successful observation")
	}
	t.Log("explicit zero fields; MaxInt64 string preserved; original native coordinates/time stable; query created no seen row")
}

func TestSeenObservationRejectsInvalidStorageStateMySQL(t *testing.T) {
	db, app, _ := consistentSeenFixture(t)
	tx := db.Begin()
	defer tx.Rollback()
	if err := tx.Exec("INSERT INTO conversation_seen(app_key,user_id,target_id,channel_type,sub_channel,seen_seq) VALUES(?,'owner','peer',1,'',-1)", app).Error; err != nil {
		t.Fatal(err)
	}
	if code, observation := QryConverSeenObservationWithTx(bases.CreateRpcCtx(app, "owner"), tx, &pbobjs.QryConverSeenReq{TargetId: "peer", ChannelType: pbobjs.ChannelType_Private}); code != errs.IMErrorCode_API_INTERNAL_RESP_FAIL || observation != nil {
		t.Fatal("negative stored sequence became a valid wire observation")
	}
	if err := tx.Exec("SELECT 1").Error; err == nil {
		t.Fatal("invalid typed state did not abort caller transaction")
	}
}
