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

// AllowedImageExts classifies sniffed image content types for chat
// attachments. GIF is accepted for attachments (though not avatars) because the
// sniffing pass is authoritative over the client-declared type.
var AllowedImageExts = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// AllowedAudioExts classifies sniffed audio content types. Browsers produce
// audio/webm and audio/ogg for recorded voice notes, and Go's sniffer reports
// those as video/webm and application/ogg respectively, so both are mapped.
var AllowedAudioExts = map[string]string{
	"audio/mpeg":      ".mp3",
	"audio/mp3":       ".mp3",
	"audio/ogg":       ".ogg",
	"application/ogg": ".ogg",
	"audio/wave":      ".wav",
	"audio/wav":       ".wav",
	"audio/x-wav":     ".wav",
	"audio/webm":      ".webm",
	"video/webm":      ".webm",
}

var safeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

var safeExtPattern = regexp.MustCompile(`^\.[a-z0-9]{1,10}$`)

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

// NewStorage creates the storage root (and its avatars/attachments
// subdirectories) if needed.
func NewStorage(root string) (*Storage, error) {
	root = filepath.Clean(root)
	if err := os.MkdirAll(filepath.Join(root, "avatars"), 0o755); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "attachments"), 0o755); err != nil {
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
	// io.Copy already counted the sniffed head bytes, so `written` is the total.
	if written > maxBytes {
		_ = f.Close()
		_ = os.Remove(abs)
		return "", ErrTooLarge
	}

	return rel, nil
}

// safeExt returns a sanitized lowercase extension for a client-supplied
// filename, defaulting to ".bin" when it is missing or unsafe.
func safeExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if safeExtPattern.MatchString(ext) {
		return ext
	}
	return ".bin"
}

// ContentTypeForExt maps a stored extension to the content type used when
// serving an attachment; unknown extensions are served as octet-stream.
func ContentTypeForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	case ".webm":
		return "audio/webm"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".pdf":
		return "application/pdf"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

// SaveAttachment persists an uploaded chat attachment under a random name and
// returns the storage-relative path, the canonical content type, the classified
// kind ("image", "audio", or "file"), and the stored byte size.
//
// The sniffed bytes decide images and audio (so a mislabeled upload cannot
// smuggle a disallowed type past the client), while anything else is stored
// verbatim as a generic file using the sanitized original extension.
func (s *Storage) SaveAttachment(name string, r io.Reader, maxBytes int64, original string) (rel, contentType, kind string, size int64, err error) {
	if err := os.MkdirAll(filepath.Join(s.root, "attachments"), 0o755); err != nil {
		return "", "", "", 0, fmt.Errorf("ensure attachments dir: %w", err)
	}

	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", "", "", 0, fmt.Errorf("read upload: %w", err)
	}
	head = head[:n]

	sniffed := http.DetectContentType(head)
	var ext string
	switch {
	case AllowedImageExts[sniffed] != "":
		ext, kind, contentType = AllowedImageExts[sniffed], "image", sniffed
	case AllowedAudioExts[sniffed] != "":
		ext, kind, contentType = AllowedAudioExts[sniffed], "audio", sniffed
	default:
		ext, kind = safeExt(original), "file"
		contentType = ContentTypeForExt(ext)
	}

	rel = filepath.Join("attachments", name+ext)
	abs := filepath.Join(s.root, rel)

	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("create attachment file: %w", err)
	}
	defer f.Close()

	written, err := io.Copy(f, io.MultiReader(bytes.NewReader(head), io.LimitReader(r, maxBytes+1)))
	if err != nil {
		_ = f.Close()
		_ = os.Remove(abs)
		return "", "", "", 0, fmt.Errorf("write attachment: %w", err)
	}
	// io.Copy already counted the sniffed head bytes, so `written` is the total.
	size = written
	if size > maxBytes {
		_ = f.Close()
		_ = os.Remove(abs)
		return "", "", "", 0, ErrTooLarge
	}

	return rel, contentType, kind, size, nil
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
