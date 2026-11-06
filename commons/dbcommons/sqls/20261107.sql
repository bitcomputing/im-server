CREATE TABLE IF NOT EXISTS `message_sequence` (
  `app_key` varchar(20) NOT NULL,
  `conver_id` varchar(100) NOT NULL,
  `channel_type` tinyint NOT NULL,
  `sub_channel` varchar(50) NOT NULL DEFAULT '',
  `seq` bigint NOT NULL DEFAULT 0,
  `send_time` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`app_key`,`conver_id`,`channel_type`,`sub_channel`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

INSERT INTO message_sequence (app_key,conver_id,channel_type,sub_channel,seq,send_time)
SELECT BINARY app_key,BINARY conver_id,1,BINARY COALESCE(sub_channel,''),GREATEST(COALESCE(MAX(msg_seq_no),0),0),GREATEST(COALESCE(MAX(send_time),0),0)
FROM p_hismsgs WHERE app_key IS NOT NULL AND app_key<>'' AND conver_id IS NOT NULL AND conver_id<>''
GROUP BY BINARY app_key,BINARY conver_id,BINARY COALESCE(sub_channel,'')
ON DUPLICATE KEY UPDATE seq=GREATEST(message_sequence.seq,VALUES(seq)),send_time=GREATEST(message_sequence.send_time,VALUES(send_time));

INSERT INTO message_sequence (app_key,conver_id,channel_type,sub_channel,seq,send_time)
SELECT BINARY app_key,BINARY conver_id,2,BINARY COALESCE(sub_channel,''),GREATEST(COALESCE(MAX(msg_seq_no),0),0),GREATEST(COALESCE(MAX(send_time),0),0)
FROM g_hismsgs WHERE app_key IS NOT NULL AND app_key<>'' AND conver_id IS NOT NULL AND conver_id<>''
GROUP BY BINARY app_key,BINARY conver_id,BINARY COALESCE(sub_channel,'')
ON DUPLICATE KEY UPDATE seq=GREATEST(message_sequence.seq,VALUES(seq)),send_time=GREATEST(message_sequence.send_time,VALUES(send_time));

INSERT INTO message_sequence (app_key,conver_id,channel_type,sub_channel,seq,send_time)
SELECT BINARY app_key,BINARY CASE WHEN channel_type=1 THEN CONCAT(LEAST(BINARY user_id,BINARY target_id),':',GREATEST(BINARY user_id,BINARY target_id)) ELSE target_id END AS canonical_conver,channel_type,BINARY sub_channel,GREATEST(MAX(seen_seq),0),0
FROM conversation_seen WHERE channel_type IN (1,2)
GROUP BY BINARY app_key,canonical_conver,channel_type,BINARY sub_channel
ON DUPLICATE KEY UPDATE seq=GREATEST(message_sequence.seq,VALUES(seq));
