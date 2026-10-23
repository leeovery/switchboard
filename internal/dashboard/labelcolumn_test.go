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
// and an ellipsis, as the column's 8 cells hold it; but Events' of two
// accounts, whose column holds a move's two names, each of 8 at the least,
// reads it whole.
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
				want := "consult…"
				if tt.name == "events" && len(doc.Accounts) == 2 {
					want = gone
				}
				if !strings.Contains(row, " "+want+" ") {
					t.Errorf("of %d accounts, the row reads %q, want it naming %q", len(doc.Accounts), row, want)
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

// A session's page names an account no longer configured, as a recorded
// session ran on, by its id: with every configured name of 2 or 3 cells,
// its stay row names old-account whole, as the stays keep a label's 8 cells
// beside the configured names' marks; and so does a turn's row, its on 8
// cells and the move's arrow and second name's 6 after them, where a column
// of the configured names' 3 would cut it.
func TestASessionsPageNamesAnAccountNoLongerConfigured(t *testing.T) {
	doc := status.Document{
		GeneratedAt: sessionNow.UTC(), Source: status.SourceRouter, Best: "ops", Router: status.Health{Healthy: true}, Sessions: 6,
		Accounts: []status.Account{pageAccount("ab", 0.95, 0.34, 0.12), pageAccount("ops", 0.13, 0.78, 0.04)},
	}.WorkedOut(claudeLike)
	shown := moved()
	gone := "old-account"
	shown.Page.Accounts[0].Account = gone
	shown.Page.Turns[0].Accounts = []string{gone}
	d, _ := pageDrawn(pageFrame(t, shown, 160, 60), doc)
	rows := drawnRows(d)
	stay, turns := rows[d.l.accountsAt], strings.Join(rows[d.l.heads:], "\n")
	if !strings.HasPrefix(stay, gone+" ") {
		t.Errorf("the stay row reads %q, want it to start %q whole", stay, gone)
	}
	if !strings.Contains(turns, " "+gone+" ") {
		t.Errorf("the turns read\n%s\nwant a turn naming %q whole", turns, gone)
	}
}

// The stays' name column keeps each name's number and marks beside its
// label: at the least, a label's 8 cells, as a stay may name an account no
// longer configured; at the natural, the widest label, 8 at the least and
// 24 at the most.
func TestTheStaysNameColumnKeepsALabelsLeast(t *testing.T) {
	tests := []struct {
		name  string
		names []accountName
		want  partCells
	}{
		{name: "short names", names: []accountName{{number: "1", label: "ab"}, {number: "2", label: "ops"}}, want: partCells{least: 12, natural: 12}},
		{name: "the primary's ◆", names: []accountName{{number: "1", label: "ab", primary: true}}, want: partCells{least: 14, natural: 14}},
		{name: "a label of 22", names: []accountName{{number: "1", label: "side-hustle-consulting"}}, want: partCells{least: 12, natural: 26}},
		{name: "a label past 24", names: []accountName{{number: "1", label: strings.Repeat("x", 30)}}, want: partCells{least: 12, natural: 28}},
	}
	for _, tt := range tests {
		if got := stayNameCells(tt.names); got != tt.want {
			t.Errorf("%s: the stays' names take %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// Events' account cuts a lone name at a label's 24 cells, as every column
// of names does, though the column is as wide as a move's two names.
func TestEventsCutALoneNameAtALabelsMost(t *testing.T) {
	label := "an-account-labelled-at-length"
	long := status.Account{ID: "long", Label: label, TokenSet: true}
	doc := routerDoc("ab", 0, status.Account{ID: "ab", TokenSet: true}, long)
	page := eventsShown(status.Event{ID: 1, At: logDayNoon.Add(-time.Hour), Kind: status.EventPin, Accounts: []string{"long"}})
	f := logFrameOf(page, 160, 24)
	row := textOf(f.Draw(doc, logDayNoon))[f.top()+1]
	if cut := prose.Cut(label, 24); !strings.Contains(row, " "+cut+" ") || strings.Contains(row, label[:24]) {
		t.Errorf("the row reads %q, want it naming %q", row, cut)
	}
}

// A move names two accounts, each counting a label's 8 cells at the least
// in the Events' column, as either may be no longer configured: with every
// configured name of 2 or 3 cells, a move from ab to old-account names
// old-account's first 7 cells at the least.
func TestEventsMoveCountsEachNameAtALabelsLeast(t *testing.T) {
	doc := routerDoc("ab", 0, status.Account{ID: "ab", TokenSet: true}, status.Account{ID: "ops", TokenSet: true})
	moved := status.Event{
		ID: 1, At: logDayNoon.Add(-time.Hour), Kind: status.EventMoved, Session: "d28c0b1e", From: "ab", To: "old-account",
		Reason: "moved: ab is at its reserve", ForcedBy: 1,
	}
	f := logFrameOf(eventsShown(moved), 160, 24)
	row := textOf(f.Draw(doc, logDayNoon))[f.top()+1]
	if !strings.Contains(row, "ab → old-acc") {
		t.Errorf("the row reads %q, want it naming the move ab → old-acc… at the least", row)
	}
}

// The Log panel's sessions today may be on an account no longer configured,
// so their account column counts a label's 8 cells at the least: with every
// configured name of 2 or 3 cells, old-account reads its first 7 and an
// ellipsis.
func TestTheLogPanelsSessionsNameAnAccountNoLongerConfigured(t *testing.T) {
	p := framesPanel(LogRequests, 0)
	p.Accounts = []status.AccountNow{{ID: "ab"}, {ID: "ops"}}
	p.Sessions[5].Account = "old-account"
	at := p.sessionColumns(78)
	if got, want := p.sessionWords(p.Sessions[5], at.account).plain(), "old-acc… "; !strings.Contains(got, " "+want) {
		t.Errorf("the row reads %q, want it naming %q", got, want)
	}
}
