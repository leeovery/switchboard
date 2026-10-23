package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// A column of names takes the widest name as its natural, each counted at
// 24 cells at the most; 8 at the least, or its natural where that's fewer;
// 24 at the most. A name's number and marks, and the gap after the column,
// are counted beside its label, never among it: with them, a name of 2 cells
// gives the column its own 2 and the marks' and gap's 8 at every bound.
func TestAColumnOfNamesTakesItsBoundsFromItsNames(t *testing.T) {
	tests := []struct {
		label                 int
		least, natural, most  int
		marked, markedNatural int
		markedMost            int
	}{
		{label: 2, least: 2, natural: 2, most: 24, marked: 10, markedNatural: 10, markedMost: 32},
		{label: 8, least: 8, natural: 8, most: 24, marked: 16, markedNatural: 16, markedMost: 32},
		{label: 11, least: 8, natural: 11, most: 24, marked: 16, markedNatural: 19, markedMost: 32},
		{label: 24, least: 8, natural: 24, most: 24, marked: 16, markedNatural: 32, markedMost: 32},
		{label: 30, least: 8, natural: 24, most: 24, marked: 16, markedNatural: 32, markedMost: 32},
	}
	for _, tt := range tests {
		label := strings.Repeat("x", tt.label)
		plain := flex.Part{Least: tt.least, Natural: tt.natural, Most: tt.most}
		if got := namesColumn(0, columnName{label: tt.label}); got != plain {
			t.Errorf("a name of %d cells, unmarked, takes %+v, want %+v", tt.label, got, plain)
		}
		if got := labelColumn(labelColumnCells(label)); got != plain {
			t.Errorf("labelColumn of a name of %d cells is %+v, want %+v", tt.label, got, plain)
		}
		if got := labelsLeast(labelColumnCells(label)); got != tt.least {
			t.Errorf("labelsLeast of a name of %d cells is %d, want %d", tt.label, got, tt.least)
		}
		// 1 ▲ ◆ and a space, 6 cells, then the gap's 2.
		marked := []accountName{{number: "1", label: label, primary: true}}
		want := flex.Part{Least: tt.marked, Natural: tt.markedNatural, Most: tt.markedMost}
		if got := accountsColumn(nameGap, marked); got != want {
			t.Errorf("a name of %d cells, numbered and marked, takes %+v, want %+v", tt.label, got, want)
		}
	}
}

// Each name keeps its own marks: the column's least is the widest of each
// name cut to its least with its marks beside it, so a short primary's ◆
// counts beside its own cells, never beside a label's least.
func TestEachNameKeepsItsOwnMarksBesideIt(t *testing.T) {
	names := []accountName{{number: "1", label: "ab", primary: true}, {number: "2", label: "personal"}}
	// "1 ab ▲ ◆" is 8 cells, "2 personal ▲" 12; ab's marks at a long name's 24, 30.
	want := flex.Part{Least: 12, Natural: 12, Most: 30}
	if got := accountsColumn(0, names); got != want {
		t.Errorf("the column of ab ◆ and personal takes %+v, want %+v", got, want)
	}
}

// A column of names a record may hold takes the widest configured as its
// natural, but never under 8, as a recorded line may name an account no
// longer configured; 8 at the least.
func TestAColumnARecordMayHoldNeverNarrowsPastALabelsLeast(t *testing.T) {
	for _, tt := range []struct{ natural, want int }{{0, 8}, {2, 8}, {8, 8}, {15, 15}, {49, 49}} {
		if got, want := recordedNames(tt.natural), (flex.Part{Least: 8, Natural: tt.want}); got != want {
			t.Errorf("names of %d cells a record may hold take %+v, want %+v", tt.natural, got, want)
		}
	}
}

// With every configured name of 2 or 3 cells, a Requests row and an Events
// row naming an account no longer configured, of 11 cells, read its first 7
// and an ellipsis, as the column's 8 cells hold it. Each is drawn of one
// account configured and of two, as a move between two makes Events'
// natural 8 already.
func TestARecordedNameNoLongerConfiguredReadsItsFirstCells(t *testing.T) {
	ab, ops := status.Account{ID: "ab", TokenSet: true}, status.Account{ID: "ops", TokenSet: true}
	gone := "consultancy"
	line := asked("1", logDayNoon.Add(-time.Hour))
	line.Account = gone
	src := views.LogSources{From: theDayBefore, Now: logDayNoon}
	tests := []struct {
		name string
		page LogPage
	}{
		{name: "requests", page: requestsOf(src, []ledger.Line{line})},
		{name: "events", page: eventsShown(status.Event{ID: 1, At: logDayNoon.Add(-time.Hour), Kind: status.EventPin, Accounts: []string{gone}})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, doc := range []status.Document{routerDoc("ops", 0, ops), routerDoc("ab", 0, ab, ops)} {
				f := logFrameOf(tt.page, 160, 24)
				row := textOf(f.Draw(doc, logDayNoon))[f.top()+1]
				if cut := prose.Cut(gone, 8); !strings.Contains(row, " "+cut+" ") || cut != "consult…" {
					t.Errorf("of %d accounts, the row reads %q, want it naming %q", len(doc.Accounts), row, cut)
				}
			}
		})
	}
}

// A sentence's names are cut to the most cells that fit, from 24 down, and
// to 8 where none fits.
func TestASentencesNamesAreCutToTheMostThatFits(t *testing.T) {
	for _, room := range []int{30, 24, 15, 8, 3} {
		want := min(max(room, 8), 24)
		if got := labelCut(func(cut int) bool { return cut <= room }); got != want {
			t.Errorf("with room for %d, the names are cut to %d, want %d", room, got, want)
		}
	}
}
