ALTER TABLE `mindlink_owner_barriers`
  ADD COLUMN `write_enabled` BOOLEAN NOT NULL DEFAULT FALSE,
  ADD CONSTRAINT `chk_mindlink_owner_write` CHECK (`write_enabled` IN (0,1));

ALTER TABLE `mindlink_principals`
  ADD COLUMN `native_appkey` VARCHAR(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci GENERATED ALWAYS AS (CONVERT(`appkey` USING utf8mb4)) STORED,
  ADD COLUMN `native_principal_id` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci GENERATED ALWAYS AS (CONVERT(`principal_id` USING utf8mb4)) STORED,
  ADD UNIQUE KEY `uniq_mindlink_native_identity` (`native_appkey`,`native_principal_id`);
