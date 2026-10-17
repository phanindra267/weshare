-- 000011_moments_features.down.sql

DROP TABLE IF EXISTS moment_reports;
DROP TABLE IF EXISTS moment_hidden;
DROP TABLE IF EXISTS moment_exclusions;
ALTER TABLE moment_comments DROP COLUMN IF EXISTS reply_to_id;
