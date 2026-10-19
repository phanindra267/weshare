package feed

import "time"

// timeFormat is the RFC3339 layout used across the API for timestamps.
const timeFormat = "2006-01-02T15:04:05Z07:00"

// AuthorResponse is the public profile of a moment or comment author.
type AuthorResponse struct {
	ID          string  `json:"id"`
	Username    *string `json:"username"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

// MediaResponse is one attachment on a moment.
type MediaResponse struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	MediaType string `json:"media_type"`
	Position  int    `json:"position"`
}

// MomentResponse is the wire shape of a moment.
type MomentResponse struct {
	ID           string          `json:"id"`
	Author       AuthorResponse  `json:"author"`
	Body         string          `json:"body"`
	Visibility   string          `json:"visibility"`
	Media        []MediaResponse `json:"media"`
	LikeCount    int64           `json:"like_count"`
	CommentCount int64           `json:"comment_count"`
	LikedByMe    bool            `json:"liked_by_me"`
	CreatedAt    string          `json:"created_at"`
}

// CommentResponse is the wire shape of a comment.
type CommentResponse struct {
	ID            string          `json:"id"`
	MomentID      string          `json:"moment_id"`
	Author        AuthorResponse  `json:"author"`
	ReplyToID     *string         `json:"reply_to_id,omitempty"`
	ReplyToAuthor *AuthorResponse `json:"reply_to_author,omitempty"`
	Body          string          `json:"body"`
	CreatedAt     string          `json:"created_at"`
}

// MediaInputRequest is one attachment supplied in a create request.
type MediaInputRequest struct {
	URL       string `json:"url"`
	MediaType string `json:"media_type"`
}

// CreateMomentRequest is the POST /moments body.
type CreateMomentRequest struct {
	Body            string              `json:"body"`
	Visibility      string              `json:"visibility"`
	ExcludedUserIDs []string            `json:"excluded_user_ids,omitempty"`
	Media           []MediaInputRequest `json:"media"`
}

// UpdateMomentRequest is the PATCH /moments/:id body.
type UpdateMomentRequest struct {
	Body       *string `json:"body"`
	Visibility *string `json:"visibility"`
}

// AddCommentRequest is the POST /moments/:id/comments body.
type AddCommentRequest struct {
	Body      string  `json:"body"`
	ReplyToID *string `json:"reply_to_id,omitempty"`
}

// ReportMomentRequest is the POST /moments/:id/report body.
type ReportMomentRequest struct {
	Reason  string  `json:"reason"`
	Details *string `json:"details,omitempty"`
}

func formatTime(t time.Time) string { return t.UTC().Format(timeFormat) }

func toAuthorResponse(a Author) AuthorResponse {
	return AuthorResponse{
		ID:          a.ID.String(),
		Username:    a.Username,
		DisplayName: a.DisplayName,
		AvatarURL:   a.AvatarURL,
	}
}

func toMomentResponse(m Moment) MomentResponse {
	media := make([]MediaResponse, 0, len(m.Media))
	for _, item := range m.Media {
		media = append(media, MediaResponse{
			ID:        item.ID.String(),
			URL:       item.URL,
			MediaType: item.MediaType,
			Position:  item.Position,
		})
	}
	return MomentResponse{
		ID:           m.ID.String(),
		Author:       toAuthorResponse(m.Author),
		Body:         m.Body,
		Visibility:   m.Visibility,
		Media:        media,
		LikeCount:    m.LikeCount,
		CommentCount: m.CommentCount,
		LikedByMe:    m.LikedByMe,
		CreatedAt:    formatTime(m.CreatedAt),
	}
}

// ToMomentResponses maps a slice of moments for the wire. Exported so other
// packages (e.g. search/discover) can serialise moments consistently.
func ToMomentResponses(moments []Moment) []MomentResponse {
	out := make([]MomentResponse, 0, len(moments))
	for _, m := range moments {
		out = append(out, toMomentResponse(m))
	}
	return out
}

func toCommentResponse(c Comment) CommentResponse {
	resp := CommentResponse{
		ID:        c.ID.String(),
		MomentID:  c.MomentID.String(),
		Author:    toAuthorResponse(c.Author),
		Body:      c.Body,
		CreatedAt: formatTime(c.CreatedAt),
	}
	if c.ReplyToID != nil {
		s := c.ReplyToID.String()
		resp.ReplyToID = &s
	}
	if c.ReplyToAuthor != nil {
		a := toAuthorResponse(*c.ReplyToAuthor)
		resp.ReplyToAuthor = &a
	}
	return resp
}

func toCommentResponses(comments []Comment) []CommentResponse {
	out := make([]CommentResponse, 0, len(comments))
	for _, c := range comments {
		out = append(out, toCommentResponse(c))
	}
	return out
}
