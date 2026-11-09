package services

import (
	"context"
	"strconv"
	"time"

	"im-server/commons/bases"
	"im-server/commons/errs"
	"im-server/commons/pbdefines/pbobjs"

	"gorm.io/gorm"
)

type SeenState struct {
	SeenSeq   string `json:"seen_seq"`
	LatestSeq string `json:"latest_seq"`
	HasNew    bool   `json:"has_new"`
}

type ConverSeenCoordinates struct {
	AppKey, RequesterId, TargetId, SubChannel string
	ChannelType                               pbobjs.ChannelType
}

type ConverSeenObservation struct {
	// Native identities bind the original query internally; they are not public page fields.
	Coordinates     ConverSeenCoordinates `json:"-"`
	State           SeenState             `json:"state"`
	StateObservedAt string                `json:"state_observed_at"`
}

// QryConverSeenObservationWithTx produces a native observation, not a committed public operation receipt.
// The caller must bind its original query, persist this state/time once, and commit before reporting success.
func QryConverSeenObservationWithTx(ctx context.Context, tx *gorm.DB, req *pbobjs.QryConverSeenReq) (errs.IMErrorCode, *ConverSeenObservation) {
	if req == nil {
		code, _ := QryConverSeenWithTx(ctx, tx, nil)
		return code, nil
	}
	query := &pbobjs.QryConverSeenReq{TargetId: req.TargetId, ChannelType: req.ChannelType, SubChannel: req.SubChannel}
	code, state := QryConverSeenWithTx(ctx, tx, query)
	if code != errs.IMErrorCode_SUCCESS {
		return code, nil
	}
	jsonState, ok := seenStateJSON(state)
	if !ok {
		tx.Rollback()
		return errs.IMErrorCode_API_INTERNAL_RESP_FAIL, nil
	}
	return errs.IMErrorCode_SUCCESS, &ConverSeenObservation{
		Coordinates: ConverSeenCoordinates{
			AppKey: bases.GetAppKeyFromCtx(ctx), RequesterId: bases.GetRequesterIdFromCtx(ctx),
			TargetId: query.TargetId, SubChannel: query.SubChannel, ChannelType: query.ChannelType,
		},
		State:           jsonState,
		StateObservedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	}
}

func seenStateJSON(state *pbobjs.ConverSeenState) (SeenState, bool) {
	if state == nil || state.SeenSeq < 0 || state.LatestSeq < 0 || state.HasNew != (state.LatestSeq > state.SeenSeq) {
		return SeenState{}, false
	}
	return SeenState{
		SeenSeq: strconv.FormatInt(state.SeenSeq, 10), LatestSeq: strconv.FormatInt(state.LatestSeq, 10), HasNew: state.HasNew,
	}, true
}
