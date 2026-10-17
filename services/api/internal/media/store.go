package media

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/phanindra267/weshare/services/api/internal/database"
)

// ErrAttachmentNotFound is returned when an attachment row does not exist.
var ErrAttachmentNotFound = errors.New("attachment not found")

// ErrAttachmentForbidden is returned when the caller may not access or delete an attachment.
var ErrAttachmentForbidden = errors.New("attachment access denied")

// AttachmentRecord is one row from message_attachments.
type AttachmentRecord struct {
	ID             uuid.UUID
	UploaderID     uuid.UUID
	MessageID      *uuid.UUID
	ConversationID *uuid.UUID
	StoragePath    string
	ContentType    string
	Kind           string
	OriginalName   string
	SizeBytes      int64
	Pending        bool
	CreatedAt      time.Time
}

// AttachmentStore persists attachment metadata in PostgreSQL.
type AttachmentStore struct {
	db *database.DB
}

// NewAttachmentStore builds an AttachmentStore.
func NewAttachmentStore(db *database.DB) *AttachmentStore {
	return &AttachmentStore{db: db}
}

// InsertPending creates a pending attachment row (before the message is sent).
func (s *AttachmentStore) InsertPending(
	ctx context.Context,
	uploaderID uuid.UUID,
	storagePath, contentType, kind, originalName string,
	sizeBytes int64,
) (AttachmentRecord, error) {
	var a AttachmentRecord
	err := s.db.Pool.QueryRow(ctx, `
		INSERT INTO message_attachments
			(uploader_id, storage_path, content_type, kind, original_name, size_bytes, pending)
		VALUES ($1, $2, $3, $4, $5, $6, TRUE)
		RETURNING id, uploader_id, message_id, conversation_id,
		          storage_path, content_type, kind, original_name, size_bytes, pending, created_at`,
		uploaderID, storagePath, contentType, kind, originalName, sizeBytes,
	).Scan(
		&a.ID, &a.UploaderID, &a.MessageID, &a.ConversationID,
		&a.StoragePath, &a.ContentType, &a.Kind, &a.OriginalName,
		&a.SizeBytes, &a.Pending, &a.CreatedAt,
	)
	if err != nil {
		return AttachmentRecord{}, fmt.Errorf("insert pending attachment: %w", err)
	}
	return a, nil
}

// Confirm links an attachment to a message by its ID, clearing the pending flag.
func (s *AttachmentStore) Confirm(
	ctx context.Context,
	attachmentID, messageID, conversationID uuid.UUID,
) error {
	tag, err := s.db.Pool.Exec(ctx, `
		UPDATE message_attachments
		SET message_id = $2, conversation_id = $3, pending = FALSE
		WHERE id = $1`,
		attachmentID, messageID, conversationID,
	)
	if err != nil {
		return fmt.Errorf("confirm attachment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAttachmentNotFound
	}
	return nil
}

// ConfirmByStoragePath links an attachment by its relative storage path.
func (s *AttachmentStore) ConfirmByStoragePath(
	ctx context.Context,
	storagePath string,
	messageID, conversationID uuid.UUID,
) error {
	tag, err := s.db.Pool.Exec(ctx, `
		UPDATE message_attachments
		SET message_id = $2, conversation_id = $3, pending = FALSE
		WHERE storage_path = $1`,
		storagePath, messageID, conversationID,
	)
	if err != nil {
		return fmt.Errorf("confirm attachment by path: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAttachmentNotFound
	}
	return nil
}

// ByID loads an attachment record by its primary key.
func (s *AttachmentStore) ByID(ctx context.Context, id uuid.UUID) (AttachmentRecord, error) {
	var a AttachmentRecord
	err := s.db.Pool.QueryRow(ctx, `
		SELECT id, uploader_id, message_id, conversation_id,
		       storage_path, content_type, kind, original_name, size_bytes, pending, created_at
		FROM message_attachments WHERE id = $1`, id,
	).Scan(
		&a.ID, &a.UploaderID, &a.MessageID, &a.ConversationID,
		&a.StoragePath, &a.ContentType, &a.Kind, &a.OriginalName,
		&a.SizeBytes, &a.Pending, &a.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AttachmentRecord{}, ErrAttachmentNotFound
	}
	if err != nil {
		return AttachmentRecord{}, fmt.Errorf("attachment by id: %w", err)
	}
	return a, nil
}

// CanAccess reports whether a user may download the attachment: they must be
// the uploader, OR a member of the conversation the attachment belongs to.
func (s *AttachmentStore) CanAccess(ctx context.Context, userID, attachmentID uuid.UUID) (bool, error) {
	var ok bool
	err := s.db.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM message_attachments a
			WHERE a.id = $1
			  AND (
				a.uploader_id = $2
				OR (
					a.conversation_id IS NOT NULL
					AND EXISTS(
						SELECT 1 FROM conversation_members cm
						WHERE cm.conversation_id = a.conversation_id
						  AND cm.user_id = $2
					)
				)
			  )
		)`, attachmentID, userID,
	).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("can access attachment: %w", err)
	}
	return ok, nil
}

// CanAccessPath reports whether a user may download an attachment identified by storage_path.
func (s *AttachmentStore) CanAccessPath(ctx context.Context, userID uuid.UUID, storagePath string) (bool, error) {
	var ok bool
	err := s.db.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM message_attachments a
			WHERE a.storage_path = $1
			  AND (
				a.uploader_id = $2
				OR (
					a.conversation_id IS NOT NULL
					AND EXISTS(
						SELECT 1 FROM conversation_members cm
						WHERE cm.conversation_id = a.conversation_id
						  AND cm.user_id = $2
					)
				)
			  )
		)`, storagePath, userID,
	).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("can access attachment path: %w", err)
	}
	return ok, nil
}

// IsTracked reports whether a storage path is tracked in message_attachments.
func (s *AttachmentStore) IsTracked(ctx context.Context, storagePath string) bool {
	var exists bool
	_ = s.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM message_attachments WHERE storage_path = $1)`,
		storagePath).Scan(&exists)
	return exists
}

// Delete removes an attachment row (uploader only) and returns the storage
// path so the caller can remove the file. Returns ErrAttachmentForbidden when
// the caller is not the uploader.
func (s *AttachmentStore) Delete(ctx context.Context, attachmentID, callerID uuid.UUID) (string, error) {
	var storagePath string
	err := s.db.Pool.QueryRow(ctx, `
		DELETE FROM message_attachments
		WHERE id = $1 AND uploader_id = $2
		RETURNING storage_path`, attachmentID, callerID,
	).Scan(&storagePath)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrAttachmentForbidden
	}
	if err != nil {
		return "", fmt.Errorf("delete attachment: %w", err)
	}
	return storagePath, nil
}

// PurgeOrphaned deletes pending attachment rows older than maxAge and returns
// their storage paths so the caller can remove the physical files.
func (s *AttachmentStore) PurgeOrphaned(ctx context.Context, maxAge time.Duration) ([]string, error) {
	rows, err := s.db.Pool.Query(ctx, `
		DELETE FROM message_attachments
		WHERE pending = TRUE AND created_at < now() - $1::interval
		RETURNING storage_path`,
		fmt.Sprintf("%f seconds", maxAge.Seconds()),
	)
	if err != nil {
		return nil, fmt.Errorf("purge orphaned attachments: %w", err)
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("scan orphaned path: %w", err)
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}
