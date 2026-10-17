# Next Steps

**Current phase:** Phase 3 — One-to-One Real-Time Messaging (Verified)
**Next phase:** Phase 4 — Groups, Media, Voice

Phases 0–3 are implemented and verified. See `docs/PHASE_STATUS.md` for actual,
reproducible test results — including two live backend harnesses (38/0 and 50/0)
and a Playwright browser E2E (1/1).

## Immediate housekeeping

1. **Commit Phase 3** — the Go backend (`internal/messaging`, `internal/realtime`),
   migration `000007_messaging`, the web feature (`features/messaging`, `/messages`),
   updated contracts, and these docs. Do not commit before the gates below pass.
2. **Re-run the full gate set** before committing:
   - `services/api`: `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`
   - `apps/web`: `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`,
     `npm run e2e` (needs the API on `:8080` and Chromium installed once via
     `npx playwright install chromium`)
3. **`users` package tests** — still exercised only indirectly; add handler/store
   tests for `GET/PATCH /me`, avatar upload validation, and `GET /users/:id` 404s.

## Phase 4 — Groups, Media, Voice (ordered plan)

Roadmap reference: `docs/IMPLEMENTATION_AUDIT.md` ("Missing / planned by phase").
The conversation model already distinguishes `kind = direct | group`, and the
message schema already reserves `image | file | audio` plus `edited_at`/
`deleted_at`, so Phase 4 extends rather than reworks.

1. **Schema**
   - Migration `000008_groups`: conversation `title`/`avatar_url` for groups,
     `conversation_members.role` (`owner|admin|member`) with defaults, and any
     needed indexes. Down migration included.
   - No new message tables needed (media reuses `message_type`).
2. **Backend**
   - Group lifecycle: create group (with initial members), add/remove members,
     leave, promote/demote, rename, set group avatar — owner/admin guarded.
   - Authorization: extend `Store.CanMessage`/membership checks for groups; keep
     the existing `404`-on-non-member behaviour.
   - System messages for membership changes (`message_type = system`, currently
     server-only and rejected from clients).
   - Media messages: upload endpoint(s) returning a `url`, then send
     `image|file|audio` messages referencing it; reuse `internal/media` storage
     and content sniffing.
   - Extend realtime events: `conversation.updated` / `member.changed` (group
     metadata + membership) and reuse `message.created` fan-out to all members.
   - Read/delivery semantics already per-member; verify they hold for >2 members
     (unread counts, receipts) and adjust fan-out where needed.
3. **Frontend**
   - Group creation flow, member management, group info panel, system messages,
     and media attachments (upload + preview) in the composer and message list.
   - Extend `features/messaging` rather than adding a parallel feature; keep the
     optimistic-send/`client_msg_id` retry and reconnect `sync` behaviour.
4. **Tests**
   - Backend: group create/add/remove/leave/role rules, non-member access, system
     messages, media validation, multi-member read/delivery.
   - Frontend: group flows and media rendering; extend Vitest suite.
   - Live: extend the Playwright E2E to a 3-user group with an image message.
5. **Docs & tracking**
   - Update `docs/PHASE_STATUS.md` (Phase 4 acceptance table + real results),
     `docs/NEXT_STEPS.md` (advance to Phase 5 — Moments/social/Discover), and the
     contracts (`openapi.yaml`, `websocket-events.md`).

## Deferred / notes

- Message editing and deletion have schema/contract placeholders but no write
  path; schedule with the relevant phase.
- `sync.result` paginates 200 messages at a time; the client should loop on the
  returned cursor for very large backlogs (currently a single page per reconnect).
- Email/SMS verification: development-only adapter; never simulate fake delivery.
- Rate limiting is in-memory per IP/user — dev-only (not distributed).
- Transitive dev-only npm advisory (braces/micromatch) noted; not runtime.

## Verification

Before marking Phase 4 complete: a group can be created, members added/removed
with correct role enforcement, membership changes surface as system messages, an
image/file/audio message sends and renders end to end, multi-member unread and
read receipts are correct, protected endpoints still reject unauthenticated
calls, all Go/web gates and both live harnesses pass, and `docs/PHASE_STATUS.md`
records actual results.
