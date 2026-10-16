package contacts

import (
	"strings"

	"github.com/google/uuid"
)

// timeFormat is the RFC3339 layout used across the API for timestamps.
const timeFormat = "2006-01-02T15:04:05Z07:00"

// UserBriefResponse is the peer profile embedded in contact payloads. It is a
// public subset: no email, and presence derived from the caller's perspective.
type UserBriefResponse struct {
	ID            string  `json:"id"`
	Username      *string `json:"username"`
	DisplayName   string  `json:"display_name"`
	AvatarURL     *string `json:"avatar_url"`
	StatusMessage *string `json:"status_message"`
	Online        bool    `json:"online"`
	LastSeenAt    *string `json:"last_seen_at"`
}

// toUserBrief maps a profile; `online` is supplied by the handler because it
// depends on the configured presence window.
func toUserBrief(p ProfileBrief, online bool) UserBriefResponse {
	resp := UserBriefResponse{
		ID:            p.ID.String(),
		Username:      p.Username,
		DisplayName:   p.DisplayName,
		AvatarURL:     p.AvatarURL,
		StatusMessage: p.StatusMessage,
		Online:        online,
	}
	if p.LastSeenAt != nil {
		s := p.LastSeenAt.UTC().Format(timeFormat)
		resp.LastSeenAt = &s
	}
	return resp
}

// ContactResponse is one accepted friendship in a list.
type ContactResponse struct {
	User        UserBriefResponse `json:"user"`
	ConnectedAt string            `json:"connected_at"`
}

func toContactResponse(c Contact, online bool) ContactResponse {
	return ContactResponse{
		User:        toUserBrief(briefFromContact(c), online),
		ConnectedAt: c.AcceptedAt.UTC().Format(timeFormat),
	}
}

// Direction values returned on contact requests.
const (
	DirectionIncoming = "incoming"
	DirectionOutgoing = "outgoing"
)

// ContactRequestResponse is a friend request viewed by the caller, always
// paired with the counterpart's profile.
type ContactRequestResponse struct {
	ID         string            `json:"id"`
	Status     string            `json:"status"`
	Direction  string            `json:"direction"`
	User       UserBriefResponse `json:"user"`
	CreatedAt  string            `json:"created_at"`
	AcceptedAt *string           `json:"accepted_at,omitempty"`
}

func toRequestResponse(r RequestWithUser, incoming bool, online bool) ContactRequestResponse {
	direction := DirectionOutgoing
	counterpart := r.ContactID
	if incoming {
		direction = DirectionIncoming
		counterpart = r.UserID
	}
	return ContactRequestResponse{
		ID:        r.ID.String(),
		Status:    r.Status,
		Direction: direction,
		User: toUserBrief(ProfileBrief{
			ID:            counterpart,
			DisplayName:   r.DisplayName,
			Username:      r.Username,
			AvatarURL:     r.AvatarURL,
			StatusMessage: r.StatusMessage,
			LastSeenAt:    r.LastSeenAt,
		}, online),
		CreatedAt: r.CreatedAt.UTC().Format(timeFormat),
	}
}

// toSingleRequest maps a just-sent or just-accepted request. The counterpart is
// always the other party: the recipient for a sent request, the sender for an
// accepted one.
func toSingleRequest(r Request, counterpart ProfileBrief, direction string, online bool) ContactRequestResponse {
	resp := ContactRequestResponse{
		ID:        r.ID.String(),
		Status:    r.Status,
		Direction: direction,
		User:      toUserBrief(counterpart, online),
		CreatedAt: r.CreatedAt.UTC().Format(timeFormat),
	}
	if r.AcceptedAt != nil {
		s := r.AcceptedAt.UTC().Format(timeFormat)
		resp.AcceptedAt = &s
	}
	return resp
}

// briefFromContact projects a joined contact row onto a ProfileBrief.
func briefFromContact(c Contact) ProfileBrief {
	return ProfileBrief{
		ID:            c.ContactID,
		DisplayName:   c.DisplayName,
		Username:      c.Username,
		AvatarURL:     c.AvatarURL,
		StatusMessage: c.StatusMessage,
		LastSeenAt:    c.LastSeenAt,
	}
}

// sendRequestInput is the body for POST /contacts/requests.
type sendRequestInput struct {
	UserID string `json:"user_id"`
}

// Validate normalizes the input and returns per-field errors.
func (in *sendRequestInput) Validate() map[string]string {
	in.UserID = strings.TrimSpace(in.UserID)
	fields := map[string]string{}
	if in.UserID == "" {
		fields["user_id"] = "A user id is required."
		return fields
	}
	if _, err := uuid.Parse(in.UserID); err != nil {
		fields["user_id"] = "That is not a valid user id."
	}
	return fields
}
