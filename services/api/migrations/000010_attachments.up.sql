-- 000010_attachments.up.sql
-- Phase 6: Message attachment metadata and video message support.

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_message_type_check;
ALTER TABLE messages ADD CONSTRAINT messages_message_type_check
    CHECK (message_type IN ('text', 'image', 'file', 'audio', 'video', 'system'));

CREATE TABLE message_attachments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    uploader_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id      UUID REFERENCES messages(id) ON DELETE CASCADE,
    conversation_id UUID REFERENCES conversations(id) ON DELETE CASCADE,
    storage_path    TEXT NOT NULL,
    content_type    TEXT NOT NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('image', 'audio', 'video', 'file')),
    original_name   TEXT NOT NULL DEFAULT '',
    size_bytes      BIGINT NOT NULL DEFAULT 0,
    pending         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_message_attachments_pending
    ON message_attachments (created_at)
    WHERE pending = TRUE;

CREATE INDEX idx_message_attachments_conversation
    ON message_attachments (conversation_id)
    WHERE conversation_id IS NOT NULL;

CREATE INDEX idx_message_attachments_uploader
    ON message_attachments (uploader_id, created_at DESC);
