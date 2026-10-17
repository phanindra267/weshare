-- 000011_moments_features.up.sql
-- Phase 7: Moments social feed enhancements: comment replies, exclusions, hide, report.

ALTER TABLE moment_comments
    ADD COLUMN reply_to_id UUID REFERENCES moment_comments(id) ON DELETE SET NULL;

CREATE INDEX idx_moment_comments_reply_to
    ON moment_comments (reply_to_id)
    WHERE reply_to_id IS NOT NULL;

-- Excluded users for restricted audience
CREATE TABLE moment_exclusions (
    moment_id UUID NOT NULL REFERENCES moments(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (moment_id, user_id)
);

CREATE INDEX idx_moment_exclusions_user ON moment_exclusions (user_id, moment_id);

-- Hidden moments per viewer
CREATE TABLE moment_hidden (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    moment_id UUID NOT NULL REFERENCES moments(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, moment_id)
);

-- Reported moments
CREATE TABLE moment_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    moment_id UUID NOT NULL REFERENCES moments(id) ON DELETE CASCADE,
    reporter_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    details TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_moment_reports_moment ON moment_reports (moment_id);
CREATE INDEX idx_moment_reports_reporter ON moment_reports (reporter_id);
