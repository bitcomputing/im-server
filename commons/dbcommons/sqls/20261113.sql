CREATE TABLE IF NOT EXISTS `mindlink_conversations` (
  `environment` VARBINARY(6) NOT NULL,
  `tenant_id` VARBINARY(128) NOT NULL,
  `appkey` VARBINARY(20) NOT NULL,
  `owner_id` VARBINARY(20) NOT NULL,
  `conversation_id` BINARY(64) NOT NULL,
  `kind` VARBINARY(24) NOT NULL,
  `channel_type` TINYINT NOT NULL,
  `native_target_id` VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `owner_im_id` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `native_appkey` VARCHAR(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci GENERATED ALWAYS AS (CONVERT(`appkey` USING utf8mb4)) STORED,
  `native_group_id` VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci GENERATED ALWAYS AS (IF(`channel_type`=2,`native_target_id`,NULL)) STORED,
  `policy_version` BIGINT NOT NULL,
  `state` VARBINARY(8) NOT NULL,
  `members_json` MEDIUMBLOB NOT NULL,
  PRIMARY KEY (`appkey`,`conversation_id`),
  UNIQUE KEY `uniq_mindlink_native_conversation` (`native_appkey`,`channel_type`,`native_target_id`,`owner_im_id`),
  UNIQUE KEY `uniq_mindlink_native_group` (`native_appkey`,`native_group_id`),
  CONSTRAINT `chk_mindlink_conversation_policy` CHECK (`policy_version` > 0),
  CONSTRAINT `chk_mindlink_conversation_state` CHECK (`state` IN ('active','disabled')),
  CONSTRAINT `chk_mindlink_conversation_kind` CHECK (
    (`kind`='owner_avatar_dm' AND `channel_type`=1) OR
    (`kind` IN ('mindlink_group','avatar_pair_group') AND `channel_type`=2))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
