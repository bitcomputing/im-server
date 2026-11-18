package dbs

import (
	"context"
	"im-server/commons/dbcommons"
	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/conversation/storages/models"
)

type GlobalConverDao struct {
	ID          int64  `gorm:"primary_key"`
	ConverId    string `gorm:"conver_id"`
	SenderId    string `gorm:"sender_id"`
	TargetId    string `gorm:"target_id"`
	ChannelType int    `gorm:"channel_type"`
	SubChannel  string `gorm:"sub_channel"`
	UpdatedTime int64  `gorm:"updated_time"`
	AppKey      string `gorm:"app_key"`
}

func (conver *GlobalConverDao) TableName() string {
	return "globalconvers"
}

func (conver *GlobalConverDao) UpsertConversation(item models.GlobalConver) error {
	err := dbcommons.GetDb().Exec("INSERT INTO globalconvers(conver_id,sender_id,target_id,channel_type,sub_channel,updated_time,app_key)VALUES(?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE sender_id=?, target_id=?, updated_time=?",
		item.ConverId, item.SenderId, item.TargetId, item.ChannelType, item.SubChannel, item.UpdatedTime, item.AppKey, item.SenderId, item.TargetId, item.UpdatedTime).Error
	return err
}

func (conver *GlobalConverDao) QryConversations(ctx context.Context, appkey, targetId, subChannel string, channelType pbobjs.ChannelType, startTime int64, count int32, isPositiveOder bool, excludeUserIds []string) ([]*models.GlobalConver, error) {
	var items []*GlobalConverDao
	params := []interface{}{}
	condition := "app_key=?"
	params = append(params, appkey)
	if channelType != pbobjs.ChannelType_Unknown {
		condition = condition + " and channel_type=?"
		params = append(params, int(channelType))
	}
	if targetId != "" {
		if channelType == pbobjs.ChannelType_Private {
			condition = condition + " and (target_id=? or sender_id=?)"
			params = append(params, targetId)
			params = append(params, targetId)
		} else if channelType == pbobjs.ChannelType_Group {
			condition = condition + " and target_id=?"
			params = append(params, targetId)
		}
	}
	if subChannel != "" {
		condition = condition + " and sub_channel=?"
		params = append(params, subChannel)
	}
	if len(excludeUserIds) > 0 {
		condition = condition + " and sender_id not in (?)"
		params = append(params, excludeUserIds)
	}
	orderStr := "updated_time desc"
	if isPositiveOder {
		condition = condition + " and updated_time>?"
		params = append(params, startTime)
		orderStr = "updated_time asc"
	} else {
		condition = condition + " and updated_time<?"
		params = append(params, startTime)
	}
	// Match the verified native fence's collation, including aliases, before SQL pagination.
	const app = "CONVERT(globalconvers.app_key USING utf8mb4) COLLATE utf8mb4_0900_ai_ci"
	const sender = "CONVERT(globalconvers.sender_id USING utf8mb4) COLLATE utf8mb4_0900_ai_ci"
	const target = "CONVERT(globalconvers.target_id USING utf8mb4) COLLATE utf8mb4_0900_ai_ci"
	query := dbcommons.GetDb().WithContext(ctx).Where(condition, params...)
	query = query.Where("NOT EXISTS (SELECT 1 FROM mindlink_principals p WHERE p.native_appkey=" + app + " AND p.native_principal_id=" + sender + ")")
	query = query.Where("(globalconvers.channel_type NOT IN (?,?) OR NOT EXISTS (SELECT 1 FROM mindlink_principals p WHERE p.native_appkey="+app+" AND p.native_principal_id="+target+"))", int(pbobjs.ChannelType_Private), int(pbobjs.ChannelType_System))
	query = query.Where("(globalconvers.channel_type<>? OR NOT EXISTS (SELECT 1 FROM groupmembers m JOIN mindlink_principals p ON p.native_appkey=m.app_key AND p.native_principal_id=m.member_id WHERE m.app_key="+app+" AND m.group_id="+target+"))", int(pbobjs.ChannelType_Group))
	err := query.Order(orderStr).Limit(int(count)).Find(&items).Error
	if err != nil {
		return []*models.GlobalConver{}, err
	}
	covners := []*models.GlobalConver{}
	for _, item := range items {
		covners = append(covners, &models.GlobalConver{
			Id:          item.ID,
			ConverId:    item.ConverId,
			SenderId:    item.SenderId,
			TargetId:    item.TargetId,
			ChannelType: pbobjs.ChannelType(item.ChannelType),
			SubChannel:  item.SubChannel,
			UpdatedTime: item.UpdatedTime,
			AppKey:      item.AppKey,
		})
	}
	return covners, nil
}
