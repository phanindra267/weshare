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

func gifBytes(n int) []byte {
	sig := []byte("GIF89a")
	buf := make([]byte, n)
	copy(buf, sig)
	return buf
}

func wavBytes(n int) []byte {
	// RIFF....WAVE header; DetectContentType recognizes the signature.
	buf := make([]byte, n)
	copy(buf, []byte("RIFF"))
	if n > 12 {
		copy(buf[8:], []byte("WAVE"))
	}
	return buf
}

func TestSaveAttachmentImage(t *testing.T) {
	s := newTestStorage(t)
	rel, contentType, kind, size, err := s.SaveAttachment("a1", bytes.NewReader(gifBytes(2048)), 1<<20, "photo.gif")
	if err != nil {
		t.Fatalf("SaveAttachment: %v", err)
	}
	if kind != "image" || contentType != "image/gif" {
		t.Fatalf("kind=%q contentType=%q", kind, contentType)
	}
	if rel != filepath.Join("attachments", "a1.gif") || size != 2048 {
		t.Fatalf("rel=%q size=%d", rel, size)
	}
	if _, err := os.Stat(filepath.Join(s.Root(), rel)); err != nil {
		t.Fatalf("file not written: %v", err)
	}
}

func TestSaveAttachmentAudio(t *testing.T) {
	s := newTestStorage(t)
	rel, _, kind, _, err := s.SaveAttachment("v1", bytes.NewReader(wavBytes(4096)), 1<<20, "voice.wav")
	if err != nil {
		t.Fatalf("SaveAttachment: %v", err)
	}
	if kind != "audio" {
		t.Fatalf("kind = %q, want audio", kind)
	}
	if filepath.Ext(rel) != ".wav" {
		t.Fatalf("ext = %q, want .wav", filepath.Ext(rel))
	}
}

func TestSaveAttachmentGenericFile(t *testing.T) {
	s := newTestStorage(t)
	rel, contentType, kind, _, err := s.SaveAttachment("f1", bytes.NewReader([]byte("hello, world")), 1<<20, "notes.txt")
	if err != nil {
		t.Fatalf("SaveAttachment: %v", err)
	}
	if kind != "file" {
		t.Fatalf("kind = %q, want file", kind)
	}
	if rel != filepath.Join("attachments", "f1.txt") {
		t.Fatalf("rel = %q", rel)
	}
	if contentType != "text/plain; charset=utf-8" {
		t.Fatalf("contentType = %q", contentType)
	}
}

func TestSaveAttachmentSanitizesExtension(t *testing.T) {
	s := newTestStorage(t)
	rel, _, _, _, err := s.SaveAttachment("f2", bytes.NewReader([]byte("data")), 1<<20, "weird.<script>")
	if err != nil {
		t.Fatalf("SaveAttachment: %v", err)
	}
	if filepath.Ext(rel) != ".bin" {
		t.Fatalf("ext = %q, want .bin", filepath.Ext(rel))
	}
}

func TestSaveAttachmentRejectsTooLarge(t *testing.T) {
	s := newTestStorage(t)
	_, _, _, _, err := s.SaveAttachment("big", bytes.NewReader([]byte("0123456789")), 4, "big.bin")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Root(), "attachments", "big.bin")); !os.IsNotExist(err) {
		t.Fatalf("expected oversized attachment removed, stat err=%v", err)
	}
}

func TestSaveAttachmentAtLimitIsAccepted(t *testing.T) {
	s := newTestStorage(t)
	data := []byte("0123456789") // exactly 10 bytes
	_, _, _, size, err := s.SaveAttachment("edge", bytes.NewReader(data), 10, "edge.bin")
	if err != nil {
		t.Fatalf("SaveAttachment at limit: %v", err)
	}
	if size != 10 {
		t.Fatalf("size = %d, want 10", size)
	}
}

func TestSaveAvatarAtLimitIsAccepted(t *testing.T) {
	s := newTestStorage(t)
	if _, err := s.SaveAvatar("edge", bytes.NewReader(pngBytes(1024)), 1024); err != nil {
		t.Fatalf("SaveAvatar at limit: %v", err)
	}
}

func mp4Bytes(n int) []byte {
	sig := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}
	buf := make([]byte, n)
	copy(buf, sig)
	return buf
}

func TestSaveAttachmentVideo(t *testing.T) {
	s := newTestStorage(t)
	rel, contentType, kind, size, err := s.SaveAttachment("vid1", bytes.NewReader(mp4Bytes(2048)), 1<<20, "video.mp4")
	if err != nil {
		t.Fatalf("SaveAttachment: %v", err)
	}
	if kind != "video" || contentType != "video/mp4" {
		t.Fatalf("kind=%q contentType=%q, want video and video/mp4", kind, contentType)
	}
	if rel != filepath.Join("attachments", "vid1.mp4") || size != 2048 {
		t.Fatalf("rel=%q size=%d", rel, size)
	}
	if _, err := os.Stat(filepath.Join(s.Root(), rel)); err != nil {
		t.Fatalf("video file not written: %v", err)
	}
}

func TestDeleteFile(t *testing.T) {
	s := newTestStorage(t)
	rel, _, _, _, err := s.SaveAttachment("del1", bytes.NewReader([]byte("temp")), 1<<20, "temp.txt")
	if err != nil {
		t.Fatalf("SaveAttachment: %v", err)
	}
	if err := s.DeleteFile(rel); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Root(), rel)); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted")
	}
}

func TestContentTypeForExt(t *testing.T) {
	cases := map[string]string{
		".png":  "image/png",
		".mp3":  "audio/mpeg",
		".ogg":  "audio/ogg",
		".mp4":  "video/mp4",
		".mov":  "video/quicktime",
		".webm": "video/webm",
		".weba": "application/octet-stream",
		".unk":  "application/octet-stream",
	}
	for ext, want := range cases {
		if got := ContentTypeForExt(ext); got != want {
			t.Fatalf("ContentTypeForExt(%q) = %q, want %q", ext, got, want)
		}
	}
}
