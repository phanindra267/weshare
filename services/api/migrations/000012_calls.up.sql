-- 000012_calls.up.sql
-- Phase 9: Audio and video calling (WebRTC).
--
-- Signalling (SDP offers/answers and ICE candidates) is relayed live over the
-- realtime WebSocket and is deliberately NOT persisted. This schema stores only
-- the call-history summary plus one row per participant, which doubles as each
-- user's call log (including missed calls).

CREATE TABLE calls (
    id UUID PRIMARY KEY,
    caller_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('audio', 'video')),
    is_group BOOLEAN NOT NULL DEFAULT FALSE,
    status TEXT NOT NULL DEFAULT 'ringing'
        CHECK (status IN ('ringing', 'active', 'ended', 'missed', 'rejected', 'canceled', 'failed')),
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_calls_caller ON calls (caller_id, created_at DESC);
CREATE INDEX idx_calls_created ON calls (created_at DESC);

CREATE TABLE call_participants (
    call_id UUID NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'callee' CHECK (role IN ('caller', 'callee')),
    state TEXT NOT NULL DEFAULT 'invited'
        CHECK (state IN ('invited', 'joined', 'rejected', 'left', 'missed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    joined_at TIMESTAMPTZ,
    left_at TIMESTAMPTZ,
    PRIMARY KEY (call_id, user_id)
);

CREATE INDEX idx_call_participants_user ON call_participants (user_id, created_at DESC);
