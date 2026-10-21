package dashboard

import (
	"fmt"
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
// USAGE and ROUTING to their next form whole, any other a row.
func nextStep(u unit) int {
	if !u.ledBy(SectionUsage, SectionRouting) {
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
