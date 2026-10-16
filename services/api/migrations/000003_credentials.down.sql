-- 000003_credentials.down.sql

DROP TRIGGER IF EXISTS user_credentials_set_updated_at ON user_credentials;
DROP TABLE IF EXISTS user_credentials;
