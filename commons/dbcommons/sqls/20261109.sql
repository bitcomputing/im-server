CREATE TABLE IF NOT EXISTS `mindlink_owner_barriers` (
  `environment` VARBINARY(6) NOT NULL,
  `tenant_id` VARBINARY(128) NOT NULL,
  `appkey` VARBINARY(20) NOT NULL,
  `owner_id` VARBINARY(20) NOT NULL,
  `read_enabled` BOOLEAN NOT NULL DEFAULT FALSE,
  PRIMARY KEY (`environment`,`tenant_id`,`appkey`,`owner_id`),
  CONSTRAINT `chk_mindlink_owner_read` CHECK (`read_enabled` IN (0,1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `mindlink_principals` (
  `environment` VARBINARY(6) NOT NULL,
  `tenant_id` VARBINARY(128) NOT NULL,
  `appkey` VARBINARY(20) NOT NULL,
  `owner_id` VARBINARY(20) NOT NULL,
  `principal_id` BINARY(32) NOT NULL,
  `avatar_id` VARBINARY(400) NOT NULL,
  `avatar_generation` BIGINT NOT NULL,
  `policy_version` BIGINT NOT NULL,
  `state` VARBINARY(8) NOT NULL,
  PRIMARY KEY (`appkey`,`principal_id`),
  UNIQUE KEY `uniq_mindlink_avatar` (`environment`,`tenant_id`,`appkey`,`avatar_id`),
  CONSTRAINT `chk_mindlink_principal_generation` CHECK (`avatar_generation` > 0),
  CONSTRAINT `chk_mindlink_principal_policy` CHECK (`policy_version` > 0),
  CONSTRAINT `chk_mindlink_principal_state` CHECK (`state` IN ('active','disabled'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
