package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordProducesArgon2idPHC(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("expected argon2id PHC hash, got %q", hash)
	}
}

func TestHashPasswordSaltsHashes(t *testing.T) {
	first, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	second, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if first == second {
		t.Fatal("expected distinct hashes due to random salt")
	}
}

func TestHashPasswordRejectsEmpty(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestVerifyPassword(t *testing.T) {
	hash, err := HashPassword("s3cret-passphrase")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	ok, err := VerifyPassword(hash, "s3cret-passphrase")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}

	ok, err = VerifyPassword(hash, "wrong-password")
	if err != nil {
		t.Fatalf("VerifyPassword (wrong): %v", err)
	}
	if ok {
		t.Fatal("expected wrong password to fail verification")
	}
}

func TestVerifyPasswordHandlesMalformedHash(t *testing.T) {
	ok, err := VerifyPassword("not-a-valid-hash", "whatever")
	if err != nil {
		t.Fatalf("expected no error for malformed hash, got %v", err)
	}
	if ok {
		t.Fatal("expected malformed hash to be treated as mismatch")
	}
}

func TestVerifyPasswordEmptyInputs(t *testing.T) {
	if ok, _ := VerifyPassword("", "pw"); ok {
		t.Fatal("empty hash must not verify")
	}
	if ok, _ := VerifyPassword("$argon2id$x", ""); ok {
		t.Fatal("empty password must not verify")
	}
}
