package dashboard

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// rowsDown is how many rows the units take laid out in turn: each its rows,
// or its heading's alone where it folds, and a blank row between two, but
// two folded.
func rowsDown(units []unit) int {
	n := 0
	for i, u := range units {
		n += u.taken()
		if i > 0 && (!units[i-1].fold || !u.fold) {
			n++
		}
	}
	return n
}

// nextStep is the fewest rows the unit grows by next, as growth grows it:
// USAGE and ROUTING to their next form whole, SESSIONS a whole session, any
// other a row.
func nextStep(u unit) int {
	if !u.ledBy(SectionUsage, SectionRouting, SectionSessions) {
		return 1
	}
	next := u.most()
	for _, s := range u.sections {
		for _, step := range s.between {
			if step > u.rows {
				next = min(next, step)
			}
		}
	}
	return next - u.rows
}

// COMING UP and LATELY sit side by side where each one's least fits beside
// the other, COMING UP's 42 cells in round(w × 0.45) from 93 columns, a name
// counted at its least of 8 whatever its width, so the switch falls at 93
// for a name of 8 cells, 14 or 22 alike; under it, LATELY is under COMING UP.
func TestCOMINGUPAndLATELYSitSideBySideWhereTheirLeastsFitWhateverTheNames(t *testing.T) {
	if got := comingLeast(); got != 42 {
		t.Fatalf("COMING UP's least is %d cells, want 42: ~Sat 06:00, two blanks, a name at 8, is primed and its countdown's 12", got)
	}
	if got := latelyLeast(); got != 21 {
		t.Fatalf("LATELY's least is %d cells, want 21: Sun 02:00, two blanks, its mark and a blank, and a name at 8", got)
	}
	for _, name := range []string{"personal", "work-team-plan", "side-hustle-consulting"} {
		named := readAccount("work", weekOf(0.33, 5*quota.Day))
		named.Label = name
		doc := routerDoc("work", 1, named)
		for width, beside := range map[int]bool{52: false, 73: false, 92: false, 93: true, 100: true, 160: true} {
			f := frameOf(width, 60)
			units := f.unitsOf(f.sectionsOf(doc, now))
			last := units[len(units)-1]
			if got := len(last.sections) == 2; got != beside {
				t.Errorf("named %q at %d columns, COMING UP and LATELY side by side %v, want %v", name, width, got, beside)
			}
		}
	}
}

// Every height from 10 to 120 at the phone's widths, the iPad's, the
// landscape phone's and the desktop's, with the sections shown at first and
// with every one, of three accounts by window and by account, and of ten: a
// unit folded never fits at its least with the others at theirs, and rows
// are left blank at the foot only where every unit not folded is at its
// most, or none's next step fits in them.
func TestNoUnitFoldsWhereItFitsAndNoRowIsLeftBlankWhereAStepFits(t *testing.T) {
	ten, _ := tenAccounts()
	worlds := []struct {
		name string
		doc  status.Document
		by   By
	}{
		{name: "three by window", doc: threeRouted()},
		{name: "three by account", doc: threeRouted(), by: ByAccount},
		{name: "ten by window", doc: ten},
	}
	for _, w := range worlds {
		for _, shown := range []Sections{{}, every} {
			for _, width := range []int{52, 73, 108, 138, 160, 220} {
				t.Run(fmt.Sprintf("%s, %v at %d", w.name, shown.Names(), width), func(t *testing.T) {
					t.Parallel()
					for height := 10; height <= 120; height++ {
						f := frameOf(width, height)
						f.Sections, f.Sessions, f.By = shown, flippingSessions(), w.by
						allottedFully(t, f, w.doc)
					}
				})
			}
		}
	}
}

// allottedFully holds the units of doc as the frame allots them to the
// rules: none folded that fits at its least, the others at theirs; and no
// row blank at the foot that a unit not folded, short of its most, could
// take its next step in.
func allottedFully(t *testing.T, f Frame, doc status.Document) {
	t.Helper()
	units, room := f.allotted(doc, now), f.viewRows()
	for i, u := range units {
		if !u.fold {
			continue
		}
		atLeast := slices.Clone(units)
		for j := range atLeast {
			atLeast[j].rows = atLeast[j].least
		}
		atLeast[i].fold = false
		if rowsDown(atLeast) <= room {
			t.Errorf("at %dx%d, %s is folded, but fits at its least, %d rows, in %d", f.Width, f.Height, u.sections[0].heading(), u.least, room)
		}
	}
	spare := room - rowsDown(units)
	for _, u := range units {
		if !u.fold && u.rows < u.most() && nextStep(u) <= spare {
			t.Errorf("at %dx%d, %s takes %d rows of its most %d, with %d blank at the foot that its next step of %d fits in",
				f.Width, f.Height, u.sections[0].heading(), u.rows, u.most(), spare, nextStep(u))
		}
	}
}

func TestSESSIONSRowsAreASessionAndHowManyMoreToEveryOneASessionAtATime(t *testing.T) {
	tests := []struct {
		name         string
		running, per int
		least, most  int
		between      []int
	}{
		{name: "none running: the row saying so", running: 0, per: 1, least: 3, most: 3},
		{name: "one: it alone", running: 1, per: 1, least: 3, most: 3},
		{name: "one, two rows a session", running: 1, per: 2, least: 4, most: 4},
		{name: "two: one and how many more is no fewer than both", running: 2, per: 1, least: 4, most: 4},
		{name: "six, a row a session", running: 6, per: 1, least: 4, most: 8, between: []int{5, 6, 7}},
		{name: "six, two rows a session", running: 6, per: 2, least: 5, most: 14, between: []int{7, 9, 11, 13}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			least, most, between := sessionsRows(tt.running, tt.per)
			if least != tt.least || most != tt.most || !slices.Equal(between, tt.between) {
				t.Errorf("sessionsRows = %d, %d, %v; want %d, %d, %v", least, most, between, tt.least, tt.most, tt.between)
			}
		})
	}
}

// SESSIONS' least is a session and the row saying how many more, or every
// one where that's no fewer: it folds only where even that doesn't fit, beside
// USAGE's 13 rows and the blank row between.
func TestSESSIONSFoldsOnlyWhereASessionAndHowManyMoreDontFit(t *testing.T) {
	tests := []struct {
		name         string
		running, per int
		rows, given  int
		fold         bool
	}{
		{name: "a row a session: one, and how many more", running: 6, per: 1, rows: 18, given: 4},
		{name: "a row a session: a row short", running: 6, per: 1, rows: 17, fold: true},
		{name: "two rows a session: one, and how many more", running: 6, per: 2, rows: 19, given: 5},
		{name: "two rows a session: a row short", running: 6, per: 2, rows: 18, fold: true},
		{name: "one running: it alone", running: 1, per: 1, rows: 17, given: 3},
		{name: "one running: a row short", running: 1, per: 1, rows: 16, fold: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			laid := planOf(frameOf(160, 50), []section{sized(SectionUsage, 13, 13), sessionsSized(tt.running, tt.per)}, 0, tt.rows)
			if got := rowsGiven(laid)[SectionSessions]; laid[1].fold != tt.fold || !tt.fold && got != tt.given {
				t.Errorf("in %d rows, SESSIONS folds %v, taking %d; want it folded %v, taking %d", tt.rows, laid[1].fold, got, tt.fold, tt.given)
			}
		})
	}
}

// The rows to spare go to SESSIONS first, a whole session at a time, two rows
// a session here, to every one, its row saying how many more going once every
// one shows; a row left that's less than a session goes on to GRAPHS, then
// COMING UP and LATELY, never left blank.
func TestSESSIONSGrowsFirstAWholeSessionAtATimeWhatsLeftGoingOn(t *testing.T) {
	sections := []section{
		sized(SectionUsage, 13, 35), sized(SectionGraphs, 9, 23), sessionsSized(6, 2),
		sized(SectionComing, 5, 9), sized(SectionLately, 5, 14),
	}
	tests := []struct {
		name                     string
		rows                     int
		sessions, graphs, coming int
	}{
		{name: "none to spare: a session and how many more", rows: 35, sessions: 5, graphs: 9, coming: 5},
		{name: "a row, less than a session: to GRAPHS", rows: 36, sessions: 5, graphs: 10, coming: 5},
		{name: "a session's two rows: to SESSIONS, before GRAPHS", rows: 37, sessions: 7, graphs: 9, coming: 5},
		{name: "a session's two rows and one: the one to GRAPHS", rows: 38, sessions: 7, graphs: 10, coming: 5},
		{name: "five sessions and how many more", rows: 43, sessions: 13, graphs: 9, coming: 5},
		{name: "every session, the last's second row where how many more was", rows: 44, sessions: 14, graphs: 9, coming: 5},
		{name: "every session, and a row to GRAPHS", rows: 45, sessions: 14, graphs: 10, coming: 5},
		{name: "GRAPHS at its most too, a row on to COMING UP and LATELY", rows: 59, sessions: 14, graphs: 23, coming: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			laid := planOf(frameOf(160, 120), sections, 0, tt.rows)
			want := map[Section]int{SectionUsage: 13, SectionGraphs: tt.graphs, SectionSessions: tt.sessions, SectionComing: tt.coming, SectionLately: tt.coming}
			if got := rowsGiven(laid); !maps.Equal(got, want) {
				t.Errorf("in %d rows, the sections take %v, want %v", tt.rows, got, want)
			}
			if got := laid[len(laid)-1].lastRow() + 1; got != tt.rows {
				t.Errorf("in %d rows, the last section ends on row %d, want no row left blank", tt.rows, got)
			}
		})
	}
}
