CREATE TABLE IF NOT EXISTS `mindlink_operations` (
  `issuer` VARBINARY(1024) NOT NULL,
  `operation_id` BINARY(64) NOT NULL,
  `request_key` BINARY(32) NOT NULL,
  `binding_hash` BINARY(32) NOT NULL,
  `binding_json` MEDIUMBLOB NOT NULL,
  `reservation_id` BINARY(32) NOT NULL,
  `result_json` MEDIUMBLOB NULL,
  `created_time` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`issuer`, `operation_id`),
  UNIQUE KEY `uniq_mindlink_request` (`request_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `mindlink_tickets` (
  `issuer` VARBINARY(1024) NOT NULL,
  `jti` BINARY(64) NOT NULL,
  `ticket_hash` BINARY(32) NOT NULL,
  `operation_id` BINARY(64) NOT NULL,
  `created_time` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`issuer`, `jti`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
