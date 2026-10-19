package feed

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/notifications"
)

const (
	defaultPageSize = 20
	maxPageSize     = 50
	// maxSearchQueryRunes caps the length of a post-search query.
	maxSearchQueryRunes = 100
)

// Publisher fans realtime events out to connected users. The realtime hub
// implements it; unit tests can supply a stub.
type Publisher interface {
	PublishToUser(userID uuid.UUID, event any) bool
}

// Notifier creates a notification for a user. It is satisfied by
// *notifications.Service; a nil notifier disables notification creation.
type Notifier interface {
	Notify(ctx context.Context, in notifications.Input) error
}

// store is the persistence surface the service depends on. *Store implements
// it; unit tests use an in-memory fake.
type store interface {
	CreateMoment(ctx context.Context, author uuid.UUID, body, visibility string, media []MediaInput, excludedUsers []uuid.UUID) (uuid.UUID, error)
	UpdateMoment(ctx context.Context, id, author uuid.UUID, body, visibility *string) (Moment, error)
	GetMoment(ctx context.Context, viewer, id uuid.UUID) (Moment, error)
	ListFeed(ctx context.Context, viewer uuid.UUID, afterCreated *time.Time, afterID *uuid.UUID, limit int) ([]Moment, error)
	SearchMoments(ctx context.Context, viewer uuid.UUID, query string, afterCreated *time.Time, afterID *uuid.UUID, limit int) ([]Moment, error)
	ListPublicMoments(ctx context.Context, viewer uuid.UUID, afterCreated *time.Time, afterID *uuid.UUID, limit int) ([]Moment, error)
	SoftDeleteMoment(ctx context.Context, id uuid.UUID) error
	HideMoment(ctx context.Context, viewer, momentID uuid.UUID) error
	ReportMoment(ctx context.Context, reporter, momentID uuid.UUID, reason, details string) error
	IsFriend(ctx context.Context, viewer, author uuid.UUID) (bool, error)
	FriendIDs(ctx context.Context, viewer uuid.UUID) ([]uuid.UUID, error)
	Like(ctx context.Context, momentID, user uuid.UUID) (bool, error)
	Unlike(ctx context.Context, momentID, user uuid.UUID) (bool, error)
	LikeCount(ctx context.Context, momentID uuid.UUID) (int64, error)
	ListComments(ctx context.Context, momentID uuid.UUID, afterCreated *time.Time, afterID *uuid.UUID, limit int) ([]Comment, error)
	GetComment(ctx context.Context, id uuid.UUID) (Comment, error)
	AddComment(ctx context.Context, momentID, author uuid.UUID, body string, replyToID *uuid.UUID) (Comment, error)
	SoftDeleteComment(ctx context.Context, commentID, caller uuid.UUID) (bool, error)
}

// Service holds the Moments use cases.
type Service struct {
	st       store
	pub      Publisher
	notifier Notifier
}

// NewService builds a feed Service. A nil publisher disables realtime fan-out.
func NewService(st store, pub Publisher) *Service { return &Service{st: st, pub: pub} }

// SetNotifier wires notification creation, which is optional so REST-only
// tests and embedders can skip it.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// notify creates a notification, ignoring failures: a like or comment must
// still succeed if the notification store is unavailable.
func (s *Service) notify(ctx context.Context, in notifications.Input) {
	if s.notifier == nil {
		return
	}
	_ = s.notifier.Notify(ctx, in)
}

// CreateMomentInput is a request to publish a moment.
type CreateMomentInput struct {
	Body            string
	Visibility      string
	ExcludedUserIDs []string
	Media           []MediaInput
}

// Create validates and stores a moment, then notifies the author's friends.
func (s *Service) Create(ctx context.Context, author Author, in CreateMomentInput) (Moment, error) {
	body := strings.TrimSpace(in.Body)
	if len([]rune(body)) > maxBodyRunes {
		return Moment{}, &ValidationError{Field: "body", Message: "must be at most 2000 characters"}
	}

	visibility, err := normalizeVisibility(in.Visibility)
	if err != nil {
		return Moment{}, err
	}

	media := make([]MediaInput, 0, len(in.Media))
	for _, item := range in.Media {
		url := strings.TrimSpace(item.URL)
		if url == "" {
			return Moment{}, &ValidationError{Field: "media", Message: "each attachment needs a url"}
		}
		kind := item.MediaType
		if kind == "" {
			kind = MediaImage
		}
		if kind != MediaImage && kind != MediaVideo {
			return Moment{}, &ValidationError{Field: "media", Message: "media_type must be image or video"}
		}
		media = append(media, MediaInput{URL: url, MediaType: kind})
	}
	if len(media) > maxMediaPerMoment {
		return Moment{}, &ValidationError{Field: "media", Message: "at most 9 attachments allowed"}
	}
	if body == "" && len(media) == 0 {
		return Moment{}, ErrEmptyMoment
	}

	var excludedUsers []uuid.UUID
	for _, raw := range in.ExcludedUserIDs {
		if uid, err := uuid.Parse(strings.TrimSpace(raw)); err == nil {
			excludedUsers = append(excludedUsers, uid)
		}
	}

	id, err := s.st.CreateMoment(ctx, author.ID, body, visibility, media, excludedUsers)
	if err != nil {
		return Moment{}, err
	}
	moment, err := s.st.GetMoment(ctx, author.ID, id)
	if err != nil {
		return Moment{}, err
	}
	s.fanOut(ctx, author.ID, moment)
	return moment, nil
}

// Update modifies an existing moment's text and/or visibility (author only).
func (s *Service) Update(ctx context.Context, author, id uuid.UUID, in UpdateMomentInput) (Moment, error) {
	var body *string
	if in.Body != nil {
		b := strings.TrimSpace(*in.Body)
		if len([]rune(b)) > maxBodyRunes {
			return Moment{}, &ValidationError{Field: "body", Message: "must be at most 2000 characters"}
		}
		body = &b
	}
	var visibility *string
	if in.Visibility != nil {
		v, err := normalizeVisibility(*in.Visibility)
		if err != nil {
			return Moment{}, err
		}
		visibility = &v
	}
	return s.st.UpdateMoment(ctx, id, author, body, visibility)
}

// Get returns a single moment if visible to the viewer.
func (s *Service) Get(ctx context.Context, viewer, id uuid.UUID) (Moment, error) {
	moment, err := s.st.GetMoment(ctx, viewer, id)
	if err != nil {
		return Moment{}, err
	}
	if err := s.authorizeView(ctx, moment, viewer); err != nil {
		return Moment{}, err
	}
	return moment, nil
}

// Feed returns one page of moments visible to the viewer, newest first.
func (s *Service) Feed(ctx context.Context, viewer uuid.UUID, cursor string, limit int) ([]Moment, string, error) {
	created, afterID, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", &ValidationError{Field: "cursor", Message: "invalid cursor"}
	}
	clamped := clampLimit(limit)
	// Fetch one extra to know whether a next page exists.
	moments, err := s.st.ListFeed(ctx, viewer, created, afterID, clamped+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(moments) > clamped {
		last := moments[clamped-1]
		next = encodeCursor(last.CreatedAt, last.ID)
		moments = moments[:clamped]
	}
	return moments, next, nil
}

// SearchPosts returns one page of the viewer's visible moments whose body
// matches query, newest first, plus the cursor for the next page.
func (s *Service) SearchPosts(ctx context.Context, viewer uuid.UUID, query, cursor string, limit int) ([]Moment, string, error) {
	q, err := normalizeSearchQuery(query)
	if err != nil {
		return nil, "", err
	}
	created, afterID, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", &ValidationError{Field: "cursor", Message: "invalid cursor"}
	}
	clamped := clampLimit(limit)
	moments, err := s.st.SearchMoments(ctx, viewer, escapeLike(q), created, afterID, clamped+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(moments) > clamped {
		last := moments[clamped-1]
		next = encodeCursor(last.CreatedAt, last.ID)
		moments = moments[:clamped]
	}
	return moments, next, nil
}

// Discover returns one page of public moments from across the app, newest
// first, for the Discover landing page.
func (s *Service) Discover(ctx context.Context, viewer uuid.UUID, cursor string, limit int) ([]Moment, string, error) {
	created, afterID, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", &ValidationError{Field: "cursor", Message: "invalid cursor"}
	}
	clamped := clampLimit(limit)
	moments, err := s.st.ListPublicMoments(ctx, viewer, created, afterID, clamped+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(moments) > clamped {
		last := moments[clamped-1]
		next = encodeCursor(last.CreatedAt, last.ID)
		moments = moments[:clamped]
	}
	return moments, next, nil
}

// Delete removes a moment if the viewer is the author.
func (s *Service) Delete(ctx context.Context, viewer, id uuid.UUID) error {
	moment, err := s.st.GetMoment(ctx, viewer, id)
	if err != nil {
		return err
	}
	if moment.Author.ID != viewer {
		return ErrNotAuthor
	}
	return s.st.SoftDeleteMoment(ctx, id)
}

// Hide hides a moment from the viewer's feed.
func (s *Service) Hide(ctx context.Context, viewer, id uuid.UUID) error {
	return s.st.HideMoment(ctx, viewer, id)
}

// Report files a report against a moment.
func (s *Service) Report(ctx context.Context, reporter, id uuid.UUID, in ReportInput) error {
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return &ValidationError{Field: "reason", Message: "reason is required"}
	}
	return s.st.ReportMoment(ctx, reporter, id, reason, strings.TrimSpace(in.Details))
}

// Like adds a like from the viewer, returning the updated count.
func (s *Service) Like(ctx context.Context, viewer, momentID uuid.UUID) (int64, error) {
	moment, err := s.visibleMoment(ctx, viewer, momentID)
	if err != nil {
		return 0, err
	}
	if _, err := s.st.Like(ctx, momentID, viewer); err != nil {
		return 0, err
	}
	// Notify the author (never yourself) that their moment was liked.
	if moment.Author.ID != viewer {
		author := moment.Author.ID
		s.notify(ctx, notifications.Input{
			UserID:   author,
			Type:     notifications.TypeMomentLike,
			ActorID:  &viewer,
			MomentID: &momentID,
			Data:     map[string]any{"preview": previewText(moment.Body)},
		})
	}
	return s.st.LikeCount(ctx, momentID)
}

// Unlike removes a like from the viewer, returning the updated count.
func (s *Service) Unlike(ctx context.Context, viewer, momentID uuid.UUID) (int64, error) {
	if err := s.ensureVisible(ctx, viewer, momentID); err != nil {
		return 0, err
	}
	if _, err := s.st.Unlike(ctx, momentID, viewer); err != nil {
		return 0, err
	}
	return s.st.LikeCount(ctx, momentID)
}

// Comments returns one page of comments for a visible moment, oldest first.
func (s *Service) Comments(ctx context.Context, viewer, momentID uuid.UUID, cursor string, limit int) ([]Comment, string, error) {
	if err := s.ensureVisible(ctx, viewer, momentID); err != nil {
		return nil, "", err
	}
	created, afterID, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", &ValidationError{Field: "cursor", Message: "invalid cursor"}
	}
	clamped := clampLimit(limit)
	comments, err := s.st.ListComments(ctx, momentID, created, afterID, clamped+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(comments) > clamped {
		last := comments[clamped-1]
		next = encodeCursor(last.CreatedAt, last.ID)
		comments = comments[:clamped]
	}
	return comments, next, nil
}

// AddComment appends a comment (or reply) to a visible moment.
func (s *Service) AddComment(ctx context.Context, viewer, momentID uuid.UUID, text string, replyToID *uuid.UUID) (Comment, error) {
	moment, err := s.visibleMoment(ctx, viewer, momentID)
	if err != nil {
		return Comment{}, err
	}
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > maxCommentRunes {
		return Comment{}, &ValidationError{Field: "body", Message: "must be between 1 and 1000 characters"}
	}
	comment, err := s.st.AddComment(ctx, momentID, viewer, text, replyToID)
	if err != nil {
		return Comment{}, err
	}
	// Notify the moment's author (never yourself) about the new comment.
	if moment.Author.ID != viewer {
		author := moment.Author.ID
		s.notify(ctx, notifications.Input{
			UserID:   author,
			Type:     notifications.TypeMomentComment,
			ActorID:  &viewer,
			MomentID: &momentID,
			Data:     map[string]any{"preview": truncate(text, 140)},
		})
	}
	return comment, nil
}

// DeleteComment removes a comment when the viewer wrote it or authored the
// parent moment.
func (s *Service) DeleteComment(ctx context.Context, viewer, commentID uuid.UUID) error {
	ok, err := s.st.SoftDeleteComment(ctx, commentID, viewer)
	if err != nil {
		return err
	}
	if !ok {
		return ErrCommentNotFound
	}
	return nil
}

// ensureVisible loads a moment and enforces visibility.
func (s *Service) ensureVisible(ctx context.Context, viewer, id uuid.UUID) error {
	_, err := s.visibleMoment(ctx, viewer, id)
	return err
}

// visibleMoment loads a moment and enforces visibility, returning it so callers
// that need the author (to notify them) can use it.
func (s *Service) visibleMoment(ctx context.Context, viewer, id uuid.UUID) (Moment, error) {
	moment, err := s.st.GetMoment(ctx, viewer, id)
	if err != nil {
		return Moment{}, err
	}
	if err := s.authorizeView(ctx, moment, viewer); err != nil {
		return Moment{}, err
	}
	return moment, nil
}

// previewText trims a moment body to a short snippet for a notification.
func previewText(body string) string { return truncate(strings.TrimSpace(body), 140) }

// truncate shortens s to at most max runes, appending an ellipsis when cut.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func (s *Service) authorizeView(ctx context.Context, moment Moment, viewer uuid.UUID) error {
	if moment.Author.ID == viewer {
		return nil
	}
	switch moment.Visibility {
	case VisibilityPublic:
		return nil
	case VisibilityFriends:
		friend, err := s.st.IsFriend(ctx, viewer, moment.Author.ID)
		if err != nil {
			return err
		}
		if friend {
			return nil
		}
	}
	return ErrMomentNotFound
}

func (s *Service) fanOut(ctx context.Context, author uuid.UUID, moment Moment) {
	if s.pub == nil || moment.Visibility == VisibilityPrivate {
		return
	}
	friends, err := s.st.FriendIDs(ctx, author)
	if err != nil {
		return
	}
	event := MomentCreatedEvent{Type: EventMomentCreated, Moment: toMomentResponse(moment)}
	for _, friend := range friends {
		s.pub.PublishToUser(friend, event)
	}
}

func normalizeVisibility(v string) (string, error) {
	switch strings.TrimSpace(v) {
	case "", VisibilityFriends:
		return VisibilityFriends, nil
	case VisibilityPublic:
		return VisibilityPublic, nil
	case VisibilityPrivate:
		return VisibilityPrivate, nil
	default:
		return "", &ValidationError{Field: "visibility", Message: "must be friends, public, or private"}
	}
}

// normalizeSearchQuery trims a post-search query and rejects it when empty or
// over-long.
func normalizeSearchQuery(raw string) (string, error) {
	q := strings.TrimSpace(raw)
	if q == "" {
		return "", &ValidationError{Field: "q", Message: "is required"}
	}
	if len([]rune(q)) > maxSearchQueryRunes {
		return "", &ValidationError{Field: "q", Message: "is too long"}
	}
	return q, nil
}

// escapeLike escapes LIKE metacharacters so a query of "50%" matches a literal
// percent sign instead of every row.
func escapeLike(q string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultPageSize
	}
	if limit > maxPageSize {
		return maxPageSize
	}
	return limit
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	return strconv.FormatInt(t.UTC().UnixMicro(), 10) + ":" + id.String()
}

// decodeCursor parses "<unix-micro>:<uuid>".
func decodeCursor(cursor string) (*time.Time, *uuid.UUID, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return nil, nil, nil
	}
	parts := strings.SplitN(cursor, ":", 2)
	if len(parts) != 2 {
		return nil, nil, strconv.ErrSyntax
	}
	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, nil, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, nil, err
	}
	created := time.UnixMicro(micros).UTC()
	return &created, &id, nil
}
