package status_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
)

// agreementMoments are the hard moments, and the moments the cross-view
// audit found views disagreeing at that the hard moments lack: work keeping
// a reserve, its session reaching it at its recent rate before it resets;
// work's 5-hour at the router's limit, its week read spent too, so it's
// back once the week resets, not as the router's limit lifts; and work held
// back by a limit and its cap at once, the cap the later, the limit naming
// its 5-hour, or no window.
func agreementMoments() []statustest.HardMoment {
	side := statustest.Routed{Account: readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.2, 5*day))}
	limited := func(windows ...string) func(*status.Account) {
		return func(a *status.Account) {
			a.Limit = status.Limit{ID: 4, Windows: windows, Until: built.Add(40 * time.Minute)}
		}
	}
	// routed is the moment of the given name, now after built, of the
	// router's document of work, as given, and side, with the event of the
	// limit work reached, where one holds it.
	routed := func(name string, now time.Duration, work statustest.Routed) statustest.HardMoment {
		doc := statustest.Router(policy, nil, work, side)
		if l := work.Limit; l.ID != 0 {
			doc.Events = []status.Event{{ID: 9, At: built.Add(-20 * time.Minute), Kind: status.EventLimit, Account: "work", Windows: l.Windows, Until: l.Until, Limit: l.ID}}
		}
		return statustest.HardMoment{Name: name, Doc: doc.WorkedOut(policy), Now: built.Add(now)}
	}
	return append(statustest.HardMoments(policy),
		routed("work keeping a reserve, its session reaching it at its recent rate", 5*time.Minute, statustest.Routed{
			Account: with(readAt("work", windowIn("5h", 0.6, 3*time.Hour), windowIn("7d", 0.3, 4*day)), reserving), Rate: 0.3,
		}),
		routed("work's 5-hour at the router's limit, its week read spent too", 10*time.Minute, statustest.Routed{
			Account: with(readAt("work", spentIn("5h", 1, 40*time.Minute), spentIn("7d", 1, 2*day)), limited("5h")),
		}),
		routed("work's 5-hour at its limit, its week at its cap, the later", 10*time.Minute, statustest.Routed{
			Account: with(with(readAt("work", spentIn("5h", 1, 40*time.Minute), windowIn("7d", 0.95, 70*time.Minute)), reserving), limited("5h")),
		}),
		routed("work at a limit naming no window, its week at its cap, the later", 10*time.Minute, statustest.Routed{
			Account: with(with(readAt("work", windowIn("5h", 0.5, 3*time.Hour), windowIn("7d", 0.85, 70*time.Minute)), func(a *status.Account) {
				a.Reserve = 0.2
			}), limited()),
		}),
	)
}

// At each moment of agreementMoments, whatever says when an account held
// back from new work is back says the one time its moment has it, its Back:
// ROUTING, of an account it says a limit or its cap holds back from every
// request; COMING UP, of an account out of quota, a back at Back where
// that's to come, and of any other, none; and status's text, its limit line
// of a limit holding back every request. A refusal's own line says when the
// refusal ends, and a limit of some models' requests alone says when it
// lifts, as ROUTING does. The routing card's say is held to Back by its own
// test at each hard moment.
func TestEachViewSaysWhenAnAccountIsBackAsItsMomentHasIt(t *testing.T) {
	for _, hm := range agreementMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			m := hm.Doc.At(hm.Now, policy)
			text := textOf(m)
			for _, a := range m.Accounts {
				checkROUTINGsBack(t, a, m.Now)
				checkCOMINGUPsBack(t, a, m)
				checkTextsBack(t, a, text[a.ID], m.Now)
			}
		})
	}
}

// checkROUTINGsBack checks that ROUTING says the account a is back at its
// Back, at now, where it says a limit or its cap holds it back from every
// request: its Back, and its first clause, where Back is known.
func checkROUTINGsBack(t *testing.T, a status.AccountNow, now time.Time) {
	t.Helper()
	r := a.Routing
	if r.Kind != status.RoutingLimited && r.Kind != status.RoutingCapped {
		return
	}
	back := status.Back(now, a.Back)
	if said := firstOr(r.Clauses, ""); !r.Back.Equal(a.Back) || back != "" && said != back {
		t.Errorf("%s: ROUTING says %q, back at %v; want back at %v, its Back", a.ID, r, r.Back, a.Back)
	}
}

// checkCOMINGUPsBack checks the backs the moment m's COMING UP tells of the
// account a, from every request: one at its Back where it's out of quota and
// that's to come; else none.
func checkCOMINGUPsBack(t *testing.T, a status.AccountNow, m status.Moment) {
	t.Helper()
	var told, want []time.Time
	for _, u := range m.ComingUp {
		if u.Account == a.ID && u.Kind == status.UpcomingBack && u.Window == "" {
			told = append(told, u.At)
		}
	}
	if a.OutOfQuota && a.Back.After(m.Now) {
		want = []time.Time{a.Back}
	}
	if !slices.EqualFunc(told, want, time.Time.Equal) {
		t.Errorf("%s: COMING UP tells of it back at %v; want %v, its Back where it's out of quota", a.ID, told, want)
	}
}

// checkTextsBack checks the limit line status's text gives the account a, as
// its lines have it, at now: of a limit holding back its every request, until
// its Back; of one holding back some models' alone, until it lifts, as
// ROUTING says; that it's reached alone where that isn't known; and none
// where no limit holds it.
func checkTextsBack(t *testing.T, a status.AccountNow, lines []string, now time.Time) {
	t.Helper()
	var said, want []string
	for _, l := range lines {
		if strings.HasPrefix(l, "limit ") {
			said = append(said, l)
		}
	}
	if a.Held {
		until := a.Hold.Until
		if a.Hold.Every {
			until = a.Back
		}
		want = []string{"limit reached"}
		if !until.IsZero() {
			want = []string{"limit until " + prose.Clock(now, until)}
		}
	}
	if !slices.Equal(said, want) {
		t.Errorf("%s: status's text says %q; want %q, as its Back has it", a.ID, said, want)
	}
}

// textOf is status's text of the moment m, each account's lines under its
// title by its id, their indent trimmed.
func textOf(m status.Moment) map[string][]string {
	text := make(map[string][]string)
	for block := range strings.SplitSeq(m.Text(), "\n\n") {
		title, rest, _ := strings.Cut(block, "\n")
		title = strings.TrimSuffix(title, " (primary)")
		i := slices.IndexFunc(m.Accounts, func(a status.AccountNow) bool { return a.Title() == title })
		if i < 0 {
			continue
		}
		for l := range strings.SplitSeq(rest, "\n") {
			if trimmed, ok := strings.CutPrefix(l, "  "); ok {
				text[m.Accounts[i].ID] = append(text[m.Accounts[i].ID], trimmed)
			}
		}
	}
	return text
}

// firstOr is the first of the things given, or none where there are none.
func firstOr[T any](things []T, none T) T {
	if len(things) == 0 {
		return none
	}
	return things[0]
}
