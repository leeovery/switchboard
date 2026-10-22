package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/prose"
)

// widenedNames are the names a stacked card's sweeps are of: the sample's,
// and a first name of 11, 24 and 30 cells.
var widenedNames = map[string]string{
	"the sample's names": "work", "a name of 11": "engineering",
	"a name of 24": "a-very-long-account-name", "a name of 30": "a-very-long-account-name-extra",
}

// accountsCard is the routing card given, of n accounts: the first named as
// given, then personal and side, then team-4 on; each as the card's row a
// third of the way along, its end as that row's.
func accountsCard(r RoutingCard, first string, n int) RoutingCard {
	rows := []RoutingRow{r.Rows[0]}
	for i := range n {
		row := r.Rows[1+i%3]
		row.Account = fmt.Sprintf("account-%d", i+1)
		row.Name = []string{first, "personal", "side"}[min(i, 2)]
		if i >= 3 {
			row.Name = fmt.Sprintf("team-%d", i+1)
		}
		rows = append(rows, row)
	}
	r.Rows, r.Picked = rows, nil
	return r
}

// shownRows are how many characters of each account's row's name and end
// the card shows on the frame, as its choice lays the row out.
func shownRows(r RoutingCard, f Frame) (names, ends []int) {
	content := r.card(f).width - 2*cardInset
	at := r.columns(content)
	for _, row := range r.Rows[1:] {
		said := r.choice(row, at, content)[0].plain()
		names = append(names, shownOf(said, row.Name))
		ends = append(ends, shownOf(said, row.sideWords().plain()))
	}
	return names, ends
}

// Stacked, whitespace gives way before words: as the frame widens, no
// account's name shows fewer cells, and no row's end says less, whatever
// the names and however many accounts there are.
func TestAsTheStackedCardWidensNoNameNorEndSaysLess(t *testing.T) {
	for name, first := range widenedNames {
		for n := 1; n <= 10; n++ {
			for mode, base := range map[string]RoutingCard{"new sessions'": widestEnds(), "a session's": sessionCard(nil)} {
				r := accountsCard(base, first, n)
				var names, ends []int
				for width := 40; width <= 82; width++ {
					f := frameOf(width, 80)
					if stackedRows(r, f) == 0 {
						continue
					}
					shownNames, shownEnds := shownRows(r, f)
					for i := range names {
						if shownNames[i] < names[i] || shownEnds[i] < ends[i] {
							t.Errorf("%s, %d accounts, %s card, at %d columns, row %d shows %d of its name and %d of its end, fewer than the %d and %d a column narrower",
								name, n, mode, width, i+1, shownNames[i], shownEnds[i], names[i], ends[i])
						}
					}
					names, ends = shownNames, shownEnds
				}
			}
		}
	}
}

// Where gaps of 1 leave it the room, a stacked row keeps its name and its
// end whole: engineering's at 59 columns, as the card drew it before its
// names were a label's column.
func TestAStackedRowsGapsNarrowBeforeItsName(t *testing.T) {
	r := routedCard(func(r *RoutingCard) {
		r.Rows[1].Name = "engineering"
		r.Rows[1].Open, r.Rows[1].Reason = false, widestHeld()
	})
	body := strings.Join(plains(r.card(frameOf(59, 60)).body), "\n")
	if want := "1 engineering closed  " + widestHeld(); !strings.Contains(body, want) {
		t.Errorf("at 59 columns, the card reads\n%s\nwant %q", body, want)
	}
}

// describedOf is how many characters of its words a description says, of the
// forms autoForms gives, each the start of the one before it: its own, but
// the ellipsis a cut ends with.
func describedOf(said string) int {
	return utf8.RuneCountInString(strings.TrimSuffix(strings.TrimRight(said, " "), prose.Ellipsis))
}

// Auto's description is a part of its row: the gaps and then the name give
// way before it's cut, so it never says less than the card said with its
// old fixed column of names, from column 16.
func TestAutosDescriptionNeverSaysLessThanBefore(t *testing.T) {
	for name, first := range widenedNames {
		if len(first) > labelMost {
			continue
		}
		for _, auto := range []string{"account-1", "account-2"} {
			r := accountsCard(sessionCard(nil), first, 3)
			r.Session.Auto = auto
			for width := 40; width <= 82; width++ {
				content := r.card(frameOf(width, 60)).width - 2*cardInset
				said := r.choice(r.Rows[0], r.columns(content), content)[0].plain()
				before := r.autoChooses(content - 16).plain()
				if describedOf(said[strings.Index(said, "the router"):]) < describedOf(before) {
					t.Errorf("%s, auto choosing %s, at %d columns, auto's row reads %q, less than the %q it said before", name, auto, width, said, before)
				}
			}
		}
	}
}
