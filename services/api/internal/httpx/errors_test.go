package httpx

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAsAppErrorPassesThroughAppError(t *testing.T) {
	orig := NotFound("nope")
	got := AsAppError(orig)
	if got != orig {
		t.Fatalf("AsAppError should return the same *AppError")
	}
	if got.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want 404", got.Status)
	}
}

func TestAsAppErrorWrapsGenericErrorWithoutLeakingCause(t *testing.T) {
	secret := errors.New("postgres: password authentication failed")
	got := AsAppError(secret)

	if got.Status != http.StatusInternalServerError {
		t.Errorf("Status = %d, want 500", got.Status)
	}
	if got.Code != CodeInternal {
		t.Errorf("Code = %q, want %q", got.Code, CodeInternal)
	}
	// The message shown to clients must never contain the internal cause.
	if got.Message == secret.Error() || got.Message == "" {
		t.Errorf("client message leaked internals: %q", got.Message)
	}
	// ...but the cause is preserved for logging.
	if !errors.Is(got, secret) {
		t.Errorf("wrapped cause should be retrievable via errors.Is")
	}
}

func TestValidationCarriesDetails(t *testing.T) {
	err := Validation("bad", gin.H{"field": "email"})
	if err.Status != http.StatusUnprocessableEntity {
		t.Errorf("Status = %d, want 422", err.Status)
	}
	if err.Details == nil {
		t.Error("expected details to be set")
	}
}

func TestRequestIDContextRoundTrip(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-123")
	if got := RequestIDFromContext(ctx); got != "req-123" {
		t.Errorf("got %q, want req-123", got)
	}
	if got := RequestIDFromContext(context.Background()); got != "" {
		t.Errorf("empty ctx should yield empty id, got %q", got)
	}
}

func TestNewPaginatedNormalisesNilSlice(t *testing.T) {
	p := NewPaginated([]string(nil), "")
	if p.Items == nil || len(p.Items) != 0 {
		t.Errorf("Items = %v, want empty non-nil slice", p.Items)
	}
}
