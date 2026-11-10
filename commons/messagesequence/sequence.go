package messagesequence

import (
	"context"
	"database/sql"
	"errors"
	"math"
)

type Watermark struct {
	Seq      int64 `bson:"seq"`
	SendTime int64 `bson:"send_time"`
}

// Transaction is the caller's open SQL transaction; Reserve never commits it.
type Transaction interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func Reserve(ctx context.Context, tx Transaction, app, conver, sub string, channel int32, cached Watermark, now int64, floor func() (Watermark, error)) (Watermark, error) {
	empty := Watermark{}
	if tx == nil || app == "" || conver == "" || (channel != 1 && channel != 2) || cached.Seq < 0 || cached.SendTime < 0 || now < 0 || floor == nil {
		return empty, errors.New("invalid message sequence scope")
	}
	key := []any{app, conver, channel, sub}
	if _, err := tx.ExecContext(ctx, "INSERT INTO message_sequence (app_key,conver_id,channel_type,sub_channel,seq,send_time) VALUES (?,?,?,?,0,0) ON DUPLICATE KEY UPDATE seq=seq", key...); err != nil {
		return empty, err
	}
	var current Watermark
	if err := tx.QueryRowContext(ctx, "SELECT seq,send_time FROM message_sequence WHERE app_key=? AND conver_id=? AND channel_type=? AND sub_channel=? FOR UPDATE", key...).Scan(&current.Seq, &current.SendTime); err != nil {
		return empty, err
	}
	if current.Seq < 0 || current.SendTime < 0 {
		return empty, errors.New("invalid message sequence watermark")
	}
	if current.Seq == 0 {
		prior, err := floor()
		if err != nil {
			return empty, err
		}
		if prior.Seq < 0 || prior.SendTime < 0 {
			return empty, errors.New("invalid legacy message sequence floor")
		}
		current.Seq = max(current.Seq, prior.Seq)
		current.SendTime = max(current.SendTime, prior.SendTime)
	}
	current.Seq = max(current.Seq, cached.Seq)
	current.SendTime = max(current.SendTime, cached.SendTime)
	if current.Seq == math.MaxInt64 || current.SendTime == math.MaxInt64 {
		return empty, errors.New("message sequence or time exhausted")
	}
	next := Watermark{Seq: current.Seq + 1, SendTime: max(now, current.SendTime+1)}
	result, err := tx.ExecContext(ctx, "UPDATE message_sequence SET seq=?,send_time=? WHERE app_key=? AND conver_id=? AND channel_type=? AND sub_channel=?", append([]any{next.Seq, next.SendTime}, key...)...)
	if err != nil {
		return empty, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return empty, err
	}
	if n != 1 {
		return empty, errors.New("message sequence reservation lost")
	}
	return next, nil
}

func SQLFloor(ctx context.Context, tx Transaction, app, conver, sub string, channel int32) (Watermark, error) {
	var floor Watermark
	table := "p_hismsgs"
	if channel == 2 {
		table = "g_hismsgs"
	} else if channel != 1 {
		return floor, errors.New("unsupported durable sequence channel")
	}
	err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(msg_seq_no),0),COALESCE(MAX(send_time),0) FROM "+table+" WHERE app_key=? AND conver_id=? AND sub_channel=?", app, conver, sub).Scan(&floor.Seq, &floor.SendTime)
	if err != nil {
		return Watermark{}, err
	}
	return WithReadFloor(ctx, tx, app, conver, sub, channel, floor)
}

// WithReadFloor also covers read positions that outlive retained SQL/Mongo history.
func WithReadFloor(ctx context.Context, tx Transaction, app, conver, sub string, channel int32, floor Watermark) (Watermark, error) {
	condition := "target_id=?"
	if channel == 1 {
		condition = "CONCAT(LEAST(BINARY user_id,BINARY target_id),':',GREATEST(BINARY user_id,BINARY target_id))=?"
	} else if channel != 2 {
		return Watermark{}, errors.New("unsupported durable sequence channel")
	}
	var seen int64
	err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(seen_seq),0) FROM conversation_seen WHERE app_key=? AND channel_type=? AND sub_channel=? AND "+condition, app, channel, sub, conver).Scan(&seen)
	if err != nil {
		return Watermark{}, err
	}
	if floor.Seq < 0 || floor.SendTime < 0 || seen < 0 {
		return Watermark{}, errors.New("invalid legacy message sequence floor")
	}
	floor.Seq = max(floor.Seq, seen)
	return floor, nil
}
