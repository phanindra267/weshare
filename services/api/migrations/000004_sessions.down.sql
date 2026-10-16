-- 000004_sessions.down.sql

DROP TRIGGER IF EXISTS sessions_set_updated_at ON sessions;
DROP INDEX IF EXISTS idx_sessions_expires_at;
DROP INDEX IF EXISTS idx_sessions_token_hash;
DROP INDEX IF EXISTS idx_sessions_user_id;
DROP TABLE IF EXISTS sessions;
