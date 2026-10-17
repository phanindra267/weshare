-- 000007_messaging.up.sql
-- One-to-one messaging: conversations, membership, and messages.
--
-- Phase 3 implements direct (1:1) conversations. The `kind` discriminator and
-- the generic member table are introduced now so group conversations (Phase 5)
-- can reuse the same storage without a rewrite.
--
-- Ordering is server-authoritative: `conversations.next_sequence` is
-- incremented atomically inside the send transaction and stamped onto the
-- message, so every conversation has a gap-free, monotonic sequence.

CREATE TABLE conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind TEXT NOT NULL DEFAULT 'direct'
        CHECK (kind IN ('direct', 'group')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    -- Normalized "least:greatest" user-id pair for direct conversations; NULL
    -- for groups. The partial unique index makes get-or-create race-safe.
    direct_key TEXT,
    next_sequence BIGINT NOT NULL DEFAULT 0,
    last_message_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_conversations_direct_key
    ON conversations (direct_key) WHERE direct_key IS NOT NULL;

CREATE TABLE conversation_members (
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member'
        CHECK (role IN ('member', 'owner')),
    -- Highest sequence the member has acknowledged reading / receiving.
    last_read_sequence BIGINT NOT NULL DEFAULT 0,
    last_delivered_sequence BIGINT NOT NULL DEFAULT 0,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, user_id)
);

CREATE INDEX idx_conversation_members_user
    ON conversation_members (user_id, conversation_id);

CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sequence_no BIGINT NOT NULL,
    -- Client idempotency key; a retry with the same value must not duplicate.
    client_msg_id TEXT,
    message_type TEXT NOT NULL DEFAULT 'text'
        CHECK (message_type IN ('text', 'image', 'file', 'audio', 'system')),
    content JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    edited_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    UNIQUE (conversation_id, sequence_no)
);

-- Idempotency is scoped to the sender so two users may coincidentally pick the
-- same client-generated id.
CREATE UNIQUE INDEX idx_messages_sender_client
    ON messages (sender_id, client_msg_id) WHERE client_msg_id IS NOT NULL;

CREATE INDEX idx_messages_conversation_seq
    ON messages (conversation_id, sequence_no DESC);

-- Delivery/read state is tracked per member (last_delivered_sequence /
-- last_read_sequence) rather than per message: a message is delivered once it
-- has been pushed to the recipient's device and read once the recipient's
-- acknowledged sequence reaches it. This keeps receipt state to one row per
-- member instead of one row per message per member, and reads back as a simple
-- status on every message in a page.

CREATE TRIGGER conversations_set_updated_at
    BEFORE UPDATE ON conversations
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
