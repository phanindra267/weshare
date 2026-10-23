package messaging

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/auth"
	"github.com/phanindra267/weshare/services/api/internal/config"
	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// Handler exposes the messaging REST surface.
type Handler struct {
	svc *Service
	cfg *config.Config
	// sendLimiter throttles message sends per user so a runaway client cannot
	// flood a conversation.
	sendLimiter *auth.Limiter
}

// NewHandler builds a messaging Handler.
func NewHandler(svc *Service, cfg *config.Config) *Handler {
	return &Handler{
		svc:         svc,
		cfg:         cfg,
		sendLimiter: auth.NewLimiter(30, time.Minute),
	}
}

// RegisterRoutes mounts the conversation and message routes under the group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := rg.Group("/conversations", requireAuth)

	g.GET("", h.ListConversations)
	g.POST("", h.OpenConversation)
	g.POST("/group", h.CreateGroup)
	g.GET("/:id", h.GetConversation)
	g.PATCH("/:id", h.UpdateGroup)
	g.GET("/:id/messages", h.ListMessages)
	g.GET("/:id/messages/search", h.SearchMessages)
	g.POST("/:id/messages", h.sendRateLimit(), h.SendMessage)
	g.POST("/:id/messages/forward", h.sendRateLimit(), h.ForwardMessage)
	g.PATCH("/:id/messages/:messageId", h.EditMessage)
	g.DELETE("/:id/messages/:messageId", h.DeleteMessage)
	g.PUT("/:id/messages/:messageId/pin", h.PinMessage)
	g.DELETE("/:id/messages/:messageId/pin", h.UnpinMessage)
	g.PUT("/:id/messages/:messageId/reactions", h.AddReaction)
	g.DELETE("/:id/messages/:messageId/reactions", h.RemoveReaction)
	g.GET("/:id/pins", h.ListPins)
	g.POST("/:id/read", h.MarkRead)
	g.GET("/:id/members", h.GetMembers)
	g.POST("/:id/members", h.AddMembers)
	g.DELETE("/:id/members/:userId", h.RemoveMember)
	g.POST("/:id/members/:userId/role", h.SetMemberRole)
	g.POST("/:id/leave", h.LeaveGroup)
}

// online reports presence using the configured staleness window.
func (h *Handler) online(lastSeen *time.Time) bool { return h.cfg.IsOnline(lastSeen) }

// sendRateLimit rejects message sends over the per-user allowance with 429.
func (h *Handler) sendRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := auth.CurrentUser(c)
		if !ok {
			httpx.Error(c, httpx.Unauthorized(""))
			c.Abort()
			return
		}
		if !h.sendLimiter.Allow(user.ID.String()) {
			httpx.Error(c, httpx.RateLimited("You are sending messages too quickly."))
			c.Abort()
			return
		}
		c.Next()
	}
}

// writeError maps a domain error onto the standard error envelope.
func writeError(c *gin.Context, err error, notFoundMessage string) {
	var invalid *ValidationError
	switch {
	case errors.As(err, &invalid):
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": invalid.Fields}))
	case errors.Is(err, ErrConversationNotFound), errors.Is(err, ErrNotMember), errors.Is(err, ErrNotGroup):
		httpx.Error(c, httpx.NotFound(notFoundMessage))
	case errors.Is(err, ErrPeerNotFound):
		httpx.Error(c, httpx.NotFound("That user does not exist."))
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrPinForbidden):
		httpx.Error(c, httpx.Forbidden("You do not have permission to do that."))
	case errors.Is(err, ErrMessageNotFound), errors.Is(err, ErrCannotEditDeleted):
		httpx.Error(c, httpx.NotFound("That message is no longer available."))
	case errors.Is(err, ErrEditWindowClosed):
		httpx.Error(c, httpx.Forbidden("This message can no longer be edited."))
	case errors.Is(err, ErrNotEditableType), errors.Is(err, ErrNotForwardable), errors.Is(err, ErrNotReactable):
		httpx.Error(c, httpx.Validation("That action is not available for this message.",
			gin.H{"fields": map[string]string{"message_id": "Unsupported for this message."}}))
	case errors.Is(err, ErrInvalidEmoji):
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": map[string]string{"emoji": "Invalid reaction."}}))
	case errors.Is(err, ErrEmptySearch):
		httpx.Error(c, httpx.Validation("Enter something to search for.",
			gin.H{"fields": map[string]string{"q": "A search query is required."}}))
	case errors.Is(err, ErrOwnerImmutable):
		httpx.Error(c, httpx.Forbidden("The group owner cannot be changed."))
	case errors.Is(err, ErrNotConnected):
		httpx.Error(c, httpx.Forbidden("You can only add people you are connected with."))
	case errors.Is(err, ErrSelfConversation):
		httpx.Error(c, httpx.Validation("You cannot message your own account.",
			gin.H{"fields": map[string]string{"user_id": "You cannot message yourself."}}))
	case errors.Is(err, ErrBlocked):
		httpx.Error(c, httpx.Forbidden("You cannot message this user."))
	case errors.Is(err, ErrEmptyMessage):
		httpx.Error(c, httpx.Validation("Message cannot be empty.",
			gin.H{"fields": map[string]string{"content": "Message cannot be empty."}}))
	case errors.Is(err, ErrMessageTooLarge):
		httpx.Error(c, httpx.Validation("Message is too large.",
			gin.H{"fields": map[string]string{"content": "Message is too large."}}))
	case errors.Is(err, ErrUnsupportedType):
		httpx.Error(c, httpx.Validation("Unsupported message type.",
			gin.H{"fields": map[string]string{"message_type": "Unsupported message type."}}))
	case errors.Is(err, ErrInvalidSequence):
		httpx.Error(c, httpx.Validation("Invalid sequence number.",
			gin.H{"fields": map[string]string{"sequence_no": "Invalid sequence number."}}))
	case errors.Is(err, ErrRateLimited):
		httpx.Error(c, httpx.RateLimited("You are sending messages too quickly."))
	default:
		httpx.Error(c, httpx.AsAppError(err))
	}
}

// parseID reads a UUID path parameter, answering 404 when malformed.
func parseID(c *gin.Context, name, notFoundMessage string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		httpx.Error(c, httpx.NotFound(notFoundMessage))
		return uuid.Nil, false
	}
	return id, true
}

// parseMessageQuery reads `limit` plus `before`/`after` cursors. A malformed
// value yields a 422 with field details.
func parseMessageQuery(c *gin.Context) (MessageQuery, bool) {
	var q MessageQuery
	fields := map[string]string{}

	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			fields["limit"] = "limit must be a positive integer."
		} else {
			q.Limit = n
		}
	}
	if raw := strings.TrimSpace(c.Query("before")); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			fields["before"] = "before must be a non-negative integer."
		} else {
			q.Before = &n
		}
	}
	if raw := strings.TrimSpace(c.Query("after")); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			fields["after"] = "after must be a non-negative integer."
		} else {
			q.After = &n
		}
	}
	if q.Before != nil && q.After != nil {
		fields["after"] = "Use either before or after, not both."
	}

	if len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": fields}))
		return MessageQuery{}, false
	}
	return q, true
}

// ListConversations handles GET /conversations.
func (h *Handler) ListConversations(c *gin.Context) {
	fields := map[string]string{}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			fields["limit"] = "limit must be a positive integer."
		} else {
			limit = n
		}
	}
	var after *ConversationCursor
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		cur, err := ParseConversationCursor(raw)
		if err != nil {
			fields["cursor"] = "cursor is not valid."
		} else {
			after = &cur
		}
	}
	if len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": fields}))
		return
	}

	user, _ := auth.CurrentUser(c)
	items, next, err := h.svc.ListConversations(c.Request.Context(), user.ID, after, limit)
	if err != nil {
		writeError(c, err, "Conversations could not be loaded.")
		return
	}

	resp := make([]ConversationResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toConversationResponse(item, h.online(item.Peer.LastSeenAt)))
	}
	httpx.OK(c, httpx.NewPaginated(resp, next))
}

// OpenConversation handles POST /conversations, opening a direct conversation
// with a given user (creating it on first contact).
func (h *Handler) OpenConversation(c *gin.Context) {
	var body openConversationInput
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}
	if fields := body.Validate(); len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": fields}))
		return
	}
	peer, _ := uuid.Parse(body.UserID)

	user, _ := auth.CurrentUser(c)
	summary, err := h.svc.OpenConversation(c.Request.Context(), user.ID, peer)
	if err != nil {
		writeError(c, err, "Could not open this conversation.")
		return
	}
	httpx.OK(c, toConversationResponse(summary, h.online(summary.Peer.LastSeenAt)))
}

// GetConversation handles GET /conversations/:id.
func (h *Handler) GetConversation(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}

	user, _ := auth.CurrentUser(c)
	summary, err := h.svc.GetConversation(c.Request.Context(), user.ID, id)
	if err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}
	httpx.OK(c, toConversationResponse(summary, h.online(summary.Peer.LastSeenAt)))
}

// ListMessages handles GET /conversations/:id/messages.
func (h *Handler) ListMessages(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}
	q, ok := parseMessageQuery(c)
	if !ok {
		return
	}

	user, _ := auth.CurrentUser(c)
	items, next, err := h.svc.ListMessages(c.Request.Context(), user.ID, id, q)
	if err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}

	resp := make([]MessageResponse, 0, len(items))
	for _, m := range items {
		resp = append(resp, ToMessageResponseFor(m, user.ID))
	}
	httpx.OK(c, httpx.NewPaginated(resp, next))
}

// SendMessage handles POST /conversations/:id/messages.
func (h *Handler) SendMessage(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}

	var body SendInput
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}

	user, _ := auth.CurrentUser(c)
	msg, duplicate, err := h.svc.SendMessage(c.Request.Context(), user.ID, id, &body)
	if err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}

	if duplicate {
		// The message already existed; return it without claiming a new one.
		httpx.OK(c, ToMessageResponseFor(msg, user.ID))
		return
	}
	httpx.Created(c, ToMessageResponseFor(msg, user.ID))
}

// MarkRead handles POST /conversations/:id/read.
func (h *Handler) MarkRead(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}

	var body markReadInput
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}
	if fields := body.Validate(); len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": fields}))
		return
	}

	user, _ := auth.CurrentUser(c)
	unread, err := h.svc.MarkRead(c.Request.Context(), user.ID, id, *body.SequenceNo)
	if err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}
	httpx.OK(c, gin.H{"unread_count": unread})
}

// parseIDList converts validated UUID strings into uuid.UUID values.
func parseIDList(ids []string) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ids))
	for _, raw := range ids {
		id, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out
}

// CreateGroup handles POST /conversations/group.
func (h *Handler) CreateGroup(c *gin.Context) {
	var body createGroupInput
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}
	if fields := body.Validate(); len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": fields}))
		return
	}

	user, _ := auth.CurrentUser(c)
	summary, err := h.svc.CreateGroup(c.Request.Context(), user.ID, body.Title, parseIDList(body.MemberIDs))
	if err != nil {
		writeError(c, err, "Could not create the group.")
		return
	}
	httpx.Created(c, toConversationResponse(summary, false))
}

// UpdateGroup handles PATCH /conversations/:id.
func (h *Handler) UpdateGroup(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}

	var body updateGroupInput
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}
	if fields := body.Validate(); len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": fields}))
		return
	}

	user, _ := auth.CurrentUser(c)
	summary, err := h.svc.UpdateGroup(c.Request.Context(), user.ID, id, body.Title, body.AvatarURL)
	if err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}
	httpx.OK(c, toConversationResponse(summary, false))
}

// GetMembers handles GET /conversations/:id/members.
func (h *Handler) GetMembers(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}

	user, _ := auth.CurrentUser(c)
	members, err := h.svc.GetMembers(c.Request.Context(), user.ID, id)
	if err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}

	resp := make([]MemberResponse, 0, len(members))
	for _, m := range members {
		resp = append(resp, toMemberResponse(m, h.online(m.LastSeenAt)))
	}
	httpx.OK(c, gin.H{"items": resp})
}

// AddMembers handles POST /conversations/:id/members.
func (h *Handler) AddMembers(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}

	var body addMembersInput
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}
	if fields := body.Validate(); len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": fields}))
		return
	}

	user, _ := auth.CurrentUser(c)
	added, err := h.svc.AddMembers(c.Request.Context(), user.ID, id, parseIDList(body.UserIDs))
	if err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}
	httpx.OK(c, gin.H{"added": added})
}

// RemoveMember handles DELETE /conversations/:id/members/:userId.
func (h *Handler) RemoveMember(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}
	target, ok := parseID(c, "userId", "Member not found.")
	if !ok {
		return
	}

	user, _ := auth.CurrentUser(c)
	if err := h.svc.RemoveMember(c.Request.Context(), user.ID, id, target); err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}
	httpx.NoContent(c)
}

// SetMemberRole handles POST /conversations/:id/members/:userId/role.
func (h *Handler) SetMemberRole(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}
	target, ok := parseID(c, "userId", "Member not found.")
	if !ok {
		return
	}

	var body setRoleInput
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}
	if fields := body.Validate(); len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": fields}))
		return
	}

	user, _ := auth.CurrentUser(c)
	if err := h.svc.SetMemberRole(c.Request.Context(), user.ID, id, target, body.Role); err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}
	httpx.NoContent(c)
}

// LeaveGroup handles POST /conversations/:id/leave.
func (h *Handler) LeaveGroup(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}

	user, _ := auth.CurrentUser(c)
	if err := h.svc.LeaveGroup(c.Request.Context(), user.ID, id); err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}
	httpx.NoContent(c)
}

// --- advanced messaging (Phase 4) -------------------------------------------

// EditMessage handles PATCH /conversations/:id/messages/:messageId.
func (h *Handler) EditMessage(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}
	messageID, ok := parseID(c, "messageId", "Message not found.")
	if !ok {
		return
	}
	var body editMessageInput
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}
	if fields := body.Validate(); len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": fields}))
		return
	}

	user, _ := auth.CurrentUser(c)
	msg, err := h.svc.EditMessage(c.Request.Context(), user.ID, id, messageID, body.Text)
	if err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}
	httpx.OK(c, ToMessageResponseFor(msg, user.ID))
}

// DeleteMessage handles DELETE /conversations/:id/messages/:messageId.
func (h *Handler) DeleteMessage(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}
	messageID, ok := parseID(c, "messageId", "Message not found.")
	if !ok {
		return
	}
	user, _ := auth.CurrentUser(c)
	msg, err := h.svc.DeleteMessage(c.Request.Context(), user.ID, id, messageID)
	if err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}
	httpx.OK(c, ToMessageResponseFor(msg, user.ID))
}

// SearchMessages handles GET /conversations/:id/messages/search.
func (h *Handler) SearchMessages(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}
	query := c.Query("q")
	if strings.TrimSpace(query) == "" {
		httpx.Error(c, httpx.Validation("Enter something to search for.",
			gin.H{"fields": map[string]string{"q": "A search query is required."}}))
		return
	}
	if len([]rune(query)) > maxSearchQueryLength {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": map[string]string{"q": "Search query is too long."}}))
		return
	}

	var before *int64
	if raw := strings.TrimSpace(c.Query("before")); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
				gin.H{"fields": map[string]string{"before": "before must be a non-negative integer."}}))
			return
		}
		before = &n
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
				gin.H{"fields": map[string]string{"limit": "limit must be a positive integer."}}))
			return
		}
		limit = n
	}

	user, _ := auth.CurrentUser(c)
	items, next, err := h.svc.SearchMessages(c.Request.Context(), user.ID, id, query, before, limit)
	if err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}
	resp := make([]MessageResponse, 0, len(items))
	for _, m := range items {
		resp = append(resp, ToMessageResponseFor(m, user.ID))
	}
	httpx.OK(c, httpx.NewPaginated(resp, next))
}

// ForwardMessage handles POST /conversations/:id/messages/forward, where :id is
// the destination conversation and the body names the source.
func (h *Handler) ForwardMessage(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}
	var body forwardInput
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}
	if fields := body.Validate(); len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": fields}))
		return
	}
	source, _ := uuid.Parse(body.SourceConversationID)
	messageID, _ := uuid.Parse(body.MessageID)

	user, _ := auth.CurrentUser(c)
	msg, err := h.svc.ForwardMessage(c.Request.Context(), user.ID, id, source, messageID)
	if err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}
	httpx.Created(c, ToMessageResponseFor(msg, user.ID))
}

// AddReaction handles PUT /conversations/:id/messages/:messageId/reactions.
func (h *Handler) AddReaction(c *gin.Context) { h.setReaction(c, true) }

// RemoveReaction handles DELETE /conversations/:id/messages/:messageId/reactions,
// reading the emoji from the `emoji` query parameter.
func (h *Handler) RemoveReaction(c *gin.Context) { h.setReaction(c, false) }

func (h *Handler) setReaction(c *gin.Context, add bool) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}
	messageID, ok := parseID(c, "messageId", "Message not found.")
	if !ok {
		return
	}

	var emoji string
	if add {
		var body reactionInput
		if err := httpx.BindJSON(c, &body); err != nil {
			httpx.Error(c, err)
			return
		}
		if fields := body.Validate(); len(fields) > 0 {
			httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
				gin.H{"fields": fields}))
			return
		}
		emoji = body.Emoji
	} else {
		input := reactionInput{Emoji: c.Query("emoji")}
		if fields := input.Validate(); len(fields) > 0 {
			httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
				gin.H{"fields": fields}))
			return
		}
		emoji = input.Emoji
	}

	user, _ := auth.CurrentUser(c)
	var (
		reactions []ReactionResponse
		err       error
	)
	if add {
		reactions, err = h.svc.AddReaction(c.Request.Context(), user.ID, id, messageID, emoji)
	} else {
		reactions, err = h.svc.RemoveReaction(c.Request.Context(), user.ID, id, messageID, emoji)
	}
	if err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}
	httpx.OK(c, gin.H{"message_id": messageID.String(), "reactions": reactions})
}

// PinMessage handles PUT /conversations/:id/messages/:messageId/pin.
func (h *Handler) PinMessage(c *gin.Context) { h.setPin(c, true) }

// UnpinMessage handles DELETE /conversations/:id/messages/:messageId/pin.
func (h *Handler) UnpinMessage(c *gin.Context) { h.setPin(c, false) }

func (h *Handler) setPin(c *gin.Context, pinned bool) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}
	messageID, ok := parseID(c, "messageId", "Message not found.")
	if !ok {
		return
	}
	user, _ := auth.CurrentUser(c)
	if err := h.svc.SetPin(c.Request.Context(), user.ID, id, messageID, pinned); err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}
	httpx.NoContent(c)
}

// ListPins handles GET /conversations/:id/pins.
func (h *Handler) ListPins(c *gin.Context) {
	id, ok := parseID(c, "id", "Conversation not found.")
	if !ok {
		return
	}
	user, _ := auth.CurrentUser(c)
	items, err := h.svc.PinnedMessages(c.Request.Context(), user.ID, id)
	if err != nil {
		writeError(c, err, "Conversation not found.")
		return
	}
	resp := make([]MessageResponse, 0, len(items))
	for _, m := range items {
		resp = append(resp, ToMessageResponseFor(m, user.ID))
	}
	httpx.OK(c, gin.H{"messages": resp})
}
