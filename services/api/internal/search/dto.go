package search

import (
	"time"

	"github.com/phanindra267/weshare/services/api/internal/messaging"
)

const timeFormat = time.RFC3339

// PeerResponse is the public profile of the other side of a direct chat.
type PeerResponse struct {
	ID          string  `json:"id"`
	Username    *string `json:"username"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

// ConversationResponse labels the conversation a message hit belongs to.
type ConversationResponse struct {
	ID    string        `json:"id"`
	Kind  string        `json:"kind"`
	Title *string       `json:"title"`
	Peer  *PeerResponse `json:"peer"`
}

// MessageHitResponse is one message-search result.
type MessageHitResponse struct {
	Message      messaging.MessageResponse `json:"message"`
	Conversation ConversationResponse      `json:"conversation"`
}

// ProfileHitResponse is one profile-search result.
type ProfileHitResponse struct {
	ID            string  `json:"id"`
	Username      *string `json:"username"`
	DisplayName   string  `json:"display_name"`
	AvatarURL     *string `json:"avatar_url"`
	StatusMessage *string `json:"status_message"`
	Online        bool    `json:"online"`
	LastSeenAt    *string `json:"last_seen_at"`
	IsContact     bool    `json:"is_contact"`
}

// ConversationHitResponse is one conversation (direct or group) search result.
type ConversationHitResponse struct {
	ID             string        `json:"id"`
	Kind           string        `json:"kind"`
	Title          *string       `json:"title"`
	Peer           *PeerResponse `json:"peer"`
	MemberCount    int64         `json:"member_count"`
	Role           string        `json:"role"`
	LastActivityAt string        `json:"last_activity_at"`
}

func toConversationHitResponse(hit ConversationHit) ConversationHitResponse {
	resp := ConversationHitResponse{
		ID:             hit.ID.String(),
		Kind:           hit.Kind,
		Title:          hit.Title,
		MemberCount:    hit.MemberCount,
		Role:           hit.Role,
		LastActivityAt: hit.ActivityAt().UTC().Format(timeFormat),
	}
	if hit.Peer != nil {
		resp.Peer = &PeerResponse{
			ID:          hit.Peer.ID.String(),
			Username:    hit.Peer.Username,
			DisplayName: hit.Peer.DisplayName,
			AvatarURL:   hit.Peer.AvatarURL,
		}
	}
	return resp
}

func toConversationHitResponses(hits []ConversationHit) []ConversationHitResponse {
	out := make([]ConversationHitResponse, 0, len(hits))
	for _, hit := range hits {
		out = append(out, toConversationHitResponse(hit))
	}
	return out
}

func toMessageHitResponse(hit MessageHit) MessageHitResponse {
	var peer *PeerResponse
	if hit.Conversation.Peer != nil {
		peer = &PeerResponse{
			ID:          hit.Conversation.Peer.ID.String(),
			Username:    hit.Conversation.Peer.Username,
			DisplayName: hit.Conversation.Peer.DisplayName,
			AvatarURL:   hit.Conversation.Peer.AvatarURL,
		}
	}
	return MessageHitResponse{
		Message: messaging.ToMessageResponse(hit.Message),
		Conversation: ConversationResponse{
			ID:    hit.Conversation.ID.String(),
			Kind:  hit.Conversation.Kind,
			Title: hit.Conversation.Title,
			Peer:  peer,
		},
	}
}

func toMessageHitResponses(hits []MessageHit) []MessageHitResponse {
	out := make([]MessageHitResponse, 0, len(hits))
	for _, hit := range hits {
		out = append(out, toMessageHitResponse(hit))
	}
	return out
}

func toProfileHitResponse(hit ProfileHit, online bool) ProfileHitResponse {
	var lastSeen *string
	if hit.LastSeenAt != nil {
		s := hit.LastSeenAt.UTC().Format(timeFormat)
		lastSeen = &s
	}
	return ProfileHitResponse{
		ID:            hit.ID.String(),
		Username:      hit.Username,
		DisplayName:   hit.DisplayName,
		AvatarURL:     hit.AvatarURL,
		StatusMessage: hit.StatusMessage,
		Online:        online,
		LastSeenAt:    lastSeen,
		IsContact:     hit.IsContact,
	}
}

func toProfileHitResponses(hits []ProfileHit, online func(*time.Time) bool) []ProfileHitResponse {
	out := make([]ProfileHitResponse, 0, len(hits))
	for _, hit := range hits {
		out = append(out, toProfileHitResponse(hit, online(hit.LastSeenAt)))
	}
	return out
}
