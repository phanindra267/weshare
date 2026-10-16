package media

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func pngBytes(n int) []byte {
	// PNG signature followed by filler; DetectContentType keys off the header.
	sig := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	buf := make([]byte, n)
	copy(buf, sig)
	for i := len(sig); i < n; i++ {
		buf[i] = 0x00
	}
	return buf
}

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	s, err := NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	return s
}

func TestSaveAvatarPNG(t *testing.T) {
	s := newTestStorage(t)
	rel, err := s.SaveAvatar("user-123", bytes.NewReader(pngBytes(1024)), 1<<20)
	if err != nil {
		t.Fatalf("SaveAvatar: %v", err)
	}
	if rel != filepath.Join("avatars", "user-123.png") {
		t.Fatalf("unexpected rel path: %q", rel)
	}
	if _, err := os.Stat(filepath.Join(s.Root(), rel)); err != nil {
		t.Fatalf("file not written: %v", err)
	}
}

func TestSaveAvatarRejectsNonImage(t *testing.T) {
	s := newTestStorage(t)
	_, err := s.SaveAvatar("x", bytes.NewReader([]byte("this is plain text, not an image")), 1<<20)
	if !errors.Is(err, ErrUnsupportedImage) {
		t.Fatalf("expected ErrUnsupportedImage, got %v", err)
	}
}

func TestSaveAvatarRejectsTooLarge(t *testing.T) {
	s := newTestStorage(t)
	_, err := s.SaveAvatar("big", bytes.NewReader(pngBytes(4096)), 1024)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
	// The oversized file must be removed.
	if _, err := os.Stat(filepath.Join(s.Root(), "avatars", "big.png")); !os.IsNotExist(err) {
		t.Fatalf("expected oversized file to be removed, stat err=%v", err)
	}
}

func TestOpenRejectsTraversal(t *testing.T) {
	s := newTestStorage(t)
	for _, name := range []string{"../secret", "..\\secret", "a/b", "a\\b", ""} {
		if _, err := s.Open("avatars", name); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("expected ErrInvalidName for %q, got %v", name, err)
		}
	}
}

func TestOpenValidFile(t *testing.T) {
	s := newTestStorage(t)
	if _, err := s.SaveAvatar("ok", bytes.NewReader(pngBytes(600)), 1<<20); err != nil {
		t.Fatalf("SaveAvatar: %v", err)
	}
	f, err := s.Open("avatars", "ok.png")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
}

func TestRemove(t *testing.T) {
	s := newTestStorage(t)
	rel, err := s.SaveAvatar("gone", bytes.NewReader(pngBytes(600)), 1<<20)
	if err != nil {
		t.Fatalf("SaveAvatar: %v", err)
	}
	if err := s.Remove(rel); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Root(), rel)); !os.IsNotExist(err) {
		t.Fatalf("expected file removed")
	}
	// Removing again is a no-op.
	if err := s.Remove(rel); err != nil {
		t.Fatalf("Remove missing should not error: %v", err)
	}
}
