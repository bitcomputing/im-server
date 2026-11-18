-- Phase 1 is reachable through an SSH tunnel using the same local ports.
INSERT INTO globalconfs (conf_key, conf_value)
VALUES ('connect_address', '{"default":["127.0.0.1:19003"]}');
