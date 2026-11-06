package convercache

import (
	"context"
	"errors"
	"math"
	"time"

	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/mongocommons"
	"im-server/commons/pbdefines/pbobjs"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

type sequenceWatermark struct {
	Seq      int64 `bson:"seq"`
	SendTime int64 `bson:"send_time"`
}

func reserveSequence(app, conver, sub string, channel pbobjs.ChannelType, cached sequenceWatermark, now int64) (sequenceWatermark, error) {
	var next sequenceWatermark
	if dbcommons.GetDb() == nil || app == "" || conver == "" {
		return next, errors.New("message sequence storage or scope unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := dbcommons.GetDb().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		key := []interface{}{app, conver, channel, sub}
		if err := tx.Exec("INSERT INTO message_sequence (app_key,conver_id,channel_type,sub_channel,seq,send_time) VALUES (?,?,?,?,0,0) ON DUPLICATE KEY UPDATE seq=seq", key...).Error; err != nil {
			return err
		}
		var current sequenceWatermark
		row := tx.Raw("SELECT seq,send_time FROM message_sequence WHERE app_key=? AND conver_id=? AND channel_type=? AND sub_channel=? FOR UPDATE", key...).Scan(&current)
		if row.Error != nil {
			return row.Error
		}
		if row.RowsAffected != 1 || current.Seq < 0 || current.SendTime < 0 {
			return errors.New("invalid message sequence watermark")
		}
		if current.Seq == 0 {
			floor, err := sequenceFloor(ctx, tx, app, conver, sub, channel)
			if err != nil {
				return err
			}
			current.Seq = max(current.Seq, floor.Seq)
			current.SendTime = max(current.SendTime, floor.SendTime)
		}
		current.Seq = max(current.Seq, cached.Seq)
		current.SendTime = max(current.SendTime, cached.SendTime)
		if current.Seq == math.MaxInt64 || current.SendTime == math.MaxInt64 || now < 0 {
			return errors.New("message sequence or time exhausted")
		}
		next = sequenceWatermark{Seq: current.Seq + 1, SendTime: max(now, current.SendTime+1)}
		args := append([]interface{}{next.Seq, next.SendTime}, key...)
		result := tx.Exec("UPDATE message_sequence SET seq=?,send_time=? WHERE app_key=? AND conver_id=? AND channel_type=? AND sub_channel=?", args...)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("message sequence reservation lost")
		}
		return nil
	})
	if err != nil {
		return sequenceWatermark{}, err
	}
	return next, nil
}

func sequenceFloor(ctx context.Context, tx *gorm.DB, app, conver, sub string, channel pbobjs.ChannelType) (sequenceWatermark, error) {
	var floor sequenceWatermark
	table := "p_hismsgs"
	if channel == pbobjs.ChannelType_Group {
		table = "g_hismsgs"
	} else if channel != pbobjs.ChannelType_Private {
		return floor, errors.New("unsupported durable sequence channel")
	}
	if configures.Config.MsgStoreEngine == configures.MsgStoreEngine_Mongo {
		// shortcut: seed Mongo history before startup if physical Mongo retention is enabled.
		collection := mongocommons.GetCollection(table)
		if collection == nil {
			return floor, errors.New("message history storage unavailable")
		}
		pipeline := mongo.Pipeline{
			bson.D{{Key: "$match", Value: bson.M{"app_key": app, "conver_id": conver, "sub_channel": sub}}},
			bson.D{{Key: "$group", Value: bson.M{"_id": nil, "seq": bson.M{"$max": "$msg_seq_no"}, "send_time": bson.M{"$max": "$send_time"}}}},
		}
		cursor, err := collection.Aggregate(ctx, pipeline)
		if err != nil {
			return floor, err
		}
		defer cursor.Close(ctx)
		var rows []sequenceWatermark
		if err := cursor.All(ctx, &rows); err != nil {
			return floor, err
		}
		if len(rows) > 0 {
			floor = rows[0]
		}
	} else {
		if err := tx.Raw("SELECT COALESCE(MAX(msg_seq_no),0) AS seq, COALESCE(MAX(send_time),0) AS send_time FROM "+table+" WHERE app_key=? AND conver_id=? AND sub_channel=?", app, conver, sub).Scan(&floor).Error; err != nil {
			return floor, err
		}
	}
	// READ may already outlive all retained history when this capability is installed.
	condition := "target_id=?"
	if channel == pbobjs.ChannelType_Private {
		condition = "CONCAT(LEAST(BINARY user_id,BINARY target_id),':',GREATEST(BINARY user_id,BINARY target_id))=?"
	}
	var seen int64
	if err := tx.Raw("SELECT COALESCE(MAX(seen_seq),0) FROM conversation_seen WHERE app_key=? AND channel_type=? AND sub_channel=? AND "+condition, app, channel, sub, conver).Scan(&seen).Error; err != nil {
		return floor, err
	}
	if floor.Seq < 0 || floor.SendTime < 0 || seen < 0 {
		return floor, errors.New("invalid legacy message sequence floor")
	}
	floor.Seq = max(floor.Seq, seen)
	return floor, nil
}
