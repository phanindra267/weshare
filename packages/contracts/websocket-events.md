# WeShare WebSocket Events

This document is the source of truth for the realtime protocol. It defines the
envelope, conventions, and the event catalogue. The catalogue grows as phases
are implemented; events listed as **Implemented** are wired and tested.

The realtime layer was introduced in **Phase 3** (one-to-one messaging) and
extended in **Phase 4** (group lifecycle events and richer message content).

## Connection

- Authenticated endpoint: `GET /api/v1/realtime` with WebSocket upgrade.
- Authentication uses the same HTTP-only session cookie as the REST API. The
  session is revalidated during the HTTP handshake; a failed session yields a
  `401` and no upgrade.
- The server checks the handshake `Origin` header against the configured CORS
  allow-list. Requests with no `Origin` (native clients, tests) are allowed.
- The socket carries text frames only. The server sends a WebSocket ping every
  30 seconds and drops a connection that does not pong within 60 seconds.
- Conversation membership is checked before any event is relayed to a client.

## Envelope

All messages, in both directions, are JSON objects with a `type` discriminator.
Unknown fields are ignored so the protocol can grow without breaking clients.

### Client → Server

```json
{
  "type": "message.send",
  "request_id": "req-example",
  "conversation_id": "conversation-id",
  "client_msg_id": "client-generated-unique-id",
  "message_type": "text",
  "content": { "text": "Hello!" }
}
```

- `request_id` — optional client-generated correlation id; the server echoes it
  in the acknowledgement so the client can resolve optimistic UI.
- `client_msg_id` — client-generated idempotency key. Re-sending the same
  `client_msg_id` must not create a duplicate message. Required.
- `content` — the message body; its shape depends on `message_type` (see the
  REST contract). Arbitrary JSON is rejected. `text` uses `{text}`; `image`
  uses `{url}`; `audio` uses `{url, duration?}`; `file` uses
  `{url, name?, size?}`. `system` messages are server-generated and cannot be
  sent by clients.

### Server → Client

```json
{
  "type": "message.created",
  "event_id": "server-event-id",
  "conversation_id": "conversation-id",
  "message": {
    "id": "server-message-id",
    "conversation_id": "conversation-id",
    "sender_id": "sender-user-id",
    "sequence_no": 1,
    "client_msg_id": "client-generated-unique-id",
    "message_type": "text",
    "content": { "text": "Hello!" },
    "status": "received",
    "created_at": "2026-01-01T00:00:00Z",
    "edited_at": null,
    "deleted_at": null
  }
}
```

- `event_id` — unique, monotonic-ish server event identifier used for
  de-duplication on the client.
- `sequence_no` — per-conversation ordering assigned by the server. The server
  is authoritative for ordering.
- `status` — the delivery status **from the receiving client's perspective**
  (`received` for an incoming message; `sent`/`delivered`/`read` for the
  sender's own messages).

## Conventions

- **Enums are explicit.** `message_type` and `status` are validated against
  closed sets.
- **Content is validated.** Arbitrary JSON is rejected; `content` shape is
  enforced per `message_type`.
- **Persistence before broadcast.** A message is persisted before any
  `message.created` / `message.ack` event is published.
- **Idempotency.** `client_msg_id` deduplicates retries; a replayed send
  returns the original message with the same `id` and does not fan out again.
- **Reconnect recovery.** Clients supply their last-seen `sequence_no` in a
  `sync` event and the server replays missed messages from persisted history.
- **Size and rate limits.** Frames are limited to 16 KiB; message sends are
  rate-limited per user (30 per minute).
- **Errors are non-fatal.** A malformed or rejected frame produces an `error`
  event; the socket stays open.

## Event catalogue

### Client → Server

| Type | Fields | Status | Phase |
| --- | --- | --- | --- |
| `ping` | — | Implemented | 3 |
| `message.send` | `request_id`, `conversation_id`, `client_msg_id`, `message_type`, `content` | Implemented | 3 |
| `typing` | `conversation_id`, `typing` (bool) | Implemented | 3 |
| `read` | `conversation_id`, `sequence_no` | Implemented | 3 |
| `sync` | `request_id`, `conversation_id`, `after` (sequence_no) | Implemented | 3 |

### Server → Client

| Type | Fields | Status | Phase |
| --- | --- | --- | --- |
| `hello` | `user_id`, `server_time` | Implemented | 3 |
| `pong` | — | Implemented | 3 |
| `message.created` | `event_id`, `conversation_id`, `message` | Implemented | 3 |
| `message.ack` | `request_id`, `conversation_id`, `message` | Implemented | 3 |
| `message.receipt` | `conversation_id`, `user_id`, `status` (`delivered`\|`read`), `up_to_sequence` | Implemented | 3 |
| `typing` | `conversation_id`, `user_id`, `typing` | Implemented | 3 |
| `sync.result` | `request_id`, `conversation_id`, `messages`, `next_cursor` | Implemented | 3 |
| `presence` | `user_id`, `online` | Implemented | 3 |
| `read.ack` | `request_id`, `conversation_id`, `unread_count` | Implemented | 3 |
| `error` | `code`, `message`, `request_id` | Implemented | 3 |
| `conversation.updated` | `conversation_id`, `title`, `avatar_url`, `member_count` | Implemented | 4 |
| `member.changed` | `conversation_id`, `user_id`, `role`, `action` (`added`\|`removed`\|`left`\|`role`) | Implemented | 4 |
| `message.updated` | `message` | Planned | 3 |
| `message.deleted` | `message_id` | Planned | 3 |
| `call.invite` / `call.answer` / `call.candidate` | — | Planned | 6 |
| `notification.created` | — | Planned | 7 |

Status values: **Planned** (designed, not implemented), **In progress**, and
**Implemented** (wired and tested).

### Group events (Phase 4)

`conversation.updated` is published to every member when a group's metadata
changes (title or avatar) or when its member count changes. `member.changed` is
published when a member is added, removed, leaves, or has their role changed;
a removed user also receives the `removed` event so their client can drop the
conversation. Both are published only to current members (plus the departing
user for `member.changed`), and never for direct conversations.

System messages (for example, "Alice added Bob.") arrive as ordinary
`message.created` events with `message_type: "system"`.

## Error codes

`error.code` mirrors the REST error codes: `VALIDATION_ERROR`, `NOT_FOUND`,
`FORBIDDEN`, `RATE_LIMITED`, `INTERNAL_ERROR`, plus socket-specific
`INVALID_PAYLOAD` and `UNKNOWN_EVENT`.
