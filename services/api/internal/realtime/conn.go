package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/phanindra267/weshare/services/api/internal/calls"
	"github.com/phanindra267/weshare/services/api/internal/messaging"
)

// WebSocket timing and sizing. pongWait must exceed pingPeriod so a healthy
// peer always has a chance to reply before the read deadline expires.
const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 16 * 1024
	sendBuffer     = 64
)

// conn is one live WebSocket connection for one authenticated user.
type conn struct {
	hub    *Hub
	ws     *websocket.Conn
	svc    *messaging.Service
	calls  *calls.Service
	userID uuid.UUID
	logger *slog.Logger

	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once
}

func newConn(hub *Hub, ws *websocket.Conn, svc *messaging.Service, callsSvc *calls.Service, userID uuid.UUID, logger *slog.Logger) *conn {
	return &conn{
		hub:    hub,
		ws:     ws,
		svc:    svc,
		calls:  callsSvc,
		userID: userID,
		logger: logger,
		send:   make(chan []byte, sendBuffer),
		done:   make(chan struct{}),
	}
}

// enqueue queues a pre-serialized frame. A connection whose buffer is full is
// closed rather than blocking the publisher: a slow consumer must never stall
// everyone else's delivery.
func (c *conn) enqueue(payload []byte) {
	select {
	case <-c.done:
		return
	case c.send <- payload:
	default:
		c.close()
	}
}

// enqueueEvent serializes and queues an event; marshal failures are logged and
// dropped since there is nothing useful to tell the client.
func (c *conn) enqueueEvent(event any) {
	payload, err := json.Marshal(event)
	if err != nil {
		c.logger.Warn("realtime: marshal event failed", "error", err.Error())
		return
	}
	c.enqueue(payload)
}

// close signals the write pump to stop and shuts the socket. It is safe to call
// more than once.
func (c *conn) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.ws.Close()
	})
}

// writePump owns all writes to the socket, serializing frames from the send
// channel and emitting periodic pings.
func (c *conn) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case payload := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.done:
			return
		}
	}
}

// readPump owns all reads. It returns when the peer disconnects or the socket
// errors; the caller then deregisters the connection.
func (c *conn) readPump() {
	c.ws.SetReadLimit(maxMessageSize)
	_ = c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		c.handle(data)
	}
}

// clientMessage is the union of fields across client-originated events. Unknown
// fields are ignored so the protocol can grow.
type clientMessage struct {
	Type           string          `json:"type"`
	RequestID      string          `json:"request_id"`
	ConversationID string          `json:"conversation_id"`
	ClientMsgID    string          `json:"client_msg_id"`
	MessageType    string          `json:"message_type"`
	Content        json.RawMessage `json:"content"`
	ReplyToID      string          `json:"reply_to_id"`
	Typing         *bool           `json:"typing"`
	SequenceNo     *int64          `json:"sequence_no"`
	After          *int64          `json:"after"`

	// Calling (Phase 9).
	CallID    string          `json:"call_id"`
	CalleeIDs []string        `json:"callee_ids"`
	Kind      string          `json:"kind"`
	TargetID  string          `json:"target_id"`
	SDP       json.RawMessage `json:"sdp"`
	Candidate json.RawMessage `json:"candidate"`
}

// handle dispatches one client frame. Errors are reported per-request without
// closing the socket so a malformed frame does not drop the connection.
func (c *conn) handle(data []byte) {
	var msg clientMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		c.sendError("", "INVALID_PAYLOAD", "Message is not valid JSON.")
		return
	}

	switch msg.Type {
	case messaging.ClientPing:
		c.enqueueEvent(struct {
			Type string `json:"type"`
		}{Type: messaging.EventPong})
	case messaging.ClientMessageSend:
		c.handleSend(msg)
	case messaging.ClientTyping:
		c.handleTyping(msg)
	case messaging.ClientRead:
		c.handleRead(msg)
	case messaging.ClientSync:
		c.handleSync(msg)
	case calls.ClientInvite, calls.ClientAccept, calls.ClientReject,
		calls.ClientEnd, calls.ClientSDP, calls.ClientCandidate:
		c.handleCall(msg)
	default:
		c.sendError(msg.RequestID, "UNKNOWN_EVENT", "Unsupported event type.")
	}
}

// handleCall dispatches a calling frame to the calls service. The service fans
// the resulting events out through the hub, so nothing is returned here.
func (c *conn) handleCall(msg clientMessage) {
	if c.calls == nil {
		c.sendError(msg.RequestID, "UNKNOWN_EVENT", "Calling is not available.")
		return
	}
	err := c.calls.Handle(context.Background(), c.userID, calls.ClientFrame{
		Type:      msg.Type,
		CallID:    msg.CallID,
		CalleeIDs: msg.CalleeIDs,
		Kind:      msg.Kind,
		TargetID:  msg.TargetID,
		SDP:       msg.SDP,
		Candidate: msg.Candidate,
	})
	if err != nil {
		c.sendCallError(msg.RequestID, err)
	}
}

// handleSend persists an outgoing message and acks it. Fan-out to members
// happens inside the service, which publishes through the same hub.
func (c *conn) handleSend(msg clientMessage) {
	convID, err := uuid.Parse(msg.ConversationID)
	if err != nil {
		c.sendError(msg.RequestID, "VALIDATION_ERROR", "conversation_id is required.")
		return
	}

	in := &messaging.SendInput{
		ClientMsgID: msg.ClientMsgID,
		MessageType: msg.MessageType,
		Content:     msg.Content,
		ReplyToID:   msg.ReplyToID,
	}
	created, _, err := c.svc.SendMessage(context.Background(), c.userID, convID, in)
	if err != nil {
		c.sendMessagingError(msg.RequestID, err)
		return
	}

	c.enqueueEvent(messaging.MessageAckEvent{
		Type:           messaging.EventMessageAck,
		ConversationID: convID.String(),
		RequestID:      msg.RequestID,
		Message:        messaging.ToMessageResponseFor(created, c.userID),
	})
}

// handleTyping relays a transient typing indicator to the other members. It is
// never persisted.
func (c *conn) handleTyping(msg clientMessage) {
	convID, err := uuid.Parse(msg.ConversationID)
	if err != nil {
		c.sendError(msg.RequestID, "VALIDATION_ERROR", "conversation_id is required.")
		return
	}
	members, err := c.svc.Members(context.Background(), c.userID, convID)
	if err != nil {
		c.sendMessagingError(msg.RequestID, err)
		return
	}

	typing := true
	if msg.Typing != nil {
		typing = *msg.Typing
	}
	event := messaging.TypingEvent{
		Type:           messaging.EventTyping,
		ConversationID: convID.String(),
		UserID:         c.userID.String(),
		Typing:         typing,
	}
	for _, member := range members {
		if member == c.userID {
			continue
		}
		c.hub.PublishToUser(member, event)
	}
}

// handleRead advances the caller's read position and notifies senders.
func (c *conn) handleRead(msg clientMessage) {
	convID, err := uuid.Parse(msg.ConversationID)
	if err != nil {
		c.sendError(msg.RequestID, "VALIDATION_ERROR", "conversation_id is required.")
		return
	}
	if msg.SequenceNo == nil {
		c.sendError(msg.RequestID, "VALIDATION_ERROR", "sequence_no is required.")
		return
	}

	unread, err := c.svc.MarkRead(context.Background(), c.userID, convID, *msg.SequenceNo)
	if err != nil {
		c.sendMessagingError(msg.RequestID, err)
		return
	}
	c.enqueueEvent(struct {
		Type           string `json:"type"`
		ConversationID string `json:"conversation_id"`
		RequestID      string `json:"request_id,omitempty"`
		UnreadCount    int64  `json:"unread_count"`
	}{
		Type:           "read.ack",
		ConversationID: convID.String(),
		RequestID:      msg.RequestID,
		UnreadCount:    unread,
	})
}

// handleSync returns messages newer than the client's last-seen sequence so a
// reconnecting client can catch up.
func (c *conn) handleSync(msg clientMessage) {
	convID, err := uuid.Parse(msg.ConversationID)
	if err != nil {
		c.sendError(msg.RequestID, "VALIDATION_ERROR", "conversation_id is required.")
		return
	}

	after := int64(0)
	if msg.After != nil {
		after = *msg.After
	}

	items, next, err := c.svc.SyncMessages(context.Background(), c.userID, convID, after)
	if err != nil {
		c.sendMessagingError(msg.RequestID, err)
		return
	}

	resp := make([]messaging.MessageResponse, 0, len(items))
	for _, m := range items {
		resp = append(resp, messaging.ToMessageResponseFor(m, c.userID))
	}
	c.enqueueEvent(messaging.SyncResultEvent{
		Type:           messaging.EventSync,
		ConversationID: convID.String(),
		RequestID:      msg.RequestID,
		Messages:       resp,
		NextCursor:     next,
	})
}

// sendError emits an error frame for a specific request.
func (c *conn) sendError(requestID, code, message string) {
	c.enqueueEvent(messaging.ErrorEvent{
		Type:      messaging.EventError,
		Code:      code,
		Message:   message,
		RequestID: requestID,
	})
}

// sendMessagingError maps a domain error onto a socket error frame. The status
// code mirrors the REST layer so clients can handle both identically.
func (c *conn) sendMessagingError(requestID string, err error) {
	code, message := classifyError(err)
	c.sendError(requestID, code, message)
}

// sendCallError maps a calls domain error onto a socket error frame.
func (c *conn) sendCallError(requestID string, err error) {
	code, message := classifyCallError(err)
	c.sendError(requestID, code, message)
}
