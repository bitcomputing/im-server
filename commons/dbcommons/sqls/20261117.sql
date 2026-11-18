CREATE TABLE IF NOT EXISTS mindlink_principal_lifecycle (
  environment VARBINARY(6) NOT NULL,
  tenant_id VARBINARY(128) NOT NULL,
  appkey VARBINARY(20) NOT NULL,
  owner_id VARBINARY(20) NOT NULL,
  principal_id BINARY(32) NOT NULL,
  avatar_id VARBINARY(400) NOT NULL,
  state VARBINARY(8) NOT NULL DEFAULT 'open',
  avatar_generation BIGINT NOT NULL DEFAULT 0,
  policy_version BIGINT NOT NULL DEFAULT 0,
  projection_absent BOOLEAN NOT NULL DEFAULT FALSE,
  operation_issuer VARBINARY(1024) NOT NULL DEFAULT '',
  operation_id VARBINARY(64) NOT NULL DEFAULT '',
  binding_hash VARBINARY(32) NOT NULL DEFAULT '',
  PRIMARY KEY (appkey,principal_id),
  UNIQUE KEY uniq_mindlink_lifecycle_avatar (environment,tenant_id,appkey,avatar_id),
  CONSTRAINT chk_mindlink_lifecycle CHECK (
    (state='open' AND avatar_generation=0 AND policy_version=0 AND projection_absent=0 AND OCTET_LENGTH(operation_issuer)>0 AND OCTET_LENGTH(operation_id)=64 AND OCTET_LENGTH(binding_hash)=32) OR
    (state='retired' AND avatar_generation>0 AND policy_version>0 AND projection_absent IN (0,1) AND OCTET_LENGTH(operation_issuer)>0 AND OCTET_LENGTH(operation_id)=64 AND OCTET_LENGTH(binding_hash)=32)
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
