// Package media stores and serves binary assets (avatars now, chat
// attachments later) on the local filesystem.
package media

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// AllowedAvatarExts maps a sniffed content type to a canonical extension.
var AllowedAvatarExts = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

var safeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

var (
	// ErrInvalidName is returned for filenames that could escape the root.
	ErrInvalidName = errors.New("invalid media name")
	// ErrUnsupportedImage is returned when an upload is not an allowed type.
	ErrUnsupportedImage = errors.New("unsupported image type")
	// ErrTooLarge is returned when an upload exceeds the size limit.
	ErrTooLarge = errors.New("upload exceeds size limit")
)

// Storage is a rooted local filesystem store.
type Storage struct {
	root string
}

// NewStorage creates the storage root (and an avatars subdirectory) if needed.
func NewStorage(root string) (*Storage, error) {
	root = filepath.Clean(root)
	if err := os.MkdirAll(filepath.Join(root, "avatars"), 0o755); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	return &Storage{root: root}, nil
}

// SaveAvatar persists avatar bytes under a random name and returns the
// storage-relative path (e.g. "avatars/<id>.jpg").
func (s *Storage) SaveAvatar(name string, r io.Reader, maxBytes int64) (string, error) {
	if err := os.MkdirAll(filepath.Join(s.root, "avatars"), 0o755); err != nil {
		return "", fmt.Errorf("ensure avatars dir: %w", err)
	}

	// Sniff the first 512 bytes to decide the real content type.
	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read upload: %w", err)
	}
	head = head[:n]

	contentType := http.DetectContentType(head)
	ext, ok := AllowedAvatarExts[contentType]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnsupportedImage, contentType)
	}

	rel := filepath.Join("avatars", name+ext)
	abs := filepath.Join(s.root, rel)

	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("create avatar file: %w", err)
	}
	defer f.Close()

	written, err := io.Copy(f, io.MultiReader(bytes.NewReader(head), io.LimitReader(r, maxBytes+1)))
	if err != nil {
		_ = f.Close()
		_ = os.Remove(abs)
		return "", fmt.Errorf("write avatar: %w", err)
	}
	if int64(len(head))+written > maxBytes {
		_ = f.Close()
		_ = os.Remove(abs)
		return "", ErrTooLarge
	}

	return rel, nil
}

// Open validates and opens a stored object by category and name.
func (s *Storage) Open(category, name string) (*os.File, error) {
	if !safeNamePattern.MatchString(name) || strings.ContainsAny(name, `/\`) {
		return nil, ErrInvalidName
	}
	if strings.Contains(category, "..") || strings.ContainsAny(category, `/\`) {
		return nil, ErrInvalidName
	}
	abs := filepath.Join(s.root, category, name)
	// Defense in depth: ensure the resolved path stays under root.
	if !strings.HasPrefix(abs, filepath.Clean(s.root)+string(os.PathSeparator)) {
		return nil, ErrInvalidName
	}
	return os.Open(abs)
}

// Remove deletes a stored object, ignoring missing files.
func (s *Storage) Remove(rel string) error {
	abs := filepath.Join(s.root, rel)
	if !strings.HasPrefix(abs, filepath.Clean(s.root)+string(os.PathSeparator)) {
		return ErrInvalidName
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Root returns the storage root (used by tests).
func (s *Storage) Root() string { return s.root }

// Ext returns the file extension (with leading dot) of a stored path.
func Ext(rel string) string { return filepath.Ext(rel) }
