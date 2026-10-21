package dashboard

import (
	"fmt"
	"slices"
	"testing"
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

// Every height from 10 to 120 at the phone's widths, the iPad's, the
// landscape phone's and the desktop's, with the sections shown at first and
// with every one: a unit folded never fits at its least with the others at
// theirs, and rows are left blank at the foot only where every unit not
// folded is at its most, or none's next step fits in them.
func TestNoUnitFoldsWhereItFitsAndNoRowIsLeftBlankWhereAStepFits(t *testing.T) {
	doc := threeRouted()
	for _, shown := range []Sections{{}, every} {
		for _, width := range []int{52, 73, 108, 138, 160, 220} {
			t.Run(fmt.Sprintf("%v at %d", shown.Names(), width), func(t *testing.T) {
				t.Parallel()
				for height := 10; height <= 120; height++ {
					f := frameOf(width, height)
					f.Sections, f.Sessions = shown, flippingSessions()
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
							t.Errorf("at %dx%d, %s is folded, but fits at its least, %d rows, in %d", width, height, u.sections[0].heading(), u.least, room)
						}
					}
					spare := room - rowsDown(units)
					for _, u := range units {
						if !u.fold && u.rows < u.most() && nextStep(u) <= spare {
							t.Errorf("at %dx%d, %s takes %d rows of its most %d, with %d blank at the foot that its next step of %d fits in",
								width, height, u.sections[0].heading(), u.rows, u.most(), spare, nextStep(u))
						}
					}
				}
			})
		}
	}
}
