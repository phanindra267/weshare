package calls

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOutcomeFor(t *testing.T) {
	caller := uuid.New()
	callee := uuid.New()
	newCall := func(status, callerState, calleeState string) Call {
		return Call{
			ID:       uuid.New(),
			CallerID: caller,
			Status:   status,
			Participants: []Participant{
				{UserID: caller, Role: RoleCaller, State: callerState},
				{UserID: callee, Role: RoleCallee, State: calleeState},
			},
		}
	}

	cases := []struct {
		name   string
		call   Call
		viewer uuid.UUID
		want   string
	}{
		{"active", newCall(StatusActive, ParticipantJoined, ParticipantJoined), caller, "active"},
		{"completed", newCall(StatusEnded, ParticipantLeft, ParticipantLeft), caller, "completed"},
		{"rejected-by-me", newCall(StatusRejected, ParticipantLeft, ParticipantRejected), callee, "rejected"},
		{"declined-by-peer", newCall(StatusRejected, ParticipantLeft, ParticipantRejected), caller, "declined"},
		{"missed", newCall(StatusCanceled, ParticipantLeft, ParticipantMissed), callee, "missed"},
		{"cancelled-by-me", newCall(StatusCanceled, ParticipantLeft, ParticipantMissed), caller, "cancelled"},
		{"failed", newCall(StatusFailed, ParticipantLeft, ParticipantMissed), caller, "failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toCallResponse(tc.call, tc.viewer, time.Now()).Outcome
			if got != tc.want {
				t.Fatalf("outcome = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestToCallResponseDirectionAndParticipants(t *testing.T) {
	viewer := uuid.New()
	peer := uuid.New()
	uname := "peer"
	c := Call{
		ID:        uuid.New(),
		CallerID:  viewer,
		Kind:      KindVideo,
		IsGroup:   false,
		Status:    StatusEnded,
		CreatedAt: time.Now().UTC(),
		Participants: []Participant{
			{UserID: viewer, Role: RoleCaller, State: ParticipantLeft},
			{UserID: peer, Role: RoleCallee, State: ParticipantJoined, Profile: ProfileBrief{ID: peer, DisplayName: "Peer", Username: &uname}},
		},
	}
	resp := toCallResponse(c, viewer, time.Now())
	if resp.Direction != "outgoing" {
		t.Fatalf("direction = %q, want outgoing", resp.Direction)
	}
	if resp.Kind != KindVideo || resp.Group {
		t.Fatalf("unexpected kind/group: %+v", resp)
	}
	if len(resp.Participants) != 2 {
		t.Fatalf("participants = %d, want 2", len(resp.Participants))
	}
	if resp.Participants[1].DisplayName != "Peer" || resp.Participants[1].Username == nil || *resp.Participants[1].Username != "peer" {
		t.Fatalf("unexpected participant: %+v", resp.Participants[1])
	}
}

func TestDurationSeconds(t *testing.T) {
	start := time.Now().Add(-90 * time.Second)
	now := time.Now()
	if got := durationSeconds(Call{StartedAt: &start}, now); got != 90 {
		t.Fatalf("open-call duration = %d, want 90", got)
	}
	end := start.Add(30 * time.Second)
	if got := durationSeconds(Call{StartedAt: &start, EndedAt: &end}, now); got != 30 {
		t.Fatalf("closed-call duration = %d, want 30", got)
	}
	if got := durationSeconds(Call{}, now); got != 0 {
		t.Fatalf("unstarted duration = %d, want 0", got)
	}
}
