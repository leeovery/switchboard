package status_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

func TestAnAssignmentGivesItsDirectoryAndWhatItsRequestInFlightIsDoing(t *testing.T) {
	at := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		name string
		a    status.Assignment
		want string
	}{
		{
			name: "both given",
			a:    status.Assignment{Model: "claude-opus-5-5", Account: "work", Dir: "~/Code/api", InFlight: status.Answering, Reason: "new", AssignedAt: at, LastSeen: at},
			want: `{"model":"claude-opus-5-5","account":"work","dir":"~/Code/api","in_flight":"answering","pinned":false,"reason":"new","assigned_at":"2026-09-28T13:12:00Z","last_seen":"2026-09-28T13:12:00Z"}`,
		},
		{
			name: "neither, left out",
			a:    status.Assignment{Model: "claude-opus-5-5", Account: "work", Reason: "new", AssignedAt: at, LastSeen: at},
			want: `{"model":"claude-opus-5-5","account":"work","pinned":false,"reason":"new","assigned_at":"2026-09-28T13:12:00Z","last_seen":"2026-09-28T13:12:00Z"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.a)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tt.want {
				t.Errorf("the assignment is given as\n%s\nwant\n%s", data, tt.want)
			}
			var back status.Assignment
			if err := json.Unmarshal(data, &back); err != nil || !reflect.DeepEqual(back, tt.a) {
				t.Errorf("it reads back as %+v (%v), want %+v", back, err, tt.a)
			}
		})
	}
}

func TestASessionFromARouterFromBeforeDirectoriesReadsWithNone(t *testing.T) {
	before := `{"session":"one","assignments":[{"model":"claude-opus-5-5","family":"opus","account":"work","pinned":false,"reason":"new","assigned_at":"2026-09-28T13:12:00Z","last_seen":"2026-09-28T13:12:00Z"}]}`
	var s status.Session
	if err := json.Unmarshal([]byte(before), &s); err != nil {
		t.Fatalf("a session from a router from before doesn't read: %v", err)
	}
	if len(s.Assignments) != 1 || s.Assignments[0].Account != "work" || s.Assignments[0].Dir != "" || s.Assignments[0].InFlight != "" {
		t.Errorf("it reads as %+v, want its assignment to work, naming no directory and nothing in flight", s)
	}
}
