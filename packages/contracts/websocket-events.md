# WeShare WebSocket Events

This document is the source of truth for the realtime protocol. It defines the
envelope, conventions, and the event catalogue. The catalogue grows as phases
are implemented; events listed as **Implemented** are wired and tested.

The realtime layer was introduced in **Phase 3** (one-to-one messaging),
extended in **Phase 4** (group lifecycle events and richer message content),
and carries WebRTC call signalling since **Phase 9**.

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
| `moment.created` | `moment` | Implemented | 7 |
| `message.updated` | `message` | Planned | 4 |
| `message.deleted` | `message_id` | Planned | 4 |
| `notification.created` | — | Planned | — |

Calling (Phase 9) has its own catalogue below, since it carries a second
set of client and server events.

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

### Moments (Phase 7)

`moment.created` is published to the author's accepted contacts when a moment
is created with `friends` visibility, and to nobody else for `private`. Public
moments are still returned by the paginated `GET /api/v1/moments` feed for any
authenticated caller, but are not broadcast, so the feed remains the source of
truth. The `moment` payload matches `#/components/schemas/Moment` in the OpenAPI
contract (author, body, media, like/comment counts, `liked_by_me`). Likes and
comments do not currently emit realtime events; clients refresh counts on
demand.

### Calling events (Phase 9)

WebRTC signalling rides this same socket. Media itself never touches the
server: peers exchange SDP and ICE candidates here and then stream directly to
each other. Every calling frame carries a client-generated `call_id` (a UUID),
so the caller never waits for a round trip before rendering its UI.

#### Client → Server

| Type | Fields |
| --- | --- |
| `call.invite` | `call_id`, `callee_ids`, `kind` (`audio`\|`video`) |
| `call.accept` | `call_id` |
| `call.reject` | `call_id` |
| `call.end` | `call_id` |
| `call.sdp` | `call_id`, `target_id`, `sdp` |
| `call.candidate` | `call_id`, `target_id`, `candidate` |

#### Server → Client

| Type | Fields |
| --- | --- |
| `call.incoming` | `call_id`, `from` (profile brief), `kind`, `group`, `participants` |
| `call.accepted` | `call_id`, `user_id` |
| `call.rejected` | `call_id`, `user_id` |
| `call.busy` | `call_id`, `user_id` |
| `call.unavailable` | `call_id`, `user_id` |
| `call.sdp` | `call_id`, `from_user_id`, `sdp` |
| `call.candidate` | `call_id`, `from_user_id`, `candidate` |
| `call.ended` | `call_id`, `reason`, `user_id?` |
| `call.participants` | `call_id`, `participants` (profile briefs) |

#### Rules

- **Contacts only.** You may only call users you are connected with; anyone else
  gets `FORBIDDEN`.
- **One call per user.** A second `call.invite` while already in a call fails
  with `FORBIDDEN` on the caller's side, and a callee who is already busy makes
  the caller receive `call.busy`. If *no* callee can be reached — every one of
  them busy — no call is created and the caller also receives `call.ended` with
  `reason: "failed"`, so an unanswerable call never rings forever.
- **Offline callee.** If no callee has a live socket the call fails
  immediately: the caller receives `call.unavailable` followed by `call.ended`
  with `reason: "failed"`, and the row is stored as `failed`.
- **Glare-free offers.** A participant who is already joined creates the SDP
  offer towards a newly joined one; a joiner only ever answers. For a 1:1 call
  this means the caller (joined at invite time) sends the offer once the callee
  accepts, so neither side has to re-negotiate on collision.
- **End semantics.** A direct call ends as a whole — everyone gets `call.ended`
  with `reason: "hangup"` (it was active), `"cancelled"` (the caller hung up
  while it was still ringing), or `"rejected"`, and the row is closed out. In a
  group call, ending is per-participant: only `reason: "left"` is sent, and the
  call closes once nobody remains.
- **Disconnect cleanup.** When a user's last socket closes they are dropped from
  any active call, so an abandoned tab cannot leave a call ringing forever.
- **History and bootstrap.** Call history is served by `GET /api/v1/calls`, and
  the STUN/TURN servers plus the participant ceiling by `GET /api/v1/calls/ice`.

#### Scalability limit (P2P vs SFU)

Media flows peer-to-peer. A 1:1 call needs one connection per side; a group call
of *n* participants needs each client to hold *n − 1* connections (a full
mesh). That is fine up to the enforced `CALL_MAX_PARTICIPANTS` ceiling, but it
is not a path to large rooms — past a handful of participants the bandwidth and
CPU cost lands on the *weakest* client and grows linearly. Scaling further
requires a Selective Forwarding Unit that terminates media on the server, which
is deliberately out of scope: it is infrastructure rather than application
code, and it would contradict the "media never touches the server" property
this phase is built on.

## Error codes

`error.code` mirrors the REST error codes: `VALIDATION_ERROR`, `NOT_FOUND`,
`FORBIDDEN`, `RATE_LIMITED`, `INTERNAL_ERROR`, plus socket-specific
`INVALID_PAYLOAD` and `UNKNOWN_EVENT`. Calling frames report failures this way
too, with `request_id` echoed when the client supplied one, so an unsupported
call (not connected, already busy, call gone) is reported on the socket that
asked for it rather than tearing the connection down.
