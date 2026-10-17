-- 000008_groups.down.sql
DROP INDEX IF EXISTS idx_conversation_members_conversation;

-- Collapse any admin rows back to member before restoring the tighter check.
UPDATE conversation_members SET role = 'member' WHERE role = 'admin';
ALTER TABLE conversation_members
    DROP CONSTRAINT conversation_members_role_check;
ALTER TABLE conversation_members
    ADD CONSTRAINT conversation_members_role_check
    CHECK (role IN ('member', 'owner'));

ALTER TABLE conversations DROP COLUMN IF EXISTS avatar_url;
ALTER TABLE conversations DROP COLUMN IF EXISTS title;
