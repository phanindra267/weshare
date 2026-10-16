-- 000006_contacts.up.sql
-- Contacts: friend requests, accepted friendships, and blocks.
--
-- One directed row per user pair. A request travels user_id -> contact_id;
-- accepting also materialises the reverse accepted row so that "is friends
-- with" is a single-direction lookup. Blocking keeps its own row and removes
-- any reverse row so a blocked pair can never be friends.

CREATE TABLE contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contact_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'blocked')),
    accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT contacts_distinct_users CHECK (user_id <> contact_id)
);

CREATE UNIQUE INDEX idx_contacts_pair ON contacts (user_id, contact_id);
CREATE INDEX idx_contacts_user_status ON contacts (user_id, status);
CREATE INDEX idx_contacts_contact_status ON contacts (contact_id, status);

CREATE TRIGGER contacts_set_updated_at
    BEFORE UPDATE ON contacts
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
