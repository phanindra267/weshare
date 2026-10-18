package calls

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/auth"
	"github.com/phanindra267/weshare/services/api/internal/config"
	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// Handler exposes the /calls HTTP surface. Live call control happens over the
// realtime socket; the REST surface only serves ICE configuration and history.
type Handler struct {
	svc *Service
	cfg *config.Config
}

// NewHandler builds a calls Handler.
func NewHandler(svc *Service, cfg *config.Config) *Handler {
	return &Handler{svc: svc, cfg: cfg}
}

// RegisterRoutes mounts the calls routes under the given group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := rg.Group("/calls", requireAuth)

	g.GET("/ice", h.ICE)
	g.GET("", h.History)
}

// ICEConfigResponse is the client's WebRTC bootstrap configuration.
type ICEConfigResponse struct {
	ICEServers      []config.ICEServer `json:"ice_servers"`
	MaxParticipants int                `json:"max_participants"`
}

// ICE returns the STUN/TURN configuration clients should use, plus the scoped
// participant ceiling for a single call.
func (h *Handler) ICE(c *gin.Context) {
	_, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	servers := h.cfg.ICEServers
	if servers == nil {
		servers = []config.ICEServer{}
	}
	httpx.OK(c, ICEConfigResponse{
		ICEServers:      servers,
		MaxParticipants: h.svc.MaxParticipants(),
	})
}

// History lists the caller's recent calls, newest first.
func (h *Handler) History(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	list, err := h.svc.History(c.Request.Context(), user.ID, limit)
	if err != nil {
		httpx.Error(c, httpx.AsAppError(err))
		return
	}
	httpx.OK(c, httpx.NewPaginated(toCallResponses(list, user.ID, h.svc.now()), ""))
}
