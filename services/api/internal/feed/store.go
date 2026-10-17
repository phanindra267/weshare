package feed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/phanindra267/weshare/services/api/internal/database"
)

// Store provides Moment persistence on top of PostgreSQL.
type Store struct {
	db *database.DB
}

// NewStore builds a feed Store.
func NewStore(db *database.DB) *Store { return &Store{db: db} }

// momentColumns is the projection shared by the single-moment and feed queries.
// $1 is always the viewer (used for the LikedByMe flag).
const momentColumns = `
	SELECT m.id, m.body, m.visibility, m.created_at,
	       u.id, u.display_name, u.username, u.avatar_url,
	       (SELECT count(*) FROM moment_likes l WHERE l.moment_id = m.id) AS like_count,
	       (SELECT count(*) FROM moment_comments c
	          WHERE c.moment_id = m.id AND c.deleted_at IS NULL) AS comment_count,
	       EXISTS(SELECT 1 FROM moment_likes l
	                WHERE l.moment_id = m.id AND l.user_id = $1) AS liked_by_me
	FROM moments m
	JOIN users u ON u.id = m.author_id`

func scanMoment(row pgx.Row) (Moment, error) {
	var m Moment
	err := row.Scan(&m.ID, &m.Body, &m.Visibility, &m.CreatedAt,
		&m.Author.ID, &m.Author.DisplayName, &m.Author.Username, &m.Author.AvatarURL,
		&m.LikeCount, &m.CommentCount, &m.LikedByMe)
	return m, err
}

// nullableUUID turns an optional id into a query argument (nil -> SQL NULL).
func nullableUUID(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return *id
}

// nullableTime is the time.Time counterpart of nullableUUID.
func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

// CreateMoment inserts a moment, its ordered media, and optional exclusions in one transaction,
// returning the new id.
func (st *Store) CreateMoment(ctx context.Context, author uuid.UUID, body, visibility string, media []MediaInput, excludedUsers []uuid.UUID) (uuid.UUID, error) {
	tx, err := st.db.Pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin moment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO moments (author_id, body, visibility)
		 VALUES ($1, $2, $3) RETURNING id`,
		author, body, visibility).Scan(&id); err != nil {
		return uuid.Nil, fmt.Errorf("insert moment: %w", err)
	}

	for i, m := range media {
		if _, err := tx.Exec(ctx,
			`INSERT INTO moment_media (moment_id, url, media_type, position)
			 VALUES ($1, $2, $3, $4)`,
			id, m.URL, m.MediaType, i); err != nil {
			return uuid.Nil, fmt.Errorf("insert moment media: %w", err)
		}
	}

	for _, exUser := range excludedUsers {
		if _, err := tx.Exec(ctx,
			`INSERT INTO moment_exclusions (moment_id, user_id)
			 VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			id, exUser); err != nil {
			return uuid.Nil, fmt.Errorf("insert moment exclusion: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit moment: %w", err)
	}
	return id, nil
}

// UpdateMoment modifies a moment's text or visibility (author only).
func (st *Store) UpdateMoment(ctx context.Context, id, author uuid.UUID, body, visibility *string) (Moment, error) {
	tag, err := st.db.Pool.Exec(ctx, `
		UPDATE moments
		SET body = COALESCE($3, body),
		    visibility = COALESCE($4, visibility),
		    updated_at = now()
		WHERE id = $1 AND author_id = $2 AND deleted_at IS NULL`,
		id, author, body, visibility)
	if err != nil {
		return Moment{}, fmt.Errorf("update moment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Moment{}, ErrMomentNotFound
	}
	return st.GetMoment(ctx, author, id)
}

// GetMoment loads a single moment joined with its author, media, and counts.
// Visibility, exclusions, and blocking are verified in SQL.
func (st *Store) GetMoment(ctx context.Context, viewer, id uuid.UUID) (Moment, error) {
	m, err := scanMoment(st.db.Pool.QueryRow(ctx,
		momentColumns+`
		WHERE m.id = $2 AND m.deleted_at IS NULL
		  AND (
		    m.author_id = $1
		    OR m.visibility = '`+VisibilityPublic+`'
		    OR (m.visibility = '`+VisibilityFriends+`' AND EXISTS (
		         SELECT 1 FROM contacts c
		         WHERE c.user_id = $1
		           AND c.contact_id = m.author_id
		           AND c.status = 'accepted'))
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM moment_exclusions ex
		    WHERE ex.moment_id = m.id AND ex.user_id = $1
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM contacts bl
		    WHERE bl.status = 'blocked'
		      AND ((bl.user_id = $1 AND bl.contact_id = m.author_id)
		        OR (bl.user_id = m.author_id AND bl.contact_id = $1))
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM moment_hidden h
		    WHERE h.user_id = $1 AND h.moment_id = m.id
		  )`, viewer, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Moment{}, ErrMomentNotFound
	}
	if err != nil {
		return Moment{}, fmt.Errorf("get moment: %w", err)
	}
	media, err := st.loadMedia(ctx, []uuid.UUID{m.ID})
	if err != nil {
		return Moment{}, err
	}
	m.Media = media[m.ID]
	return m, nil
}

// loadMedia fetches media for a set of moments, grouped by moment id and ordered by position.
func (st *Store) loadMedia(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]Media, error) {
	out := make(map[uuid.UUID][]Media, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := st.db.Pool.Query(ctx,
		`SELECT id, moment_id, url, media_type, position
		 FROM moment_media WHERE moment_id = ANY($1)
		 ORDER BY moment_id, position`, ids)
	if err != nil {
		return nil, fmt.Errorf("load moment media: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var m Media
		var momentID uuid.UUID
		if err := rows.Scan(&m.ID, &momentID, &m.URL, &m.MediaType, &m.Position); err != nil {
			return nil, fmt.Errorf("scan moment media: %w", err)
		}
		out[momentID] = append(out[momentID], m)
	}
	return out, rows.Err()
}

// ListFeed returns one page of moments visible to the viewer, newest first,
// with their media attached. Exclusions, blocking, and hidden posts are filtered out.
func (st *Store) ListFeed(ctx context.Context, viewer uuid.UUID, afterCreated *time.Time, afterID *uuid.UUID, limit int) ([]Moment, error) {
	rows, err := st.db.Pool.Query(ctx, momentColumns+`
		WHERE m.deleted_at IS NULL
		  AND (
		    m.author_id = $1
		    OR m.visibility = '`+VisibilityPublic+`'
		    OR (m.visibility = '`+VisibilityFriends+`' AND EXISTS (
		         SELECT 1 FROM contacts c
		         WHERE c.user_id = $1
		           AND c.contact_id = m.author_id
		           AND c.status = 'accepted'))
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM moment_exclusions ex
		    WHERE ex.moment_id = m.id AND ex.user_id = $1
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM contacts bl
		    WHERE bl.status = 'blocked'
		      AND ((bl.user_id = $1 AND bl.contact_id = m.author_id)
		        OR (bl.user_id = m.author_id AND bl.contact_id = $1))
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM moment_hidden h
		    WHERE h.user_id = $1 AND h.moment_id = m.id
		  )
		  AND ($2::timestamptz IS NULL
		       OR (m.created_at, m.id) < ($2::timestamptz, $3::uuid))
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT $4`,
		viewer, nullableTime(afterCreated), nullableUUID(afterID), limit)
	if err != nil {
		return nil, fmt.Errorf("list feed: %w", err)
	}
	defer rows.Close()

	moments := make([]Moment, 0, limit)
	ids := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		m, err := scanMoment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan feed moment: %w", err)
		}
		moments = append(moments, m)
		ids = append(ids, m.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feed: %w", err)
	}

	media, err := st.loadMedia(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range moments {
		moments[i].Media = media[moments[i].ID]
	}
	return moments, nil
}

// SoftDeleteMoment marks a moment deleted.
func (st *Store) SoftDeleteMoment(ctx context.Context, id uuid.UUID) error {
	if _, err := st.db.Pool.Exec(ctx,
		`UPDATE moments SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`,
		id); err != nil {
		return fmt.Errorf("delete moment: %w", err)
	}
	return nil
}

// HideMoment records a viewer's choice to hide a moment from their feed.
func (st *Store) HideMoment(ctx context.Context, viewer, momentID uuid.UUID) error {
	_, err := st.db.Pool.Exec(ctx, `
		INSERT INTO moment_hidden (user_id, moment_id)
		VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		viewer, momentID)
	if err != nil {
		return fmt.Errorf("hide moment: %w", err)
	}
	return nil
}

// ReportMoment records a user report on a moment.
func (st *Store) ReportMoment(ctx context.Context, reporter, momentID uuid.UUID, reason, details string) error {
	_, err := st.db.Pool.Exec(ctx, `
		INSERT INTO moment_reports (moment_id, reporter_id, reason, details)
		VALUES ($1, $2, $3, $4)`,
		momentID, reporter, reason, details)
	if err != nil {
		return fmt.Errorf("report moment: %w", err)
	}
	return nil
}

// IsFriend reports whether `viewer` has an accepted contact row for `author`.
func (st *Store) IsFriend(ctx context.Context, viewer, author uuid.UUID) (bool, error) {
	var ok bool
	err := st.db.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM contacts
			WHERE user_id = $1 AND contact_id = $2 AND status = 'accepted'
		)`, viewer, author).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("is friend: %w", err)
	}
	return ok, nil
}

// FriendIDs returns the viewer's accepted contacts (used to fan out new moments).
func (st *Store) FriendIDs(ctx context.Context, viewer uuid.UUID) ([]uuid.UUID, error) {
	rows, err := st.db.Pool.Query(ctx,
		`SELECT contact_id FROM contacts
		 WHERE user_id = $1 AND status = 'accepted'`, viewer)
	if err != nil {
		return nil, fmt.Errorf("friend ids: %w", err)
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan friend id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Like records a like, reporting whether it was newly created (idempotent).
func (st *Store) Like(ctx context.Context, momentID, user uuid.UUID) (bool, error) {
	tag, err := st.db.Pool.Exec(ctx,
		`INSERT INTO moment_likes (moment_id, user_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`, momentID, user)
	if err != nil {
		return false, fmt.Errorf("like moment: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// Unlike removes a like, reporting whether one existed.
func (st *Store) Unlike(ctx context.Context, momentID, user uuid.UUID) (bool, error) {
	tag, err := st.db.Pool.Exec(ctx,
		`DELETE FROM moment_likes WHERE moment_id = $1 AND user_id = $2`,
		momentID, user)
	if err != nil {
		return false, fmt.Errorf("unlike moment: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// LikeCount returns the current number of likes on a moment.
func (st *Store) LikeCount(ctx context.Context, momentID uuid.UUID) (int64, error) {
	var n int64
	if err := st.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM moment_likes WHERE moment_id = $1`,
		momentID).Scan(&n); err != nil {
		return 0, fmt.Errorf("like count: %w", err)
	}
	return n, nil
}

func scanCommentRow(row pgx.Row) (Comment, error) {
	var c Comment
	var replyToID *uuid.UUID
	var replyAuthorID *uuid.UUID
	var replyAuthorName *string
	var replyAuthorUsername *string
	var replyAuthorAvatar *string

	err := row.Scan(
		&c.ID, &c.MomentID, &c.Body, &c.CreatedAt,
		&c.Author.ID, &c.Author.DisplayName, &c.Author.Username, &c.Author.AvatarURL,
		&replyToID,
		&replyAuthorID, &replyAuthorName, &replyAuthorUsername, &replyAuthorAvatar,
	)
	if err != nil {
		return Comment{}, err
	}
	c.ReplyToID = replyToID
	if replyAuthorID != nil && replyAuthorName != nil {
		c.ReplyToAuthor = &Author{
			ID:          *replyAuthorID,
			DisplayName: *replyAuthorName,
			Username:    replyAuthorUsername,
			AvatarURL:   replyAuthorAvatar,
		}
	}
	return c, nil
}

const commentSelectSQL = `
	SELECT c.id, c.moment_id, c.body, c.created_at,
	       u.id, u.display_name, u.username, u.avatar_url,
	       c.reply_to_id,
	       ru.id, ru.display_name, ru.username, ru.avatar_url
	FROM moment_comments c
	JOIN users u ON u.id = c.author_id
	LEFT JOIN moment_comments rc ON rc.id = c.reply_to_id
	LEFT JOIN users ru ON ru.id = rc.author_id`

// ListComments returns one page of a moment's comments, oldest first, with reply targets.
func (st *Store) ListComments(ctx context.Context, momentID uuid.UUID, afterCreated *time.Time, afterID *uuid.UUID, limit int) ([]Comment, error) {
	rows, err := st.db.Pool.Query(ctx, commentSelectSQL+`
		WHERE c.moment_id = $1 AND c.deleted_at IS NULL
		  AND ($2::timestamptz IS NULL
		       OR (c.created_at, c.id) > ($2::timestamptz, $3::uuid))
		ORDER BY c.created_at ASC, c.id ASC
		LIMIT $4`,
		momentID, nullableTime(afterCreated), nullableUUID(afterID), limit)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	comments := make([]Comment, 0, limit)
	for rows.Next() {
		c, err := scanCommentRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// GetComment loads a single live comment with its author and reply target.
func (st *Store) GetComment(ctx context.Context, id uuid.UUID) (Comment, error) {
	c, err := scanCommentRow(st.db.Pool.QueryRow(ctx,
		commentSelectSQL+`
		WHERE c.id = $1 AND c.deleted_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Comment{}, ErrCommentNotFound
	}
	if err != nil {
		return Comment{}, fmt.Errorf("get comment: %w", err)
	}
	return c, nil
}

// AddComment inserts a comment (optionally replying to an existing comment) and returns it.
func (st *Store) AddComment(ctx context.Context, momentID, author uuid.UUID, body string, replyToID *uuid.UUID) (Comment, error) {
	var id uuid.UUID
	if err := st.db.Pool.QueryRow(ctx,
		`INSERT INTO moment_comments (moment_id, author_id, body, reply_to_id)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		momentID, author, body, replyToID).Scan(&id); err != nil {
		return Comment{}, fmt.Errorf("insert comment: %w", err)
	}
	return st.GetComment(ctx, id)
}

// SoftDeleteComment deletes a comment when the caller is its author or the
// author of the parent moment, reporting whether a row was removed.
func (st *Store) SoftDeleteComment(ctx context.Context, commentID, caller uuid.UUID) (bool, error) {
	tag, err := st.db.Pool.Exec(ctx, `
		UPDATE moment_comments c SET deleted_at = now()
		WHERE c.id = $1 AND c.deleted_at IS NULL
		  AND (c.author_id = $2
		       OR EXISTS (SELECT 1 FROM moments m
		                    WHERE m.id = c.moment_id AND m.author_id = $2))`,
		commentID, caller)
	if err != nil {
		return false, fmt.Errorf("delete comment: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
