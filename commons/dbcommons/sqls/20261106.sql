CREATE TABLE IF NOT EXISTS `conversation_seen` (
  `app_key` varchar(20) NOT NULL,
  `user_id` varchar(32) NOT NULL,
  `target_id` varchar(32) NOT NULL,
  `channel_type` tinyint NOT NULL,
  `sub_channel` varchar(32) NOT NULL DEFAULT '',
  `seen_seq` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`app_key`,`user_id`,`target_id`,`channel_type`,`sub_channel`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
