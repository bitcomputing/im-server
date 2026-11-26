package services

import (
	"context"
	"database/sql"
	"strconv"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"im-server/commons/bases"
	"im-server/commons/errs"
	"im-server/commons/mindlink"
	"im-server/commons/pbdefines/pbobjs"
	conversation "im-server/services/conversation/services"
	"im-server/services/conversation/storages/dbs"
)

func nativeMindlinkRead(ctx context.Context, tx *sql.Tx, b mindlink.ReadBinding, position *int64) (mindlink.ReadState, error) {
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: tx, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return mindlink.ReadState{}, mindlink.ErrUnknownResult
	}
	nativeCtx := bases.CreateRpcCtx(b.AppKey, b.RequesterID)
	var state *pbobjs.ConverSeenState
	var code errs.IMErrorCode
	if position == nil {
		code, state = conversation.QryConverSeenWithTx(nativeCtx, db.WithContext(ctx), &pbobjs.QryConverSeenReq{TargetId: b.TargetID, ChannelType: pbobjs.ChannelType(b.ChannelType)})
	} else {
		code, state = conversation.SetConverSeenWithTx(nativeCtx, db.WithContext(ctx), &pbobjs.SetConverSeenReq{TargetId: b.TargetID, ChannelType: pbobjs.ChannelType(b.ChannelType), SeenSeq: *position})
	}
	if code != errs.IMErrorCode_SUCCESS || state == nil {
		if code == errs.IMErrorCode_GROUP_NOTGROUPMEMBER {
			return mindlink.ReadState{}, mindlink.ErrForbidden
		}
		if code == errs.IMErrorCode_API_INTERNAL_RESP_FAIL {
			return mindlink.ReadState{}, mindlink.ErrUnknownResult
		}
		if code == errs.IMErrorCode_MSG_PARAM_ILLEGAL {
			return mindlink.ReadState{}, mindlink.ErrInvalidRequest
		}
		return mindlink.ReadState{}, mindlink.ErrNotReady
	}
	if position != nil && b.Kind != "avatar_pair_group" {
		channel := pbobjs.ChannelType(b.ChannelType)
		index, err := (dbs.SeenDao{DB: db.WithContext(ctx)}).OwnerReadIndex(b.AppKey, b.RequesterID, b.TargetID, channel, state.SeenSeq)
		if err != nil {
			return mindlink.ReadState{}, err
		}
		if index > 0 {
			_, err = tx.ExecContext(ctx, "UPDATE conversations SET latest_read_msg_index=GREATEST(latest_read_msg_index,LEAST(?,latest_unread_msg_index)) WHERE app_key=? AND user_id=? AND target_id=? AND channel_type=? AND sub_channel=''", index, b.AppKey, b.RequesterID, b.TargetID, channel)
			if err != nil {
				return mindlink.ReadState{}, err
			}
		}
	}
	return mindlink.ReadState{SeenSeq: strconv.FormatInt(state.SeenSeq, 10), LatestSeq: strconv.FormatInt(state.LatestSeq, 10), HasNew: state.HasNew}, nil
}
