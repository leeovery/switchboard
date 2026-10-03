package dashboard

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

func TestAccountsAreCalledByTheirNames(t *testing.T) {
	doc := status.Document{Accounts: []status.Account{{ID: "work", Label: "Work\x1b[2J"}, {ID: "side"}}}
	tests := []struct {
		id, want string
		place    int
	}{
		{id: "work", want: "Work [2J", place: 1},
		{id: "side", want: "side", place: 2},
		{id: "gone", want: "gone", place: 0},
	}
	for _, tt := range tests {
		if got := named(doc, tt.id); got != tt.want {
			t.Errorf("named(%q) = %q, want %q: its label, cleaned, else its id", tt.id, got, tt.want)
		}
		if got := place(doc, tt.id); got != tt.place {
			t.Errorf("place(%q) = %d, want %d", tt.id, got, tt.place)
		}
	}
	if got, want := names(doc, []string{"work", "side"}), "Work [2J and side"; got != want {
		t.Errorf("names() = %q, want %q", got, want)
	}
}

func TestWindowsInWords(t *testing.T) {
	for key, want := range map[string]string{"5h": "5-hour window", "7d": "7-day window", "7d_oi": "7-day window", "overage": "overage"} {
		if got := spanOf(key); got != want {
			t.Errorf("spanOf(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestSpansAndSessionsInWords(t *testing.T) {
	for since, want := range map[time.Duration]string{30 * time.Minute: "last-30-min", 18 * time.Minute: "last-18-min", 2 * time.Hour: "last-2h"} {
		if got := lately(now.Add(-since), now); got != want {
			t.Errorf("lately() over %v = %q, want %q", since, got, want)
		}
	}
	if got := sessionID("c61b2f8d-04a7-4e93"); got != "c61b" {
		t.Errorf("sessionID() = %q, want its first four characters", got)
	}
}
