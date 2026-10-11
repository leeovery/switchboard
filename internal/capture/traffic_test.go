package capture

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// todaysSummary is the summary of the day now falls on, as the ledger reads
// its lines into it.
func todaysSummary(t *testing.T, l views.Ledger, now time.Time) ledger.Summary {
	t.Helper()
	days := l.Days(clockAt(now, 0, 0, 0, 0))
	if len(days) != 1 || days[0].Day != now.Format(time.DateOnly) {
		t.Fatalf("the ledger gives days %v from today's start, want today alone", days)
	}
	return days[0]
}

// short is a session's id as the frames cut it.
func short(id string) string {
	return status.ShortID(id)[:4]
}

// The Log's counts are today's lines read as it reads them, through the auto
// world's readers, by kind, status and session, and today's summary agrees.
// The Log's 1,275 lines and 1,240 requests, and c61b's 5, are left out: c61b
// has four lines, its fifth request answering at 14:42:07 as log1.tail draws
// it, not yet a line.
func TestTheLogCountsTodaysLinesAsItsPanelsDo(t *testing.T) {
	now := localMoment()
	l := autoWorld().ledger()
	lines, _, _ := l.Today(ledger.Mark{})
	kinds, failed, requests := make(map[string]int), 0, make(map[string]int)
	for _, h := range lines {
		kinds[h.Kind]++
		if h.Status >= 400 || h.Canceled || h.CutOff {
			failed++
		}
		if h.Kind == ledger.KindMessage {
			requests[short(h.Session)]++
		}
	}
	if kinds[ledger.KindCheck] != 31 || kinds[ledger.KindCount] != 4 || failed != 3 {
		t.Errorf("today's lines hold %d checks, %d counts and %d failed, want 31, 4 and 3", kinds[ledger.KindCheck], kinds[ledger.KindCount], failed)
	}
	running := map[string]int{"d28c": 445, "9c2e": 267, "7f3a": 84, "db8a": 126, "41e0": 158}
	more := 0
	for id, n := range requests {
		want, ok := running[id]
		switch {
		case ok && n != want:
			t.Errorf("today's lines hold %d of %s's requests, want %d", n, id, want)
		case !ok && id != "c61b":
			more += n
		}
	}
	if len(requests) != 14 || more != 155 {
		t.Errorf("today's lines are of %d sessions, %d of the 8 more's requests, want 14 and 155", len(requests), more)
	}
	day := todaysSummary(t, l, now)
	var summed, checks, counts int
	for _, a := range day.Accounts {
		for _, m := range a.Models {
			summed, checks, counts = summed+m.Requests(), checks+m.Checks, counts+m.Counts
		}
	}
	if summed != kinds[ledger.KindMessage] || checks != 31 || counts != 4 || day.Lines != len(lines) {
		t.Errorf("today's summary counts %d requests, %d checks, %d counts in %d lines, want %d, 31, 4 in %d",
			summed, checks, counts, day.Lines, kinds[ledger.KindMessage], len(lines))
	}
}

// What today's lines sum to on each account is what Accounts' week takes
// today's to be, its days before today holding the rest: 592 requests, $57.08,
// on work, 463, $48.13, on personal, and 184, $20.74, on side, each model
// version's worth as weekUses has it, to a tenth of a cent; and none on the
// seven more of the frames of ten, whose world holds the three's lines.
func TestTodaysLinesSumOnEachAccountAsAccountsWeekTakesThem(t *testing.T) {
	now := localMoment()
	priced := drawnPrices.Priced(todaysSummary(t, tenWorld().ledger(), now), now.Format(time.DateOnly))
	drawn := map[string]string{"work": "592 requests, $57.08", "personal": "463 requests, $48.13", "side": "184 requests, $20.74"}
	for _, a := range priced.Accounts {
		var requests int
		var worth ledger.Picodollars
		byModel := make(map[string]ledger.Picodollars)
		for _, m := range a.Models {
			requests += m.Requests()
			worth += *m.Worth
			byModel[m.Model] += *m.Worth
		}
		mills := make(map[string]int64)
		for model, w := range byModel {
			mills[model] = int64(math.Round(float64(w) / 1e9))
		}
		if want, ok := drawn[a.Account]; ok && fmt.Sprintf("%d requests, %s", requests, worth.Cents()) != want {
			t.Errorf("today's lines sum to %d requests, %s, on %s, want %s", requests, worth.Cents(), a.Account, want)
		}
		delete(drawn, a.Account)
		want := weekUses[a.Account].today
		wantMills := make(map[string]int64)
		for v, w := range want.worths {
			wantMills[v] = int64(math.Round(w * 1000))
		}
		if requests != want.requests || !maps.Equal(mills, wantMills) {
			t.Errorf("today's lines on %s are %d requests worth %v mills by version, want %d worth %v, as Accounts' week takes them",
				a.Account, requests, mills, want.requests, wantMills)
		}
	}
	if len(drawn) > 0 {
		t.Errorf("today's summary has no day of %v", slices.Sorted(maps.Keys(drawn)))
	}
}

// listedAs is a session as the List has it: the account it's on, what it's
// doing, when it started, when it ended, to the minute, its requests today
// and their worth, and the move that brought it to its account, from where
// and at what cost.
type listedAs struct {
	account, state string
	started        time.Time
	ended          string
	requests       int
	worth          string
	from, cost     string
}

// listedOf is the session as the List has it, at now.
func listedOf(s views.ListedSession, now time.Time) listedAs {
	got := listedAs{account: s.Account, state: s.State, started: s.Started.In(now.Location()), requests: s.Requests, worth: shownWorth(s.Worth)}
	if !s.Ended.IsZero() {
		got.ended = s.Ended.In(now.Location()).Format("15:04")
	}
	if s.Moved != nil {
		got.from = s.Moved.From
		if s.Moved.Cost != nil {
			got.cost = s.Moved.Cost.Cents()
		}
	}
	return got
}

// The List's sessions at 14:42:07 are today's, as it lists them from the
// router's and the ledger's: the six running, and the eight that ended today.
// Left out: c61b's 5 requests, $0.29 (four lines, $0.22, its fifth answering);
// 8a2c's move from personal at its limit, $0.61, which came yesterday, where
// the List says only a move today; and the today line's 1,240 requests,
// $126.02 (1,239, $125.94, c61b's fifth not among them).
func TestTheListIsTodaysSessionsAsTheRouterAndTheLedgerTellThem(t *testing.T) {
	now := localMoment()
	at := func(days, hour, minute, second int) time.Time { return clockAt(now, days, hour, minute, second) }
	want := map[string]listedAs{
		"41e0": {"personal", "", at(0, 12, 47, 12), "", 158, "$17.86", "", ""},
		"9c2e": {"personal", "", at(0, 11, 55, 31), "", 267, "$27.15", "", ""},
		"d28c": {"side", status.Asking, at(0, 12, 2, 10), "", 445, "$48.20", "work", "$1.82"},
		"7f3a": {"side", status.Answering, at(0, 14, 12, 30), "", 84, "$8.94", "work", "$0.98"},
		"db8a": {"side", "", at(-1, 15, 8, 0), "", 126, "$11.37", "work", "$0.66"},
		"5b19": {"personal", "", at(0, 10, 2, 18), "11:21", 38, "$3.12", "", ""},
		"3d7a": {"work", "", at(0, 9, 40, 5), "10:15", 12, "$0.84", "", ""},
		"e0c4": {"work", "", at(0, 9, 12, 40), "10:02", 19, "$1.47", "", ""},
		"a8f1": {"side", "", at(0, 8, 31, 2), "09:28", 24, "$1.95", "", ""},
		"2b6d": {"work", "", at(0, 7, 58, 30), "09:05", 31, "$2.61", "", ""},
		"5e7d": {"side", "", at(-1, 23, 47, 30), "00:41", 17, "$1.28", "", ""},
		"4b0e": {"work", "", at(-1, 15, 52, 14), "00:31", 5, "$0.31", "", ""},
		"8a2c": {"side", "", at(-1, 16, 55, 20), "00:18", 9, "$0.63", "", ""},
	}
	list := views.ListSessions(views.SessionSources{Running: listedSessions(now), Ledger: autoWorld().ledger(), Prices: drawnPrices, Now: now})
	if len(list.Sessions) != 14 || list.Today.Sessions != 14 {
		t.Errorf("the List has %d sessions, its today line %d, want 14", len(list.Sessions), list.Today.Sessions)
	}
	running := 0
	for _, s := range list.Sessions {
		if s.Running {
			running++
		}
		id := short(s.Session)
		if w, ok := want[id]; ok && listedOf(s, now) != w {
			t.Errorf("the List has %s as %+v, want %+v", id, listedOf(s, now), w)
		}
	}
	if running != 6 {
		t.Errorf("the List has %d sessions running, want 6", running)
	}
}

// The router tells each session started as its first answer comes, 0.7s
// after its first request, and each move of a session's own conversation as
// the request that moved it came: the lines of the auto world's records are
// the ones its events tell of.
func TestTheRoutersEventsTellOfTheSessionsLines(t *testing.T) {
	now := localMoment()
	held, told := autoWorld().records.lines, autoWorld().records.events
	lines := make([]ledger.Line, len(held))
	for i, h := range held {
		lines[i] = h.Line
	}
	for _, e := range told {
		switch e.Kind {
		case status.EventStarted:
			i := slices.IndexFunc(lines, func(l ledger.Line) bool { return l.Session == e.Session })
			if i < 0 || !lines[i].At.Add(startedAfter).Equal(e.At) || lines[i].Account != e.Account {
				t.Errorf("%s started on %s at %v, its first line %+v", short(e.Session), e.Account, e.At, lines[max(i, 0)])
			}
		case status.EventMoved:
			moved := slices.ContainsFunc(lines, func(l ledger.Line) bool {
				return l.Session == e.Session && l.Model == e.Model && l.At.Equal(e.At) && l.From == e.From && l.Account == e.To && l.Reason == e.Reason
			})
			if !moved {
				t.Errorf("%s moved %s → %s at %v, for %q, with no line of its", short(e.Session), e.From, e.To, e.At, e.Reason)
			}
		}
	}
	for _, l := range lines {
		moved := slices.ContainsFunc(told, func(e events.Line) bool {
			return e.Kind == status.EventMoved && e.Session == l.Session && e.At.Equal(l.At)
		})
		if l.From != "" && l.Class == classMain && l.At.After(routerRun(now)) && !moved {
			t.Errorf("%s's line of %v moved it %s → %s, with no event of it", short(l.Session), l.At, l.From, l.Account)
		}
	}
}
