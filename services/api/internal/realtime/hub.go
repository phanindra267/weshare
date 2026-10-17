package realtime

import (
	"encoding/json"
	"sync"

	"github.com/google/uuid"
)

// Hub tracks live WebSocket connections grouped by user and fans events out to
// every connection a user has open. It implements messaging.Publisher, so the
// messaging service can push without knowing about the transport.
type Hub struct {
	mu    sync.RWMutex
	conns map[uuid.UUID]map[*conn]struct{}
}

// NewHub builds an empty Hub.
func NewHub() *Hub {
	return &Hub{conns: map[uuid.UUID]map[*conn]struct{}{}}
}

// add registers a connection.
func (h *Hub) add(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.conns[c.userID]
	if set == nil {
		set = map[*conn]struct{}{}
		h.conns[c.userID] = set
	}
	set[c] = struct{}{}
}

// remove deregisters a connection and drops the user's entry when it was their
// last one.
func (h *Hub) remove(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.conns[c.userID]
	if set == nil {
		return
	}
	delete(set, c)
	if len(set) == 0 {
		delete(h.conns, c.userID)
	}
}

// PublishToUser marshals event and queues it on every connection the user has.
// It reports whether the user had at least one live connection, which the
// messaging service uses to advance delivery state.
func (h *Hub) PublishToUser(userID uuid.UUID, event any) bool {
	payload, err := json.Marshal(event)
	if err != nil {
		return false
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	set := h.conns[userID]
	if len(set) == 0 {
		return false
	}
	for c := range set {
		c.enqueue(payload)
	}
	return true
}

// Online reports whether a user has at least one live connection.
func (h *Hub) Online(userID uuid.UUID) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns[userID]) > 0
}
