-- 000014_advanced_messaging.up.sql
-- Phase 4: Advanced messaging — replies, forwards, reactions, pinning,
-- editing, and deletion.
--
-- A message can now reference another message two ways:
--   * reply_to_id  — a quote-reply; the referenced message is rendered inline.
--   * forwarded_from — a forward; the body was copied from the source message
--     and the reference records provenance. Both are SET NULL so removing a
--     conversation (e.g. the last member leaving a group) cannot leave a
--     dangling pointer.
--
-- Reactions are one row per (message, user, emoji) so the aggregate renders as
-- a compact emoji bar with a per-user "mine" flag.
--
-- Pinning is a conversation-scoped flag on the message row; a partial index
-- serves the pinned-messages panel without scanning history.

-- The Go layer defines a `video` message type (Phase 6) that the original
-- CHECK constraint omitted, so media uploads of videos could never be
-- persisted. Widen the closed set to match the code.
ALTER TABLE messages DROP CONSTRAINT messages_message_type_check;
ALTER TABLE messages ADD CONSTRAINT messages_message_type_check
    CHECK (message_type IN ('text', 'image', 'file', 'audio', 'video', 'system'));

ALTER TABLE messages
    ADD COLUMN reply_to_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    ADD COLUMN forwarded_from UUID REFERENCES messages(id) ON DELETE SET NULL,
    ADD COLUMN pinned_at TIMESTAMPTZ;

CREATE INDEX idx_messages_reply_to
    ON messages (reply_to_id) WHERE reply_to_id IS NOT NULL;

CREATE INDEX idx_messages_pinned
    ON messages (conversation_id, pinned_at DESC) WHERE pinned_at IS NOT NULL;

CREATE TABLE message_reactions (
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji TEXT NOT NULL CHECK (char_length(emoji) BETWEEN 1 AND 16),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, user_id, emoji)
);

-- Reaction aggregates are read per message page; the message_id index serves
-- the batched lookup, the user_id index serves "my reactions".
CREATE INDEX idx_message_reactions_message ON message_reactions (message_id);
CREATE INDEX idx_message_reactions_user ON message_reactions (user_id);
