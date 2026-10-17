package feed

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultPageSize = 20
	maxPageSize     = 50
)

// Publisher fans realtime events out to connected users. The realtime hub
// implements it; unit tests can supply a stub.
type Publisher interface {
	PublishToUser(userID uuid.UUID, event any) bool
}

// store is the persistence surface the service depends on. *Store implements
// it; unit tests use an in-memory fake.
type store interface {
	CreateMoment(ctx context.Context, author uuid.UUID, body, visibility string, media []MediaInput) (uuid.UUID, error)
	GetMoment(ctx context.Context, viewer, id uuid.UUID) (Moment, error)
	ListFeed(ctx context.Context, viewer uuid.UUID, afterCreated *time.Time, afterID *uuid.UUID, limit int) ([]Moment, error)
	SoftDeleteMoment(ctx context.Context, id uuid.UUID) error
	IsFriend(ctx context.Context, viewer, author uuid.UUID) (bool, error)
	FriendIDs(ctx context.Context, viewer uuid.UUID) ([]uuid.UUID, error)
	Like(ctx context.Context, momentID, user uuid.UUID) (bool, error)
	Unlike(ctx context.Context, momentID, user uuid.UUID) (bool, error)
	LikeCount(ctx context.Context, momentID uuid.UUID) (int64, error)
	ListComments(ctx context.Context, momentID uuid.UUID, afterCreated *time.Time, afterID *uuid.UUID, limit int) ([]Comment, error)
	GetComment(ctx context.Context, id uuid.UUID) (Comment, error)
	AddComment(ctx context.Context, momentID, author uuid.UUID, body string) (Comment, error)
	SoftDeleteComment(ctx context.Context, commentID, caller uuid.UUID) (bool, error)
}

// Service holds the Moments use cases.
type Service struct {
	st  store
	pub Publisher
}

// NewService builds a feed Service. A nil publisher disables realtime fan-out.
func NewService(st store, pub Publisher) *Service { return &Service{st: st, pub: pub} }

// CreateMomentInput is a request to publish a moment.
type CreateMomentInput struct {
	Body       string
	Visibility string
	Media      []MediaInput
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

	id, err := s.st.CreateMoment(ctx, author.ID, body, visibility, media)
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

// Feed returns a page of moments visible to the viewer plus the next cursor.
func (s *Service) Feed(ctx context.Context, viewer uuid.UUID, cursor string, limit int) ([]Moment, string, error) {
	size := clampLimit(limit)
	afterCreated, afterID, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", &ValidationError{Field: "cursor", Message: "is not a valid cursor"}
	}

	moments, err := s.st.ListFeed(ctx, viewer, afterCreated, afterID, size+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(moments) > size {
		last := moments[size-1]
		next = encodeCursor(last.CreatedAt, last.ID)
		moments = moments[:size]
	}
	return moments, next, nil
}

// Get returns a single moment if the viewer is allowed to see it.
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

// Delete soft-deletes a moment the viewer authored.
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

// Like adds a like (idempotent) and returns the resulting count.
func (s *Service) Like(ctx context.Context, viewer, id uuid.UUID) (int64, error) {
	if err := s.ensureVisible(ctx, viewer, id); err != nil {
		return 0, err
	}
	if _, err := s.st.Like(ctx, id, viewer); err != nil {
		return 0, err
	}
	return s.st.LikeCount(ctx, id)
}

// Unlike removes a like (idempotent) and returns the resulting count.
func (s *Service) Unlike(ctx context.Context, viewer, id uuid.UUID) (int64, error) {
	if err := s.ensureVisible(ctx, viewer, id); err != nil {
		return 0, err
	}
	if _, err := s.st.Unlike(ctx, id, viewer); err != nil {
		return 0, err
	}
	return s.st.LikeCount(ctx, id)
}

// Comments returns a page of comments oldest-first plus the next cursor.
func (s *Service) Comments(ctx context.Context, viewer, momentID uuid.UUID, cursor string, limit int) ([]Comment, string, error) {
	if err := s.ensureVisible(ctx, viewer, momentID); err != nil {
		return nil, "", err
	}
	size := clampLimit(limit)
	afterCreated, afterID, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", &ValidationError{Field: "cursor", Message: "is not a valid cursor"}
	}

	comments, err := s.st.ListComments(ctx, momentID, afterCreated, afterID, size+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(comments) > size {
		last := comments[size-1]
		next = encodeCursor(last.CreatedAt, last.ID)
		comments = comments[:size]
	}
	return comments, next, nil
}

// AddComment appends a comment to a moment the viewer can see.
func (s *Service) AddComment(ctx context.Context, viewer uuid.UUID, momentID uuid.UUID, body string) (Comment, error) {
	if err := s.ensureVisible(ctx, viewer, momentID); err != nil {
		return Comment{}, err
	}
	text := strings.TrimSpace(body)
	if text == "" || len([]rune(text)) > maxCommentRunes {
		return Comment{}, &ValidationError{Field: "body", Message: "must be between 1 and 1000 characters"}
	}
	return s.st.AddComment(ctx, momentID, viewer, text)
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

// ensureVisible loads a moment and enforces visibility, collapsing "not found"
// and "not allowed" into the same error so private moments cannot be probed.
func (s *Service) ensureVisible(ctx context.Context, viewer, id uuid.UUID) error {
	moment, err := s.st.GetMoment(ctx, viewer, id)
	if err != nil {
		return err
	}
	return s.authorizeView(ctx, moment, viewer)
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

// ensureVisible helper reused by Like/Unlike/Comments/AddComment.
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

// decodeCursor parses "<unix-micro>:<uuid>". An empty cursor means "from the
// start" and returns nil pointers.
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
