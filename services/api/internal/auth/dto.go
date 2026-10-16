package auth

import (
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)

// RegisterInput is the validated registration payload.
type RegisterInput struct {
	Email       string
	Username    string
	Password    string
	DisplayName string
}

// LoginInput is the validated login payload.
type LoginInput struct {
	Email    string
	Password string
}

// RequestMeta captures request-derived session metadata.
type RequestMeta struct {
	UserAgent string
	IP        string
}

const (
	maxEmailLength       = 254
	maxPasswordLength    = 128
	maxDisplayNameLength = 64
	maxBioLength         = 500
	maxStatusLength      = 140
)

// Validate normalizes the input in place and returns per-field errors.
func (in *RegisterInput) Validate(minPasswordLength int) map[string]string {
	in.Email = normalizeEmail(in.Email)
	in.Username = normalizeUsername(in.Username)
	in.DisplayName = strings.TrimSpace(in.DisplayName)

	fields := map[string]string{}

	switch {
	case in.Email == "":
		fields["email"] = "Email is required."
	case len(in.Email) > maxEmailLength:
		fields["email"] = "Email is too long."
	case !validEmail(in.Email):
		fields["email"] = "Enter a valid email address."
	}

	switch {
	case in.Username == "":
		fields["username"] = "Username is required."
	case !usernamePattern.MatchString(in.Username):
		fields["username"] = "Use 3-32 lowercase letters, numbers, or underscores."
	}

	switch {
	case len(in.Password) < minPasswordLength:
		fields["password"] = "Password must be at least " + strconv.Itoa(minPasswordLength) + " characters."
	case len(in.Password) > maxPasswordLength:
		fields["password"] = "Password is too long."
	}

	if in.DisplayName == "" {
		in.DisplayName = in.Username
	}
	if len(in.DisplayName) > maxDisplayNameLength {
		fields["display_name"] = "Display name must be at most 64 characters."
	}

	return fields
}

// Validate normalizes the input in place and returns per-field errors.
func (in *LoginInput) Validate() map[string]string {
	in.Email = normalizeEmail(in.Email)
	fields := map[string]string{}
	if in.Email == "" {
		fields["email"] = "Email is required."
	}
	if in.Password == "" {
		fields["password"] = "Password is required."
	}
	return fields
}

func validEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}
	return strings.EqualFold(addr.Address, email) && strings.Contains(email, "@")
}

// --- responses -----------------------------------------------------------

// UserResponse is the client-facing user shape. Email is only populated for
// the requesting user (self).
type UserResponse struct {
	ID            string     `json:"id"`
	Username      *string    `json:"username"`
	Email         string     `json:"email,omitempty"`
	DisplayName   string     `json:"display_name"`
	Bio           *string    `json:"bio"`
	AvatarURL     *string    `json:"avatar_url"`
	StatusMessage *string    `json:"status_message"`
	Online        bool       `json:"online"`
	LastSeenAt    *time.Time `json:"last_seen_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

func toUserResponse(u User, includeEmail bool) UserResponse {
	resp := UserResponse{
		ID:            u.ID.String(),
		Username:      u.Username,
		DisplayName:   u.DisplayName,
		Bio:           u.Bio,
		AvatarURL:     u.AvatarURL,
		StatusMessage: u.StatusMessage,
		Online:        u.Online,
		LastSeenAt:    u.LastSeenAt,
		CreatedAt:     u.CreatedAt,
	}
	if includeEmail {
		resp.Email = u.Email
	}
	return resp
}

// SessionResponse is the client-facing session shape.
type SessionResponse struct {
	ID         string    `json:"id"`
	UserAgent  string    `json:"user_agent,omitempty"`
	IP         string    `json:"ip,omitempty"`
	LastUsedAt time.Time `json:"last_used_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
	IsCurrent  bool      `json:"is_current"`
}

func toSessionResponse(s Session) SessionResponse {
	return SessionResponse{
		ID:         s.ID.String(),
		UserAgent:  s.UserAgent,
		IP:         s.IP,
		LastUsedAt: s.LastUsedAt,
		ExpiresAt:  s.ExpiresAt,
		CreatedAt:  s.CreatedAt,
		IsCurrent:  s.IsCurrent,
	}
}
