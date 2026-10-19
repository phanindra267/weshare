# Next Steps

**Current phase:** Phase 8 — Discover and Global Search (Verified); Phase 10 —
Notifications and Synchronization (Verified)
**Next phase:** Phase 4 — Advanced Messaging

Phases 0, 1–3, and 5–10 are implemented and verified. **Phase 4 — Advanced
Messaging** (message edit, delete, reply, forward, react, pin, and in-chat
search) is the only unstarted phase below the frontier. See
`docs/PHASE_STATUS.md` for actual, reproducible test results — including the live
browser E2E suites (`messaging`, `groups`, `moments`, `calls`) and the Go/
frontend gate runs.

**Numbering note.** Earlier revisions of this file claimed "the plan has no
Phase 8". That was wrong: Phase 8 is **Discover and Global Search**, Phase 9 is
Audio and Video Calling, and Phase 10 is Notifications and Synchronization. This
file and `docs/PHASE_STATUS.md` now follow the master prompt; the reconciliation
is recorded in `docs/PHASE_STATUS.md` and `docs/IMPLEMENTATION_AUDIT.md`. Older
revisions also called the calling work "Phase 6" and notifications "Phase 7";
those numbers belonged to Media/Attachments/Voice (6) and Moments (7).

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

## Phase 10 — Notifications and Synchronization (delivered)

- **Schema** — `000013_notifications`: per-user rows with cascading optional FKs
  (conversation/message/moment/call), a `data` JSONB payload, `read_at`, a
  listing index `(user_id, created_at DESC, id DESC)`, and a partial unread
  index.
- **Backend** — `internal/notifications` (models, errors, events, store, service,
  DTOs, handler). REST: `GET /notifications`, `GET /notifications/unread-count`,
  `POST /notifications/read`, `DELETE /notifications/:id`, `DELETE /notifications`.
  Triggers: `contact_request`/`contact_accepted` (contacts),
  `moment_like`/`moment_comment` (feed), `missed_call` (calls). Delivery is
  best-effort via a `notification.created` socket frame; persistence is the
  source of truth.
- **Frontend** — `NotificationBell` (unread badge) in `AppHeader`,
  `NotificationsProvider` (loads on connect/reconnect, live-prepends on
  `notification.created`, exposed as empty for signed-out visitors), and the
  `/notifications` page.
- **Contracts** — `openapi.yaml` Notifications tag/paths/schemas;
  `websocket-events.md` moves `notification.created` to Implemented.

## Phase 8 — Discover and Global Search (delivered)

- **Backend** — moment search lives in `internal/feed` (`GET /search/posts`,
  `GET /discover/moments`) and shares one SQL visibility predicate with the feed;
  everything else lives in `internal/search` (`GET /search/{messages,users,
  conversations,groups}` and `GET /discover/people`). All endpoints return the
  standard paginated envelope, clamp `limit`, escape LIKE metacharacters, and
  validate `q`/`kind`/`cursor` (failures → `422` with `{"fields": {...}}`).
- **Frontend** — `/discover` (search shortcut, people suggestions with add-friend,
  public-moments feed with load-more) and an upgraded `/search` (one query box,
  an **All** tab plus Messages / People / Chats / Posts tabs, debounced,
  load-more, loading/empty/error states); `Discover` link in `AppHeader`.
- **Contracts** — `openapi.yaml` Search/Discover tags, paths, and schemas. No new
  WebSocket events (search/discover are read-only).
- **Deferred** — the discover **mini-app directory** and mini-app search are
  Phase 12; the feature directories are empty scaffolding, so the directory is
  documented rather than shipped as a dead end.

## Phase 9 — Audio and Video Calling (delivered)

Roadmap reference: `docs/IMPLEMENTATION_AUDIT.md` ("Missing / planned by phase").

1. **Schema / config** — done.
   - `000012_calls` adds `calls` and `call_participants`; history is a summary
     only and signalling is never persisted.
   - `ICE_SERVERS` (JSON) is surfaced through `GET /api/v1/calls/ice`, together
     with the `CALL_MAX_PARTICIPANTS` ceiling (default 4, validated ≥ 2).
   - The default is an empty ICE list, i.e. host candidates only. That works on
     localhost; TURN remains deployment-specific and is not assumed.
2. **Backend** — done, in `internal/calls`.
   - Frames are relayed over the existing hub through the same
     `PublishToUser(uuid, any) bool` interface the feed uses:
     `call.invite`, `call.accept`, `call.reject`, `call.end`, `call.sdp`,
     `call.candidate`.
   - Every callee must be an accepted contact with no block in either direction;
     otherwise the caller gets `FORBIDDEN` and nothing is disclosed.
   - One active call per user, enforced in memory. A busy callee produces
     `call.busy`; if nobody is reachable the caller also gets `call.ended`
     (`reason: "failed"`) so an unanswerable call cannot ring forever.
   - No server-side media handling of any kind.
3. **Frontend** — done, in `src/features/calls`.
   - `CallProvider` owns one call and a full `RTCPeerConnection` per remote
     participant, with ICE/STUN from `/api/v1/calls/ice`, trickle ICE, mute and
     camera toggles, and teardown on unmount.
   - Offers are glare-free: the already-joined peer offers to a newly joined
     one, so a joiner only answers.
   - `CallOverlay` renders the incoming-call prompt, the in-call panel, the local
     preview, and one tile per remote peer. Audio calls attach the remote stream
     to an `<audio>` element, without which the stream would never be played.
   - Entry points: per-contact call/video buttons on `/contacts` and call buttons
     in the direct-conversation header.
4. **Tests** — done.
   - Backend: relay authorization, busy handling, offline callee, offer/answer
     relay, participant state, and history persistence.
   - Frontend: component tests with a fake `RTCPeerConnection` and mocked
     `getUserMedia` (accept, reject, hang up, mute, and the offer/answer rules).
   - Live: `e2e/calls.spec.ts` drives two browser contexts with Chromium's fake
     media devices and asserts that both reach `active` (only reachable from
     `connectionState === "connected"`) and that remote video is decoding frames.
5. **Docs & tracking** — done.
   - `docs/PHASE_STATUS.md` (Phase 9 acceptance table + real results),
     `docs/IMPLEMENTATION_AUDIT.md`, and this file were reconciled; the contracts
     were extended (`openapi.yaml` Calls tag/paths/schemas, and
     `websocket-events.md`'s calling catalogue, rules, and P2P-vs-SFU limit).

### Known limits accepted in this phase

- Group calls are a full P2P mesh (`n − 1` connections per client), bounded by
  `CALL_MAX_PARTICIPANTS`; there is no SFU and no plan to add one here.
- Call history returns the most recent page; there is no cursor for older calls.
- No call notifications: an invite only reaches a callee with a live socket.

## Deferred / notes

- Notifications/Search/Sync: `notification.created` is reserved but not emitted;
  message/contact search; `sync` catches up one 200-message page per reconnect.
- Chat power features (edit, delete, reply, forward, react, pin, search):
  schema and contract placeholders only.
- Moments likes/comments have no realtime events; counts refresh on demand.
- Email/SMS verification: development-only adapter; never simulate fake delivery.
- Rate limiting is in-memory per IP/user — dev-only (not distributed).
- Transitive dev-only npm advisory (braces/micromatch) noted; not runtime.

## Verification

Before marking any phase complete: run the full gate set above, exercise the
feature through a live browser E2E against the real API and database, and record
actual results in `docs/PHASE_STATUS.md`. For calling specifically, that means
two contacts can place and answer an audio or video call and see/hear each other
over WebRTC, non-contacts and blocked users cannot signal, a busy callee is
reported, rejection and hang-up tear the call down, the history summary is
persisted, protected endpoints still reject unauthenticated calls, and non-call
E2E suites still pass.
