package dbs

import (
	"im-server/commons/dbcommons"
	"im-server/commons/pbdefines/pbobjs"
)

type SeenDao struct{}

func (SeenDao) Find(appkey, userId, targetId, subChannel string, channelType pbobjs.ChannelType) (int64, error) {
	var row struct{ SeenSeq int64 }
	err := dbcommons.GetDb().Raw("SELECT seen_seq FROM conversation_seen WHERE app_key=? AND user_id=? AND target_id=? AND sub_channel=? AND channel_type=?", appkey, userId, targetId, subChannel, channelType).Scan(&row).Error
	return row.SeenSeq, err
}

func (SeenDao) Advance(appkey, userId, targetId, subChannel string, channelType pbobjs.ChannelType, seq int64) error {
	return dbcommons.GetDb().Exec("INSERT INTO conversation_seen (app_key,user_id,target_id,sub_channel,channel_type,seen_seq) VALUES (?,?,?,?,?,?) ON DUPLICATE KEY UPDATE seen_seq=GREATEST(seen_seq,VALUES(seen_seq))", appkey, userId, targetId, subChannel, channelType, seq).Error
}
