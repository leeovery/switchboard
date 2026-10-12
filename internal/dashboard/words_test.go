package dashboard

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

func TestAccountsAreCalledByTheirNames(t *testing.T) {
	doc := status.Document{Accounts: []status.Account{{ID: "work", Label: "Work\x1b[2J"}, {ID: "side"}}}
	m := doc.At(now, claudeLike)
	tests := []struct {
		id, want string
		place    int
	}{
		{id: "work", want: "Work [2J", place: 1},
		{id: "side", want: "side", place: 2},
		{id: "gone", want: "gone", place: 0},
	}
	for _, tt := range tests {
		if got := named(m, tt.id); got != tt.want {
			t.Errorf("named(%q) = %q, want %q: its label, cleaned, else its id", tt.id, got, tt.want)
		}
		if got := place(m, tt.id); got != tt.place {
			t.Errorf("place(%q) = %d, want %d", tt.id, got, tt.place)
		}
		if got := docNamed(doc, tt.id); got != tt.want {
			t.Errorf("docNamed(%q) = %q, want %q, as named has it", tt.id, got, tt.want)
		}
		if got := docPlace(doc, tt.id); got != tt.place {
			t.Errorf("docPlace(%q) = %d, want %d, as place has it", tt.id, got, tt.place)
		}
	}
	if got, want := names(m, []string{"work", "side"}), "Work [2J and side"; got != want {
		t.Errorf("names() = %q, want %q", got, want)
	}
	if got, want := docNames(doc, []string{"work", "side"}), "Work [2J and side"; got != want {
		t.Errorf("docNames() = %q, want %q, as names has it", got, want)
	}
}

func TestEitherListsTheNamesAsProseDoesWhatsChosenBetween(t *testing.T) {
	tests := []struct {
		names []string
		want  string
	}{
		{names: nil, want: ""},
		{names: []string{"work"}, want: "work"},
		{names: []string{"work", "side"}, want: "work or side"},
		{names: []string{"work", "side", "personal"}, want: "work, side or personal"},
		{names: []string{"work", "side", "personal", "client"}, want: "work, side, personal or client"},
	}
	for _, tt := range tests {
		got := either(tt.names, titleInk, secondaryInk)
		if got.plain() != tt.want {
			t.Errorf("either(%q) = %q, want %q", tt.names, got.plain(), tt.want)
		}
		for _, s := range got {
			if named := slices.Contains(tt.names, s.text); named != (s.ink == titleInk) {
				t.Errorf("either(%q) drew %q in %+v, want the names in theirs and the words between in theirs", tt.names, s.text, s.ink)
			}
		}
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
