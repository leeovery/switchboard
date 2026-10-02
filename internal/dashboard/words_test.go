package dashboard

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

func TestWhenShowsTheDayOnlyBeyondADay(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{name: "later today", t: now.Add(2 * time.Hour), want: "15:12"},
		{name: "tomorrow, within a day", t: now.Add(14 * time.Hour), want: "03:12"},
		{name: "earlier, within a day", t: now.Add(-20 * time.Hour), want: "17:12"},
		{name: "a day on", t: now.Add(day), want: "Tue 13:12"},
		{name: "days ago", t: now.Add(-3 * day), want: "Fri 13:12"},
		{name: "in UTC, shown in now's time zone", t: now.Add(time.Hour).UTC(), want: "14:12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := when(now, tt.t); got != tt.want {
				t.Errorf("when() = %q, want %q", got, tt.want)
			}
		})
	}
}

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
	for label, want := range map[string]string{"Session": "session", "Week": "week", "Fable week": "Fable week", "": ""} {
		if got := prosed(label); got != want {
			t.Errorf("prosed(%q) = %q, want %q", label, got, want)
		}
	}
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
	if got, want := until(now, now.Add(71*time.Minute+53*time.Second)), "in 1h 11m"; got != want {
		t.Errorf("until() = %q, want %q", got, want)
	}
}
