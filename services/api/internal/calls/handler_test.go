package calls

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/auth"
	"github.com/phanindra267/weshare/services/api/internal/config"
)

// testRouter mounts the calls routes with a stub auth middleware that injects
// the given user, mirroring auth.RequireAuth's context contract.
func testRouter(h *Handler, user auth.User) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	requireAuth := func(c *gin.Context) {
		c.Set("auth.current_user", user)
		c.Next()
	}
	h.RegisterRoutes(api, requireAuth)
	return r
}

func TestICEHandlerReturnsConfigAndLimit(t *testing.T) {
	st := newFakeStore()
	svc := NewService(st, newPublisher(), 3)
	cfg := &config.Config{ICEServers: []config.ICEServer{
		{URLs: []string{"stun:stun.example.org:3478"}},
	}}
	h := NewHandler(svc, cfg)

	r := testRouter(h, auth.User{ID: uuid.New()})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/calls/ice", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data ICEConfigResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.MaxParticipants != 3 {
		t.Fatalf("max_participants = %d, want 3", body.Data.MaxParticipants)
	}
	if len(body.Data.ICEServers) != 1 || body.Data.ICEServers[0].URLs[0] != "stun:stun.example.org:3478" {
		t.Fatalf("unexpected ice servers: %+v", body.Data.ICEServers)
	}
}

func TestICEHandlerReturnsEmptyArrayWhenUnconfigured(t *testing.T) {
	svc := NewService(newFakeStore(), newPublisher(), 4)
	h := NewHandler(svc, &config.Config{})

	r := testRouter(h, auth.User{ID: uuid.New()})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/calls/ice", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	// Empty (not null) so the client can rely on an array.
	if body := rec.Body.String(); !strings.Contains(body, `"ice_servers":[]`) {
		t.Fatalf("expected an empty ice_servers array, got %s", body)
	}
}

func TestHistoryHandlerReturnsViewerRecords(t *testing.T) {
	me := uuid.New()
	peer := uuid.New()
	started := time.Now().Add(-2 * time.Minute)
	ended := time.Now().Add(-time.Minute)
	st := newFakeStore()
	st.history = []Call{{
		ID:        uuid.New(),
		CallerID:  me,
		Kind:      KindAudio,
		Status:    StatusEnded,
		StartedAt: &started,
		EndedAt:   &ended,
		CreatedAt: started,
		Participants: []Participant{
			{UserID: me, Role: RoleCaller, State: ParticipantLeft},
			{UserID: peer, Role: RoleCallee, State: ParticipantLeft, Profile: ProfileBrief{ID: peer, DisplayName: "Peer"}},
		},
	}}
	svc := NewService(st, newPublisher(), 4)
	h := NewHandler(svc, &config.Config{})

	r := testRouter(h, auth.User{ID: me})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/calls?limit=10", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			Items      []CallResponse `json:"items"`
			NextCursor string         `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(body.Data.Items))
	}
	item := body.Data.Items[0]
	if item.Direction != "outgoing" || item.Outcome != "completed" {
		t.Fatalf("unexpected item: %+v", item)
	}
	if item.DurationSeconds != 60 {
		t.Fatalf("duration = %d, want 60", item.DurationSeconds)
	}
}
