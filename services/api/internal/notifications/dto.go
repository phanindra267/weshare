package notifications

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// timeFormat is the RFC3339 shape used across the REST contract.
const timeFormat = time.RFC3339

// ActorResponse is the public profile of whoever triggered a notification.
type ActorResponse struct {
	ID          string  `json:"id"`
	Username    *string `json:"username"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

// NotificationResponse is the wire shape of one notification.
type NotificationResponse struct {
	ID             string         `json:"id"`
	Type           string         `json:"type"`
	Actor          *ActorResponse `json:"actor"`
	ConversationID *string        `json:"conversation_id"`
	MessageID      *string        `json:"message_id"`
	MomentID       *string        `json:"moment_id"`
	CallID         *string        `json:"call_id"`
	Data           map[string]any `json:"data"`
	Read           bool           `json:"read"`
	ReadAt         *string        `json:"read_at"`
	CreatedAt      string         `json:"created_at"`
}

// MarkReadRequest is the body of POST /notifications/read. An empty `ids`
// marks every unread notification as read.
type MarkReadRequest struct {
	IDs []string `json:"ids"`
}

func formatTime(t time.Time) string { return t.UTC().Format(timeFormat) }

func uuidString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

// toResponse renders one notification as the client-facing shape.
func toResponse(item Item) NotificationResponse {
	var actor *ActorResponse
	if item.Actor != nil {
		actor = &ActorResponse{
			ID:          item.Actor.ID.String(),
			Username:    item.Actor.Username,
			DisplayName: item.Actor.DisplayName,
			AvatarURL:   item.Actor.AvatarURL,
		}
	}

	data := map[string]any{}
	if len(item.Data) > 0 {
		// A malformed payload should never fail the whole page.
		_ = json.Unmarshal(item.Data, &data)
	}

	var readAt *string
	if item.ReadAt != nil {
		formatted := formatTime(*item.ReadAt)
		readAt = &formatted
	}

	return NotificationResponse{
		ID:             item.ID.String(),
		Type:           item.Type,
		Actor:          actor,
		ConversationID: uuidString(item.ConversationID),
		MessageID:      uuidString(item.MessageID),
		MomentID:       uuidString(item.MomentID),
		CallID:         uuidString(item.CallID),
		Data:           data,
		Read:           item.ReadAt != nil,
		ReadAt:         readAt,
		CreatedAt:      formatTime(item.CreatedAt),
	}
}

func toResponses(items []Item) []NotificationResponse {
	out := make([]NotificationResponse, 0, len(items))
	for _, item := range items {
		out = append(out, toResponse(item))
	}
	return out
}

// encodeCursor renders "<unix-micro>:<uuid>" for a notification row.
func encodeCursor(at time.Time, id uuid.UUID) string {
	return strconv.FormatInt(at.UTC().UnixMicro(), 10) + ":" + id.String()
}

// decodeCursor parses the cursor produced by encodeCursor. An empty cursor is a
// valid request for the first page.
func decodeCursor(raw string) (*Cursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return nil, strconv.ErrSyntax
	}
	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, err
	}
	return &Cursor{At: time.UnixMicro(micros).UTC(), ID: id}, nil
}
