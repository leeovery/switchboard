package capture

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/status"
)

// boardWidths are the widths the Switchboard's accounts and sessions are
// counted at: the landscape phone's lanes, and the lines at the iPad's, the
// MacBook side screen's and the desktop's.
var boardWidths = []int{73, 108, 138, 160}

// Every Switchboard fixture, at each of boardWidths, from 10 rows to 40: the
// accounts and the sessions the Switchboard shows never fall as the
// terminal grows a row taller, whether it scrolls an account at a time or a
// row at a time.
func TestWhatTheSwitchboardShowsNeverFallsAsTheTerminalGrows(t *testing.T) {
	for _, f := range sweptFixtures(t) {
		t.Run(f.Name, func(t *testing.T) {
			t.Parallel()
			m := withNothingOver(t, f.settle(f.Size), f.Size)
			if viewOf(t, m, f.Size) != "Sessions Switchboard" {
				t.Skip("not the Switchboard")
			}
			names, ids := accountNames(f), shownIDs(f)
			for _, width := range boardWidths {
				was := boardSaid{}
				for height := 10; height <= 40; height++ {
					size := watch.Size{Width: width, Height: height}
					is := boardSaidIn(fluidAt(m, size).body(), names, ids)
					if is.accounts < was.accounts || is.sessions < was.sessions {
						t.Errorf("at %s, the Switchboard shows %d accounts and %d sessions, fewer than the %d and %d a row shorter",
							sizeName(size), is.accounts, is.sessions, was.accounts, was.sessions)
					}
					was = is
				}
			}
		})
	}
}

// boardSaid is how many accounts and sessions the Switchboard shows.
type boardSaid struct {
	accounts, sessions int
}

// shownIDs are the fixture's sessions' ids as the Switchboard shows them.
func shownIDs(f Fixture) []string {
	ids := make([]string, len(f.router.sessions))
	for i, s := range f.router.sessions {
		ids[i] = status.ShownID(s.ID)
	}
	return ids
}

// accountNumbered is an account's number on a line or a lane, then what
// follows it, its name first.
var accountNumbered = regexp.MustCompile(`(?:^|\s)[0-9]+ `)

// boardSaidIn is how many of the accounts named and the sessions by the ids
// given the rows show: an account where its number leads its name, a
// session where its id is a word of a row.
func boardSaidIn(rows, names, ids []string) boardSaid {
	var said boardSaid
	for _, name := range names {
		if slices.ContainsFunc(rows, func(row string) bool { return numbersName(row, name) }) {
			said.accounts++
		}
	}
	for _, id := range ids {
		if slices.ContainsFunc(rows, func(row string) bool { return slices.Contains(strings.Fields(row), id) }) {
			said.sessions++
		}
	}
	return said
}

// numbersName reports whether the row leads a name with an account's number,
// and that name is the one given, whole or cut.
func numbersName(row, name string) bool {
	for _, at := range accountNumbered.FindAllStringIndex(row, -1) {
		if leadsWith(row[at[1]:], name) {
			return true
		}
	}
	return false
}
