package feed

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// recordingPublisher captures every event fanned out to each user and reports
// whether a user is "connected".
type recordingPublisher struct {
	connected map[uuid.UUID]bool
	events    []publishedEvent
}

type publishedEvent struct {
	user  uuid.UUID
	event any
}

func newRecordingPublisher(connected ...uuid.UUID) *recordingPublisher {
	p := &recordingPublisher{connected: map[uuid.UUID]bool{}}
	for _, id := range connected {
		p.connected[id] = true
	}
	return p
}

func (p *recordingPublisher) PublishToUser(userID uuid.UUID, event any) bool {
	p.events = append(p.events, publishedEvent{user: userID, event: event})
	return p.connected[userID]
}

// fakeStore is a configurable in-memory implementation of the store interface.
type fakeStore struct {
	moments       map[uuid.UUID]Moment
	media         map[uuid.UUID][]Media
	likes         map[uuid.UUID]map[uuid.UUID]bool
	comments      map[uuid.UUID][]Comment
	commentByID   map[uuid.UUID]Comment
	friends       map[uuid.UUID]map[uuid.UUID]bool
	excludedUsers map[uuid.UUID][]uuid.UUID
	hiddenMoments map[uuid.UUID]map[uuid.UUID]bool
	reports       []reportedEntry

	// configurable results / errors
	feed               []Moment
	searchHits         []Moment
	publicHits         []Moment
	gotSearchQuery     string
	gotDiscoverLimit   int
	createdMomentID    uuid.UUID
	createErr          error
	getErr             error
	listErr            error
	likeErr            error
	unlikeErr          error
	likeCount          int64
	likeCountSet       bool
	addCommentErr      error
	deleteMomentErr    error
	deleteCommentOK    bool
	deleteCommentOKSet bool
	updateErr          error
	hideErr            error
	reportErr          error
}

type reportedEntry struct {
	reporter, momentID uuid.UUID
	reason, details    string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		moments:       map[uuid.UUID]Moment{},
		media:         map[uuid.UUID][]Media{},
		likes:         map[uuid.UUID]map[uuid.UUID]bool{},
		comments:      map[uuid.UUID][]Comment{},
		commentByID:   map[uuid.UUID]Comment{},
		friends:       map[uuid.UUID]map[uuid.UUID]bool{},
		excludedUsers: map[uuid.UUID][]uuid.UUID{},
		hiddenMoments: map[uuid.UUID]map[uuid.UUID]bool{},
	}
}

// addFriend marks an accepted bidirectional contact.
func (fs *fakeStore) addFriend(a, b uuid.UUID) {
	if fs.friends[a] == nil {
		fs.friends[a] = map[uuid.UUID]bool{}
	}
	if fs.friends[b] == nil {
		fs.friends[b] = map[uuid.UUID]bool{}
	}
	fs.friends[a][b] = true
	fs.friends[b][a] = true
}

// seedMoment inserts a moment directly, bypassing the service.
func (fs *fakeStore) seedMoment(id, author uuid.UUID, body, visibility string, created time.Time) Moment {
	m := Moment{
		ID:         id,
		Author:     Author{ID: author, DisplayName: "Author"},
		Body:       body,
		Visibility: visibility,
		CreatedAt:  created,
	}
	fs.moments[id] = m
	return m
}

func (fs *fakeStore) CreateMoment(ctx context.Context, author uuid.UUID, body, visibility string, media []MediaInput, excludedUsers []uuid.UUID) (uuid.UUID, error) {
	if fs.createErr != nil {
		return uuid.Nil, fs.createErr
	}
	id := fs.createdMomentID
	if id == uuid.Nil {
		id = uuid.New()
	}
	m := Moment{
		ID:         id,
		Author:     Author{ID: author, DisplayName: "Author"},
		Body:       body,
		Visibility: visibility,
		CreatedAt:  time.Now().UTC().Truncate(time.Millisecond),
	}
	med := make([]Media, 0, len(media))
	for i, item := range media {
		med = append(med, Media{ID: uuid.New(), URL: item.URL, MediaType: item.MediaType, Position: i})
	}
	m.Media = med
	fs.moments[id] = m
	fs.media[id] = med
	if len(excludedUsers) > 0 {
		fs.excludedUsers[id] = excludedUsers
	}
	return id, nil
}

func (fs *fakeStore) UpdateMoment(ctx context.Context, id, author uuid.UUID, body, visibility *string) (Moment, error) {
	if fs.updateErr != nil {
		return Moment{}, fs.updateErr
	}
	m, ok := fs.moments[id]
	if !ok {
		return Moment{}, ErrMomentNotFound
	}
	if m.Author.ID != author {
		return Moment{}, ErrNotAuthor
	}
	if body != nil {
		m.Body = *body
	}
	if visibility != nil {
		m.Visibility = *visibility
	}
	fs.moments[id] = m
	return m, nil
}

func (fs *fakeStore) HideMoment(ctx context.Context, viewer, momentID uuid.UUID) error {
	if fs.hideErr != nil {
		return fs.hideErr
	}
	if _, ok := fs.moments[momentID]; !ok {
		return ErrMomentNotFound
	}
	if fs.hiddenMoments[viewer] == nil {
		fs.hiddenMoments[viewer] = map[uuid.UUID]bool{}
	}
	fs.hiddenMoments[viewer][momentID] = true
	return nil
}

func (fs *fakeStore) ReportMoment(ctx context.Context, reporter, momentID uuid.UUID, reason, details string) error {
	if fs.reportErr != nil {
		return fs.reportErr
	}
	if _, ok := fs.moments[momentID]; !ok {
		return ErrMomentNotFound
	}
	fs.reports = append(fs.reports, reportedEntry{
		reporter: reporter,
		momentID: momentID,
		reason:   reason,
		details:  details,
	})
	return nil
}

func (fs *fakeStore) GetMoment(ctx context.Context, viewer, id uuid.UUID) (Moment, error) {
	if fs.getErr != nil {
		return Moment{}, fs.getErr
	}
	m, ok := fs.moments[id]
	if !ok {
		return Moment{}, ErrMomentNotFound
	}
	m.Media = fs.media[id]
	m.LikeCount = int64(len(fs.likes[id]))
	m.CommentCount = int64(len(fs.comments[id]))
	m.LikedByMe = fs.likes[id][viewer]
	return m, nil
}

func (fs *fakeStore) ListFeed(ctx context.Context, viewer uuid.UUID, afterCreated *time.Time, afterID *uuid.UUID, limit int) ([]Moment, error) {
	if fs.listErr != nil {
		return nil, fs.listErr
	}
	return fs.feed, nil
}

func (fs *fakeStore) SearchMoments(ctx context.Context, viewer uuid.UUID, query string, afterCreated *time.Time, afterID *uuid.UUID, limit int) ([]Moment, error) {
	fs.gotSearchQuery = query
	if fs.listErr != nil {
		return nil, fs.listErr
	}
	return fs.searchHits, nil
}

func (fs *fakeStore) ListPublicMoments(ctx context.Context, viewer uuid.UUID, afterCreated *time.Time, afterID *uuid.UUID, limit int) ([]Moment, error) {
	fs.gotDiscoverLimit = limit
	if fs.listErr != nil {
		return nil, fs.listErr
	}
	return fs.publicHits, nil
}

func (fs *fakeStore) SoftDeleteMoment(ctx context.Context, id uuid.UUID) error {
	if fs.deleteMomentErr != nil {
		return fs.deleteMomentErr
	}
	delete(fs.moments, id)
	delete(fs.media, id)
	return nil
}

func (fs *fakeStore) IsFriend(ctx context.Context, viewer, author uuid.UUID) (bool, error) {
	return fs.friends[viewer][author], nil
}

func (fs *fakeStore) FriendIDs(ctx context.Context, viewer uuid.UUID) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(fs.friends[viewer]))
	for id, ok := range fs.friends[viewer] {
		if ok {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (fs *fakeStore) Like(ctx context.Context, momentID, user uuid.UUID) (bool, error) {
	if fs.likeErr != nil {
		return false, fs.likeErr
	}
	if fs.likes[momentID] == nil {
		fs.likes[momentID] = map[uuid.UUID]bool{}
	}
	if fs.likes[momentID][user] {
		return false, nil
	}
	fs.likes[momentID][user] = true
	return true, nil
}

func (fs *fakeStore) Unlike(ctx context.Context, momentID, user uuid.UUID) (bool, error) {
	if fs.unlikeErr != nil {
		return false, fs.unlikeErr
	}
	if !fs.likes[momentID][user] {
		return false, nil
	}
	delete(fs.likes[momentID], user)
	return true, nil
}

func (fs *fakeStore) LikeCount(ctx context.Context, momentID uuid.UUID) (int64, error) {
	if fs.likeCountSet {
		return fs.likeCount, nil
	}
	return int64(len(fs.likes[momentID])), nil
}

func (fs *fakeStore) ListComments(ctx context.Context, momentID uuid.UUID, afterCreated *time.Time, afterID *uuid.UUID, limit int) ([]Comment, error) {
	list := fs.comments[momentID]
	if list == nil {
		return []Comment{}, nil
	}
	return list, nil
}

func (fs *fakeStore) GetComment(ctx context.Context, id uuid.UUID) (Comment, error) {
	c, ok := fs.commentByID[id]
	if !ok {
		return Comment{}, ErrCommentNotFound
	}
	return c, nil
}

func (fs *fakeStore) AddComment(ctx context.Context, momentID, author uuid.UUID, body string, replyToID *uuid.UUID) (Comment, error) {
	if fs.addCommentErr != nil {
		return Comment{}, fs.addCommentErr
	}
	c := Comment{
		ID:        uuid.New(),
		MomentID:  momentID,
		Author:    Author{ID: author, DisplayName: "Commenter"},
		ReplyToID: replyToID,
		Body:      body,
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	if replyToID != nil {
		if parent, ok := fs.commentByID[*replyToID]; ok {
			c.ReplyToAuthor = &parent.Author
		}
	}
	fs.commentByID[c.ID] = c
	fs.comments[momentID] = append(fs.comments[momentID], c)
	return c, nil
}

func (fs *fakeStore) SoftDeleteComment(ctx context.Context, commentID, caller uuid.UUID) (bool, error) {
	if !fs.deleteCommentOKSet {
		_, exists := fs.commentByID[commentID]
		if !exists {
			return false, nil
		}
		delete(fs.commentByID, commentID)
		return true, nil
	}
	if fs.deleteCommentOK {
		delete(fs.commentByID, commentID)
		return true, nil
	}
	return false, nil
}

// --- test helpers ---

func testService(store *fakeStore, pub Publisher) *Service {
	return NewService(store, pub)
}

func mustBeError(t *testing.T, err, target error, msg string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error %v, got nil", msg, target)
	}
	if !errors.Is(err, target) {
		t.Fatalf("%s: expected %v, got %v", msg, target, err)
	}
}

// --- Create ---

func TestCreateRejectsOverlongBody(t *testing.T) {
	svc := testService(newFakeStore(), nil)
	body := make([]rune, maxBodyRunes+1)
	for i := range body {
		body[i] = 'a'
	}
	_, err := svc.Create(context.Background(), Author{ID: uuid.New()},
		CreateMomentInput{Body: string(body), Visibility: VisibilityFriends})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "body" {
		t.Fatalf("expected body validation error, got %v", err)
	}
}

func TestCreateRejectsInvalidVisibility(t *testing.T) {
	svc := testService(newFakeStore(), nil)
	_, err := svc.Create(context.Background(), Author{ID: uuid.New()},
		CreateMomentInput{Body: "hi", Visibility: "everyone"})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "visibility" {
		t.Fatalf("expected visibility validation error, got %v", err)
	}
}

func TestCreateRejectsEmptyMoment(t *testing.T) {
	svc := testService(newFakeStore(), nil)
	_, err := svc.Create(context.Background(), Author{ID: uuid.New()},
		CreateMomentInput{Body: "   ", Visibility: VisibilityFriends})
	mustBeError(t, err, ErrEmptyMoment, "empty moment")
}

func TestCreateRejectsTooMuchMedia(t *testing.T) {
	svc := testService(newFakeStore(), nil)
	media := make([]MediaInput, maxMediaPerMoment+1)
	for i := range media {
		media[i] = MediaInput{URL: "https://cdn/x.jpg", MediaType: MediaImage}
	}
	_, err := svc.Create(context.Background(), Author{ID: uuid.New()},
		CreateMomentInput{Visibility: VisibilityFriends, Media: media})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "media" {
		t.Fatalf("expected media validation error, got %v", err)
	}
}

func TestCreateRejectsBadMediaType(t *testing.T) {
	svc := testService(newFakeStore(), nil)
	_, err := svc.Create(context.Background(), Author{ID: uuid.New()},
		CreateMomentInput{Visibility: VisibilityFriends,
			Media: []MediaInput{{URL: "https://cdn/x", MediaType: "gif"}}})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "media" {
		t.Fatalf("expected media validation error, got %v", err)
	}
}

func TestCreateStoresAndReturnsMoment(t *testing.T) {
	author := uuid.New()
	fs := newFakeStore()
	svc := testService(fs, nil)

	m, err := svc.Create(context.Background(), Author{ID: author, DisplayName: "A"},
		CreateMomentInput{Body: "  hello  ", Visibility: ""})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if m.Body != "hello" {
		t.Fatalf("body should be trimmed, got %q", m.Body)
	}
	if m.Visibility != VisibilityFriends {
		t.Fatalf("default visibility should be friends, got %q", m.Visibility)
	}
	if m.Author.ID != author {
		t.Fatalf("author mismatch: %s", m.Author.ID)
	}
	if _, ok := fs.moments[m.ID]; !ok {
		t.Fatal("moment not persisted")
	}
}

func TestCreateFansOutToFriendsOnly(t *testing.T) {
	author := uuid.New()
	friend := uuid.New()
	stranger := uuid.New()
	fs := newFakeStore()
	fs.addFriend(author, friend)
	pub := newRecordingPublisher(friend, stranger)
	svc := testService(fs, pub)

	if _, err := svc.Create(context.Background(), Author{ID: author},
		CreateMomentInput{Body: "post", Visibility: VisibilityFriends}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("expected exactly 1 fan-out event, got %d", len(pub.events))
	}
	if pub.events[0].user != friend {
		t.Fatalf("event went to %s, want friend %s", pub.events[0].user, friend)
	}
}

func TestCreatePrivateMomentDoesNotFanOut(t *testing.T) {
	author := uuid.New()
	friend := uuid.New()
	fs := newFakeStore()
	fs.addFriend(author, friend)
	pub := newRecordingPublisher(friend)
	svc := testService(fs, pub)

	if _, err := svc.Create(context.Background(), Author{ID: author},
		CreateMomentInput{Body: "secret", Visibility: VisibilityPrivate}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(pub.events) != 0 {
		t.Fatalf("private moment must not fan out, got %d events", len(pub.events))
	}
}

// --- visibility authorization ---

func TestGetEnforcesFriendsVisibility(t *testing.T) {
	author := uuid.New()
	friend := uuid.New()
	stranger := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, author, "m", VisibilityFriends, time.Now())
	fs.addFriend(author, friend)
	svc := testService(fs, nil)

	if _, err := svc.Get(context.Background(), author, id); err != nil {
		t.Fatalf("author should view own moment: %v", err)
	}
	if _, err := svc.Get(context.Background(), friend, id); err != nil {
		t.Fatalf("friend should view moment: %v", err)
	}
	if _, err := svc.Get(context.Background(), stranger, id); !errors.Is(err, ErrMomentNotFound) {
		t.Fatalf("stranger should get ErrMomentNotFound, got %v", err)
	}
}

func TestGetEnforcesPrivateVisibility(t *testing.T) {
	author := uuid.New()
	friend := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, author, "m", VisibilityPrivate, time.Now())
	fs.addFriend(author, friend)
	svc := testService(fs, nil)

	if _, err := svc.Get(context.Background(), friend, id); !errors.Is(err, ErrMomentNotFound) {
		t.Fatalf("friend must not see private moment, got %v", err)
	}
}

func TestGetAllowsPublicVisibility(t *testing.T) {
	author := uuid.New()
	stranger := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, author, "m", VisibilityPublic, time.Now())
	svc := testService(fs, nil)

	if _, err := svc.Get(context.Background(), stranger, id); err != nil {
		t.Fatalf("stranger should view public moment: %v", err)
	}
}

// --- delete ownership ---

func TestDeleteRequiresAuthorship(t *testing.T) {
	author := uuid.New()
	other := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, author, "m", VisibilityFriends, time.Now())
	svc := testService(fs, nil)

	if err := svc.Delete(context.Background(), other, id); !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("expected ErrNotAuthor, got %v", err)
	}
	if _, ok := fs.moments[id]; !ok {
		t.Fatal("moment should survive a non-author delete")
	}

	if err := svc.Delete(context.Background(), author, id); err != nil {
		t.Fatalf("author delete: %v", err)
	}
	if _, ok := fs.moments[id]; ok {
		t.Fatal("moment should be deleted by its author")
	}
}

// --- likes ---

func TestLikeIsIdempotentAndCounts(t *testing.T) {
	author := uuid.New()
	user := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, author, "m", VisibilityFriends, time.Now())
	fs.addFriend(author, user)
	svc := testService(fs, nil)

	n1, err := svc.Like(context.Background(), user, id)
	if err != nil {
		t.Fatalf("first like: %v", err)
	}
	if n1 != 1 {
		t.Fatalf("want 1 like, got %d", n1)
	}
	n2, err := svc.Like(context.Background(), user, id)
	if err != nil {
		t.Fatalf("second like: %v", err)
	}
	if n2 != 1 {
		t.Fatalf("like must be idempotent, got %d", n2)
	}

	n3, err := svc.Unlike(context.Background(), user, id)
	if err != nil {
		t.Fatalf("unlike: %v", err)
	}
	if n3 != 0 {
		t.Fatalf("want 0 likes after unlike, got %d", n3)
	}
	n4, err := svc.Unlike(context.Background(), user, id)
	if err != nil {
		t.Fatalf("second unlike: %v", err)
	}
	if n4 != 0 {
		t.Fatalf("unlike must be idempotent, got %d", n4)
	}
}

func TestLikeDeniedForHiddenMoment(t *testing.T) {
	author := uuid.New()
	stranger := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, author, "m", VisibilityFriends, time.Now())
	svc := testService(fs, nil)

	if _, err := svc.Like(context.Background(), stranger, id); !errors.Is(err, ErrMomentNotFound) {
		t.Fatalf("stranger must not like a hidden moment, got %v", err)
	}
}

// --- comments ---

func TestAddCommentValidatesBody(t *testing.T) {
	author := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, author, "m", VisibilityFriends, time.Now())
	svc := testService(fs, nil)

	if _, err := svc.AddComment(context.Background(), author, id, "  ", nil); err == nil {
		t.Fatal("blank comment should fail")
	}
	long := make([]rune, maxCommentRunes+1)
	for i := range long {
		long[i] = 'x'
	}
	_, err := svc.AddComment(context.Background(), author, id, string(long), nil)
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "body" {
		t.Fatalf("expected body validation error, got %v", err)
	}
}

func TestAddCommentDeniedForHiddenMoment(t *testing.T) {
	author := uuid.New()
	stranger := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, author, "m", VisibilityPrivate, time.Now())
	svc := testService(fs, nil)

	if _, err := svc.AddComment(context.Background(), stranger, id, "hi", nil); !errors.Is(err, ErrMomentNotFound) {
		t.Fatalf("stranger must not comment on a private moment, got %v", err)
	}
}

func TestCommentsRequireVisibility(t *testing.T) {
	author := uuid.New()
	stranger := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, author, "m", VisibilityFriends, time.Now())
	svc := testService(fs, nil)

	if _, _, err := svc.Comments(context.Background(), stranger, id, "", 0); !errors.Is(err, ErrMomentNotFound) {
		t.Fatalf("stranger must not list comments, got %v", err)
	}
	comments, _, err := svc.Comments(context.Background(), author, id, "", 0)
	if err != nil {
		t.Fatalf("author list comments: %v", err)
	}
	if comments == nil {
		t.Fatal("comments should be non-nil slice")
	}
}

func TestDeleteCommentNotFoundWhenStoreRejects(t *testing.T) {
	fs := newFakeStore()
	fs.deleteCommentOKSet = true
	fs.deleteCommentOK = false
	svc := testService(fs, nil)

	if err := svc.DeleteComment(context.Background(), uuid.New(), uuid.New()); !errors.Is(err, ErrCommentNotFound) {
		t.Fatalf("expected ErrCommentNotFound, got %v", err)
	}
}

func TestDeleteCommentSucceeds(t *testing.T) {
	fs := newFakeStore()
	c, err := fs.AddComment(context.Background(), uuid.New(), uuid.New(), "keep", nil)
	if err != nil {
		t.Fatalf("seed comment: %v", err)
	}
	svc := testService(fs, nil)

	if err := svc.DeleteComment(context.Background(), c.Author.ID, c.ID); err != nil {
		t.Fatalf("delete comment: %v", err)
	}
	if _, ok := fs.commentByID[c.ID]; ok {
		t.Fatal("comment should be removed")
	}
}

// --- pagination ---

func TestFeedReturnsNextCursorWhenMorePages(t *testing.T) {
	author := uuid.New()
	fs := newFakeStore()
	base := time.Now().UTC().Truncate(time.Microsecond)
	page := make([]Moment, defaultPageSize+1)
	for i := range page {
		page[i] = Moment{ID: uuid.New(), Author: Author{ID: author}, CreatedAt: base}
	}
	fs.feed = page
	svc := testService(fs, nil)

	moments, next, err := svc.Feed(context.Background(), author, "", 0)
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	if len(moments) != defaultPageSize {
		t.Fatalf("want %d moments, got %d", defaultPageSize, len(moments))
	}
	if next == "" {
		t.Fatal("expected a next cursor when more pages exist")
	}
	// The cursor must decode back to the last item returned.
	ct, cid, err := decodeCursor(next)
	if err != nil {
		t.Fatalf("cursor decode: %v", err)
	}
	if *cid != moments[len(moments)-1].ID {
		t.Fatalf("cursor id %s != last returned %s", *cid, moments[len(moments)-1].ID)
	}
	if !ct.Equal(moments[len(moments)-1].CreatedAt.UTC()) {
		t.Fatalf("cursor time %s != last created %s", ct, moments[len(moments)-1].CreatedAt)
	}
}

func TestFeedReturnsNoCursorOnLastPage(t *testing.T) {
	author := uuid.New()
	fs := newFakeStore()
	fs.feed = []Moment{{ID: uuid.New(), Author: Author{ID: author}, CreatedAt: time.Now()}}
	svc := testService(fs, nil)

	moments, next, err := svc.Feed(context.Background(), author, "", 0)
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	if len(moments) != 1 {
		t.Fatalf("want 1 moment, got %d", len(moments))
	}
	if next != "" {
		t.Fatalf("last page should have empty cursor, got %q", next)
	}
}

func TestFeedRejectsMalformedCursor(t *testing.T) {
	svc := testService(newFakeStore(), nil)
	_, _, err := svc.Feed(context.Background(), uuid.New(), "not-a-cursor", 0)
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "cursor" {
		t.Fatalf("expected cursor validation error, got %v", err)
	}
}

func TestSearchPostsRequiresQuery(t *testing.T) {
	svc := testService(newFakeStore(), nil)
	_, _, err := svc.SearchPosts(context.Background(), uuid.New(), "   ", "", 0)
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "q" {
		t.Fatalf("expected q validation error, got %v", err)
	}
}

func TestSearchPostsRejectsOverlongQuery(t *testing.T) {
	svc := testService(newFakeStore(), nil)
	_, _, err := svc.SearchPosts(context.Background(), uuid.New(), strings.Repeat("a", maxSearchQueryRunes+1), "", 0)
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "q" {
		t.Fatalf("expected q validation error, got %v", err)
	}
}

func TestSearchPostsEscapesLikeMetacharacters(t *testing.T) {
	author := uuid.New()
	fs := newFakeStore()
	fs.searchHits = []Moment{{ID: uuid.New(), Author: Author{ID: author}, CreatedAt: time.Now()}}
	svc := testService(fs, nil)

	if _, _, err := svc.SearchPosts(context.Background(), author, "50%_x", "", 0); err != nil {
		t.Fatalf("SearchPosts: %v", err)
	}
	if fs.gotSearchQuery != `50\%\_x` {
		t.Fatalf("query = %q, want escaped metacharacters", fs.gotSearchQuery)
	}
}

func TestSearchPostsPaginates(t *testing.T) {
	author := uuid.New()
	fs := newFakeStore()
	base := time.Now().UTC().Truncate(time.Microsecond)
	page := make([]Moment, defaultPageSize+1)
	for i := range page {
		page[i] = Moment{ID: uuid.New(), Author: Author{ID: author}, CreatedAt: base}
	}
	fs.searchHits = page
	svc := testService(fs, nil)

	moments, next, err := svc.SearchPosts(context.Background(), author, "hi", "", 0)
	if err != nil {
		t.Fatalf("SearchPosts: %v", err)
	}
	if len(moments) != defaultPageSize || next == "" {
		t.Fatalf("len=%d next=%q, want %d and non-empty", len(moments), next, defaultPageSize)
	}
}

func TestDiscoverReturnsPublicMomentsWithCursor(t *testing.T) {
	author := uuid.New()
	fs := newFakeStore()
	base := time.Now().UTC().Truncate(time.Microsecond)
	page := make([]Moment, defaultPageSize+1)
	for i := range page {
		page[i] = Moment{ID: uuid.New(), Author: Author{ID: author}, Visibility: VisibilityPublic, CreatedAt: base}
	}
	fs.publicHits = page
	svc := testService(fs, nil)

	moments, next, err := svc.Discover(context.Background(), author, "", 0)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(moments) != defaultPageSize || next == "" {
		t.Fatalf("len=%d next=%q, want %d and non-empty", len(moments), next, defaultPageSize)
	}
	if fs.gotDiscoverLimit != defaultPageSize+1 {
		t.Fatalf("store limit = %d, want %d", fs.gotDiscoverLimit, defaultPageSize+1)
	}
}

func TestDiscoverRejectsMalformedCursor(t *testing.T) {
	svc := testService(newFakeStore(), nil)
	_, _, err := svc.Discover(context.Background(), uuid.New(), "nope", 0)
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "cursor" {
		t.Fatalf("expected cursor validation error, got %v", err)
	}
}

func TestCommentsReturnsNextCursor(t *testing.T) {
	author := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, author, "m", VisibilityFriends, time.Now())
	base := time.Now().UTC().Truncate(time.Microsecond)
	list := make([]Comment, defaultPageSize+1)
	for i := range list {
		list[i] = Comment{ID: uuid.New(), MomentID: id, Author: Author{ID: author}, CreatedAt: base}
	}
	fs.comments[id] = list
	svc := testService(fs, nil)

	comments, next, err := svc.Comments(context.Background(), author, id, "", 0)
	if err != nil {
		t.Fatalf("comments: %v", err)
	}
	if len(comments) != defaultPageSize {
		t.Fatalf("want %d comments, got %d", defaultPageSize, len(comments))
	}
	if next == "" {
		t.Fatal("expected next cursor")
	}
	if _, _, err := decodeCursor(next); err != nil {
		t.Fatalf("cursor decode: %v", err)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	id := uuid.New()
	ts := time.UnixMicro(1730000000123456).UTC()
	c := encodeCursor(ts, id)
	gotT, gotID, err := decodeCursor(c)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if *gotID != id {
		t.Fatalf("id mismatch: %s != %s", *gotID, id)
	}
	if !gotT.Equal(ts) {
		t.Fatalf("time mismatch: %s != %s", *gotT, ts)
	}
}

// --- Phase 7 tests: Update, Hide, Report, Replies, Exclusions ---

func TestUpdateMoment(t *testing.T) {
	author := uuid.New()
	other := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, author, "Original text", VisibilityFriends, time.Now())
	svc := testService(fs, nil)

	// Update text and visibility by author
	newBody := "Updated text"
	newVis := VisibilityPublic
	updated, err := svc.Update(context.Background(), author, id, UpdateMomentInput{
		Body:       &newBody,
		Visibility: &newVis,
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Body != newBody || updated.Visibility != newVis {
		t.Fatalf("unexpected updated values: body=%q vis=%q", updated.Body, updated.Visibility)
	}

	// Update by non-author should fail
	_, err = svc.Update(context.Background(), other, id, UpdateMomentInput{
		Body: &newBody,
	})
	if !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("expected ErrNotAuthor, got %v", err)
	}
}

func TestUpdateMomentValidation(t *testing.T) {
	author := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, author, "Original text", VisibilityFriends, time.Now())
	svc := testService(fs, nil)

	// Invalid visibility
	badVis := "invalid_vis"
	_, err := svc.Update(context.Background(), author, id, UpdateMomentInput{
		Visibility: &badVis,
	})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "visibility" {
		t.Fatalf("expected visibility validation error, got %v", err)
	}

	// Body too long
	long := make([]rune, maxBodyRunes+1)
	for i := range long {
		long[i] = 'a'
	}
	tooLong := string(long)
	_, err = svc.Update(context.Background(), author, id, UpdateMomentInput{
		Body: &tooLong,
	})
	if !errors.As(err, &ve) || ve.Field != "body" {
		t.Fatalf("expected body validation error, got %v", err)
	}
}

func TestHideMoment(t *testing.T) {
	viewer := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, uuid.New(), "Post", VisibilityFriends, time.Now())
	svc := testService(fs, nil)

	if err := svc.Hide(context.Background(), viewer, id); err != nil {
		t.Fatalf("hide failed: %v", err)
	}
	if !fs.hiddenMoments[viewer][id] {
		t.Fatal("expected moment to be marked hidden for viewer")
	}
}

func TestReportMoment(t *testing.T) {
	reporter := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, uuid.New(), "Post", VisibilityFriends, time.Now())
	svc := testService(fs, nil)

	// Missing reason should fail validation
	err := svc.Report(context.Background(), reporter, id, ReportInput{
		Reason: "   ",
	})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "reason" {
		t.Fatalf("expected reason validation error, got %v", err)
	}

	// Valid report
	err = svc.Report(context.Background(), reporter, id, ReportInput{
		Reason:  "spam",
		Details: "Spam advertising links",
	})
	if err != nil {
		t.Fatalf("report failed: %v", err)
	}
	if len(fs.reports) != 1 || fs.reports[0].reason != "spam" {
		t.Fatalf("expected report recorded, got %+v", fs.reports)
	}
}

func TestAddCommentReply(t *testing.T) {
	author := uuid.New()
	id := uuid.New()
	fs := newFakeStore()
	fs.seedMoment(id, author, "Hello world", VisibilityFriends, time.Now())
	svc := testService(fs, nil)

	// Top level comment
	c1, err := svc.AddComment(context.Background(), author, id, "First comment", nil)
	if err != nil {
		t.Fatalf("add comment: %v", err)
	}
	if c1.ReplyToID != nil {
		t.Fatalf("expected nil reply_to_id, got %v", c1.ReplyToID)
	}

	// Reply to comment
	c2, err := svc.AddComment(context.Background(), author, id, "Replying to first", &c1.ID)
	if err != nil {
		t.Fatalf("add reply: %v", err)
	}
	if c2.ReplyToID == nil || *c2.ReplyToID != c1.ID {
		t.Fatalf("expected reply_to_id %s, got %v", c1.ID, c2.ReplyToID)
	}
	if c2.ReplyToAuthor == nil || c2.ReplyToAuthor.ID != author {
		t.Fatalf("expected reply_to_author %s, got %v", author, c2.ReplyToAuthor)
	}
}

func TestCreateMomentWithExclusions(t *testing.T) {
	author := uuid.New()
	excluded := uuid.New()
	fs := newFakeStore()
	svc := testService(fs, nil)

	moment, err := svc.Create(context.Background(), Author{ID: author, DisplayName: "Alice"}, CreateMomentInput{
		Body:            "Excluding someone",
		Visibility:      VisibilityFriends,
		ExcludedUserIDs: []string{excluded.String()},
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if len(fs.excludedUsers[moment.ID]) != 1 || fs.excludedUsers[moment.ID][0] != excluded {
		t.Fatalf("expected exclusion for %s, got %v", excluded, fs.excludedUsers[moment.ID])
	}
}
