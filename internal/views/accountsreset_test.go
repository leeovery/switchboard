package views_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// The moments of a week at its limit reset before the router reads it again,
// as AccountsMoments names them.
const (
	aSecondBefore  = "a week at its limit at 20:59:59, a second before it resets"
	beforeTheRead  = "a week reset at 21:00, at 21:00:01, before the router reads it again"
	onceItsReFiled = "a week reset at 21:00, reread at 21:00:30, the reset filed"
)

// accountsMoment is the moment of AccountsMoments with the given name.
func accountsMoment(t *testing.T, name string) viewstest.AccountsMoment {
	t.Helper()
	for _, am := range viewstest.AccountsMoments() {
		if am.Name == name {
			return am
		}
	}
	t.Fatalf("no moment %q", name)
	return viewstest.AccountsMoment{}
}

// weeksSaid says every figure of the page's weeks, each row's and every
// account's: its left at its last reset and the day its week ran out, its
// peaks and its verdict; and the accounts whose weeks end with the most and
// the least left.
func weeksSaid(page views.AccountsPage) string {
	var said []string
	for _, row := range append(page.Rows, page.All) {
		said = append(said, fmt.Sprintf("%s: left %s, ran out %q, peaks %s, %s", row.Totals.Account,
			shareRead(row.WeekLeft), row.RanOut, peaksRead(row.Peaks), verdictRead(row.Verdict)))
	}
	most, least, _ := page.Ends()
	return strings.Join(append(said, "ends "+most.Totals.Account+" and "+least.Totals.Account), "\n")
}

// A week reset since it was read, before the router reads it again, changes
// every figure of Accounts' weeks on the second WEEK USED reads none, as
// though the days held the reset, and once they do, they're the same again:
// at 21:00:01 as once it's reread and filed, both other than at 20:59:59.
func TestAWeekResetBeforeTheRereadChangesItsWeeksAsFiled(t *testing.T) {
	before, _ := accountsAt(accountsMoment(t, aSecondBefore))
	unread, _ := accountsAt(accountsMoment(t, beforeTheRead))
	filed, _ := accountsAt(accountsMoment(t, onceItsReFiled))

	if got, want := weeksSaid(unread), weeksSaid(filed); got != want {
		t.Errorf("before the reread, the weeks say\n%s\nwant them as once the reset is filed\n%s", got, want)
	}
	if got, was := weeksSaid(unread), weeksSaid(before); got == was {
		t.Errorf("at 21:00:01, the weeks say as at 20:59:59\n%s\nwant them changed by the reset", got)
	}
}

// Once the days hold the reset, a moment still reading the week as it was
// before it, its document not yet read again, doesn't count it twice: not
// even where it read less of the week than the days filed as it reset.
func TestAWeekResetTheDaysHoldIsntCountedTwice(t *testing.T) {
	filed := accountsMoment(t, onceItsReFiled)
	want, _ := accountsAt(filed)
	for _, used := range []float64{1, 0.9} {
		stale := weekReadAs(accountsMoment(t, beforeTheRead), "work", used)
		stale.Days = filed.Days
		got, _ := accountsAt(stale)
		if weeksSaid(got) != weeksSaid(want) {
			t.Errorf("with the reset filed, its document read before it at %v, the weeks say\n%s\nwant\n%s", used, weeksSaid(got), weeksSaid(want))
		}
	}
}

// A week reset before the reread closes at the most read of it, the days'
// highest or the document's: where the document read less than the days
// did, or the days less than the document, it had 0% left all the same.
func TestAWeekResetBeforeTheRereadClosesAtTheMostReadOfIt(t *testing.T) {
	tests := []struct {
		name string
		am   viewstest.AccountsMoment
	}{
		{name: "read at 90%, the days' highest 100%", am: weekReadAs(accountsMoment(t, beforeTheRead), "work", 0.9)},
		{name: "read at 100%, the days' highest 80%", am: weekHighestAtMost(accountsMoment(t, beforeTheRead), "work", "2026-09-29", 0.8)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, _ := accountsAt(tt.am)
			if got := shareRead(rowOf(t, page, "work").WeekLeft); got != "0%" {
				t.Errorf("work's week had %s left as it reset, want 0%%", got)
			}
		})
	}
}

// weekReadAs is the moment am, its document reading the week of the
// account with the given id used as given.
func weekReadAs(am viewstest.AccountsMoment, id string, used float64) viewstest.AccountsMoment {
	am.Doc.Accounts = slices.Clone(am.Doc.Accounts)
	for i, a := range am.Doc.Accounts {
		if a.ID != id {
			continue
		}
		windows := slices.Clone(a.Windows)
		for j := range windows {
			if windows[j].Key == claude.WeekWindow {
				windows[j].Utilization = used
			}
		}
		am.Doc.Accounts[i].Windows = windows
	}
	return am
}

// weekHighestAtMost is the moment am, the days from the one with the date
// from on holding the week of the account with the given id at most as
// high as given.
func weekHighestAtMost(am viewstest.AccountsMoment, id, from string, most float64) viewstest.AccountsMoment {
	days := slices.Clone(am.Days)
	for i := range days {
		if days[i].Day < from {
			continue
		}
		days[i].Accounts = slices.Clone(days[i].Accounts)
		for j, a := range days[i].Accounts {
			if highest, ok := a.Highest[claude.WeekWindow]; ok && a.Account == id {
				days[i].Accounts[j].Highest = maps.Clone(a.Highest)
				days[i].Accounts[j].Highest[claude.WeekWindow] = min(highest, most)
			}
		}
	}
	am.Days = days
	return am
}

// Standing the page at a moment changes its weeks alone, and leaves the page
// it stood as it was built: its totals, its plans and its days are the
// period's, which a reset doesn't change.
func TestStandingThePageAtAResetLeavesTheBuiltPageAndThePeriodAsTheyWere(t *testing.T) {
	am := accountsMoment(t, beforeTheRead)
	built := views.NewPast(accountsIn(am), am.Days).Accounts(am.Period)
	was := weeksSaid(built)
	stood := built.At(am.Doc.At(am.Now, claude.Policy))

	if weeksSaid(built) != was {
		t.Errorf("standing the page changed the page it stood: its weeks say\n%s\nwant\n%s", weeksSaid(built), was)
	}
	if weeksSaid(stood) == was {
		t.Errorf("standing the page at the reset left its weeks as built\n%s", was)
	}
	for i, row := range stood.Rows {
		if !reflect.DeepEqual(row.Totals, built.Rows[i].Totals) || !reflect.DeepEqual(row.Plan, built.Rows[i].Plan) || !slices.Equal(row.Days, built.Rows[i].Days) {
			t.Errorf("row %d stood counts %+v, %+v and %v, want as built, %+v, %+v and %v", i,
				row.Totals, row.Plan, row.Days, built.Rows[i].Totals, built.Rows[i].Plan, built.Rows[i].Days)
		}
	}
}
