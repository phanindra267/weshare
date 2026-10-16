package auth

import "testing"

func TestRegisterInputValid(t *testing.T) {
	in := RegisterInput{
		Email:    "  Alice@Example.COM ",
		Username: " Alice_01 ",
		Password: "supersecret",
	}
	fields := in.Validate(8)
	if len(fields) != 0 {
		t.Fatalf("expected no field errors, got %v", fields)
	}
	if in.Email != "alice@example.com" {
		t.Fatalf("email not normalized: %q", in.Email)
	}
	if in.Username != "alice_01" {
		t.Fatalf("username not normalized: %q", in.Username)
	}
	if in.DisplayName != "alice_01" {
		t.Fatalf("display name should default to username, got %q", in.DisplayName)
	}
}

func TestRegisterInputRejectsBadFields(t *testing.T) {
	in := RegisterInput{
		Email:    "not-an-email",
		Username: "ab", // too short
		Password: "short",
	}
	fields := in.Validate(8)
	for _, key := range []string{"email", "username", "password"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("expected error for %q, got %v", key, fields)
		}
	}
}

func TestRegisterInputUsernameCharset(t *testing.T) {
	in := RegisterInput{
		Email:    "a@b.com",
		Username: "Bad-Name!",
		Password: "longenough",
	}
	fields := in.Validate(8)
	if _, ok := fields["username"]; !ok {
		t.Fatalf("expected username charset error, got %v", fields)
	}
}

func TestRegisterInputTrimsDisplayName(t *testing.T) {
	in := RegisterInput{
		Email:       "a@b.com",
		Username:    "validname",
		Password:    "longenough",
		DisplayName: "  Alice  ",
	}
	fields := in.Validate(8)
	if len(fields) != 0 {
		t.Fatalf("unexpected errors: %v", fields)
	}
	if in.DisplayName != "Alice" {
		t.Fatalf("expected trimmed display name, got %q", in.DisplayName)
	}
}

func TestLoginInputValidate(t *testing.T) {
	in := LoginInput{Email: "  USER@Example.com ", Password: "x"}
	if fields := in.Validate(); len(fields) != 0 {
		t.Fatalf("unexpected errors: %v", fields)
	}
	if in.Email != "user@example.com" {
		t.Fatalf("email not normalized: %q", in.Email)
	}

	empty := LoginInput{}
	fields := empty.Validate()
	if _, ok := fields["email"]; !ok {
		t.Fatalf("expected email required, got %v", fields)
	}
	if _, ok := fields["password"]; !ok {
		t.Fatalf("expected password required, got %v", fields)
	}
}
