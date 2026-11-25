package dbs

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"im-server/commons/pbdefines/pbobjs"
	"im-server/services/commonservices"
	groupdbs "im-server/services/group/dbs"
	hisdbs "im-server/services/historymsg/storages/dbs"
)

// Latest matches native history visibility using only the supplied transaction.
func (dao SeenDao) Latest(app, user, target, sub string, channel pbobjs.ChannelType) (int64, error) {
	return dao.latest(app, user, target, sub, channel, nil)
}

type VisiblePreview struct {
	MsgId    string
	MsgBody  []byte
	MsgSeqNo int64
}

func (dao SeenDao) LatestPreview(app, user, target, sub string, channel pbobjs.ChannelType) (VisiblePreview, error) {
	var row VisiblePreview
	_, err := dao.latest(app, user, target, sub, channel, &row)
	return row, err
}

func (dao SeenDao) latest(app, user, target, sub string, channel pbobjs.ChannelType, preview *VisiblePreview) (int64, error) {
	if dao.DB == nil {
		return 0, errors.New("transaction required for READ visibility")
	}
	conver := commonservices.GetConversationId(user, target, channel)
	var joinedAt int64
	if channel == pbobjs.ChannelType_Group {
		var member groupdbs.GroupMemberDao
		if err := dao.DB.Where("app_key=? AND group_id=? AND member_id=?", app, target, user).Take(&member).Error; err != nil {
			return 0, err
		}
		var groupSettings []groupdbs.GroupExtDao
		if err := dao.DB.Where("app_key=? AND group_id=? AND item_type=?", app, target, commonservices.AttItemType_Setting).Find(&groupSettings).Error; err != nil {
			return 0, err
		}
		var memberSettings []groupdbs.GroupMemberExtDao
		if err := dao.DB.Where("app_key=? AND group_id=? AND member_id=? AND item_type=?", app, target, user, commonservices.AttItemType_Setting).Find(&memberSettings).Error; err != nil {
			return 0, err
		}
		settings := map[string]string{}
		for _, item := range groupSettings {
			settings[item.ItemKey] = item.ItemValue
		}
		for _, item := range memberSettings {
			settings[item.ItemKey] = item.ItemValue
		}
		var policy commonservices.GrpMemberSettings
		commonservices.FillObjField(&policy, settings)
		if policy.HideGrpMsg {
			joinedAt = member.CreatedTime.UnixMilli()
		}
	}
	var boundary struct{ CleanTime int64 }
	cleanQuery := fmt.Sprintf(`SELECT GREATEST(?,
COALESCE((SELECT clean_time FROM %s WHERE app_key=? AND conver_id=? AND channel_type=? AND sub_channel=? LIMIT 1),0),
COALESCE((SELECT clean_time FROM %s WHERE app_key=? AND user_id=? AND target_id=? AND channel_type=? AND sub_channel=? LIMIT 1),0)) AS clean_time`,
		(hisdbs.HisMsgConverCleanTimeDao{}).TableName(), (hisdbs.HisMsgUserCleanTimeDao{}).TableName())
	if err := dao.DB.Raw(cleanQuery, joinedAt, app, conver, channel, sub, app, user, target, channel, sub).Scan(&boundary).Error; err != nil {
		return 0, err
	}
	// Keep deletion/effective-time and portion semantics identical to the native history DAOs.
	now := time.Now().UnixMilli()
	query := fmt.Sprintf(`SELECT his.msg_seq_no FROM %s AS his LEFT JOIN %s AS delhis
ON his.app_key=delhis.app_key AND delhis.user_id=? AND delhis.target_id=? AND his.sub_channel=delhis.sub_channel AND his.msg_id=delhis.msg_id
WHERE his.app_key=? AND his.conver_id=? AND his.sub_channel=?`, (hisdbs.PrivateHisMsgDao{}).TableName(), (hisdbs.PrivateDelHisMsgDao{}).TableName())
	params := []interface{}{user, target, app, conver, sub}
	if channel == pbobjs.ChannelType_Group {
		query = fmt.Sprintf(`SELECT his.msg_seq_no FROM %s AS his LEFT JOIN %s AS delhis
ON his.app_key=delhis.app_key AND delhis.user_id=? AND delhis.target_id=his.conver_id AND delhis.sub_channel=his.sub_channel AND his.msg_id=delhis.msg_id AND (delhis.effective_time=0 OR delhis.effective_time<?)
LEFT JOIN %s AS rel ON his.app_key=rel.app_key AND his.conver_id=rel.conver_id AND his.sub_channel=rel.sub_channel AND his.msg_id=rel.msg_id AND rel.user_id=?
WHERE his.app_key=? AND his.conver_id=? AND his.sub_channel=? AND (his.is_portion=0 OR rel.msg_id IS NOT NULL)`,
			(hisdbs.GroupHisMsgDao{}).TableName(), (hisdbs.GroupDelHisMsgDao{}).TableName(), (hisdbs.GroupPortionRelDao{}).TableName())
		params = []interface{}{user, now, user, app, conver, sub}
	}
	query += " AND his.send_time<? AND his.is_delete=0 AND (his.destroy_time=0 OR his.destroy_time>?) AND delhis.msg_id IS NULL"
	params = append(params, int64(math.MaxInt64), now)
	if boundary.CleanTime > 0 {
		query += " AND his.send_time>?"
		params = append(params, boundary.CleanTime)
	}
	if preview != nil {
		query = strings.Replace(query, "SELECT his.msg_seq_no FROM", "SELECT his.msg_seq_no,his.msg_id,his.msg_body FROM", 1)
	}
	query += " ORDER BY his.send_time DESC,his.msg_seq_no DESC LIMIT 1"
	row := preview
	if row == nil {
		row = &VisiblePreview{}
	}
	result := dao.DB.Raw(query, params...).Scan(row)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected > 0 && row.MsgSeqNo <= 0 {
		return 0, errors.New("visible history has no durable sequence")
	}
	return row.MsgSeqNo, nil
}
