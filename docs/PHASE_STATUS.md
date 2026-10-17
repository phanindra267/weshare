# Phase Status

**Project:** WeShare
**Phase:** 3 — One-to-One Real-Time Messaging
**Status:** Verified
**Last updated:** 2026-10-16

## Phase overview

| Phase | Title | Status |
|---|---|---|
| 0 | Foundation | Verified |
| 1 | Authentication, Accounts, and Profiles | Verified |
| 2 | Contacts and Presence | Verified |
| 3 | One-to-One Real-Time Messaging | Verified |
| 4 | Groups, Media, Voice | Not started |

Roadmap reference: `docs/IMPLEMENTATION_AUDIT.md` ("Missing / planned by phase").

---

## Phase 3 — One-to-One Real-Time Messaging

### Acceptance criteria

| Item | Status | Notes |
|---|---|---|
| Schema: conversations, members, messages | Completed | Migration `000007_messaging` applied (schema at v7); `message_receipts` intentionally removed in favour of per-member delivery/read state |
| Per-member delivery/read tracking | Completed | `conversation_members.last_delivered_sequence` / `last_read_sequence`; message `status` derived per viewer in SQL |
| Monotonic sequences per conversation | Completed | `conversations.next_sequence` allocated before insert; `next_sequence == max(messages.sequence_no)` |
| REST surface | Completed | `GET/POST /api/v1/conversations`, `GET /api/v1/conversations/:id`, `GET/POST /api/v1/conversations/:id/messages`, `POST /api/v1/conversations/:id/read` |
| Idempotent sends | Completed | `client_msg_id` replay returns `200` with the stored message; first send returns `201` |
| Message history pagination | Completed | `?limit&before\|after`; ascending results; `next_cursor` is the next sequence or `""` when exhausted |
| Authorization | Completed | Direct conversations require a mutual accepted contact and no block (`Store.CanMessage`); non-members get `404` |
| Content validation | Completed | Closed shape per type; text ≤ 4000 runes / 8 KiB; image/file/audio require `url`; `system` rejected; `message_type` defaults to `text` |
| WebSocket transport | Completed | `GET /api/v1/realtime` (auth at handshake, origin allow-list); ping/pong keepalive, reconnect-safe |
| Realtime events | Completed | Server: `hello`, `pong`, `message.created`, `message.ack`, `message.receipt`, `typing`, `sync.result`, `read.ack`, `presence`, `error`; client: `ping`, `message.send`, `typing`, `read`, `sync` |
| Presence fan-out | Completed | Connect/disconnect broadcast `presence` to accepted friends (`contacts.Store.FriendIDs`) and seed the connecting user with online friends |
| Rate limiting | Completed | Message send `30/min` per user; contact requests `30/15min` per IP; registration `10/hr` per IP |
| Frontend: conversations + thread | Completed | `/messages` with list + thread, optimistic send + retry, history paging, unread badges, typing, read receipts, presence |
| Frontend: realtime client | Completed | Reconnecting `RealtimeClient` (exponential backoff), `RealtimeProvider` context, `sync` catch-up on reconnect |
| Contracts | Completed | `packages/contracts/openapi.yaml` (Messaging tag + schemas) and `packages/contracts/websocket-events.md` rewritten to match the implementation |

### Design notes

- Messaging lives in a single `internal/messaging` package; `internal/realtime`
  depends on it only through the `messaging.Publisher` interface
  (`PublishToUser(userID, event) bool`).
- Delivery/read state is tracked **per member**, not per message; a message's
  `status` is computed relative to the viewer (`sent → delivered → read`, or
  `received` for the recipient).
- Conversation cursors are `<unixMicro>|<uuid>`; message cursors are sequence
  numbers.

### Test results (actual, not aspirational)

**Go backend** (`services/api`):
- `go build ./...` → exit 0
- `go vet ./...` → exit 0
- `gofmt -l .` → clean
- `go test ./...` → pass in 10 packages: `auth`, `config`, `contacts`,
  `database`, `health`, `http`, `httpx`, `media`, `messaging`, `middleware`
- Migrations: applied `[1..7]`, pending `[]`

**Frontend** (`apps/web`):
- `npm run lint` → exit 0
- `npm run typecheck` → exit 0
- `npm test` → 30/30 passed across 7 files (`thread`, `realtime`, `ConversationsView`, plus existing `LoginForm`, `api`, `Button`)
- `npm run build` → succeeds; routes include `/messages`
- `npm run e2e` (Playwright, Chromium) → 1/1 passed

**Integration (live):**
- Backend messaging harness (`weshare-e2e-messaging.ps1`) → **50 passed, 0 failed**:
  registration, open-before-connect 403, self 422, unknown peer 404, mutual
  connect, open/idempotent re-open/same conversation both sides, send 201 seq 1,
  replay 200 same id, validation 422s, non-member 404s, recipient view, list +
  last_message + unread, mark-read, pagination, WS `hello`/typing/`message.created`/
  `message.ack`/`read`/`message.receipt`/`sync`/ping-pong/unknown-event error, and
  send rate limit 429.
- Browser E2E (`apps/web/e2e/messaging.spec.ts`) → **1 passed**: two users
  register, become contacts, open a chat, presence shows online, Alice sends,
  Bob sees it, Bob replies and Alice receives it live over the WebSocket, and the
  read receipt flips Alice's tick to Read.

---

## Phase 2 — Contacts and Presence

### Acceptance criteria

| Item | Status | Notes |
|---|---|---|
| Schema: contacts + friend requests | Completed | Migration `000006_contacts` |
| Contact CRUD + requests | Completed | `GET/POST/DELETE /api/v1/contacts`, `GET /api/v1/contacts/requests`, `POST /api/v1/contacts/requests/:id/{accept,decline}`, `POST /api/v1/users/:id/block` (+ unblock) |
| Presence heartbeat | Completed | `POST /api/v1/presence/heartbeat`; `online` derived from `PRESENCE_STALE_AFTER` (default 60s) |
| Rate limiting | Completed | Contact requests `30/15min` per IP |
| Frontend | Completed | `features/contacts` + `features/presence`; `/contacts` route gated by `RequireAuth`; presence badges; heartbeat every 30s |
| Mutual connections | Completed | Accepting stores the friendship bidirectionally; single-direction lookup is complete |

### Test results

**Integration (live):** `weshare-e2e-contacts.ps1` → **38 passed, 0 failed**
(request/accept, remove, block/unblock, idempotency, presence online/recent
heartbeat, unauthenticated 401, mutual connection).

**Frontend:** contacts view unit tests included in the suite.

---

## Phase 1 — Authentication, Accounts, and Profiles

| Item | Status | Notes |
|---|---|---|
| Auth schema (users, credentials, sessions) | Completed | `000002_users`, `000003_auth`, `000004_sessions`, `000005_devices` |
| Password hashing (Argon2id) | Completed | `alexedwards/argon2id`; Memory 64 MiB / Parallelism NumCPU / 32-byte key |
| Sessions: opaque token, DB-backed | Completed | 256-bit token; only SHA-256 hex stored in `sessions.token_hash` |
| Secure cookies | Completed | HTTP-only `weshare_session`; SameSite=Lax (default); Secure in prod |
| CSRF (double-submit cookie) | Completed | `weshare_csrf` echoed as `X-CSRF-Token`; enforced for unsafe methods with a session |
| Rate limiting + login lockout | Completed | Per-IP: login 30/15min, register 10/hr, password 10/hr; lockout after `MAX_LOGIN_ATTEMPTS` |
| Registration / login / logout / me | Completed | `POST /api/v1/auth/{register,login,logout}`, `GET /api/v1/auth/me` |
| Change password + session revocation | Completed | `POST /api/v1/auth/password`; `GET`/`DELETE /api/v1/auth/sessions[/:id]` |
| Profiles (read/update/avatar) | Completed | `GET`/`PATCH /api/v1/me`, `POST /api/v1/me/avatar`, `GET /api/v1/users/:id` |
| Avatar storage + serving | Completed | Local FS (`STORAGE_DIR`), content-sniffed jpeg/png/webp, auth-gated serving |
| Frontend auth screens + protected routes | Completed | `/login`, `/register`, `/profile` (gated by `RequireAuth`), `AuthProvider`, `AppHeader` |

---

## Known gaps

- `users` package has no unit tests (covered indirectly by live E2E curl checks).
- Phase 1 E2E was validated with curl; the browser-level auth flow is now covered
  by the messaging E2E (register + gated routes).
- Group conversations, media/voice messages, and message editing/deletion are
  not implemented yet (Phase 4+). Message `edited_at`/`deleted_at` exist in the
  schema and contract but no write path is exposed.
- Rate limiting is in-memory per IP/user (dev-only; not distributed).
- `sync.result` catches up one page (200) at a time; repeated `sync` is required
  for very large backlogs. Documented, not automated, in the UI.
- Transitive dev-only npm advisory (braces/micromatch) noted; not runtime.

## Next action

Proceed to **Phase 4 — Groups, Media, Voice**. Read `docs/NEXT_STEPS.md` for the
ordered plan. Keep `PHASE_STATUS.md`/`NEXT_STEPS.md` in sync as work lands.
