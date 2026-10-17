-- 000010_attachments.down.sql

DROP TABLE IF EXISTS message_attachments;

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_message_type_check;
ALTER TABLE messages ADD CONSTRAINT messages_message_type_check
    CHECK (message_type IN ('text', 'image', 'file', 'audio', 'system'));
