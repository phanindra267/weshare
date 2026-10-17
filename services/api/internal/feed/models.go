package feed

import (
	"time"

	"github.com/google/uuid"
)

// Moment visibility values stored in moments.visibility.
const (
	// VisibilityFriends is the default: visible to the author and their
	// accepted contacts.
	VisibilityFriends = "friends"
	// VisibilityPublic is visible to everyone.
	VisibilityPublic = "public"
	// VisibilityPrivate is visible only to the author.
	VisibilityPrivate = "private"
)

// Media type values stored in moment_media.media_type.
const (
	MediaImage = "image"
	MediaVideo = "video"
)

// Limits applied when creating moments and comments.
const (
	maxMediaPerMoment = 9
	maxBodyRunes      = 2000
	maxCommentRunes   = 1000
)

// Author is the public profile of a moment or comment author.
type Author struct {
	ID          uuid.UUID
	DisplayName string
	Username    *string
	AvatarURL   *string
}

// Media is one attachment on a moment.
type Media struct {
	ID        uuid.UUID
	URL       string
	MediaType string
	Position  int
}

// Moment is a post joined with its author, media, and engagement counts.
// LikedByMe is relative to the requesting viewer.
type Moment struct {
	ID           uuid.UUID
	Author       Author
	Body         string
	Visibility   string
	Media        []Media
	LikeCount    int64
	CommentCount int64
	LikedByMe    bool
	CreatedAt    time.Time
}

// Comment is a reply joined with its author.
type Comment struct {
	ID        uuid.UUID
	MomentID  uuid.UUID
	Author    Author
	Body      string
	CreatedAt time.Time
}

// MediaInput is one attachment supplied when creating a moment.
type MediaInput struct {
	URL       string
	MediaType string
}
