package capture

import (
	"maps"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// recordedWorld is a world with its records, as a fixture over it holds them.
type recordedWorld struct {
	accountWorld
	records *records
}

// recorded is the world given with its records.
func recorded(w accountWorld) recordedWorld {
	return recordedWorld{accountWorld: w, records: worldsNow().recordsOf(w)}
}

// The accounts' worlds at the frames' moment, and the auto world and the
// ten's with their records, each built once for the tests, as building the
// year and its days takes a while.
var (
	worldsNow = sync.OnceValue(func() accountWorlds { return newAccountWorlds(moment(time.Local)) })
	autoWorld = sync.OnceValue(func() recordedWorld { return recorded(worldsNow().auto()) })
	tenWorld  = sync.OnceValue(func() recordedWorld { return recorded(worldsNow().many(10)) })
)

// readers are the world's records' readers, read at the frames' moment.
func (w recordedWorld) readers() (views.Ledger, views.Readings, views.Events) {
	return w.records.readers(func() time.Time { return worldsNow().now })
}

// ledger is the world's records' request ledger, read at the frames' moment.
func (w recordedWorld) ledger() views.Ledger {
	l, _, _ := w.readers()
	return l
}

// historyFrom is the History the views build of the world's days from day d
// of the year to today, through its records' ledger, of its accounts, at the
// frames' prices, as History and Accounts read it: weeks from Monday, the
// accounts' weeks Claude's.
func historyFrom(w recordedWorld, d int) views.History {
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
		world recordedWorld
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
		{name: "pinned", world: recorded(worlds.pinned()), minutes: map[string]atCapAndLimit{"work": {11, 0}}},
		{name: "routing's limit", world: recorded(worlds.limit()), minutes: map[string]atCapAndLimit{"work": {11, 0}, "personal": {0, 30}}},
		{name: "the bad hour", world: recorded(worlds.badHour()), minutes: map[string]atCapAndLimit{"work": {27, 0}, "personal": {0, 30}}},
		{name: "six accounts", world: recorded(worlds.many(6)), traffic: true, minutes: map[string]atCapAndLimit{"work": {11, 0}, "lab": {}}},
		{name: "ten accounts", world: tenWorld(), traffic: true, minutes: map[string]atCapAndLimit{"work": {11, 0}, "team": {0, 44}}},
		{name: "work alone", world: recorded(worlds.one()), minutes: map[string]atCapAndLimit{"work": {}}},
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
