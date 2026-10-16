# WeShare WebSocket Events

This document is the source of truth for the realtime protocol. It defines the
envelope, conventions, and the event catalogue. The catalogue grows as phases
are implemented; events listed as **Implemented** are actually wired and tested.

The realtime layer is introduced in **Phase 3** (one-to-one messaging). Phase 0
establishes the conventions below so later work does not diverge.

## Connection

- Authenticated endpoint: `GET /api/v1/realtime` with WebSocket upgrade.
- Authentication uses the same HTTP-only session cookie as the REST API
  (introduced in Phase 1). The session is revalidated on every reconnect.
- The server checks the `Origin` header against the configured CORS allow-list.
- Conversation membership is checked before any event is relayed to a client.

## Envelope

All messages, in both directions, are JSON objects with a `type` discriminator.

### Client → Server

```json
{
  "type": "message.send",
  "request_id": "req-example",
  "conversation_id": "conversation-id",
  "client_msg_id": "client-generated-unique-id",
  "message_type": "text",
  "payload": { "text": "Hello!" }
}
```

- `request_id` — optional client-generated correlation id; the server echoes it
  in the acknowledgement so the client can resolve optimistic UI.
- `client_msg_id` — client-generated idempotency key. Re-sending the same
  `client_msg_id` must not create a duplicate message.

### Server → Client

```json
{
  "type": "message.created",
  "event_id": "server-event-id",
  "conversation_id": "conversation-id",
  "message": {
    "id": "server-message-id",
    "client_msg_id": "client-generated-unique-id",
    "sender_id": "sender-user-id",
    "sequence_no": 1,
    "message_type": "text",
    "payload": { "text": "Hello!" }
  }
}
```

- `event_id` — unique, monotonic-ish server event identifier used for
  de-duplication on the client.
- `sequence_no` — per-conversation ordering assigned by the server. The server
  is authoritative for ordering.

## Conventions

- **Enums are explicit.** `message_type` is validated against a closed set.
- **Payloads are validated.** Arbitrary JSON is rejected.
- **Persistence before broadcast.** A message is persisted before the
  `message.created` event is published.
- **Idempotency.** `client_msg_id` deduplicates retries.
- **Reconnect recovery.** Clients supply their last known cursor and the server
  replays missed events from persisted history.
- **Size and rate limits.** Event size, input size, and send frequency are
  bounded.

## Event catalogue

| Type | Direction | Status | Phase |
| --- | --- | --- | --- |
| `ping` / `pong` | both | Planned | 3 |
| `message.send` | C→S | Planned | 3 |
| `message.created` | S→C | Planned | 3 |
| `message.updated` | S→C | Planned | 3 |
| `message.deleted` | S→C | Planned | 3 |
| `message.receipt` | both | Planned | 3 |
| `typing.start` / `typing.stop` | both | Planned | 3 |
| `presence.update` | S→C | Planned | 2 |
| `call.invite` / `call.answer` / `call.candidate` | both | Planned | 6 |
| `notification.created` | S→C | Planned | 7 |

Status values: **Planned** (designed, not implemented), **In progress**, and
**Implemented** (wired and tested).
