# Phase Status

**Project:** WeShare
**Phase:** 5 — Moments, Social, Discover
**Status:** Verified
**Last updated:** 2026-10-17

## Phase overview

| Phase | Title | Status |
|---|---|---|
| 0 | Foundation | Verified |
| 1 | Authentication, Accounts, and Profiles | Verified |
| 2 | Contacts and Presence | Verified |
| 3 | One-to-One Real-Time Messaging | Verified |
| 4 | Groups, Media, Voice | Verified |
| 5 | Moments, Social, Discover | Verified |

Roadmap reference: `docs/IMPLEMENTATION_AUDIT.md` ("Missing / planned by phase").

---

## Phase 5 — Moments, Social, Discover

### Acceptance criteria

| Item | Status | Notes |
|---|---|---|
| Schema: moments, media, likes, comments | Completed | Migration `000009_feed` (schema at v9); `moment_media`, `moment_likes`, `moment_comments` cascade with their moment |
| Visibility model | Completed | `friends` (default), `public`, `private`; enforced in SQL for both feed and single-moment reads |
| REST surface | Completed | `GET/POST /api/v1/moments`, `GET/DELETE /api/v1/moments/:id`, `POST/DELETE /api/v1/moments/:id/likes`, `GET/POST /api/v1/moments/:id/comments`, `DELETE /api/v1/moments/:id/comments/:commentId` |
| Feed ordering + pagination | Completed | Newest first; opaque keyset `cursor` + `limit`; `next_cursor` empty on last page |
| Comments | Completed | Oldest first with keyset pagination; author or moment owner may delete |
| Likes | Completed | Idempotent like/unlike; returns `{like_count, liked}`; denied on hidden moments |
| Media | Completed | Reuses `POST /api/v1/media/attachments`; image/video only (other types rejected); max 9 per moment |
| Realtime fan-out | Completed | `moment.created` published to the author's accepted contacts for `friends` visibility; not broadcast for `private`; `public` reachable via the feed only |
| Validation | Completed | Body ≤ 2000 runes; a moment needs text or media; visibility is a closed set; comment ≤ 1000 chars |
| Authorization | Completed | Non-viewers get `404` (not `403`) so hidden moments are not disclosed; only the author may delete their moment |
| Frontend: `/moments` | Completed | Composer (text, visibility, up to 9 photos/videos), feed cards with media, optimistic like/unlike, expandable comments (load/add/delete, load-more), author delete, live prepend on `moment.created` |
| Nav + route gating | Completed | Moments link in `AppHeader`; `/moments` gated by `RequireAuth` |
| Contracts | Completed | `openapi.yaml` (Moments tag + paths + `Moment`/`Comment`/`MomentPage`/`CommentPage`/`LikeState` schemas) and `websocket-events.md` (`moment.created`) |

### Design notes

- Moments live in a single `internal/feed` package (store, service, handler, DTOs,
  events, errors); the realtime hub is consumed only through the existing
  `PublishToUser` interface, so the feed does not depend on `messaging`.
- Visibility is applied in the SQL predicates, so a hidden moment is
  indistinguishable from a missing one (both yield `404`).
- Deletes are soft (`deleted_at`) for both moments and comments; the feed and
  comment listings filter them out.
- Moment media rows record a `position` so the client renders attachments in the
  order they were chosen.

### Test results (actual, not aspirational)

**Go backend** (`services/api`):
- `go build ./...` → exit 0
- `go vet ./...` → exit 0
- `gofmt -l .` → clean
- `go test ./...` → pass in 11 packages: `auth`, `config`, `contacts`,
  `database`, `feed`, `health`, `http`, `httpx`, `media`, `messaging`,
  `middleware`
- `go test ./internal/feed -v` → 24 passed, 0 failed (create/fan-out, visibility
  authorization, like/unlike idempotency and counts, comment rules and
  ownership, pagination cursors, validation and media-type rejection)
- Migrations: applied `[1..9]`, pending `[]`

**Frontend** (`apps/web`):
- `npm run lint` → exit 0
- `npm run typecheck` → exit 0
- `npm test` → 39/39 passed across 9 files (`Button`, `LoginForm`,
  `ContactsView`, `ConversationsView`, `ConversationsView.groups`,
  `MomentsView`, `realtime`, `thread`, `api`)
- `npm run build` → succeeds; routes include `/moments`
- `npm run e2e` (Playwright, Chromium) → 3/3 passed

**Integration (live):**
- Browser E2E (`apps/web/e2e/moments.spec.ts`) → **1 passed**: two users become
  contacts, Alice shares a friends-only moment, Bob sees it in his feed, likes it
  (optimistic toggle to `Unlike`), comments, and Alice sees the comment after
  reloading and opening the thread.
- Regression: `e2e/messaging.spec.ts` and `e2e/groups.spec.ts` still pass
  (Phase 3 and Phase 4 unaffected by the Phase 5 changes).

---

## Phase 4 — Groups, Media, Voice

### Acceptance criteria

| Item | Status | Notes |
|---|---|---|
| Schema: group metadata + roles | Completed | Migration `000008_groups`: `conversations.title`/`avatar_url`, `conversation_members.role` (`owner\|admin\|member`) |
| Group lifecycle | Completed | `POST /api/v1/conversations/group`, `PATCH /api/v1/conversations/:id`, `GET/POST /api/v1/conversations/:id/members`, `DELETE /api/v1/conversations/:id/members/:userId`, `POST /api/v1/conversations/:id/members/:userId/role`, `POST /api/v1/conversations/:id/leave` |
| Authorization | Completed | Owner/admin guarded; non-members receive `404` (no existence disclosure) |
| System messages | Completed | Membership and metadata changes post a server-generated `message_type: system` message; clients cannot send `system` |
| Media messages | Completed | Reuses `POST /api/v1/media/attachments`; `image`/`file`/`audio` messages render in the thread |
| Realtime events | Completed | `conversation.updated` and `member.changed` published to current members (a departing user also receives their own `member.changed`) |
| Multi-member read/delivery | Completed | Per-member `last_delivered_sequence`/`last_read_sequence` hold for groups; unread counts and receipts verified |
| Frontend | Completed | `CreateGroupDialog`, `MemberPicker`, `GroupInfoDialog`; group list/thread, member management, system messages, media attachments in the composer and list |
| Tests | Completed | `internal/messaging` service tests cover group create/add/remove/leave/role and fan-out; `ConversationsView.groups.test.tsx` + `e2e/groups.spec.ts` |

### Test results

- `go test ./internal/messaging` → pass (group lifecycle, system messages,
  authorization, fan-out assertions).
- Browser E2E (`apps/web/e2e/groups.spec.ts`) → **1 passed**: three users form a
  group, an image is shared live to another member, the member sees the group
  panel without admin tools, and a non-member receives `404` reading the group's
  history.

---

## Phase 3 — One-to-One Real-Time Messaging

### Acceptance criteria

| Item | Status | Notes |
|---|---|---|
| Schema: conversations, members, messages | Completed | Migration `000007_messaging`; `message_receipts` intentionally removed in favour of per-member delivery/read state |
| Per-member delivery/read tracking | Completed | `conversation_members.last_delivered_sequence` / `last_read_sequence`; message `status` derived per viewer in SQL |
| Monotonic sequences per conversation | Completed | `conversations.next_sequence` allocated before insert; `next_sequence == max(messages.sequence_no)` |
| REST surface | Completed | `GET/POST /api/v1/conversations`, `GET /api/v1/conversations/:id`, `GET/POST /api/v1/conversations/:id/messages`, `POST /api/v1/conversations/:id/read` |
| Idempotent sends | Completed | `client_msg_id` replay returns `200` with the stored message; first send returns `201` |
| Message history pagination | Completed | `?limit&before\|after`; ascending results; `next_cursor` is the next sequence or `""` when exhausted |
| Authorization | Completed | Direct conversations require a mutual accepted contact and no block (`Store.CanMessage`); non-members get `404` |
| Content validation | Completed | Closed shape per type; text ≤ 4000 runes / 8 KiB; image/file/audio require `url`; `system` rejected; `message_type` defaults to `text` |
| WebSocket transport | Completed | `GET /api/v1/realtime` (auth at handshake, origin allow-list); ping/pong keepalive, reconnect-safe |
| Realtime events | Completed | Server: `hello`, `pong`, `message.created`, `message.ack`, `message.receipt`, `typing`, `sync.result`, `read.ack`, `presence`, `error`; client: `ping`, `message.send`, `typing`, `read`, `sync` |
| Presence fan-out | Completed | Connect/disconnect broadcast `presence` to accepted friends and seed the connecting user with online friends |
| Rate limiting | Completed | Message send `30/min` per user; contact requests `30/15min` per IP; registration `10/hr` per IP (env-configurable) |
| Frontend: conversations + thread | Completed | `/messages` with list + thread, optimistic send + retry, history paging, unread badges, typing, read receipts, presence |
| Frontend: realtime client | Completed | Reconnecting `RealtimeClient` (exponential backoff), `RealtimeProvider` context, `sync` catch-up on reconnect |
| Contracts | Completed | `openapi.yaml` (Messaging tag + schemas) and `websocket-events.md` |

### Design notes

- Messaging lives in a single `internal/messaging` package; `internal/realtime`
  depends on it only through the `messaging.Publisher` interface
  (`PublishToUser(userID, event) bool`).
- Delivery/read state is tracked **per member**, not per message; a message's
  `status` is computed relative to the viewer (`sent → delivered → read`, or
  `received` for the recipient).
- Conversation cursors are `<unixMicro>|<uuid>`; message cursors are sequence
  numbers.

### Test results

- `go test ./internal/messaging` → pass.
- Browser E2E (`apps/web/e2e/messaging.spec.ts`) → **1 passed**: two users
  register, become contacts, open a chat, presence shows online, Alice sends, Bob
  sees it, Bob replies and Alice receives it live over the WebSocket, and the
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

- `go test ./internal/contacts` → pass; `ContactsView.test.tsx` covers the UI.
- Live harness `weshare-e2e-contacts.ps1` → **38 passed, 0 failed** (recorded
  with the Phase 2 commit).

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

- `users` package has no unit tests (covered indirectly by live E2E checks).
- Message editing/deletion have schema and contract placeholders (`edited_at`,
  `deleted_at`, `message.updated`/`message.deleted` events) but no write path;
  schedule with the relevant phase.
- Moments likes and comments do not emit realtime events; clients refresh counts
  on demand (documented in `websocket-events.md`).
- WebRTC call signalling (`call.*` events) and notifications are **Planned**
  (Phases 6–7) and not implemented.
- Rate limiting is in-memory per IP/user (dev-only; not distributed).
- `sync.result` catches up one page (200) at a time; repeated `sync` is required
  for very large backlogs.
- Transitive dev-only npm advisory (braces/micromatch) noted; not runtime.

## Next action

Proceed to **Phase 6 — WebRTC Calls**. Read `docs/NEXT_STEPS.md` for the ordered
plan. Keep `PHASE_STATUS.md`/`NEXT_STEPS.md` in sync as work lands.
