package dbs

import (
	"im-server/commons/dbcommons"
	"im-server/commons/pbdefines/pbobjs"

	"gorm.io/gorm"
)

type SeenDao struct{ DB *gorm.DB }

func (dao SeenDao) database() *gorm.DB {
	if dao.DB != nil {
		return dao.DB
	}
	return dbcommons.GetDb()
}

func (dao SeenDao) Find(appkey, userId, targetId, subChannel string, channelType pbobjs.ChannelType) (int64, error) {
	var row struct{ SeenSeq int64 }
	query := "SELECT seen_seq FROM conversation_seen WHERE app_key=? AND user_id=? AND target_id=? AND sub_channel=? AND channel_type=?"
	if dao.DB != nil {
		// A caller's earlier repeatable-read snapshot must not hide a newer committed max.
		query += " FOR UPDATE"
	}
	err := dao.database().Raw(query, appkey, userId, targetId, subChannel, channelType).Scan(&row).Error
	return row.SeenSeq, err
}

func (dao SeenDao) Advance(appkey, userId, targetId, subChannel string, channelType pbobjs.ChannelType, seq int64) error {
	return dao.database().Exec("INSERT INTO conversation_seen (app_key,user_id,target_id,sub_channel,channel_type,seen_seq) VALUES (?,?,?,?,?,?) ON DUPLICATE KEY UPDATE seen_seq=GREATEST(seen_seq,VALUES(seen_seq))", appkey, userId, targetId, subChannel, channelType, seq).Error
}
