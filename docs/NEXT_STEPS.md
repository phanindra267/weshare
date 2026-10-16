# Next Steps

**Current phase:** Phase 1 — Authentication, Accounts, and Profiles (Verified)  
**Next phase:** Phase 2 — Contacts and Presence

Phase 0 (foundation) and Phase 1 (auth/accounts/profiles) are both implemented
and verified. See `docs/PHASE_STATUS.md` for the actual test results.

## Immediate housekeeping (before starting Phase 2)

1. **`go mod tidy`** in `services/api` — drops the unused `github.com/alexedwards/scs/v2`
   dependency (manual DB-backed sessions were chosen instead).
2. **Unit tests for `internal/users`** — currently exercised only by live E2E curl
   checks; add handler/store tests for `GET/PATCH /me`, avatar upload validation,
   and `GET /users/:id` 404s.
3. **Playwright browser install + first E2E** — `npx playwright install` (deferred so
   far); add one smoke flow: register → land on `/profile` → change name → logout.
4. **Docs completion** — fill `docs/API.md` (auth + profile endpoints),
   `docs/SECURITY.md` (Argon2id params, session/cookie/CSRF/rate-limit design),
   `docs/DATABASE.md` (users/credentials/sessions schema), and update `README.md`
   (currently still says "Phase 0 is Verified").

## Phase 2 — Contacts and Presence (ordered plan)

Phase roadmap reference: `docs/IMPLEMENTATION_AUDIT.md` ("Phases 1-16").

1. **Schema**
   - Migration `000006_contacts`: contacts / friend requests (requester_id,
     addressee_id, status pending|accepted|blocked, timestamps, unique pair).
   - Presence columns already exist on `users` (`online`, `last_seen_at`) — add a
     heartbeat-driven update path rather than a new table unless needed.
   - Down migrations for all.
2. **Backend**
   - Repositories + services for contact CRUD, accept/decline/undo requests,
     and block/unblock with the same generic error style (no enumeration leaks).
   - Presence: `POST /api/v1/presence/heartbeat` (auth) updating `last_seen_at`,
     and derive `online` from a short staleness window; public exposure rules per
     profile privacy.
   - Endpoints: `GET/POST/DELETE /api/v1/contacts`, `GET /api/v1/contacts/requests`,
     `POST /api/v1/contacts/requests/:id/{accept,decline}`, `POST /api/v1/users/:id/block`.
   - Rate-limit write endpoints; reuse the existing in-memory `Limiter`.
   - Preserve the existing envelopes, request-ID, and CSRF conventions.
3. **Frontend**
   - `features/contacts` API client + types, contacts/requests screens, presence
     badges on profile & list rows, toasts for request actions.
   - Wire into `AppHeader` / a nav entry; reuse `RequireAuth` for gated routes.
4. **Tests**
   - Backend: request/accept/block flows, privacy of `online`/`last_seen_at`,
     auth-gated endpoints, rate limits.
   - Frontend: contacts list state, request accept/decline UI, form errors.
5. **Docs & tracking**
   - Update `docs/PHASE_STATUS.md` (Phase 2 acceptance table + real results),
     `docs/NEXT_STEPS.md` (advance to Phase 3), `packages/contracts/openapi.yaml`,
     and `packages/contracts/websocket-events.md` if presence events are defined
     (the realtime layer itself lands in Phase 3).

## Deferred / notes

- Email/SMS verification: development-only adapter; never simulate fake delivery
  unless explicitly tracked.
- Rate limiting is in-memory per IP — documented as dev-only (not distributed).
- Transitive dev-only npm advisory (braces/micromatch) noted; not runtime.

## Verification

Before marking Phase 2 complete: two accounts can find each other, send/accept a
request, block/unblock; presence reflects a heartbeat and decays when idle;
protected endpoints still reject unauthenticated calls; all Go/web tests, lint,
typecheck, and builds pass; `PHASE_STATUS.md` records actual results.
