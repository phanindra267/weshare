-- 000005_devices.down.sql

DROP TRIGGER IF EXISTS devices_set_updated_at ON devices;
DROP INDEX IF EXISTS idx_devices_last_seen_at;
DROP INDEX IF EXISTS idx_devices_user_id;
DROP TABLE IF EXISTS devices;
