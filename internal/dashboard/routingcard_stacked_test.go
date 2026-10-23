package dashboard

import (
	"fmt"
	"slices"
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
// given, then personal and side, then team-4 on, as namedCard has them.
func accountsCard(r RoutingCard, first string, n int) RoutingCard {
	names := []string{first, "personal", "side"}
	for i := 3; i < n; i++ {
		names = append(names, fmt.Sprintf("team-%d", i+1))
	}
	return namedCard(r, names[:n]...)
}

// namedCard is the routing card given, of an account each of the names
// given, each as the card's row a third of the way along, its end as that
// row's.
func namedCard(r RoutingCard, names ...string) RoutingCard {
	rows := []RoutingRow{r.Rows[0]}
	for i, name := range names {
		row := r.Rows[1+i%3]
		row.Account, row.Name = fmt.Sprintf("account-%d", i+1), name
		rows = append(rows, row)
	}
	r.Rows, r.Picked = rows, nil
	return r
}

// No name column is wider than its names, auto's among them, and none of 8
// cells or fewer is cut; and each row's end is whole wherever the name at
// its least, gaps of 1, the state and the end at its least fit: names all
// under 8 cells as well as longer ones.
func TestAStackedNameColumnIsNeverWiderThanItsNames(t *testing.T) {
	configs := map[string][]string{
		"a name shorter than auto's": {"ab"}, "work alone": {"work"}, "work and side": {"work", "side"},
		"the sample's names": {"work", "personal", "side"}, "a name of 11": {"engineering", "side"},
	}
	for name, names := range configs {
		for mode, base := range map[string]RoutingCard{"new sessions'": widestEnds(), "a session's": sessionCard(nil)} {
			r := namedCard(base, names...)
			least := choiceNameAt + min(labelLeast, r.widestName()) + narrowGap + stateCells() + narrowGap + endLeast()
			for width := 40; width <= 82; width++ {
				content := r.card(frameOf(width, 80)).width - 2*cardInset
				at := r.columns(content)
				if at.name > r.widestName() {
					t.Errorf("%s, %s card, at %d columns, the name column is %d cells, wider than its names' %d", name, mode, width, at.name, r.widestName())
				}
				for _, row := range r.Rows {
					if said := r.choice(row, at, content)[0].plain(); r.widestName() <= labelLeast && !strings.Contains(said, " "+row.Name+" ") {
						t.Errorf("%s, %s card, at %d columns, the row reads %q, want %q whole, its column under 8", name, mode, width, said, row.Name)
					}
				}
				if content < least || r.Mode == ThisSession && content < least+r.autoNatural()-(stateCells()+narrowGap+endLeast()) {
					continue
				}
				for _, row := range r.Rows[1:] {
					if said, end := r.choice(row, at, content)[0].plain(), row.sideWords().plain(); !strings.Contains(said, end) {
						t.Errorf("%s, %s card, at %d columns, the row reads %q, want %q whole", name, mode, width, said, end)
					}
				}
			}
		}
	}
}

// Stacked, where its row fits with them, its gaps are 2: the state starts 2
// after the name's column, and auto's description with it.
func TestAStackedRowsGapsAreTwoWhereTheyFit(t *testing.T) {
	r := sessionCard(nil)
	body := strings.Join(plains(r.card(frameOf(70, 60)).body), "\n")
	for _, want := range []string{"0 auto      the router chooses: side for now", "2 personal  open"} {
		if !strings.Contains(body, want) {
			t.Errorf("at 70 columns, the card reads\n%s\nwant %q", body, want)
		}
	}
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
// end whole: engineering's at 59 columns, as the base's column of 11 draws
// it.
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
// way before it's cut, so it never says less than the base's, its names in
// a column of 11, from column 16.
func TestAutosDescriptionNeverSaysLessThanBefore(t *testing.T) {
	for name, first := range widenedNames {
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

// Auto's row names an account as a label's column does, 24 cells at the
// most, so a longer name lays the rows out as one of 24.
func TestAutoNamesAnAccountAsALabelsColumnDoes(t *testing.T) {
	named := func(name string) RoutingCard {
		return sessionCard(func(r *RoutingCard) { r.Rows[1].Name = name })
	}
	long, most := named("a-very-long-account-name-extra"), named("a-very-long-account-name")
	for width := 40; width <= 100; width++ {
		if got, want := long.columns(width), most.columns(width); got != want {
			t.Errorf("at %d cells, a name of 30 lays the rows out %+v, want %+v as one of 24", width, got, want)
		}
	}
	long.Session.Auto = long.Rows[1].Account
	_, body := cardText(long, frameOf(100, 60))
	if want := "the router chooses: a-very-long-account-nam… for now"; !slices.ContainsFunc(body, func(l string) bool { return strings.Contains(l, want) }) {
		t.Errorf("at 100 columns, the card reads\n%s\nwant auto's row to say %q", strings.Join(body, "\n"), want)
	}
}

// Where the rows lie is a fact of the card and its configuration, never of
// which account the router would choose: the columns are the same whatever
// auto's row names, any account configured or none, whatever their names.
func TestTheColumnsHoldWhateverTheRouterChooses(t *testing.T) {
	for name, first := range widenedNames {
		r := sessionCard(func(r *RoutingCard) { r.Rows[1].Name = first })
		for width := 40; width <= 100; width++ {
			r.Session.Auto = ""
			want := r.columns(width)
			for _, row := range r.Rows[1:] {
				r.Session.Auto = row.Account
				if got := r.columns(width); got != want {
					t.Errorf("%s, at %d cells, auto choosing %s lays the rows out %+v, want %+v as with none", name, width, row.Account, got, want)
				}
			}
		}
	}
}
