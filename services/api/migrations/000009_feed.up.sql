-- 000009_feed.up.sql
-- Phase 5: Moments, the friends-only social feed.
--
-- A moment is a short post (text and/or media) authored by one user. It is
-- visible to the author, to their accepted contacts, and — when visibility is
-- `public` — to everyone. Likes and comments hang off a moment; both cascade
-- with it so deleting a moment cannot leave orphaned rows.

CREATE TABLE moments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body TEXT NOT NULL DEFAULT '',
    visibility TEXT NOT NULL DEFAULT 'friends'
        CHECK (visibility IN ('friends', 'public', 'private')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

-- The feed reads newest-first across a set of authors; this composite index
-- serves both the author timeline and the global keyset scan.
CREATE INDEX idx_moments_created ON moments (created_at DESC, id DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_moments_author_created ON moments (author_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TRIGGER moments_set_updated_at
    BEFORE UPDATE ON moments
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE moment_media (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    moment_id UUID NOT NULL REFERENCES moments(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    media_type TEXT NOT NULL DEFAULT 'image'
        CHECK (media_type IN ('image', 'video')),
    position INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_moment_media_moment ON moment_media (moment_id, position);

CREATE TABLE moment_likes (
    moment_id UUID NOT NULL REFERENCES moments(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (moment_id, user_id)
);

CREATE TABLE moment_comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    moment_id UUID NOT NULL REFERENCES moments(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_moment_comments_moment ON moment_comments (moment_id, created_at);
