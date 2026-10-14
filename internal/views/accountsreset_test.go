package views_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// The moments of a week at its limit reset before the router reads it again,
// as AccountsMoments names them.
const (
	aSecondBefore  = "a week at its limit at 20:59:59, a second before it resets"
	beforeTheRead  = "a week reset at 21:00, at 21:00:01, before the router reads it again"
	onceItsReFiled = "a week reset at 21:00, reread at 21:00:30, the reset filed"
	rereadUnfiled  = "a week reset at 21:00, reread at 21:00:30, the days read before it"
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
// at 21:00:01, and reread over days read before it, as once it's reread and
// filed, all other than at 20:59:59.
func TestAWeekResetBeforeTheRereadChangesItsWeeksAsFiled(t *testing.T) {
	before, _ := accountsAt(accountsMoment(t, aSecondBefore))
	unread, _ := accountsAt(accountsMoment(t, beforeTheRead))
	reread, _ := accountsAt(accountsMoment(t, rereadUnfiled))
	filed, _ := accountsAt(accountsMoment(t, onceItsReFiled))

	if got, want := weeksSaid(unread), weeksSaid(filed); got != want {
		t.Errorf("before the reread, the weeks say\n%s\nwant them as once the reset is filed\n%s", got, want)
	}
	if got, want := weeksSaid(reread), weeksSaid(filed); got != want {
		t.Errorf("reread before the days file the reset, the weeks say\n%s\nwant them as once it's filed\n%s", got, want)
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

// Reread, the document gives the run it reads a start a little off the
// reset the days file, as Claude gives a reset to the second or so: over the
// filed days it isn't counted again, and over days read before it, it's
// folded in at that start, the weeks as once it's filed either way.
func TestARereadWeekStartingOffTheFiledResetIsCountedOnce(t *testing.T) {
	want, _ := accountsAt(accountsMoment(t, onceItsReFiled))
	for _, name := range []string{onceItsReFiled, rereadUnfiled} {
		for _, off := range []time.Duration{-90 * time.Second, 90 * time.Second} {
			got, _ := accountsAt(weekResetsOff(accountsMoment(t, name), "work", off))
			if weeksSaid(got) != weeksSaid(want) {
				t.Errorf("%s, the week starting %v off, the weeks say\n%s\nwant\n%s", name, off, weeksSaid(got), weeksSaid(want))
			}
		}
	}
}

// weekResetsOff is the moment am, its document reading the week of the
// account with the given id to reset off as given.
func weekResetsOff(am viewstest.AccountsMoment, id string, off time.Duration) viewstest.AccountsMoment {
	am.Doc.Accounts = slices.Clone(am.Doc.Accounts)
	for i, a := range am.Doc.Accounts {
		if a.ID != id {
			continue
		}
		windows := slices.Clone(a.Windows)
		for j := range windows {
			if windows[j].Key == claude.WeekWindow {
				windows[j].ResetsAt = windows[j].ResetsAt.Add(off)
			}
		}
		am.Doc.Accounts[i].Windows = windows
	}
	return am
}

// A week reset before the reread closes at the most read of it, the days'
// highest or the document's of the week it read before it reset: where the
// document read less than the days did, or the days less than the document,
// it had 0% left all the same. A document read again since reads the next
// week, so the days' highest alone closes the week before it.
func TestAWeekResetBeforeTheRereadClosesAtTheMostReadOfIt(t *testing.T) {
	tests := []struct {
		name string
		am   viewstest.AccountsMoment
		want string
	}{
		{name: "read at 90%, the days' highest 100%", am: weekReadAs(accountsMoment(t, beforeTheRead), "work", 0.9), want: "0%"},
		{name: "read at 100%, the days' highest 80%", am: weekHighestAtMost(accountsMoment(t, beforeTheRead), "work", "2026-09-29", 0.8), want: "0%"},
		{name: "reread at 50% of the next week, the days' highest 40%",
			am: weekHighestAtMost(weekReadAs(accountsMoment(t, rereadUnfiled), "work", 0.5), "work", "2026-09-29", 0.4), want: "60%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, _ := accountsAt(tt.am)
			if got := shareRead(rowOf(t, page, "work").WeekLeft); got != tt.want {
				t.Errorf("work's week had %s left as it reset, want %s", got, tt.want)
			}
		})
	}
}

// Where the days hold no reset of the week that began it, it closes at the
// most read of it in the days after the one it began on, a week before it
// reset: not at a higher week's weeks before, nor since an older reset.
func TestAWeekResetBeforeTheRereadClosesAtTheMostReadInTheWeekItEnded(t *testing.T) {
	for _, from := range []string{"2026-08-10", "2026-09-14"} {
		am := weekReadAs(accountsMoment(t, beforeTheRead), "work", 0.4)
		am = weekHighestAtMost(withoutWeekResets(am, "work", from), "work", "2026-09-29", 0.4)
		page, _ := accountsAt(am)
		if got := shareRead(rowOf(t, page, "work").WeekLeft); got != "60%" {
			t.Errorf("work's resets from %s gone, its week had %s left as it reset, want 60%%, the 40%% read of it", from, got)
		}
	}
}

// withoutWeekResets is the moment am, its days from the one with the date
// from on holding no reset of the week of the account with the given id.
func withoutWeekResets(am viewstest.AccountsMoment, id, from string) viewstest.AccountsMoment {
	days := slices.Clone(am.Days)
	for i := range days {
		if days[i].Day < from {
			continue
		}
		days[i].Accounts = slices.Clone(days[i].Accounts)
		for j, a := range days[i].Accounts {
			if a.Account == id {
				days[i].Accounts[j].Resets = slices.DeleteFunc(slices.Clone(a.Resets), func(r ledger.Reset) bool { return r.Window == claude.WeekWindow })
			}
		}
	}
	am.Days = days
	return am
}

// A week reset before the reread with nothing read of it, the days' highest
// none and the document's, isn't folded in, as the ledger never files a
// reset of nothing: the page stands as it was built.
func TestAWeekResetBeforeTheRereadOfNothingReadIsntFoldedIn(t *testing.T) {
	am := weekHighestAtMost(weekReadAs(accountsMoment(t, beforeTheRead), "work", 0), "work", "2026-09-29", 0)
	built := views.NewPast(accountsIn(am), am.Days).Accounts(am.Period)
	if stood := built.At(am.Doc.At(am.Now, claude.Policy)); weeksSaid(stood) != weeksSaid(built) {
		t.Errorf("standing the page at a reset of nothing read, its weeks say\n%s\nwant them as built\n%s", weeksSaid(stood), weeksSaid(built))
	}
}

// The week the router reads began with no reset the days hold, as the ledger
// files none of a week it never read, of one of no use, or of one that
// lapsed but at its own reset: on a fresh install, an account added
// part-way through its week, a week that lapsed and one of no use before
// it. That week ends none before it, so standing the page at its moment
// leaves its weeks as built: no week closes at its start at its own use.
func TestAWeekBegunWithNoResetTheDaysHoldEndsNoneBeforeIt(t *testing.T) {
	for _, name := range []string{
		"a fresh install", "an account added part-way through the period",
		"a week lapsed before the one the router reads", "a week of no use before the one the router reads",
	} {
		t.Run(name, func(t *testing.T) {
			am := accountsMoment(t, name)
			built := builtOf(am)
			if stood := built.At(am.Doc.At(am.Now, claude.Policy)); weeksSaid(stood) != weeksSaid(built) {
				t.Errorf("standing the page, its weeks say\n%s\nwant them as built\n%s", weeksSaid(stood), weeksSaid(built))
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

// A window a week long of a model's own, as Fable's week, reset since it was
// read, leaves the page as it was built: Accounts' weeks are of the accounts'
// own week alone, so nothing of it is worked out again.
func TestAResetOfAWindowButTheAccountsWeekLeavesThePageAsBuilt(t *testing.T) {
	am := accountsMoment(t, aSecondBefore)
	fable := quota.Window{Key: "7d_oi", Label: "Fable week", Utilization: 0.5, ResetsAt: am.Now.Add(-15 * time.Second)}
	am = withWindow(am, "work", fable)
	built := views.NewPast(accountsIn(am), am.Days).Accounts(am.Period)
	if stood := built.At(am.Doc.At(am.Now, claude.Policy)); &stood.Rows[0] != &built.Rows[0] {
		t.Errorf("standing the page at Fable's week reset worked its rows out again, want the page as built")
	}
}

// withWindow is the moment am, its document reading the window given of the
// account with the given id, beside those it reads.
func withWindow(am viewstest.AccountsMoment, id string, w quota.Window) viewstest.AccountsMoment {
	am.Doc.Accounts = slices.Clone(am.Doc.Accounts)
	for i, a := range am.Doc.Accounts {
		if a.ID == id {
			am.Doc.Accounts[i].Windows = append(slices.Clone(a.Windows), w)
		}
	}
	return am
}

// Accounts' page, and Weeks', each stands alike at every moment of a
// document from the one it's stood at till Until, at each of Accounts' hard
// moments: a second before work's week resets, till the reset, after which
// it stands otherwise, the week closed.
func TestAPageStandsAlikeTillUntil(t *testing.T) {
	for _, am := range viewstest.AccountsMoments() {
		t.Run(am.Name, func(t *testing.T) {
			page, weeks := builtOf(am), pastOf(am).Weeks()
			m := am.Doc.At(am.Now, claude.Policy)
			until := page.Until(m)
			if got := weeks.Until(m); !got.Equal(until) || !until.After(am.Now) {
				t.Fatalf("Weeks stands till %v, Accounts' page till %v, want them alike, after %v", got, until, am.Now)
			}
			stood, stoodWeeks := page.At(m), weeks.At(m)
			for _, at := range []time.Time{am.Now.Add(until.Sub(am.Now) / 2), until.Add(-time.Millisecond)} {
				later := am.Doc.At(at, claude.Policy)
				if got := page.At(later); weeksSaid(got) != weeksSaid(stood) {
					t.Errorf("at %v, before %v, Accounts' page stands\n%s\nwant as at %v\n%s", at, until, weeksSaid(got), am.Now, weeksSaid(stood))
				}
				if got := weeks.At(later); !reflect.DeepEqual(got.Accounts, stoodWeeks.Accounts) || !reflect.DeepEqual(got.Capacity, stoodWeeks.Capacity) {
					t.Errorf("at %v, before %v, Weeks stands otherwise than at %v", at, until, am.Now)
				}
			}
		})
	}
	am := accountsMoment(t, aSecondBefore)
	page, m := builtOf(am), am.Doc.At(am.Now, claude.Policy)
	if until, want := page.Until(m), am.Now.Add(time.Second); !until.Equal(want) {
		t.Fatalf("a second before work's week resets, the page stands till %v, want %v", until, want)
	}
	if after := page.At(am.Doc.At(am.Now.Add(2*time.Second), claude.Policy)); weeksSaid(after) == weeksSaid(page.At(m)) {
		t.Errorf("past work's week's reset, the page stands as it did before it, want the week closed")
	}
}
