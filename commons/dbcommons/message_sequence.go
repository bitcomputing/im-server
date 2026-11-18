package dbcommons

import (
	"context"
	"errors"
	"math"

	"gorm.io/gorm"
)

type MessageSequenceWatermark struct {
	Seq, SendTime int64
}

// ReserveMessageSequenceWithTx joins the existing journal/native transaction.
// loadFloor runs only for an uninitialized watermark, while its row is locked.
func ReserveMessageSequenceWithTx(ctx context.Context, tx *gorm.DB, app, conver, sub string, channel int32, cached MessageSequenceWatermark, now int64, loadFloor func() (MessageSequenceWatermark, error)) (MessageSequenceWatermark, error) {
	var zero MessageSequenceWatermark
	if ctx == nil || tx == nil || tx.Statement == nil || app == "" || conver == "" || now < 0 {
		return zero, errors.New("message sequence transaction or scope unavailable")
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return zero, errors.New("message sequence requires the caller transaction")
	}
	tx = tx.WithContext(ctx)
	key := []any{app, conver, channel, sub}
	if err := tx.Exec("INSERT INTO message_sequence (app_key,conver_id,channel_type,sub_channel,seq,send_time) VALUES (?,?,?,?,0,0) ON DUPLICATE KEY UPDATE seq=seq", key...).Error; err != nil {
		return zero, err
	}
	var current MessageSequenceWatermark
	row := tx.Raw("SELECT seq,send_time FROM message_sequence WHERE app_key=? AND conver_id=? AND channel_type=? AND sub_channel=? FOR UPDATE", key...).Scan(&current)
	if row.Error != nil {
		return zero, row.Error
	}
	if row.RowsAffected != 1 || current.Seq < 0 || current.SendTime < 0 {
		return zero, errors.New("invalid message sequence watermark")
	}
	if current.Seq == 0 {
		if loadFloor == nil {
			return zero, errors.New("message sequence floor unavailable")
		}
		floor, err := loadFloor()
		if err != nil {
			return zero, err
		}
		if floor.Seq < 0 || floor.SendTime < 0 {
			return zero, errors.New("invalid message sequence floor")
		}
		current.Seq, current.SendTime = max(current.Seq, floor.Seq), max(current.SendTime, floor.SendTime)
	}
	current.Seq, current.SendTime = max(current.Seq, cached.Seq), max(current.SendTime, cached.SendTime)
	if current.Seq == math.MaxInt64 || current.SendTime == math.MaxInt64 {
		return zero, errors.New("message sequence or time exhausted")
	}
	next := MessageSequenceWatermark{current.Seq + 1, max(now, current.SendTime+1)}
	args := append([]any{next.Seq, next.SendTime}, key...)
	result := tx.Exec("UPDATE message_sequence SET seq=?,send_time=? WHERE app_key=? AND conver_id=? AND channel_type=? AND sub_channel=?", args...)
	if result.Error != nil {
		return zero, result.Error
	}
	if result.RowsAffected != 1 {
		return zero, errors.New("message sequence reservation lost")
	}
	return next, nil
}

// MySQLMessageSequenceFloor reads native history and independent seen watermarks.
func MySQLMessageSequenceFloor(ctx context.Context, tx *gorm.DB, app, conver, sub string, channel int32) (MessageSequenceWatermark, error) {
	var floor MessageSequenceWatermark
	table := "p_hismsgs"
	if channel == 2 {
		table = "g_hismsgs"
	} else if channel != 1 {
		return floor, errors.New("unsupported durable sequence channel")
	}
	if err := tx.WithContext(ctx).Raw("SELECT COALESCE(MAX(msg_seq_no),0) AS seq,COALESCE(MAX(send_time),0) AS send_time FROM "+table+" WHERE app_key=? AND conver_id=? AND sub_channel=?", app, conver, sub).Scan(&floor).Error; err != nil {
		return floor, err
	}
	condition := "target_id=?"
	if channel == 1 {
		condition = "CONCAT(LEAST(BINARY user_id,BINARY target_id),':',GREATEST(BINARY user_id,BINARY target_id))=?"
	}
	var seen int64
	if err := tx.WithContext(ctx).Raw("SELECT COALESCE(MAX(seen_seq),0) FROM conversation_seen WHERE app_key=? AND channel_type=? AND sub_channel=? AND "+condition, app, channel, sub, conver).Scan(&seen).Error; err != nil {
		return floor, err
	}
	if floor.Seq < 0 || floor.SendTime < 0 || seen < 0 {
		return floor, errors.New("invalid legacy message sequence floor")
	}
	floor.Seq = max(floor.Seq, seen)
	return floor, nil
}
