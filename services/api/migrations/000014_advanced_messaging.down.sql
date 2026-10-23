-- 000014_advanced_messaging.down.sql
DROP TABLE IF EXISTS message_reactions;

ALTER TABLE messages
    DROP COLUMN IF EXISTS reply_to_id,
    DROP COLUMN IF EXISTS forwarded_from,
    DROP COLUMN IF EXISTS pinned_at;

DROP INDEX IF EXISTS idx_messages_pinned;
DROP INDEX IF EXISTS idx_messages_reply_to;

-- Restore the original (video-less) message_type set.
ALTER TABLE messages DROP CONSTRAINT messages_message_type_check;
ALTER TABLE messages ADD CONSTRAINT messages_message_type_check
    CHECK (message_type IN ('text', 'image', 'file', 'audio', 'system'));
