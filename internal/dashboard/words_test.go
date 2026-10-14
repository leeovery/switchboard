package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
)

// A model is named by its id less claude-, cleaned first, so a recorded id
// with stray spaces or control characters is named as any other.
func TestAModelIsNamedByItsIDLessClaudeCleaned(t *testing.T) {
	for id, want := range map[string]string{
		"claude-opus-5-5":        "opus-5-5",
		"  claude-opus-5-5\t":    "opus-5-5",
		"claude-opus-5-5\x1b[2J": "opus-5-5 [2J",
		"gpt-x":                  "gpt-x",
	} {
		if got := modelNamed(id); got != want {
			t.Errorf("modelNamed(%q) = %q, want %q", id, got, want)
		}
	}
}

// Each page that names a model by its id names it as modelNamed does: Tokens'
// version rows, and a session's page, where it goes and, ended, where it
// went.
func TestEveryPageNamesAModelAsModelNamedDoes(t *testing.T) {
	const odd = "  claude-opus-5-5\t"
	tokens := tokensFrame(160, 37, TokensByModel)
	tokens.History.Tokens.Periods[0].ByModel[0].Model = odd
	if rows := tokensRowsOf(tokens); !slices.ContainsFunc(rows, func(row string) bool { return strings.HasPrefix(row, "└ opus-5-5 ") }) {
		t.Errorf("Tokens by model reads\n%s\nwant a row of └ opus-5-5", strings.Join(rows, "\n"))
	}
	for name, shown := range map[string]*SessionShown{"going": moved(), "ended": ended()} {
		shown.Page.Goes[0].Model = odd
		text := strings.Join(pageText(pageFrame(t, shown, 160, 37)), "\n")
		if !strings.Contains(text, "opus-5-5") || strings.Contains(text, "claude-opus") {
			t.Errorf("a session's page, %s, reads\n%s\nwant opus-5-5, and no claude-opus", name, text)
		}
	}
}

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
	}
	if got, want := names(m, []string{"work", "side"}), "Work [2J and side"; got != want {
		t.Errorf("names() = %q, want %q", got, want)
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

// A move's names fit their cells, the arrow kept: each whole where they fit;
// else the longer cut first, to the other's width, the one left first where
// they're alike, then the two alike, the odd cell the one moved to's; and
// with less room than an ellipsis each, the arrow and the one moved to.
func TestAMovesNamesAreCutTheLongerFirstKeepingTheArrow(t *testing.T) {
	tests := []struct {
		from, to string
		cells    int
		want     string
	}{
		{from: "personal", to: "side", cells: 16, want: "personal → side"},
		{from: "personal", to: "side", cells: 15, want: "personal → side"},
		{from: "personal", to: "side", cells: 14, want: "person… → side"},
		{from: "personal", to: "side", cells: 12, want: "pers… → side"},
		{from: "personal", to: "side", cells: 11, want: "per… → side"},
		{from: "personal", to: "side", cells: 10, want: "pe… → side"},
		{from: "personal", to: "side", cells: 9, want: "pe… → si…"},
		{from: "personal", to: "side", cells: 8, want: "p… → si…"},
		{from: "personal", to: "side", cells: 6, want: "… → s…"},
		{from: "personal", to: "side", cells: 5, want: "… → …"},
		{from: "personal", to: "side", cells: 4, want: "→si…"},
		{from: "personal", to: "side", cells: 1, want: "→"},
		{from: "side", to: "personal", cells: 12, want: "side → pers…"},
		{from: "side", to: "personal", cells: 10, want: "si… → per…"},
		{from: "work", to: "side", cells: 11, want: "work → side"},
		{from: "work", to: "side", cells: 10, want: "wo… → side"},
		{from: "work", to: "side", cells: 9, want: "wo… → si…"},
	}
	for _, tt := range tests {
		t.Run(tt.from+" to "+tt.to+" in "+strconv.Itoa(tt.cells), func(t *testing.T) {
			got := moveNames(tt.from, tt.to, tt.cells, secondaryInk)
			if got.plain() != tt.want || got.width() > tt.cells {
				t.Errorf("moveNames(%q, %q, %d) = %q, want %q", tt.from, tt.to, tt.cells, got.plain(), tt.want)
			}
		})
	}
	want := line{{"pers…", secondaryInk}, {" → ", mutedInk}, {"side", secondaryInk}}
	if got := moveNames("personal", "side", 12, secondaryInk); !slices.Equal(got, want) {
		t.Errorf("moveNames() is inked %+v, want the names in the ink given and the arrow in text.muted", got)
	}
}
