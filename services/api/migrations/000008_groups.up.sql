-- 000008_groups.up.sql
-- Phase 4: group conversations, member roles, and group metadata.
--
-- Phase 3 introduced the `kind = 'group'` discriminator and the generic member
-- table, so groups reuse the same storage. This migration adds the group-owned
-- metadata (title, avatar), widens the member role set to include `admin`, and
-- indexes the membership table for group member listings.

ALTER TABLE conversations ADD COLUMN title TEXT;
ALTER TABLE conversations ADD COLUMN avatar_url TEXT;

-- Phase 3 allowed only member/owner. Groups need a middle admin role.
ALTER TABLE conversation_members
    DROP CONSTRAINT conversation_members_role_check;
ALTER TABLE conversation_members
    ADD CONSTRAINT conversation_members_role_check
    CHECK (role IN ('member', 'admin', 'owner'));

-- Member listings walk a conversation's members ordered by join time.
CREATE INDEX idx_conversation_members_conversation
    ON conversation_members (conversation_id, joined_at);
