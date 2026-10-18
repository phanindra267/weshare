package calls

import (
	"time"

	"github.com/google/uuid"
)

// timeFormat is the RFC3339 layout used across the API for timestamps.
const timeFormat = "2006-01-02T15:04:05Z07:00"

// ParticipantResponse is one user's membership in a call-history row.
type ParticipantResponse struct {
	UserID      string  `json:"user_id"`
	Username    *string `json:"username"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	Role        string  `json:"role"`
	State       string  `json:"state"`
}

// CallResponse is the wire shape of a call-history record, resolved from the
// requesting user's point of view.
type CallResponse struct {
	ID              string                `json:"id"`
	Kind            string                `json:"kind"`
	Group           bool                  `json:"group"`
	Status          string                `json:"status"`
	Direction       string                `json:"direction"`
	Outcome         string                `json:"outcome"`
	StartedAt       *string               `json:"started_at"`
	EndedAt         *string               `json:"ended_at"`
	CreatedAt       string                `json:"created_at"`
	DurationSeconds int64                 `json:"duration_seconds"`
	Participants    []ParticipantResponse `json:"participants"`
}

// toCallResponses maps history rows to wire responses for the given viewer.
func toCallResponses(list []Call, viewer uuid.UUID, now time.Time) []CallResponse {
	out := make([]CallResponse, 0, len(list))
	for _, c := range list {
		out = append(out, toCallResponse(c, viewer, now))
	}
	return out
}

func toCallResponse(c Call, viewer uuid.UUID, now time.Time) CallResponse {
	direction := "incoming"
	if c.CallerID == viewer {
		direction = "outgoing"
	}

	participants := make([]ParticipantResponse, 0, len(c.Participants))
	viewerState := ""
	for _, p := range c.Participants {
		if p.UserID == viewer {
			viewerState = p.State
		}
		participants = append(participants, ParticipantResponse{
			UserID:      p.UserID.String(),
			Username:    p.Profile.Username,
			DisplayName: p.Profile.DisplayName,
			AvatarURL:   p.Profile.AvatarURL,
			Role:        p.Role,
			State:       p.State,
		})
	}

	return CallResponse{
		ID:              c.ID.String(),
		Kind:            c.Kind,
		Group:           c.IsGroup,
		Status:          c.Status,
		Direction:       direction,
		Outcome:         outcomeFor(c, direction, viewerState),
		StartedAt:       formatTimePtr(c.StartedAt),
		EndedAt:         formatTimePtr(c.EndedAt),
		CreatedAt:       c.CreatedAt.UTC().Format(timeFormat),
		DurationSeconds: durationSeconds(c, now),
		Participants:    participants,
	}
}

// outcomeFor collapses the stored status and the viewer's participant state
// into a single label the UI can render (and count as a missed call).
func outcomeFor(c Call, direction, viewerState string) string {
	switch c.Status {
	case StatusActive:
		return "active"
	case StatusEnded:
		return "completed"
	case StatusRejected:
		if viewerState == ParticipantRejected {
			return "rejected"
		}
		return "declined"
	case StatusMissed:
		return "missed"
	case StatusFailed:
		return "failed"
	case StatusCanceled:
		// A direct call that never connected is a missed call for the callee.
		if direction == "incoming" && viewerState == ParticipantMissed {
			return "missed"
		}
		return "cancelled"
	default:
		return "unknown"
	}
}

func durationSeconds(c Call, now time.Time) int64 {
	if c.StartedAt == nil {
		return 0
	}
	end := now
	if c.EndedAt != nil {
		end = *c.EndedAt
	}
	if end.Before(*c.StartedAt) {
		return 0
	}
	return int64(end.Sub(*c.StartedAt).Seconds())
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(timeFormat)
	return &s
}
