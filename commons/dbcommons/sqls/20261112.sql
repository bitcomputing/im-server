-- Internal avatar names preserve up to 80 Unicode scalar values (320 UTF-8 bytes).
ALTER TABLE `users` MODIFY COLUMN `nickname` varchar(80) DEFAULT NULL COMMENT '昵称';
