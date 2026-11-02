package capture

import (
	"maps"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// autoWorld is the auto world, as the worlds keep it.
func autoWorld() accountWorld {
	return worldsNow().auto()
}

// tenWorld is the world of ten accounts, as the worlds keep it.
func tenWorld() accountWorld {
	return worldsNow().many(10)
}

// readers are the world's records' readers, read at the frames' moment.
func (w accountWorld) readers() (views.Ledger, views.Readings, views.Events) {
	return w.records().readers(func() time.Time { return worldsNow().now })
}

// ledger is the world's records' request ledger, read at the frames' moment.
func (w accountWorld) ledger() views.Ledger {
	l, _, _ := w.readers()
	return l
}

// historyFrom is the History the views build of the world's days from day d
// of the year to today, through its records' ledger, of its accounts, at the
// frames' prices, as History and Accounts read it: weeks from Monday, the
// accounts' weeks Claude's.
func historyFrom(w accountWorld, d int) views.History {
	now := worldsNow().now
	return views.NewHistory(views.HistoryInput{
		Ledger: w.ledger(), From: dayStart(now, d), Now: now, Prices: drawnPrices, Accounts: w.accounts,
		WeekStarts: time.Monday, WeekWindow: claude.WeekWindow, Family: claude.Provider{}.Family,
	})
}

// dollars is a sum of money, as the ledger counts it, in whole dollars, as
// the frames draw worth.
func dollars(t *testing.T, w views.Worth) int {
	t.Helper()
	if w.Amount == nil || len(w.Unpriced) > 0 {
		t.Fatalf("worth %v isn't priced whole", w)
	}
	return int(math.Round(float64(*w.Amount) / 1e12))
}

// percent is a share in whole percent, as the frames draw it.
func percent(share float64) int {
	return int(math.Round(share * 100))
}

// tenths is a ratio to a tenth, as the frames draw against its price.
func tenths(ratio *float64) float64 {
	if ratio == nil {
		return math.NaN()
	}
	return math.Round(*ratio*10) / 10
}

// dayOfAccount is the account with the given id's day among the summary's.
func dayOfAccount(t *testing.T, s ledger.Summary, id string) ledger.AccountDay {
	t.Helper()
	i := slices.IndexFunc(s.Accounts, func(a ledger.AccountDay) bool { return a.Account == id })
	if i < 0 {
		t.Fatalf("%s's summary has no day of %s", s.Day, id)
	}
	return s.Accounts[i]
}

// atCapAndLimit are an account's minutes at its cap and at a limit on a
// day, as its summary counts them.
type atCapAndLimit struct {
	cap, limit int
}

// minutesOf are the account's day's minutes at its cap and at a limit.
func minutesOf(t *testing.T, a ledger.AccountDay) atCapAndLimit {
	t.Helper()
	if a.MinutesAtCap == nil || a.MinutesAtLimit == nil {
		t.Fatalf("%s's day counts no minutes at its cap or a limit", a.Account)
	}
	return atCapAndLimit{*a.MinutesAtCap, *a.MinutesAtLimit}
}

// Each world the frames draw has records of its own, read through its
// readers: the year's days of its accounts, to today's; its readings; its
// accounts' caps; and, where it holds them, today's lines and the router's
// events, of the three alone.
func TestEachWorldsRecordsAreItsDaysReadingsCapsAndTraffic(t *testing.T) {
	worlds := worldsNow()
	tests := []struct {
		name  string
		world accountWorld
		// traffic is whether today's traffic of the three, the sessions'
		// world's lines and the router's events, is the world's.
		traffic bool
		// minutes are, of the accounts named, the minutes today at their caps
		// and at a limit, as the world's readings and its accounts' caps give
		// them: work's from its cap at 14:31:03, or at 14:15 in the bad hour,
		// and personal's from its limit at 14:12.
		minutes map[string]atCapAndLimit
	}{
		{name: "auto", world: autoWorld(), traffic: true, minutes: map[string]atCapAndLimit{"work": {11, 0}, "personal": {}}},
		{name: "pinned", world: worlds.pinned(), minutes: map[string]atCapAndLimit{"work": {11, 0}}},
		{name: "routing's limit", world: worlds.limit(), minutes: map[string]atCapAndLimit{"work": {11, 0}, "personal": {0, 30}}},
		{name: "the bad hour", world: worlds.badHour(), minutes: map[string]atCapAndLimit{"work": {27, 0}, "personal": {0, 30}}},
		{name: "six accounts", world: worlds.many(6), traffic: true, minutes: map[string]atCapAndLimit{"work": {11, 0}, "lab": {}}},
		{name: "ten accounts", world: tenWorld(), traffic: true, minutes: map[string]atCapAndLimit{"work": {11, 0}, "team": {0, 44}}},
		{name: "work alone", world: worlds.one(), minutes: map[string]atCapAndLimit{"work": {}}},
		{name: "f41a resumed", world: worlds.resumed(), traffic: true, minutes: map[string]atCapAndLimit{"work": {11, 0}}},
		{name: "all three verified Ultra", world: worlds.ultra(), traffic: true, minutes: map[string]atCapAndLimit{"work": {}, "personal": {}}},
		{name: "work and personal verified Ultra", world: worlds.mixed(), traffic: true, minutes: map[string]atCapAndLimit{"work": {}, "side": {}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, read, happened := tt.world.readers()
			days := l.Days(dayStart(worlds.now, 0))
			if len(days) != today+1 || days[0].Day != "2025-09-29" || days[today].Day != "2026-10-01" {
				t.Fatalf("the ledger gives %d days, from %s to %s, want the year's %d, from 29 Sep 2025 to today", len(days), days[0].Day,
					days[len(days)-1].Day, today+1)
			}
			summarised := make(map[string]bool)
			for _, s := range days {
				for _, a := range s.Accounts {
					summarised[a.Account] = true
				}
			}
			if got, want := slices.Sorted(maps.Keys(summarised)), slices.Sorted(slices.Values(tt.world.accounts.IDs())); !slices.Equal(got, want) {
				t.Errorf("the days summarise %v, want the world's accounts, %v", got, want)
			}
			if !slices.IsSortedFunc(tt.world.records().readings, func(a, b readings.Reading) int { return a.At.Compare(b.At) }) {
				t.Error("the world's readings aren't in the order they were read, which its reader searches them by")
			}
			if n := len(slices.Collect(read.Between(time.Time{}, worlds.now))); n != len(tt.world.history) {
				t.Errorf("the readings reader gives %d readings, want the world's %d", n, len(tt.world.history))
			}
			for id, want := range tt.minutes {
				if got := minutesOf(t, dayOfAccount(t, days[today], id)); got != want {
					t.Errorf("%s spent %+v minutes at its cap and a limit today, want %+v", id, got, want)
				}
			}
			lines, _, _ := l.Today(ledger.Mark{})
			told := slices.Collect(happened.Between(dayStart(worlds.now, today-1), worlds.now))
			if (len(lines) > 0) != tt.traffic || (len(told) > 0) != tt.traffic {
				t.Errorf("the world holds %d of today's lines and %d events since yesterday, want them %t", len(lines), len(told), tt.traffic)
			}
			for _, h := range lines {
				if h.Account != "" && !slices.Contains(three, h.Account) {
					t.Errorf("a line of today's went to %s, want the three's alone", h.Account)
				}
			}
		})
	}
}

// Work reaches its cap at 14:31:03, as each record tells it: its 5-hour window
// read at 95%, off the answer to 7f3a's request of 14:30:51, the last on work
// to end before it; the router's event of it; and today's summary, at its
// highest there and at its cap since.
func TestWorksCapIsTheSameMomentInTheReadingsTheEventsTheLinesAndTheSummary(t *testing.T) {
	now := worldsNow().now
	capped := dayAt(now, today, 14, 31, 3)
	l, read, happened := autoWorld().readers()
	var reached time.Time
	for r := range read.Between(dayStart(now, today), now) {
		if r.Account == "work" && r.Key == sessionKey && r.Utilization >= 1-workReserve {
			reached = r.At
			break
		}
	}
	if !reached.Equal(capped) {
		t.Errorf("work's 5-hour window was first read at its cap at %s, want 14:31:03", reached.In(now.Location()))
	}
	i := slices.IndexFunc(slices.Collect(happened.Between(dayStart(now, today), now)), func(e events.Line) bool {
		return e.Kind == status.EventCap && e.Account == "work" && e.At.Equal(capped)
	})
	if i < 0 {
		t.Error("the router tells of no cap of work's at 14:31:03")
	}
	lines, _, _ := l.Today(ledger.Mark{})
	var last ledger.Held
	for _, h := range lines {
		if ended := endOf(h.Line); h.Account == "work" && !ended.After(capped) && ended.After(endOf(last.Line)) {
			last = h
		}
	}
	if last.Session != id7F3A || !last.At.Equal(dayAt(now, today, 14, 30, 51)) || capped.Sub(endOf(last.Line)) >= time.Second {
		t.Errorf("the last request on work to end before its cap is %q's of %s, ending %s, want 7f3a's of 14:30:51, ending in the second before",
			last.Session, last.At.In(now.Location()).Format(time.TimeOnly), endOf(last.Line).In(now.Location()).Format(time.TimeOnly))
	}
	work := dayOfAccount(t, todaysSummary(t, l, now), "work")
	if work.Highest[sessionKey] != 1-workReserve || minutesOf(t, work) != (atCapAndLimit{cap: 11}) {
		t.Errorf("today's summary has work's 5-hour window at %v at its highest, %+v minutes at its cap and a limit, want 0.95, 11 at its cap",
			work.Highest[sessionKey], minutesOf(t, work))
	}
}

// verifiedWorld is a world of verified Ultra accounts, by name, as the worlds
// keep it, and the ids of those of its accounts that are verified.
type verifiedWorld struct {
	name     string
	world    accountWorld
	verified []string
}

// verifiedWorlds are the worlds of verified Ultra accounts: all three, and
// work and personal alone, side keeping its 5-hour window.
func verifiedWorlds() []verifiedWorld {
	worlds := worldsNow()
	return []verifiedWorld{
		{name: "ultra", world: worlds.ultra(), verified: three},
		{name: "mixed", world: worlds.mixed(), verified: []string{"work", "personal"}},
	}
}

// isVerified reports whether the account with the given id is one of the
// world's verified.
func (v verifiedWorld) isVerified(id string) bool {
	return slices.Contains(v.verified, id)
}

// unreportedAt reports whether the account with the given id reports no
// 5-hour window at at in the world: verified, and at no earlier than Friday
// 25 September's start.
func (v verifiedWorld) unreportedAt(id string, at time.Time) bool {
	return v.isVerified(id) && !at.Before(dayStart(worldsNow().now, today-6))
}

// heldOffAt reports whether reason, given at at, moves a session off one of
// the world's verified accounts at its reserve or its limit once it reports
// no 5-hour window, as only a cap or limit on that window could.
func (v verifiedWorld) heldOffAt(reason string, at time.Time) bool {
	for _, id := range v.verified {
		off := status.ReasonMovedOff + id + " "
		if v.unreportedAt(id, at) && (reason == off+status.WhyReserve || reason == off+status.WhyLimit) {
			return true
		}
	}
	return false
}

// isFiveHourHeader reports whether an answer's header, as a line keeps it,
// is one of the 5-hour window's.
func isFiveHourHeader(name string) bool {
	return strings.HasPrefix(name, fiveHourKey+"-")
}

// A verified Ultra account's answers carry no 5-hour window, so the router's
// document holds none of its, nor a trail of one: of every account in the
// ultra world, and of work's and personal's in the mixed one, where side
// keeps its own, at 13% now, resetting at 18:50.
func TestTheDocumentHoldsNoFiveHourWindowOfAVerifiedAccount(t *testing.T) {
	now := worldsNow().now
	for _, v := range verifiedWorlds() {
		t.Run(v.name, func(t *testing.T) {
			doc := v.world.document(now)
			for _, a := range doc.Accounts {
				w, held := a.Window(sessionKey)
				if held == v.isVerified(a.ID) {
					t.Errorf("the document holds %s's 5-hour window: %t, want %t", a.ID, held, !v.isVerified(a.ID))
				}
				if held && (percent(w.Utilization) != 13 || !w.ResetsAt.Equal(dayAt(now, today, 18, 50, 0))) {
					t.Errorf("%s's 5-hour window reads %d%%, resetting %s, want 13%%, resetting 18:50", a.ID, percent(w.Utilization),
						w.ResetsAt.In(now.Location()))
				}
			}
			for _, tr := range v.world.trails(doc, now) {
				if tr.window == sessionKey && v.isVerified(tr.account) {
					t.Errorf("the router gives a trail of %s's 5-hour window, want none", tr.account)
				}
			}
		})
	}
}

// The readings history keeps a verified account's 5-hour readings from
// before Friday 25 September, as real history does, and holds none from
// then on; side, in the mixed world, keeps its own to today's.
func TestTheHistoryKeepsAVerifiedAccountsFiveHourReadingsBeforeItsCutoffAlone(t *testing.T) {
	cutoff := dayStart(worldsNow().now, today-6)
	for _, v := range verifiedWorlds() {
		t.Run(v.name, func(t *testing.T) {
			before, after := make(map[string]int), make(map[string]int)
			for _, r := range v.world.records().readings {
				switch {
				case r.Key != sessionKey:
				case r.At.Before(cutoff):
					before[r.Account]++
				default:
					after[r.Account]++
				}
			}
			for _, id := range v.world.accounts.IDs() {
				if before[id] == 0 {
					t.Errorf("the history holds no 5-hour reading of %s's before the cutoff, want its past ones", id)
				}
				if v.isVerified(id) != (after[id] == 0) {
					t.Errorf("the history holds %d 5-hour readings of %s's from the cutoff, want them %t", after[id], id, !v.isVerified(id))
				}
			}
		})
	}
}

// Nothing the router tells, records or streams after a verified account's
// cutoff names its 5-hour window: no event of it, in the document or the
// records; no limit or pressure on it; no answer's 5-hour headers in a
// line; and no move off it at its reserve or its limit, in an event, a
// line, an account a line tried, a session listed or the stream.
func TestNothingAfterTheCutoffNamesAVerifiedAccountsFiveHourWindow(t *testing.T) {
	now := worldsNow().now
	for _, v := range verifiedWorlds() {
		t.Run(v.name, func(t *testing.T) {
			told := slices.Clone(v.world.events)
			for _, l := range v.world.records().events {
				told = append(told, l.Event)
			}
			for _, e := range told {
				if v.unreportedAt(e.Account, e.At) && slices.Contains(e.Windows, sessionKey) {
					t.Errorf("the router tells of %s's 5-hour window at %s, in a %s event, want nothing", e.Account, e.At.In(now.Location()), e.Kind)
				}
				if v.heldOffAt(e.Reason, e.At) {
					t.Errorf("the router's %s event of %s says %q, want no move off a verified account at its reserve or limit", e.Kind,
						e.At.In(now.Location()), e.Reason)
				}
			}
			for _, a := range v.world.document(now).Accounts {
				if v.isVerified(a.ID) && (slices.Contains(a.Limit.Windows, sessionKey) || a.Pressure.Window == sessionKey) {
					t.Errorf("%s's document holds a limit on %v and pressure on %q, want neither on its 5-hour window", a.ID, a.Limit.Windows, a.Pressure.Window)
				}
			}
			for _, f := range v.world.records().lines {
				l := f.Line
				if v.unreportedAt(l.Account, l.At) && slices.ContainsFunc(slices.Collect(maps.Keys(l.Limits)), isFiveHourHeader) {
					t.Errorf("a line on %s at %s keeps the 5-hour headers of %v, want none", l.Account, l.At.In(now.Location()), l.Limits)
				}
				if v.heldOffAt(l.Reason, l.At) {
					t.Errorf("a line at %s says %q, want no move off a verified account at its reserve or limit", l.At.In(now.Location()), l.Reason)
				}
				for _, tried := range l.Tried {
					if v.unreportedAt(tried.Account, l.At) && tried.Why == status.WhyLimit {
						t.Errorf("a line at %s tried %s, which %s, want it never at a limit", l.At.In(now.Location()), tried.Account, tried.Why)
					}
				}
			}
			for _, s := range v.world.routed(now) {
				for _, a := range s.Assignments {
					if v.heldOffAt(a.Reason, a.AssignedAt) {
						t.Errorf("the router lists %s's %s as %q, want no move off a verified account at its reserve or limit", s.ID, a.Model, a.Reason)
					}
				}
			}
			for _, e := range v.world.stream {
				if v.unreportedAt(e.Account, e.At) && e.Verdict == router.StreamLimited || v.heldOffAt(e.Reason, e.At) {
					t.Errorf("the stream tells of %s on %s, %q, %q, want no limit on a verified account", e.Request, e.Account, e.Verdict, e.Reason)
				}
			}
		})
	}
}

// Without a 5-hour window, work never reaches its cap at 14:31:03: the
// router tells of its token refused then instead, moving its three sessions
// to side as the cap did, each counted in the refusal, which has lifted by
// the moment, so work is open.
func TestWorksCapIsARefusalLiftedByTheMoment(t *testing.T) {
	now := worldsNow().now
	for _, v := range verifiedWorlds() {
		t.Run(v.name, func(t *testing.T) {
			told := v.world.events
			i := slices.IndexFunc(told, func(e status.Event) bool { return e.Kind == status.EventRefused && e.Account == "work" })
			if i < 0 {
				t.Fatal("the router tells of no refusal of work's")
			}
			refused := told[i]
			if !refused.At.Equal(dayAt(now, today, 14, 31, 3)) || refused.Until.After(now) || refused.Count != 3 || refused.To != "side" {
				t.Errorf("work was refused at %s till %s, moving %d to %s, want at 14:31:03, lifted by now, moving 3 to side",
					refused.At.In(now.Location()), refused.Until.In(now.Location()), refused.Count, refused.To)
			}
			var moved []string
			for _, e := range told {
				if e.Kind == status.EventMoved && e.ForcedBy == refused.ID {
					moved = append(moved, e.Session)
				}
			}
			if want := []string{idD28C, id7F3A, idDB8A}; !slices.Equal(moved, want) {
				t.Errorf("the refusal moved %v, want d28c, 7f3a and db8a", moved)
			}
			if work := v.world.document(now).Accounts[0]; work.Shut(now, claude.Policy()) {
				t.Errorf("work is shut at the moment, refused till %s, want it open", work.Refused.Until)
			}
		})
	}
}

// The verified accounts are on Ultra; side, keeping its 5-hour window in
// the mixed world, on Max.
func TestTheVerifiedWorldsPlans(t *testing.T) {
	tests := []struct {
		name  string
		world accountWorld
		want  []string
	}{
		{name: "ultra", world: worldsNow().ultra(), want: []string{planUltra, planUltra, planUltra}},
		{name: "mixed", world: worldsNow().mixed(), want: []string{planUltra, planUltra, planMax}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, a := range tt.world.accounts {
				got = append(got, a.Plan)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("the accounts are on %v, want %v", got, tt.want)
			}
		})
	}
}
