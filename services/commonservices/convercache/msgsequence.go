package convercache

import (
	"context"
	"errors"
	"time"

	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/messagesequence"
	"im-server/commons/mongocommons"
	"im-server/commons/pbdefines/pbobjs"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

type sequenceWatermark = messagesequence.Watermark

func reserveSequence(app, conver, sub string, channel pbobjs.ChannelType, cached sequenceWatermark, now int64) (sequenceWatermark, error) {
	var next sequenceWatermark
	if dbcommons.GetDb() == nil || app == "" || conver == "" {
		return next, errors.New("message sequence storage or scope unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := dbcommons.GetDb().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		next, err = messagesequence.Reserve(ctx, tx.Statement.ConnPool, app, conver, sub, int32(channel), cached, now, func() (sequenceWatermark, error) { return sequenceFloor(ctx, tx, app, conver, sub, channel) })
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
		return messagesequence.SQLFloor(ctx, tx.Statement.ConnPool, app, conver, sub, int32(channel))
	}
	return messagesequence.WithReadFloor(ctx, tx.Statement.ConnPool, app, conver, sub, int32(channel), floor)
}
