package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// tokenNoteOf is what Accounts tells of work's token's year, good until
// until, at now, its document built a day before now, in UTC.
func tokenNoteOf(t *testing.T, until, now time.Time) (views.TokenNote, bool) {
	t.Helper()
	doc := status.Document{
		GeneratedAt: now.Add(-24 * time.Hour).UTC(),
		Accounts:    []status.Account{{ID: "work", Label: "Work", TokenSet: true, TokenGoodUntil: until}},
	}
	work, ok := doc.At(now, claude.Policy()).Account("work")
	if !ok {
		t.Fatal("the moment has no account work")
	}
	return views.TokenNoteOf(work)
}

// Accounts tells of a token's year ending within 30 days, or up: Detail in
// full, the notification's words, Compare short, each with its warning, the
// nearest warning's days it ends within, 30 or 7. The days are counted at
// the moment's now, in its time zone, never the document's time or UTC. A
// year further off, or not known, is told of nowhere.
func TestAccountsTellsOfATokensYearEndingWithin30DaysOrUp(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 30, 0, 0, zone)
	until := func(days int) time.Time { return now.AddDate(0, 0, days).Add(time.Hour).UTC() }
	tests := []struct {
		name         string
		until        time.Time
		told, short  string
		warning      int
		noteExpected bool
	}{
		{
			name: "at 21 days", until: until(21), warning: status.TokenWarning, noteExpected: true,
			told:  "token expires 17 Nov 2026 · 21 days left — switchboard setup makes a new one",
			short: "token: 21 days left",
		},
		{
			name: "at 7 days", until: until(7), warning: status.TokenLastWarning, noteExpected: true,
			told:  "token expires 3 Nov 2026 · 7 days left — switchboard setup makes a new one",
			short: "token: 7 days left",
		},
		{
			name: "on its last day", until: until(0), warning: status.TokenLastWarning, noteExpected: true,
			told:  "token expires 27 Oct 2026 · last day — switchboard setup makes a new one",
			short: "token: last day",
		},
		{
			name: "expired", until: now.Add(-time.Hour).UTC(), warning: status.TokenLastWarning, noteExpected: true,
			told:  "token expired 26 Oct 2026 — switchboard setup makes a new one",
			short: "token expired",
		},
		{name: "at 31 days", until: until(31)},
		{name: "its year unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note, ok := tokenNoteOf(t, tt.until, now)
			if ok != tt.noteExpected {
				t.Fatalf("TokenNoteOf() reports %v, want %v", ok, tt.noteExpected)
			}
			if !ok {
				return
			}
			if got := prose.TokenSaid(note.Told); got != tt.told {
				t.Errorf("Told = %q, want %q", got, tt.told)
			}
			if got := prose.TokenSaid(note.Short); got != tt.short {
				t.Errorf("Short = %q, want %q", got, tt.short)
			}
			if note.Warning != tt.warning {
				t.Errorf("Warning = %d, want %d", note.Warning, tt.warning)
			}
		})
	}
}

// What warns, the days left or that the year's up, and the command that
// makes a new token, are parts of their own, for a drawing to colour.
func TestATokenNotesWarningAndCommandArePartsOfTheirOwn(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, zone)
	for _, tt := range []struct {
		name    string
		until   time.Time
		warning string
	}{
		{name: "days left", until: now.AddDate(0, 0, 21), warning: "21 days left"},
		{name: "up", until: now.Add(-time.Hour), warning: "expired"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			note, _ := tokenNoteOf(t, tt.until.UTC(), now)
			for _, parts := range [][]prose.TokenPart{note.Told, note.Short} {
				if !slices.Contains(parts, prose.TokenPart{Text: tt.warning, Kind: prose.TokenWarning}) {
					t.Errorf("%q has no warning part %q", prose.TokenSaid(parts), tt.warning)
				}
			}
			if !slices.Contains(note.Told, prose.TokenPart{Text: "switchboard setup", Kind: prose.TokenCommand}) {
				t.Errorf("%q has no command part", prose.TokenSaid(note.Told))
			}
		})
	}
}
