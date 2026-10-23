package capture

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/status"
)

// boardWidths are the widths what the Switchboard shows is counted at: in
// lanes, a phone's 52 and 73 upright; and in lines, from their least, 100,
// the iPad's 108, the phone's 138 on its side, the desktop's 160 and a wide
// screen's 230.
var boardWidths = []int{52, 73, 100, 108, 138, 160, 230}

// boardHeights are the heights what the Switchboard shows is counted at:
// each from 8 rows to 30, where it scrolls, then every other one to 60.
func boardHeights() []int {
	var heights []int
	for h := 8; h <= 60; h++ {
		if h <= 30 || h%2 == 0 {
			heights = append(heights, h)
		}
	}
	return heights
}

// boardWorlds are the Switchboard's fixtures of a world each, or of a
// scroll: the scene, its long names, ten accounts, calls moved, accounts
// pinned, one account, and the scene scrolled to its foot. The rest are the
// scene's again, at another size, chosen or with the router gone.
var boardWorlds = []string{
	"sessions-switchboard-160", "sessions-switchboard-long-names-108x44", "sessions-ten-accounts-160",
	"sessions-moved-160", "sessions-pinned-160", "sessions-one-account-160", "page-sessions-short",
}

// Each of boardWorlds, at each of boardWidths and boardHeights: the
// accounts, the sessions and the rows the Switchboard shows never fall as
// the terminal grows taller, whether it scrolls an account at a time or a
// row at a time.
func TestWhatTheSwitchboardShowsNeverFallsAsTheTerminalGrows(t *testing.T) {
	for _, name := range boardWorlds {
		f := fixture(t, name)
		t.Run(f.Name, func(t *testing.T) {
			t.Parallel()
			m := withNothingOver(t, f.settle(f.Size), f.Size)
			if view := viewOf(t, m, f.Size); view != "Sessions Switchboard" {
				t.Fatalf("draws %s, want the Switchboard", view)
			}
			names, ids := accountNames(f), shownIDs(f)
			for _, width := range boardWidths {
				t.Run(strconv.Itoa(width), func(t *testing.T) {
					t.Parallel()
					was := boardSaid{}
					for _, height := range boardHeights() {
						size := watch.Size{Width: width, Height: height}
						is := boardSaidIn(fluidAt(m, size).body(), names, ids)
						if is.fewer(was) {
							t.Errorf("at %s, the Switchboard shows %+v, fewer than the %+v shorter", sizeName(size), is, was)
						}
						was = is
					}
				})
			}
		})
	}
}

// boardSaid is how many accounts and sessions the Switchboard shows, and
// how many rows of it.
type boardSaid struct {
	accounts, sessions, rows int
}

// fewer reports whether s shows fewer accounts, sessions or rows than was.
func (s boardSaid) fewer(was boardSaid) bool {
	return s.accounts < was.accounts || s.sessions < was.sessions || s.rows < was.rows
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
// given the rows show, an account where its number leads its name, a
// session where its id is a word of a row; and how many of the rows aren't
// blank, but for those saying what's out of view.
func boardSaidIn(rows, names, ids []string) boardSaid {
	var said boardSaid
	for _, row := range rows {
		if strings.TrimSpace(row) != "" && !outLine.MatchString(row) {
			said.rows++
		}
	}
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
