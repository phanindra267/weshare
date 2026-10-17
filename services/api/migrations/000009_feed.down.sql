-- 000009_feed.down.sql
DROP INDEX IF EXISTS idx_moment_comments_moment;
DROP TABLE IF EXISTS moment_comments;

DROP TABLE IF EXISTS moment_likes;

DROP INDEX IF EXISTS idx_moment_media_moment;
DROP TABLE IF EXISTS moment_media;

DROP INDEX IF EXISTS idx_moments_author_created;
DROP INDEX IF EXISTS idx_moments_created;
DROP TRIGGER IF EXISTS moments_set_updated_at ON moments;
DROP TABLE IF EXISTS moments;
