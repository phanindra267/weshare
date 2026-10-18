package realtime

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/phanindra267/weshare/services/api/internal/auth"
	"github.com/phanindra267/weshare/services/api/internal/calls"
	"github.com/phanindra267/weshare/services/api/internal/config"
	"github.com/phanindra267/weshare/services/api/internal/httpx"
	"github.com/phanindra267/weshare/services/api/internal/messaging"
)

// Contacts supplies a user's friend ids for presence fan-out. It is satisfied
// by contacts.Store.
type Contacts interface {
	FriendIDs(ctx context.Context, me uuid.UUID) ([]uuid.UUID, error)
}

// Handler upgrades authenticated requests to WebSocket connections and drives
// the realtime protocol.
type Handler struct {
	svc      *messaging.Service
	calls    *calls.Service
	hub      *Hub
	contacts Contacts
	cfg      *config.Config
	logger   *slog.Logger
	upgrader websocket.Upgrader
}

// NewHandler builds a realtime Handler. contacts may be nil, in which case
// presence updates are not fanned out to friends. calls may be nil, in which
// case call frames are rejected.
func NewHandler(svc *messaging.Service, callsSvc *calls.Service, hub *Hub, contacts Contacts, cfg *config.Config, logger *slog.Logger) *Handler {
	h := &Handler{
		svc:      svc,
		calls:    callsSvc,
		hub:      hub,
		contacts: contacts,
		cfg:      cfg,
		logger:   logger,
	}
	h.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		// Browsers do not apply CORS to WebSocket handshakes, so origin
		// checking here is the CSRF-equivalent defense for the socket.
		CheckOrigin: h.originAllowed,
	}
	return h
}

// RegisterRoutes mounts the realtime endpoint under the group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	rg.GET("/realtime", requireAuth, h.Serve)
}

// originAllowed reports whether the handshake Origin is permitted. Requests
// with no Origin (native clients, tests) are allowed; browser origins must be
// in the configured CORS allow-list.
func (h *Handler) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	allowed := h.cfg.CORSAllowedOrigins
	if len(allowed) == 1 && allowed[0] == "*" {
		return !strings.Contains(origin, "*")
	}
	for _, candidate := range allowed {
		if strings.EqualFold(strings.TrimSpace(candidate), origin) {
			return true
		}
	}
	return false
}

// Serve upgrades the request and runs the connection until it closes.
func (h *Handler) Serve(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}

	ws, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Upgrade already wrote an HTTP error response.
		h.logger.Warn("realtime: upgrade failed", "error", err.Error())
		return
	}

	conn := newConn(h.hub, ws, h.svc, h.calls, user.ID, h.logger)
	h.hub.add(conn)
	go conn.writePump()

	// Flush delivery for messages that arrived while the user was away and
	// tell the socket the connection is live.
	conn.enqueueEvent(messaging.HelloEvent{
		Type:       messaging.EventHello,
		UserID:     user.ID.String(),
		ServerTime: time.Now().UTC().Format(time.RFC3339),
	})
	if err := h.svc.HandleConnect(context.Background(), user.ID); err != nil {
		h.logger.Warn("realtime: handle connect failed", "error", err.Error())
	}
	h.broadcastPresence(user.ID, true)

	conn.readPump()

	h.hub.remove(conn)
	conn.close()
	// Only announce offline once the user's last connection is gone.
	if !h.hub.Online(user.ID) {
		h.broadcastPresence(user.ID, false)
		// A user whose last socket just closed cannot stay on a call.
		if h.calls != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			h.calls.HandleDisconnect(ctx, user.ID)
		}
	}
}

// broadcastPresence tells a user's friends that they came online or went
// offline, and (on connect) tells the user which friends are currently online.
func (h *Handler) broadcastPresence(userID uuid.UUID, online bool) {
	if h.contacts == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	friends, err := h.contacts.FriendIDs(ctx, userID)
	if err != nil {
		h.logger.Warn("realtime: friend ids failed", "error", err.Error())
		return
	}

	event := messaging.PresenceEvent{
		Type:   messaging.EventPresence,
		UserID: userID.String(),
		Online: online,
	}
	for _, friend := range friends {
		h.hub.PublishToUser(friend, event)
	}

	if !online {
		return
	}
	// Seed the newly connected user with the current presence of their friends.
	for _, friend := range friends {
		if h.hub.Online(friend) {
			h.hub.PublishToUser(userID, messaging.PresenceEvent{
				Type:   messaging.EventPresence,
				UserID: friend.String(),
				Online: true,
			})
		}
	}
}
