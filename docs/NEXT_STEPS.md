# Next Steps

**Current phase:** Phase 5 — Moments, Social, Discover (Verified)
**Next phase:** Phase 6 — WebRTC Calls

Phases 0–5 are implemented and verified. See `docs/PHASE_STATUS.md` for actual,
reproducible test results — including the live browser E2E suites
(`messaging`, `groups`, `moments`) and the Go/frontend gate runs.

## Immediate housekeeping

1. **Re-run the full gate set** before committing any change:
   - `services/api`: `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`
   - `apps/web`: `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`,
     `npm run e2e` (needs the API on `:8080` and Chromium installed once via
     `npx playwright install chromium`)
2. **`users` package tests** — still exercised only indirectly; add handler/store
   tests for `GET/PATCH /me`, avatar upload validation, and `GET /users/:id` 404s.
3. **Message editing/deletion** — schema and contract placeholders exist
   (`edited_at`, `deleted_at`, `message.updated`/`message.deleted`) but no write
   path; schedule explicitly (see the user-facing "chat power features" list).

## Phase 6 — WebRTC Calls (ordered plan)

Roadmap reference: `docs/IMPLEMENTATION_AUDIT.md` ("Missing / planned by phase").
Calls reuse the existing realtime hub for signalling; media flows peer-to-peer
over WebRTC. Audio and video only (no group calls in this phase). The
`call.invite` / `call.answer` / `call.candidate` events are already reserved in
`packages/contracts/websocket-events.md` under Phase 6.

1. **Schema / config**
   - Optional `calls` table for a call history summary
     (`id`, `caller_id`, `callee_id`, `kind` audio|video, `status`, `started_at`,
     `ended_at`); down migration included. History is a summary only — signalling
     is not persisted.
   - STUN/TURN configuration surfaced to clients (env-driven `ICE_SERVERS`).
     A public STUN server is fine for local/demo; TURN is deployment-specific and
     must not be assumed.
2. **Backend**
   - Signalling relay over the existing hub: `call.invite`, `call.answer`,
     `call.candidate`, `call.reject`, `call.end`, `call.busy`. Relay only between
     two users who are accepted contacts with no block in either direction;
     otherwise drop silently (do not disclose presence).
   - Enforce "one active call per user" (a second invite yields `call.busy`).
   - Calls are signalling-only: no server-side media handling.
   - Persist the call-history summary on invite/accept/end.
3. **Frontend**
   - Reuse `RealtimeClient`/`RealtimeProvider` to carry signalling events.
   - `RTCPeerConnection` wrapper: offer/answer, ICE trickle, mute, camera
     toggle, hang up; permission prompt handling and error states.
   - UI: incoming-call prompt (accept/reject), in-call panel with local/remote
     video, audio-only fallback, and duration timer. Start calls from the
     conversation header and the contact list.
4. **Tests**
   - Backend: relay authorization (contact/block), busy handling, event shape,
     history persistence.
   - Frontend: component tests with a mocked `RTCPeerConnection` and mocked
     `getUserMedia` (accept → answering event, reject → `call.reject`, hang up →
     `call.end`, mute toggles local track).
   - Live: a Playwright E2E using Chromium's fake media devices
     (`--use-fake-device-for-media-stream`, `--use-fake-ui-for-media-stream`)
     between two contexts on localhost. If a full media negotiation proves
     flaky in CI, keep the signalling/ring UI E2E and document manual media
     verification in `docs/TESTING.md` rather than asserting a fake pass.
5. **Docs & tracking**
   - Update `docs/PHASE_STATUS.md` (Phase 6 acceptance table + real results),
     `docs/NEXT_STEPS.md` (advance to Phase 7 — Notifications/Search/Sync), and
     the contracts (`openapi.yaml` for any call-history endpoint,
     `websocket-events.md` flipping the `call.*` rows to Implemented).

## Deferred / notes

- Message editing and deletion: schema/contract placeholders only.
- `sync.result` paginates 200 messages at a time; the client should loop on the
  returned cursor for very large backlogs (currently a single page per reconnect).
- Moments likes/comments have no realtime events; counts refresh on demand.
- Email/SMS verification: development-only adapter; never simulate fake delivery.
- Rate limiting is in-memory per IP/user — dev-only (not distributed).
- Transitive dev-only npm advisory (braces/micromatch) noted; not runtime.

## Verification

Before marking Phase 6 complete: two contacts can place an audio/video call and
see/hear each other over WebRTC, non-contacts and blocked users cannot signal,
a busy callee is reported, rejection and hang-up tear the call down, the call
history summary is persisted, protected endpoints still reject unauthenticated
calls, all Go/web gates and the live E2E suites pass, and `docs/PHASE_STATUS.md`
records actual results.
