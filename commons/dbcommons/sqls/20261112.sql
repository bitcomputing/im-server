-- Private reliable-send projection. Provisioning is a separate trusted writer;
-- neither a signed send nor a business retirement snapshot creates these rows.
CREATE TABLE IF NOT EXISTS `mindlink_send_conversations` (
  `environment` VARBINARY(6) NOT NULL,
  `tenant_id` VARBINARY(128) NOT NULL,
  `appkey` VARBINARY(20) NOT NULL,
  `owner_id` VARBINARY(20) NOT NULL,
  `conversation_id` BINARY(64) NOT NULL,
  `kind` VARBINARY(24) NOT NULL,
  `policy_version` BIGINT NOT NULL,
  `state` VARBINARY(8) NOT NULL,
  `channel_type` TINYINT NOT NULL,
  `native_conversation_id` VARBINARY(100) NOT NULL,
  `sub_channel` VARBINARY(32) NOT NULL DEFAULT '',
  `members_json` MEDIUMBLOB NOT NULL,
  `members_hash` BINARY(32) NOT NULL,
  PRIMARY KEY (`environment`,`tenant_id`,`appkey`,`owner_id`,`conversation_id`),
  UNIQUE KEY `uniq_mindlink_send_native` (`appkey`,`channel_type`,`native_conversation_id`,`sub_channel`),
  CONSTRAINT `chk_mindlink_send_kind` CHECK (`kind` IN ('avatar_owner_dm','mindlink_group','avatar_pair_group')),
  CONSTRAINT `chk_mindlink_send_policy` CHECK (`policy_version` > 0),
  CONSTRAINT `chk_mindlink_send_state` CHECK (`state` IN ('active','disabled')),
  CONSTRAINT `chk_mindlink_send_channel` CHECK (`channel_type` IN (1,2))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- One immutable event for the original history commit, not a second message
-- database or a claim that legacy dispatch/client delivery has completed.
CREATE TABLE IF NOT EXISTS `mindlink_message_outbox` (
  `issuer` VARBINARY(1024) NOT NULL,
  `operation_id` BINARY(64) NOT NULL,
  `event_id` BINARY(64) NOT NULL,
  `appkey` VARBINARY(20) NOT NULL,
  `message_id` VARBINARY(20) NOT NULL,
  `request_body` MEDIUMBLOB NOT NULL,
  `native_binding_json` BLOB NOT NULL,
  `native_binding_hash` BINARY(32) NOT NULL,
  `members_json` MEDIUMBLOB NOT NULL,
  `native_payload` MEDIUMBLOB NOT NULL,
  `state` VARBINARY(16) NOT NULL DEFAULT 'pending',
  `created_time` DATETIME(3) NOT NULL,
  PRIMARY KEY (`issuer`,`operation_id`),
  UNIQUE KEY `uniq_mindlink_message_event` (`event_id`),
  UNIQUE KEY `uniq_mindlink_message_id` (`appkey`,`message_id`),
  CONSTRAINT `chk_mindlink_outbox_state` CHECK (`state` IN ('pending','sent','blocked'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
