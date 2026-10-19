-- 000013_notifications.up.sql
-- Phase 10: notifications.
--
-- A notification is a persisted, per-user record of something that happened
-- while the recipient may not have been looking at the relevant screen: a
-- contact request, an accepted request, a like or comment on a moment, or a
-- missed call. Realtime delivery is best-effort (a `notification.created` frame
-- over the socket); persistence makes the history authoritative, so a client
-- that was offline simply lists what it missed.
--
-- The optional foreign keys reference the thing the notification is about and
-- cascade away with it, so deleting a moment or conversation cannot leave a
-- dangling notification. `data` carries a small denormalised payload (for
-- example a text preview) so the client can render a row without extra reads.

CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    conversation_id UUID REFERENCES conversations(id) ON DELETE CASCADE,
    message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
    moment_id UUID REFERENCES moments(id) ON DELETE CASCADE,
    call_id UUID REFERENCES calls(id) ON DELETE CASCADE,
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Listing walks the recipient's notifications newest-first.
CREATE INDEX idx_notifications_user_created
    ON notifications (user_id, created_at DESC, id DESC);

-- The unread badge is a cheap partial-index count.
CREATE INDEX idx_notifications_user_unread
    ON notifications (user_id) WHERE read_at IS NULL;
