# Phase Status

**Project:** WeShare
**Phase:** 4 — Advanced Messaging
**Status:** Verified
**Last updated:** 2026-10-23

## Phase overview

| Phase | Title | Status |
|---|---|---|
| 0 | Foundation | Verified |
| 1 | Authentication, Accounts, and Profiles | Verified |
| 2 | Contacts and Presence | Verified |
| 3 | One-to-One Real-Time Messaging | Verified |
| 4 | Advanced Messaging (edit, delete, reply, forward, react, pin, search) | Verified |
| 5 | Groups | Verified |
| 6 | Media, Attachments, and Voice Messages | Verified |
| 7 | Moments and Social Feed | Verified |
| 8 | Discover and Global Search | Verified |
| 9 | Audio and Video Calling | Verified |
| 10 | Notifications and Synchronization | Verified |

### Numbering reconciliation

Earlier revisions of this document used a drifted numbering: Phase 4 was
labelled "Groups, Media, Voice" and Phase 5 "Moments, Social, Discover", which
duplicated Phases 6 and 7 and left the file claiming **"the plan has no Phase
8"**. That claim was wrong. This document now follows the master prompt:

- **Phase 4 — Advanced Messaging** is the chat power-feature phase (edit,
  delete, reply, forward, react, pin, in-chat search). It is **implemented and
  verified** (migration `000014_advanced_messaging`).
- **Phase 5 — Groups** is what the old "Phase 4 — Groups, Media, Voice" section
  described. Media and voice messages belong to Phase 6.
- **Phase 6 — Media, Attachments, and Voice Messages** (unchanged label).
- **Phase 7 — Moments and Social Feed** absorbs the old "Phase 5 — Moments,
  Social, Discover" section. Discover moved to its own Phase 8.
- **Phase 8 — Discover and Global Search** is new (this document).
- **Phase 9 — Audio and Video Calling** (unchanged label).
- **Phase 10 — Notifications and Synchronization** is new (this document).

Roadmap reference: `docs/IMPLEMENTATION_AUDIT.md` ("Missing / planned by phase").

---

## Phase 4 — Advanced Messaging

### Acceptance criteria

| Item | Status | Notes |
|---|---|---|
| Schema | Completed | Migration `000014_advanced_messaging`: `messages.reply_to_id` (self FK), `forwarded_from_id`, `edited_at`, `deleted_at`, `pinned_at`, `pinned_by`, and `message_reactions` (`message_id`, `user_id`, `emoji`, unique per user/emoji). Also widens `messages_message_type_check` to accept `'video'`, which the Phase 6 media path could already write |
| Edit a message | Completed | `PATCH /api/v1/conversations/:id/messages/:messageId`; sender only, text messages only, within a 15-minute `editWindow`, not deleted; sets `edited_at` and republishes `message.updated` |
| Delete a message | Completed | `DELETE /api/v1/conversations/:id/messages/:messageId`; soft delete for everyone (tombstone, content scrubbed to `{}`). Sender may delete their own; group admins/owners may delete others'. Publishes `message.deleted` |
| Reply | Completed | `reply_to_id` on send (REST and socket), stored as a self-FK and hydrated into `reply_to` with the quoted sender/body; `422` when the target is missing or in another conversation |
| Forward | Completed | `POST /api/v1/conversations/:id/messages/forward` where `:id` is the **target**; body carries `source_conversation_id` + `message_id`. Copies `message_type`/`content`, sets `forwarded_from_id`, and fans out to the target members; system messages are rejected |
| React / unreact | Completed | `PUT`/`DELETE /api/v1/conversations/:id/messages/:messageId/reactions` with an emoji; `message_reactions` is unique per `(message, user, emoji)`; the response returns the full aggregate with a viewer-specific `mine` flag |
| Pin / unpin | Completed | `PUT`/`DELETE /api/v1/conversations/:id/messages/:messageId/pin`; any member may pin; `pinned_at`/`pinned_by` recorded; `GET /api/v1/conversations/:id/pins` lists pinned messages newest-first |
| In-chat search | Completed | `GET /api/v1/conversations/:id/messages/search?q=`; membership-scoped, text-only, excludes deleted messages, escaped `%`/`_`/`\` with SQL `ESCAPE '\'`, paginated with the standard envelope |
| Realtime | Completed | `message.updated`, `message.deleted`, `message.reaction`, `message.pinned`; `message.reaction`/`message.updated` are fanned out per member with viewer-specific `mine` aggregation, while `message.deleted`/`message.pinned` use a single broadcast. Socket `message.send` gains `reply_to_id` |
| Authorization | Completed | Every route is membership-scoped (non-members get `404`); edit is sender-only; reaction/pin changes are validated against the conversation |
| Validation | Completed | Edit: non-empty text ≤ 4000 runes; emoji: non-empty, ≤ 32 bytes; search `q` required (≤ 100 runes); unknown/malformed ids → `422`; failures use the standard `{"fields": {...}}` envelope |
| Frontend: message actions | Completed | Hover action bar (Reply, Forward, Edit, Delete, Pin/Unpin, Add reaction), quoted reply preview, "Forwarded" label, pinned badge, "(edited)" marker, tombstone, reaction chips + picker with counts |
| Frontend: composer | Completed | Reply/edit banner, prefilled composer with a "Save" label for edits, cancel returns to a normal send |
| Frontend: forward + search + pins | Completed | `ForwardDialog` target picker, in-thread search bar with result jump, and a pinned-messages panel |
| Frontend: realtime reducers | Completed | `applyReactions`/`applyPinned` keep state consistent across `message.updated`/`message.reaction`/`message.deleted`/`message.pinned`, including viewer-specific `mine` |
| Contracts | Completed | `openapi.yaml` gains the 7 new paths and `ReactionResponse`, `EditMessageRequest`, `ForwardMessageRequest`, `ReactionRequest`, `SendMessageRequest.reply_to_id`; `websocket-events.md` documents the new frames and `message.send.reply_to_id` |

### Design notes

- **Soft delete, for everyone.** A deleted message keeps its row (and sequence) so
  clients can render a stable tombstone instead of a disappearing gap; the body
  is scrubbed to `{}` so no content survives in the database.
- **Reply targets are references, not copies.** `reply_to_id` is a self-FK and
  the quoted message is hydrated with a batched `MessagesByIDs`, so an edit to
  the original is reflected in the quote. A reply to a tombstone is rendered
  without content.
- **Forwards are copies with provenance.** Forwarding copies `message_type` and
  `content` (including media URLs) and records `forwarded_from_id`; it does not
  create a live link to the source, so later edits to the source are not
  propagated.
- **`ToMessageResponse` stays viewer-agnostic.** Reaction `mine` and message
  `status` depend on the viewer, so the service uses `ToMessageResponseFor` and
  `broadcastViewer` to send each member their own aggregate; a single broadcast
  would otherwise show another user's `mine` flag.
- **Pin is membership-wide.** Any member can pin; the pin list is shared. This
  is a chat-annotation feature, not an admin-only moderation tool (delete is the
  moderation action).
- **Route shape avoids Gin conflicts.** `/:id/messages/search` and
  `/:id/messages/forward` are declared before `/:id/messages/:messageId`, and
  `pins` lives at `/:id/pins`, so no static segment is shadowed by a parameter.
- **Search is substring `ILIKE`.** No ranking or full-text index (same accepted
  limit as Phase 8 search).

### Test results (actual, not aspirational)

**Go backend** (`services/api`):
- `go build ./...` → exit 0
- `go vet ./...` → exit 0
- `gofmt -l .` → clean
- `go test ./...` → pass in every package; `internal/messaging` covers edit
  permissions/window/type, soft delete (self and group moderation), reaction
  idempotency and aggregates, pin/unpin, search escaping and membership, forward
  provenance, and reply hydration.

**Frontend** (`apps/web`):
- `npx tsc --noEmit` → exit 0
- `npx eslint src` → 0 problems
- `npx vitest run` → 75/75 passed across 13 files (adds the Phase 4
  `ConversationsView` suite and `applyReactions`/`applyPinned` reducer tests)
- `npx next build` → succeeds (12 static pages)

**Live REST** — `phase4_live.ps1`, all 57 checks pass, covering edit, reply,
forward, reactions, pin/unpin/pins, in-chat search, delete, group moderation,
and validation/authorization failures.

**Live browser E2E** — `apps/web/e2e/advanced-messaging.spec.ts` passes with two
real Chromium sessions against the real API, database, and WebSocket: send →
edit → react → pin → reply → forward → search → delete, with Bob observing each
mutation live (no reload).

---

## Phase 8 — Discover and Global Search

### Acceptance criteria

| Item | Status | Notes |
|---|---|---|
| Global search across posts | Completed | `GET /api/v1/search/posts` (owned by `internal/feed`, which owns moment visibility SQL). Substring match on the body with the same visibility predicate as the feed, so search can never leak a moment the feed would hide |
| Global search across messages | Completed | `GET /api/v1/search/messages`; only messages in conversations the caller is a member of |
| Global search across users/contacts | Completed | `GET /api/v1/search/users`; `is_contact` flags accepted contacts |
| Global search across conversations | Completed | `GET /api/v1/search/conversations`; matches a direct peer's name/username or a group title. Optional `kind=group` narrows to groups |
| Global search across groups | Completed | `GET /api/v1/search/groups` (alias for `conversations?kind=group`); returns groups the caller belongs to |
| Discover people | Completed | `GET /api/v1/discover/people`; suggests accounts the caller is not connected to, newest first, excluding self |
| Discover public moments | Completed | `GET /api/v1/discover/moments`; newest public moments across the app (`internal/feed`) |
| Pagination | Completed | Every endpoint returns the standard `{items, next_cursor}` envelope; keyset cursors, clamped `limit` |
| Authorization / permission scoping | Completed | Search never widens access: messages and groups are membership-scoped, posts obey moment visibility (`public`/`friends`/`private` + exclusions + blocks), user search never discloses contacts of others |
| Validation | Completed | `q` required (≤ 100 runes); `kind` is a closed set; `cursor`/`limit` validated; failures return `422` with `{"fields": {...}}` |
| Query escaping | Completed | `%`/`_`/`\` escaped in Go and matched with SQL `ESCAPE '\'`, so a query cannot become a wildcard |
| Frontend: `/discover` | Completed | Discover landing page with a search shortcut, "People you may know" (with add-friend), and a public-moments feed with load-more |
| Frontend: `/search` | Completed | One query box with an **All** tab that queries every domain at once plus **Messages / People / Chats / Posts** tabs, debounced, with load-more, loading/empty/error states |
| Frontend navigation | Completed | `Discover` link in `AppHeader`; `/discover` gated by `RequireAuth` |
| Realtime | Completed | Search and discover are read-only; no new WebSocket events |
| Contracts | Completed | `openapi.yaml` gains the `Search`/`Discover` tags, paths, and schemas (`ProfileHit`, `MessageSearchHit`, `ConversationHit`, `SearchConversation`, `SearchPeer`) |

### Design notes

- **Ownership.** Moment search lives in `internal/feed` (which owns the moment
  visibility SQL) and is mounted at `/search/posts` and `/discover/moments`; it
  does **not** live under `/moments/:id`, so there is no static/parameter route
  clash. Everything else lives in `internal/search` (`/search/*` and
  `/discover/people`).
- **One visibility predicate.** `feed.momentVisibilityPredicate` is a single SQL
  fragment used by `ListFeed`, `GetMoment`, `SearchMoments`, and
  `ListPublicMoments`, so search and feed can never drift apart.
- **Public discover.** `ListPublicMoments` filters to `visibility = 'public'`
  and still applies block filters, so a public moment from a blocked author is
  hidden.
- **Cursors.** People and conversation results order by `created_at DESC, id
  DESC` with a `<unix-micro>:<uuid>` keyset cursor (the same encoding the feed
  and conversation lists use).
- **No ranking.** Matching is `ILIKE` substring search (no full-text or
  relevance ranking); results are newest-first. This is documented rather than
  faked.
- **Mini-apps.** The master prompt lists a mini-app directory and mini-app search
  under discover; mini-apps are **Phase 12** and the `internal/miniapps` and
  `features/miniapps` directories are empty scaffolding. The directory is
  therefore deferred to Phase 12 rather than shipped as a dead end here.

### Test results (actual, not aspirational)

**Go backend** (`services/api`):
- `go build ./...` → exit 0
- `go vet ./...` → exit 0
- `gofmt -l .` → clean
- `go test ./...` → pass in every package, including `feed` and `search`
- `internal/feed` tests cover `SearchPosts` (required query, over-length query,
  LIKE-metacharacter escaping, pagination). `internal/search` tests cover
  `Conversations` (rejects an unknown `kind`, escapes metacharacters, paginates)
  and `DiscoverPeople` (does not share the query with other domains). New
  route-mount tests (`handler_test.go`) assert `/search/posts`,
  `/discover/moments`, `/search/*`, and `/discover/people` resolve to the auth
  guard (401) rather than 404, alongside the existing `/moments/:id` route.

**Frontend** (`apps/web`):
- `npm run lint` → exit 0
- `npm run typecheck` → exit 0
- `npm test` → 61/61 passed across 13 files (adds `SearchView` and
  `DiscoverView` suites)
- `npm run build` → succeeds; routes include `/discover`

**Integration (live)** — `phase8_live.ps1`, all checks pass:
- As an author: message search finds direct **and** group messages; user search
  marks a contact; conversation search finds a direct chat by peer name; group
  search finds the group by title and returns `kind: "group"` with the right
  `member_count`; post search finds both the public and friends-only moments;
  `limit=1` returns exactly one item **and** a `next_cursor`.
- As a stranger: message search returns 0 (membership-scoped); group search
  returns 0 (not a member); post search returns **only** the public moment;
  `discover/people` includes the author, excludes self; `discover/moments`
  returns only `public` moments and excludes the friends-only one.
- Validation: an unknown `kind` and an empty `q` both return `422`; an
  unauthenticated request returns `401`.

---

## Phase 10 — Notifications and Synchronization

### Acceptance criteria

| Item | Status | Notes |
|---|---|---|
| Persisted notifications | Completed | Migration `000013_notifications`: per-user rows with cascading optional FKs (conversation/message/moment/call), a `data` JSONB payload, `read_at`, a `(user_id, created_at DESC, id DESC)` listing index, and a partial unread index |
| Notification triggers | Completed | `contact_request` and `contact_accepted` (contacts), `moment_like` and `moment_comment` (feed), `missed_call` (calls) — each emitted through the notifications service |
| REST surface | Completed | `GET /api/v1/notifications`, `GET /api/v1/notifications/unread-count`, `POST /api/v1/notifications/read`, `DELETE /api/v1/notifications/:id`, `DELETE /api/v1/notifications` |
| Realtime delivery | Completed | `notification.created` is published to the recipient's socket over the existing hub (best-effort); persistence makes the history authoritative for offline clients |
| Authorization | Completed | Notifications are strictly per-user; marking or deleting another user's notification yields `404` |
| Frontend | Completed | `NotificationBell` (unread badge) in `AppHeader`, `/notifications` page (list, mark read, delete one, clear all), and `NotificationsProvider` (loads on connect/reconnect, live-prepends on `notification.created`, exposes an empty state to signed-out visitors) |
| Loading / empty / error states | Completed | Skeletons, empty states, and retryable errors on the notifications list |
| Contracts | Completed | `openapi.yaml` gains the `Notifications` tag/paths/schemas; `websocket-events.md` moves `notification.created` to **Implemented** and documents the payload |

### Design notes

- A notification is a durable record; the socket frame is an optimization, not
  the source of truth. A client that was offline simply lists what it missed.
- `data` (JSONB) carries a small denormalised payload (for example a text
  preview) so a row renders without extra reads.
- The unread count is a partial-index count (`WHERE read_at IS NULL`).

### Test results

- `go test ./...` including `internal/notifications` and the trigger tests in
  `contacts`, `feed`, and `calls` (`notify_test.go`) → pass.
- Live (`phase10_live.ps1`): request/accepted/like/comment notifications are
  created for the right recipient, the unread count moves, mark-read clears it,
  and deleting a notification twice returns `404` the second time.

---

## Phase 9 — Audio and Video Calling

### Acceptance criteria

| Item | Status | Notes |
|---|---|---|
| Audio calls | Completed | `call.invite` with `kind: "audio"`, `getUserMedia({audio: true, video: false})`, remote stream bound to a hidden `<audio autoPlay>` |
| Video calls | Completed | `kind: "video"`; local preview plus one `<video>` tile per remote peer; camera on/off toggles the outbound track's `enabled` flag |
| WebRTC signalling over the realtime hub | Completed | `call.invite`, `call.accept`, `call.reject`, `call.end`, `call.sdp`, `call.candidate` relayed by `internal/calls` through the existing `PublishToUser` interface — no server-side media |
| Real peer connections and media | Completed | `CallProvider` holds one `RTCPeerConnection` per participant, exchanges SDP/ICE, and attaches remote tracks; the call only reports `active` once `connectionState === "connected"` |
| Incoming-call prompt | Completed | `CallOverlay` shows the caller's display name with Accept/Reject while ringing |
| Accept / reject / hang up | Completed | Accept grants media and answers the offer; reject and end send the matching frame and reset local state |
| Mute and camera toggles | Completed | `toggleMute`/`toggleCamera` flip `enabled` on the live tracks, so changes take effect without renegotiation |
| Active-call bookkeeping | Completed | In-memory registry enforces one call per user; a busy callee produces `call.busy` and an unreachable one `call.unavailable` |
| Contacts-only calling | Completed | `Store.AreConnected` (accepted contact, no block either way) is checked for every callee before a call is created; otherwise `FORBIDDEN` |
| Call history persistence | Completed | Migration `000012_calls` (`calls`, `call_participants`); `GET /api/v1/calls` returns the caller's history newest first |
| ICE bootstrap | Completed | `GET /api/v1/calls/ice` returns `ICE_SERVERS` plus the `CALL_MAX_PARTICIPANTS` ceiling |
| Missed-call notifications | Completed | An unanswered call records a `missed_call` notification for the callee (Phase 10) |
| Disconnect cleanup | Completed | Losing the last socket drops the user from any active call, so an abandoned tab cannot leave a call ringing |
| Entry points | Completed | Call and video-call buttons per contact row, and call buttons in the direct-conversation thread header |
| Loading, empty, and error states | Completed | Call buttons hide unless a `CallProvider` is mounted and no call is in progress; `getUserMedia` failure declines the call instead of hanging |
| Contracts | Completed | `openapi.yaml` (Calls tag, `/calls`, `/calls/ice`, `Call`/`CallParticipant`/`CallPage`/`IceConfig`/`ICEServer`) and `websocket-events.md` (calling catalogue, rules, P2P-vs-SFU limit) |

### Design notes

- Calling lives in `internal/calls` (models, errors, events, store, service, DTO,
  handler) and depends on the realtime hub only through the same
  `PublishToUser(uuid, any) bool` interface the feed uses, so there is no import
  cycle and no coupling to `messaging`.
- `call_id` is generated by the client, so the caller renders its own UI before
  any network round trip and the server only has to validate and relay.
- Offers are glare-free by construction: the participant who is **already
  joined** creates the offer towards a newly joined one, so a joiner only ever
  answers. In 1:1 that means the caller offers after the callee accepts.
- A direct call is torn down as a whole (`hangup`, `cancelled`, or `rejected`,
  plus `missed` for anyone who never answered), while ending a group call just
  removes that participant (`reason: "left"`) and closes the call once nobody
  remains.
- Audio calls attach the remote stream to an `<audio>` element. Without it the
  stream would be connected to the peer but never played.

### Scalability limit

Group calls are a full peer-to-peer mesh: each client holds one connection per
other participant (`n − 1`), so the bandwidth and CPU cost grows linearly and
lands on the weakest client. That is acceptable up to the enforced
`CALL_MAX_PARTICIPANTS` ceiling but is not a path to large rooms. Scaling
further requires an SFU that terminates media on the server, which is
deliberately out of scope — it is infrastructure rather than application code
and would break the "media never touches the server" property this phase is
built on. This is documented in `websocket-events.md` and
`docs/IMPLEMENTATION_AUDIT.md`.

---

## Phase 7 — Moments and Social Feed

### Acceptance criteria

| Item | Status | Notes |
|---|---|---|
| Schema: moments, media, likes, comments | Completed | Migration `000009_feed` (schema at v9); `moment_media`, `moment_likes`, `moment_comments` cascade with their moment |
| Create text and media posts | Completed | Create moments with up to 9 photos/videos and a text body up to 2000 characters |
| Edit and delete your own posts | Completed | Inline editing with `PATCH /api/v1/moments/:id` and author soft deletion via `DELETE /api/v1/moments/:id` |
| Display authors, timestamps, and media previews | Completed | Author display name, avatar, relative timestamp, photo grid, and `<video controls>` |
| Visibility model | Completed | `friends` (default), `public`, `private`; enforced in a single SQL predicate for feed, single-moment reads, post search, and public discover |
| REST surface | Completed | `GET/POST /api/v1/moments`, `GET/DELETE /api/v1/moments/:id`, `POST/DELETE /api/v1/moments/:id/likes`, `GET/POST /api/v1/moments/:id/comments`, `DELETE /api/v1/moments/:id/comments/:commentId` |
| Feed ordering + pagination | Completed | Newest first; opaque keyset `cursor` + `limit`; `next_cursor` empty on the last page |
| Like and unlike posts | Completed | Idempotent reactions with `POST/DELETE /api/v1/moments/:id/likes` returning the updated count |
| Comment on posts and reply | Completed | Expandable comment thread; threaded replies supporting `reply_to_id` with a joined `reply_to_author` profile |
| Delete permitted comments | Completed | Both the comment author and the moment author may soft-delete comments |
| Prevent duplicate likes and requests | Completed | Unique constraints plus idempotent toggle handlers prevent duplicates |
| Restricted audiences / excluded users | Completed | `moment_exclusions` table and API support to exclude specific contacts from viewing a moment |
| Enforce visibility and blocking on the backend | Completed | Bidirectional contact-blocking filter in SQL prevents blocked users from seeing or interacting with posts |
| Post notifications | Completed | Likes and comments create `moment_like` / `moment_comment` notifications for the author (Phase 10) |
| Hide or report posts | Completed | `POST /api/v1/moments/:id/hide` filters a post from the viewer's feed; `POST /api/v1/moments/:id/report` persists a report |
| Loading, empty, and error states | Completed | Feed skeleton loaders, `EmptyState`, `ErrorMessage` with retry, loading buttons, and toasts |
| Persist posts, comments, likes, media | Completed | Migration `000011_moments_features` adds exclusions, hidden posts, reports, and replies |
| Realtime fan-out | Completed | `moment.created` published to the author's accepted contacts for `friends` visibility; not broadcast for `private`; `public` reachable via the feed/discover only |
| Validation | Completed | Body ≤ 2000 runes; a moment needs text or media; visibility is a closed set; comment ≤ 1000 chars |
| Authorization | Completed | Non-viewers get `404` (not `403`) so hidden moments are not disclosed; only the author may delete their moment |
| Frontend: `/moments` | Completed | Composer (text, visibility, up to 9 photos/videos), feed cards with media, optimistic like/unlike, expandable comments, author delete, live prepend on `moment.created` |
| Nav + route gating | Completed | Moments link in `AppHeader`; `/moments` gated by `RequireAuth` |
| Contracts | Completed | `openapi.yaml` (Moments tag + paths + `Moment`/`Comment`/`MomentPage`/`CommentPage`/`LikeState`) and `websocket-events.md` (`moment.created`) |

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

- `go test ./internal/feed -v` → create/fan-out, visibility authorization,
  like/unlike idempotency and counts, comment rules and ownership, pagination
  cursors, validation and media-type rejection → pass.
- Frontend: `MomentsView` suite passes; `npm run build` includes `/moments`.
- Integration (live): `e2e/moments.spec.ts` → two users become contacts, Alice
  shares a friends-only moment, Bob sees it, likes it, comments, and Alice sees
  the comment after reloading.

---

## Phase 6 — Media, Attachments, and Voice Messages

### Acceptance criteria

| Item | Status | Notes |
|---|---|---|
| Upload and share images | Completed | `POST /api/v1/media/attachments`; sniffed and validated |
| Upload and share videos | Completed | `video/mp4`, `video/webm`, `video/quicktime` classification, `<video controls>` preview, and download |
| Upload and download ordinary files | Completed | Stored safely with sanitized extensions; served with `Content-Disposition` headers |
| Record and play voice messages | Completed | In-browser `MediaRecorder` recording + HTML5 `<audio controls>` playback |
| Display audio controls and media previews | Completed | Dedicated video player, audio player with duration, image loading skeletons |
| Show upload progress and errors | Completed | Real-time `UploadProgress` bar via XHR progress events; toasts on failure |
| Validate file size, file type, and content | Completed | Client-side 50 MB check + server-side bounded `MaxBytesReader` and content sniffing |
| Store media metadata in PostgreSQL | Completed | Migration `000010_attachments` creates `message_attachments` with ownership & conversation linkage |
| Store media bytes outside PostgreSQL | Completed | Rooted filesystem storage with subdirectories for `attachments` and `avatars` |
| Authorization for private attachments | Completed | `CanAccess` verifies conversation membership or uploader identity; `403` on unauthorized access |
| Prevent path traversal and arbitrary file access | Completed | Regexp validation, path cleaning, and prefix checking prevent escaping the storage root |
| Attachment ownership and message associations | Completed | Foreign keys to `users(id)`, `messages(id)`, `conversations(id)`; uploader-only delete |
| Handle abandoned uploads / temporary files | Completed | Background sweeper periodically purges orphaned pending attachments older than 24 h |
| Preserve uploaded media across restarts | Completed | Persistent disk storage decoupled from the process lifecycle |

---

## Phase 5 — Groups

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
| Group search | Completed | `GET /api/v1/search/groups` finds groups by title for the caller's own groups (Phase 8) |
| Frontend | Completed | `CreateGroupDialog`, `MemberPicker`, `GroupInfoDialog`; group list/thread, member management, system messages, media attachments |
| Tests | Completed | `internal/messaging` service tests cover group create/add/remove/leave/role and fan-out; `ConversationsView.groups.test.tsx` + `e2e/groups.spec.ts` |

### Test results

- `go test ./internal/messaging` → pass.
- Browser E2E (`apps/web/e2e/groups.spec.ts`) → pass: three users form a group,
  an image is shared live to another member, the member sees the group panel
  without admin tools, and a non-member receives `404` reading the group's
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
| Rate limiting | Completed | Message send `30/min` per user; contact requests `30/15 min` per IP; registration `10/hr` per IP (env-configurable) |
| Frontend: conversations + thread | Completed | `/messages` with list + thread, optimistic send + retry, history paging, unread badges, typing, read receipts, presence |
| Frontend: realtime client | Completed | Reconnecting `RealtimeClient` (exponential backoff), `RealtimeProvider` context, `sync` catch-up on reconnect |
| Message search | Completed | `GET /api/v1/search/messages` (Phase 8) |
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
- Browser E2E (`apps/web/e2e/messaging.spec.ts`) → pass: two users register,
  become contacts, open a chat, presence shows online, Alice sends, Bob sees it,
  Bob replies and Alice receives it live over the WebSocket, and the read receipt
  flips Alice's tick to Read.

---

## Phase 2 — Contacts and Presence

### Acceptance criteria

| Item | Status | Notes |
|---|---|---|
| Schema: contacts + friend requests | Completed | Migration `000006_contacts` |
| Contact CRUD + requests | Completed | `GET/POST/DELETE /api/v1/contacts`, `GET /api/v1/contacts/requests`, `POST /api/v1/contacts/requests/:id/{accept,decline}`, `POST /api/v1/users/:id/block` (+ unblock) |
| Presence heartbeat | Completed | `POST /api/v1/presence/heartbeat`; `online` derived from `PRESENCE_STALE_AFTER` (default 60 s) |
| Rate limiting | Completed | Contact requests `30/15 min` per IP |
| Notifications | Completed | Requests and acceptances create `contact_request` / `contact_accepted` notifications (Phase 10) |
| Frontend | Completed | `features/contacts` + `features/presence`; `/contacts` gated by `RequireAuth`; presence badges; heartbeat every 30 s |
| Mutual connections | Completed | Accepting stores the friendship bidirectionally; single-direction lookup is complete |

### Test results

- `go test ./internal/contacts` → pass; `ContactsView.test.tsx` covers the UI.

---

## Phase 1 — Authentication, Accounts, and Profiles

| Item | Status | Notes |
|---|---|---|
| Auth schema (users, credentials, sessions) | Completed | `000002_users`, `000003_credentials`, `000004_sessions`, `000005_devices` |
| Password hashing (Argon2id) | Completed | `alexedwards/argon2id`; Memory 64 MiB / Parallelism NumCPU / 32-byte key |
| Sessions: opaque token, DB-backed | Completed | 256-bit token; only SHA-256 hex stored in `sessions.token_hash` |
| Secure cookies | Completed | HTTP-only `weshare_session`; SameSite=Lax (default); Secure in prod |
| CSRF (double-submit cookie) | Completed | `weshare_csrf` echoed as `X-CSRF-Token`; enforced for unsafe methods with a session |
| Rate limiting + login lockout | Completed | Per-IP: login 30/15 min, register 10/hr, password 10/hr; lockout after `MAX_LOGIN_ATTEMPTS` |
| Registration / login / logout / me | Completed | `POST /api/v1/auth/{register,login,logout}`, `GET /api/v1/auth/me` |
| Change password + session revocation | Completed | `POST /api/v1/auth/password`; `GET`/`DELETE /api/v1/auth/sessions[/:id]` |
| Profiles (read/update/avatar) | Completed | `GET`/`PATCH /api/v1/me`, `POST /api/v1/me/avatar`, `GET /api/v1/users/:id` |
| Avatar storage + serving | Completed | Local FS (`STORAGE_DIR`), content-sniffed jpeg/png/webp, auth-gated serving |
| Frontend auth screens + protected routes | Completed | `/login`, `/register`, `/profile` (gated by `RequireAuth`), `AuthProvider`, `AppHeader` |

---

## Known gaps

- **Mini-apps (Phase 12)**, the **demo wallet (Phase 11)**, **settings/privacy/
  accessibility (Phase 13)**, and the **social** surface are not implemented; the
  corresponding feature directories are empty scaffolding.
- `users` package has no unit tests (covered indirectly by live E2E checks).
- Search matching is substring `ILIKE` with no ranking or full-text index; fine
  at current scale, and it does not scale to large corpora without one.
- `openapi.yaml` still does not document the Auth, Users, or Contacts surfaces
  (System, Messaging, Media, Moments, Search, Discover, Notifications, and Calls
  are covered).
- Notifications are delivered over the live socket best-effort; there is no push
  (APNs/FCM) or email transport.
- `sync.result` catches up one page (200) at a time; repeated `sync` is required
  for very large backlogs.
- Rate limiting is in-memory per IP/user (dev-only; not distributed).
- Group calls are a peer-to-peer mesh bounded by `CALL_MAX_PARTICIPANTS`; no SFU.
- Transitive dev-only npm advisory (braces/micromatch) noted; not runtime.

## Next action

**Phase 11 — Demo Wallet** is the next unstarted phase below the frontier
(Phases 0–10 are verified). Read `docs/NEXT_STEPS.md` for the ordered plan. Keep
`PHASE_STATUS.md` / `NEXT_STEPS.md` in sync as work lands.
