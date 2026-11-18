package convercache

import (
	"context"
	"errors"
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
		value, err := dbcommons.ReserveMessageSequenceWithTx(ctx, tx, app, conver, sub, int32(channel), dbcommons.MessageSequenceWatermark{Seq: cached.Seq, SendTime: cached.SendTime}, now, func() (dbcommons.MessageSequenceWatermark, error) {
			floor, err := sequenceFloor(ctx, tx, app, conver, sub, channel)
			return dbcommons.MessageSequenceWatermark{Seq: floor.Seq, SendTime: floor.SendTime}, err
		})
		next = sequenceWatermark{Seq: value.Seq, SendTime: value.SendTime}
		return err
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
