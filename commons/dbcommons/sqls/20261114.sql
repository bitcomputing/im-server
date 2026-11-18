CREATE TABLE IF NOT EXISTS mindlink_message_outbox (
  outbox_id BINARY(64) NOT NULL,
  issuer VARBINARY(1024) NOT NULL,
  operation_id BINARY(64) NOT NULL,
  request_body MEDIUMBLOB NOT NULL,
  native_binding MEDIUMBLOB NOT NULL,
  payload MEDIUMBLOB NOT NULL,
  recipients_json MEDIUMBLOB NOT NULL,
  state VARBINARY(16) NOT NULL DEFAULT 'pending',
  created_time DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (outbox_id),
  UNIQUE KEY uniq_mindlink_message_operation (issuer,operation_id),
  KEY idx_mindlink_message_pending (state,created_time,outbox_id)
) ENGINE=InnoDB;
