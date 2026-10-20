package watch

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// refilterBack is how many days back api ran in the world the refilter's
// tests read, its one line refilterLine: the find's pick of it lands there,
// and the ledger keeps no day before it.
const (
	refilterBack = 4
	refilterLine = "0b000004"
)

// refilterLog is h showing the Log, started, following, over a world
// written to the state directory given: web's 60 lines today, filling the
// view, and 20 on each of the six days before, by turns on work and side;
// api's line refilterBack days back, an hour after web's first that day; a
// pin of work's each day before today, half an hour after web's first; and
// the lines and events given beside them. The ledger keeps its lines
// refilterBack days, as daysCounted counts its reads.
func refilterLog(t *testing.T, state string, beside []ledger.Held, filed ...events.Line) (*harness, *daysCounted) {
	t.Helper()
	return refilterLogKeeping(t, state, refilterBack*day, beside, filed...)
}

// refilterLogKeeping is refilterLog, the ledger keeping its lines as long
// as keep says.
func refilterLogKeeping(t *testing.T, state string, keep time.Duration, beside []ledger.Held, filed ...events.Line) (*harness, *daysCounted) {
	t.Helper()
	writeRefilterWorld(t, state, beside, filed...)
	h := newHarness(t, probedWithoutTheRouter())
	return h, refilterLogOn(h, state, keep)
}

// refilterLogOn has h show the Log of the world written to the state
// directory given, the ledger keeping its lines as long as keep says, and
// starts it.
func refilterLogOn(h *harness, state string, keep time.Duration) *daysCounted {
	h.t.Helper()
	l := onTheLog(h, state)
	h.model.cfg.Events = events.NewReader(state, h.clock.Now, logs.For("watch"))
	h.model.cfg.LedgerKeep = keep
	h.start()
	return l
}

// writeRefilterWorld writes refilterLog's world to the state directory
// given, the lines and events given beside it.
func writeRefilterWorld(t *testing.T, state string, beside []ledger.Held, filed ...events.Line) {
	t.Helper()
	var lines []ledger.Held
	for i := range 60 {
		lines = append(lines, ranIn(fmt.Sprintf("0c%06d", i), webSession, "~/Code/web", -time.Duration(70-i)*time.Minute))
	}
	for d := 1; d <= 6; d++ {
		for i := range 20 {
			l := ranIn(fmt.Sprintf("0d%02d%04d", d, i), webSession, "~/Code/web", -time.Duration(d)*day+time.Duration(i)*time.Minute)
			if i%2 == 1 {
				l.Account = "side"
			}
			lines = append(lines, l)
		}
		filed = append(filed, events.Line{ID: d, At: past(-time.Duration(d)*day + 30*time.Minute), Kind: status.EventPin, Accounts: []string{"work"}, Run: past(-40 * day)})
	}
	lines = append(lines, ranIn(refilterLine, apiSession, "~/Code/api", -refilterBack*day+time.Hour))
	writeLines(t, state, slices.Concat(lines, beside)...)
	writeEvents(t, state, filed...)
}

// filledAsOneRead are the requests a Log opened afresh on the world written
// to the state directory given shows, following, the ledger keeping its
// lines refilterBack days, once the filter given is applied, as it reads
// back to fill its view through it; and the day it spans from: those of
// one read whole, as its days are read through that filter alone.
func filledAsOneRead(t *testing.T, state string, filter views.LogFilter) ([]string, string) {
	t.Helper()
	h := newHarness(t, probedWithoutTheRouter())
	refilterLogOn(h, state, refilterBack*day)
	appliedLog(h, func(f *views.LogFilter) { *f = filter })
	return shownRequests(h), h.model.log.from.Date
}

// pickedAPI has the Log's find pick api, as / and enter do: it lands
// paused at api's line, every day back to it held through the find's
// filter, the days between ruled out by their summaries.
func pickedAPI(t testing.TB, h *harness) {
	t.Helper()
	h.typed("/", "a", "p", "i")
	h.keys(enter)
	p := h.model.log.page
	if row, _, chosen := p.Chosen(); !chosen || !p.Paused() || row.Request != refilterLine || h.model.log.read.Serves(views.LogFilter{}) {
		t.Fatalf("the find's pick of api is paused %t at %t a line, %s, its read serving every line %t; want it paused at api's, %s, its days back held through the find's filter",
			p.Paused(), chosen, row.Request, h.model.log.read.Serves(views.LogFilter{}), refilterLine)
	}
}

// applyingLog applies the Log's panel, as appliedLog does, but holds back
// the Log's reads of its own the change asks, as holdingReads does, and
// returns them undelivered.
func applyingLog(h *harness, tick func(*views.LogFilter)) []tea.Msg {
	h.t.Helper()
	h.keyed("f")
	tick(&h.model.log.panel.Filter)
	return h.holdingReads(h.update(enter)...)
}

// landedOne has the first of the Log's reads held land, delivering what it
// sends back but the Log's reads it asks, which it holds back beside the
// rest held, and returns them.
func landedOne(h *harness, held []tea.Msg) []tea.Msg {
	h.t.Helper()
	return append(held[1:], h.holdingReads(h.update(held[0])...)...)
}

// daysBack are the dates of the days the given numbers of days back.
func daysBack(back ...int) []string {
	dates := make([]string, len(back))
	for i, d := range back {
		dates[i] = dayfile.DateOf(past(-time.Duration(d) * day))
	}
	return dates
}

// The find's filter let go while the Log is paused on the line it landed
// on, as the panel lets api go: each day held through the filter is read
// again whole, a day a read, the newest first, the Requests showing the
// find's row alone, through the filter every day still serves, till every
// day serves every line; then every line from api's day on, paused still
// at api's line, as one read whole shows them. Though the ledger keeps the
// days before api's, and the find's row is atop the Requests, paused, which
// would read back the day before, no day is read back before every day
// serves every line, nor after, api's line no longer atop them. The panel,
// opened meanwhile, counts today's lines through every line from the first,
// as it does once every day serves it.
func TestAFilterLetGoWhilePausedReadsEachDayAgainThenShowsEveryLineAtItsLine(t *testing.T) {
	h, l := refilterLogKeeping(t, t.TempDir(), 6*day, nil)
	pickedAPI(t, h)
	before := len(l.days)

	reads := applyingLog(h, func(f *views.LogFilter) { f.Sessions = nil })
	h.keyed("f")
	counted := h.model.log.panel.Lines
	for n := 0; len(reads) > 0; n++ {
		if got := shownRequests(h); !slices.Equal(got, []string{refilterLine}) {
			t.Errorf("with %d days read again, the Requests show %v, want api's line alone till every day serves every line", n, got)
		}
		servedAsOneRead(t, h.model, l.Ledger, h.clock.now)
		reads = landedOne(h, reads)
	}
	if got, want := l.days[before:], daysBack(1, 2, 3, 4); !slices.Equal(got, want) || h.model.log.from.Date != want[3] {
		t.Errorf("the filter let go, the Log read again the days %v, spanning from %s; want %v, a day a read, the newest first, from api's day", got,
			h.model.log.from.Date, want)
	}
	if got := h.model.log.panel.Lines; !reflect.DeepEqual(got, counted) {
		t.Errorf("as the days were read again, the panel counted today's lines %+v, want %+v, as it does once every day serves its filter", counted, got)
	}
	p := h.model.log.page
	if row, _, chosen := p.Chosen(); !p.Paused() || !chosen || row.Request != refilterLine || len(shownRequests(h)) == 1 {
		t.Errorf("every day read again, the Log is paused %t at %t a line, %s, showing %d lines; want it paused at api's, %s, showing every line",
			p.Paused(), chosen, row.Request, len(shownRequests(h)), refilterLine)
	}
	sameLogAsOneRead(t, h.model, l.Ledger, h.clock.now)
}

// The Log's Requests are built through no filter the days its read holds
// don't serve: their lines never tabled, the read holding a day through
// api's filter, through none, though the page shows every line; once tabled
// through api's, through api's still, till every day serves every line.
func TestTheRequestsAreBuiltThroughNoFilterTheirDaysDontServe(t *testing.T) {
	h, l := refilterLog(t, t.TempDir(), nil)
	api := views.LogFilter{Sessions: []string{apiSession}}
	back := views.ReadDay(l, daysBack(refilterBack)[0], api)
	read, _ := views.ReadLog(ledger.Pricing).TakenBack(l, back, h.model.held, views.Stream{}, h.clock.now)
	tab := logTab{page: dashboard.LogPage{Filter: views.LogOpening()}, read: read}
	if f, ok := tab.requestsFilter(); ok {
		t.Errorf("never tabled, the Requests are built through %+v, which the day held through api's filter doesn't serve; want them built through none", f)
	}

	tab.lines = views.LogLinesOf(views.LogLines{}, views.LogSources{Read: read, Filter: api, Now: h.clock.now})
	tab.built.lines = &linesBasis{filter: api}
	if f, ok := tab.requestsFilter(); !ok || !f.Equal(api) {
		t.Errorf("tabled through api's filter, the Requests are built through %+v, %t; want api's, which every day serves", f, ok)
	}
}

// A filter widened while the Log follows lets go of the days its read holds
// that don't serve it, reading none of them again: the find's filter let
// go by esc, as it follows again, then clears it; or by the panel, as it
// follows. Every line shown is today's, as one read whole of today shows
// them, today's lines filling the view.
func TestAFilterWidenedWhileFollowingLetsGoOfTheDaysThatDontServeIt(t *testing.T) {
	tests := []struct {
		name string
		// widened widens the filter, returning how many reads of the Log's
		// own it made.
		widened func(h *harness) int
	}{
		{
			name: "esc after the find's pick, following again, then clearing the filter",
			widened: func(h *harness) int {
				return deliveredOf[logReadMsg](h, h.update(escape)...) + deliveredOf[logReadMsg](h, h.update(escape)...)
			},
		},
		{
			name: "the panel letting api go, following",
			widened: func(h *harness) int {
				h.keys(escape)
				return appliedLog(h, func(f *views.LogFilter) { f.Sessions = nil })
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, l := refilterLog(t, t.TempDir(), nil)
			pickedAPI(t, h)
			before := len(l.days)

			reads := tt.widened(h)
			m := h.model
			whole := m.log.page.Filter.Equal(views.LogOpening())
			if today := dayfile.DateOf(past(0)); reads > 0 || len(l.days) > before || m.log.from.Date != today || m.log.page.Paused() || !whole {
				t.Errorf("widened, the Log made %d reads, reading the days %v, spanning from %s, paused %t, its Requests through every line %t; "+
					"want no read, from today, %s, following, through every line", reads, l.days[before:], m.log.from.Date, m.log.page.Paused(), whole, today)
			}
			sameLogAsOneRead(t, m, l.Ledger, h.clock.now)
		})
	}
}

// A filter the days the Log's read holds don't serve, applied as it
// follows, or as it follows again while they're read again, has it let go
// of each such day, the Requests that filled their view from one reading
// back through their own filter to fill it again, as one built through a
// filter not theirs never marks it filled: so they show what one read whole
// shows, from the day it spans from, as filledAsOneRead has it, though today
// alone fills none of them but every line. The find's pick of api widened
// by docs', which ran two days back; crossed by side's; and let go while
// paused, the Log following again before a day is read again. web's, paused
// at the Requests' top, the day before read back through it, crossed by
// side's, the Log following again before the day is read again: built
// through web's meanwhile, today's lines fill the view.
func TestALogLettingGoOfItsDaysFillsItsViewAsOneReadWhole(t *testing.T) {
	tests := []struct {
		name string
		// changed changes the filter of the Log, following, as it opened.
		changed func(h *harness)
	}{
		{
			name: "api's pick widened by docs', following",
			changed: func(h *harness) {
				pickedAPI(h.t, h)
				h.keys(escape)
				appliedLog(h, func(f *views.LogFilter) { f.Sessions = []string{apiSession, docsSession} })
			},
		},
		{
			name: "api's pick crossed by side's, following",
			changed: func(h *harness) {
				pickedAPI(h.t, h)
				h.keys(escape)
				appliedLog(h, func(f *views.LogFilter) { f.Sessions, f.Accounts = nil, []string{"side"} })
			},
		},
		{
			name: "api's pick let go while paused, following again as the days are read again",
			changed: func(h *harness) {
				pickedAPI(h.t, h)
				reads := applyingLog(h, func(f *views.LogFilter) { f.Sessions = nil })
				h.keys(escape)
				h.deliver(reads...)
			},
		},
		{
			name: "web's crossed by side's while paused at their top, following again as the day is read again",
			changed: func(h *harness) {
				appliedLog(h, func(f *views.LogFilter) { f.Sessions = []string{webSession} })
				h.keys(tea.KeyPressMsg{Code: tea.KeyPgUp})
				reads := applyingLog(h, func(f *views.LogFilter) { f.Sessions, f.Accounts = nil, []string{"side"} })
				h.keys(escape)
				h.deliver(reads...)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			h, l := refilterLog(t, state, []ledger.Held{ranIn("0f000002", docsSession, "~/Code/docs", -2*day+3*time.Hour)})

			tt.changed(h)
			m := h.model
			want, from := filledAsOneRead(t, state, m.log.page.Filter)
			if got := shownRequests(h); !slices.Equal(got, want) || m.log.from.Date != from || m.log.page.Paused() {
				t.Errorf("the Log shows %d lines, from %s, paused %t; want the %d of one read whole, from %s, following", len(got), m.log.from.Date,
					m.log.page.Paused(), len(want), from)
			}
			sameLogAsOneRead(t, m, l.Ledger, h.clock.now)
		})
	}
}

// The Events shown, the Requests following through a filter their days are
// held through, the Log lets go of those days, reading none of them again,
// the Events filling their view from today; then the Requests shown again,
// they read back through their filter to fill theirs, as one read whole
// does, as filledAsOneRead has it.
func TestTheEventsShownLetGoOfTheDaysHeldThroughTheRequestsFilter(t *testing.T) {
	var pins []events.Line
	for i := range 60 {
		pins = append(pins, events.Line{ID: 100 + i, At: past(-time.Duration(70-i) * time.Minute), Kind: status.EventPin, Accounts: []string{"work"}, Run: past(-40 * day)})
	}
	state := t.TempDir()
	h, l := refilterLog(t, state, nil, pins...)
	appliedLog(h, func(f *views.LogFilter) { f.Sessions = []string{apiSession} })
	if unserved := h.model.log.read.Unserved(views.LogFilter{}); len(unserved) == 0 || h.model.log.page.Paused() {
		t.Fatalf("api's filter applied, the Log holds through it the days %v, paused %t; want it following, the days back held through api's", datesOf(unserved),
			h.model.log.page.Paused())
	}
	before := len(l.days)

	reads := deliveredOf[logReadMsg](h, h.press("]")...)
	if m, today := h.model, dayfile.DateOf(past(0)); reads > 0 || len(l.days) > before || m.log.from.Date != today || !m.log.read.Serves(views.LogFilter{}) {
		t.Errorf("the Events shown, the Log made %d reads, reading the days %v, spanning from %s, serving every line %t; want no read, from today, %s, "+
			"the days held through api's let go", reads, l.days[before:], m.log.from.Date, m.log.read.Serves(views.LogFilter{}), today)
	}

	h.deliver(h.press("[")...)
	want, from := filledAsOneRead(t, state, h.model.log.page.Filter)
	if got := shownRequests(h); !slices.Equal(got, want) || h.model.log.from.Date != from {
		t.Errorf("the Requests shown again, they show %v, from %s; want %v, from %s, as one read whole", got, h.model.log.from.Date, want, from)
	}
	sameLogAsOneRead(t, h.model, l.Ledger, h.clock.now)
}

// A filter narrowed, or of other kinds, every day the Log's read holds
// serving it, is shown at once, the Requests built through it as it's
// applied, no day read: an account of api's, api's failed alone, or every
// kind of line, as kinds choose little.
func TestAFilterNarrowedOrOfOtherKindsIsShownAtOnceReadingNoDay(t *testing.T) {
	tests := []struct {
		name string
		tick func(*views.LogFilter)
	}{
		{name: "an account of api's", tick: func(f *views.LogFilter) { f.Accounts = []string{"work"} }},
		{name: "api's failed alone", tick: func(f *views.LogFilter) { f.Failed = true }},
		{name: "every kind", tick: func(f *views.LogFilter) { f.Kinds = views.RowKinds() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, l := refilterLog(t, t.TempDir(), nil)
			pickedAPI(t, h)
			before := len(l.days)

			reads := applyingLog(h, tt.tick)
			m := h.model
			if tabled := m.log.lines.Filter(); len(reads) > 0 || len(l.days) > before || !tabled.Equal(m.log.page.Filter) {
				t.Errorf("narrowed, the Log asked %d reads, reading the days %v, its Requests tabled through %+v; want none, through %+v at once",
					len(reads), l.days[before:], tabled, m.log.page.Filter)
			}
			sameLogAsOneRead(t, m, l.Ledger, h.clock.now)
		})
	}
}

// Filters applied while a day is read again, the find's filter let go
// while paused, change nothing of the read under way: what it found is
// taken in as it lands, and the filter shown then judged afresh. Back to
// api's, every day serves it, and nothing more is read; on to web's, each
// day still held through api's is read again through the two joined. At
// each read landed the Requests are those of one read whole through the
// filter they're tabled through, which every day serves.
func TestFiltersAppliedWhileADayIsReadAgainAreJudgedAsItLands(t *testing.T) {
	api := func(f *views.LogFilter) { f.Sessions = []string{apiSession} }
	web := func(f *views.LogFilter) { f.Sessions = []string{webSession} }
	tests := []struct {
		name   string
		during []func(*views.LogFilter)
		// read are the days back read again, in turn.
		read []int
	}{
		{name: "web's, then api's again", during: []func(*views.LogFilter){web, api}, read: []int{1}},
		{name: "api's again, then web's", during: []func(*views.LogFilter){api, web}, read: []int{1, 2, 3, 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, l := refilterLog(t, t.TempDir(), nil)
			pickedAPI(t, h)
			before := len(l.days)

			reads := applyingLog(h, func(f *views.LogFilter) { f.Sessions = nil })
			for _, tick := range tt.during {
				if more := applyingLog(h, tick); len(more) > 0 {
					t.Errorf("a filter applied while a day is read again asked %d reads more, want none till it lands", len(more))
				}
				servedAsOneRead(t, h.model, l.Ledger, h.clock.now)
			}
			for len(reads) > 0 {
				reads = landedOne(h, reads)
				servedAsOneRead(t, h.model, l.Ledger, h.clock.now)
			}
			if got, want := l.days[before:], daysBack(tt.read...); !slices.Equal(got, want) {
				t.Errorf("the Log read again the days %v, want %v", got, want)
			}
			sameLogAsOneRead(t, h.model, l.Ledger, h.clock.now)
		})
	}
}

// A filter crossing the one the days are held through, a session's then an
// account's, has each day read again through the two joined, never
// narrowed to the account's: here every line, as the session's ticks no
// account and the account's no session. So the session's filter applied
// again is served at once, no day read.
func TestACrossingFilterReadsEachDayAgainThroughBothJoined(t *testing.T) {
	h, l := refilterLog(t, t.TempDir(), nil)
	pickedAPI(t, h)
	before := len(l.days)

	reads := applyingLog(h, func(f *views.LogFilter) { f.Sessions, f.Accounts = nil, []string{"side"} })
	for len(reads) > 0 {
		servedAsOneRead(t, h.model, l.Ledger, h.clock.now)
		reads = landedOne(h, reads)
	}
	if got, want := l.days[before:], daysBack(1, 2, 3, 4); !slices.Equal(got, want) || !h.model.log.read.Serves(views.LogFilter{}) {
		t.Errorf("crossed by side's filter, the Log read again the days %v, serving every line %t; want %v, each held whole, api's and side's joined",
			got, h.model.log.read.Serves(views.LogFilter{}), want)
	}
	sameLogAsOneRead(t, h.model, l.Ledger, h.clock.now)

	again := len(l.days)
	if reads := appliedLog(h, func(f *views.LogFilter) { f.Sessions, f.Accounts = []string{apiSession}, nil }); reads > 0 || len(l.days) > again {
		t.Errorf("api's filter applied again, the Log made %d reads, reading the days %v; want none, its days served", reads, l.days[again:])
	}
	sameLogAsOneRead(t, h.model, l.Ledger, h.clock.now)
}

// The find picking again, the days back held through api's pick's filter,
// lets go of every such day first, reading none of them again, as a pick
// afresh reads: web, running today, has the Log follow from today, reading
// nothing; docs, which ran last two days back, has the days back to it read
// through its filter in one read, landing paused at docs' line.
func TestPickingAgainLetsGoOfThePreviousPicksDaysFirst(t *testing.T) {
	docs := ranIn("0f000002", docsSession, "~/Code/docs", -2*day+3*time.Hour)
	tests := []struct {
		name  string
		typed []string
		// reads are how many reads of the Log's own the pick makes, read the
		// days back whose lines they read, from the day back the Log spans
		// from once they land, and at the line the pick pauses at, none where
		// it follows.
		reads int
		read  []int
		from  int
		at    string
	}{
		{name: "web, running today", typed: []string{"w", "e", "b"}},
		{name: "docs, ended two days back", typed: []string{"d", "o", "c", "s"}, reads: 1, read: []int{1, 2}, from: 2, at: docs.Request},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, l := refilterLog(t, t.TempDir(), []ledger.Held{docs})
			pickedAPI(t, h)
			before := len(l.days)
			h.typed(append([]string{"/"}, tt.typed...)...)

			reads := deliveredOf[logReadMsg](h, h.update(enter)...)
			if got, want := l.days[before:], daysBack(tt.read...); reads != tt.reads || !slices.Equal(got, want) || h.model.log.from.Date != daysBack(tt.from)[0] {
				t.Errorf("picked, the Log made %d reads, reading the days %v, spanning from %s; want %d, reading %v, from %s", reads, got, h.model.log.from.Date,
					tt.reads, want, daysBack(tt.from)[0])
			}
			p := h.model.log.page
			if row, _, _ := p.Chosen(); p.Paused() != (tt.at != "") || row.Request != tt.at {
				t.Errorf("picked, the Log is paused %t at %q; want it at %q", p.Paused(), row.Request, tt.at)
			}
			sameLogAsOneRead(t, h.model, l.Ledger, h.clock.now)
		})
	}
}

// The find's pick lands only on a table built through its filter: docs
// picked as a read of the tail is under way, so the days back held through
// api's pick's filter aren't let go of, then the Requests paused and a page
// asked of the line chosen, which waits on the tail; as the tail's read
// lands, the page's is asked, so no read for the Log is: docs' pick is yet
// to land, the days not serving its filter.
func TestThePickLandsOnlyOnceEveryDayServesIt(t *testing.T) {
	h, _ := refilterLog(t, t.TempDir(), []ledger.Held{ranIn("0f000002", docsSession, "~/Code/docs", -2*day+3*time.Hour)})
	pickedAPI(t, h)
	tail := holdingBack[tailedMsg](h, h.press("R")...)
	h.typed("/", "d", "o", "c", "s")
	h.keys(enter, upKey)
	h.typed("s")
	if len(tail) != 1 || h.model.pending == "" || h.model.log.read.Serves(h.model.log.page.Filter) {
		t.Fatalf("docs picked as the tail is read, a page asked of the line chosen, %d reads of the tail are under way, the page asked %q, the days serving docs' %t; "+
			"want one, the page waiting on it, the days held through api's", len(tail), h.model.pending, h.model.log.read.Serves(h.model.log.page.Filter))
	}

	page := holdingBack[sessionReadMsg](h, h.update(tail[0])...)
	if m := h.model; len(page) != 1 || m.log.reading || m.log.find.landing == nil {
		t.Errorf("the tail's read landed, %d reads of the page are under way, the Log reading %t, docs' pick landed %t; want the page's read alone, "+
			"docs' pick yet to land, the days not serving its filter", len(page), m.log.reading, m.log.find.landing == nil)
	}
}

// A day read again is dropped where the Log's read is no longer the one it
// read from as it lands, whatever changed it, and read again of the read
// held, so what changed it is never undone: a line a read of the tail took
// in, kept; or the day let go of, never held again. One of a day the read
// it read from held none of, as Refiltered refuses, changes nothing.
func TestADayReadAgainIsDroppedWhereTheLogsReadChangedMeanwhile(t *testing.T) {
	late := ranIn("0e000001", webSession, "~/Code/web", -time.Minute)
	tests := []struct {
		name string
		// meanwhile changes the Log as the read held lands, returning that
		// read or another in its place.
		meanwhile func(t *testing.T, h *harness, state string, held tea.Msg) tea.Msg
		// holds reports whether the Log of m holds still what meanwhile
		// changed.
		holds func(m Model) bool
	}{
		{
			name: "a line taken in by a read of the tail",
			meanwhile: func(t *testing.T, h *harness, state string, held tea.Msg) tea.Msg {
				writeLines(t, state, late)
				taking := views.LogTaking{Held: h.model.held, Now: h.clock.now}
				h.model.log.read = h.model.log.read.Taken(views.RecordsOf([]ledger.Held{late}), false, taking)
				return held
			},
			holds: func(m Model) bool { return slices.Contains(shownRequestsOf(m), late.Request) },
		},
		{
			name: "the day let go of",
			meanwhile: func(_ *testing.T, h *harness, _ string, held tea.Msg) tea.Msg {
				h.model.log = h.model.log.spanningFrom(views.Today(past(0)))
				return held
			},
			holds: func(m Model) bool {
				return m.log.from.Date == dayfile.DateOf(past(0)) && len(daysHeld(m.log, m.log.from.Date)) == 0
			},
		},
		{
			name: "a day the read doesn't hold",
			meanwhile: func(_ *testing.T, h *harness, _ string, _ tea.Msg) tea.Msg {
				r := logRefilter{readers: h.model.logReaders(), read: h.model.log.read, held: h.model.held, now: h.clock.now}
				return r.of(daysBack(refilterBack + 1)[0])
			},
			holds: func(m Model) bool { return m.log.from.Date == daysBack(refilterBack)[0] },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			h, l := refilterLog(t, state, nil)
			pickedAPI(t, h)
			held := applyingLog(h, func(f *views.LogFilter) { f.Sessions = nil })
			if len(held) != 1 {
				t.Fatalf("the filter let go while paused, the Log asked %d reads, want the newest day read again alone", len(held))
			}

			msg := tt.meanwhile(t, h, state, held[0])
			was := h.model.log.read
			next := h.update(msg)
			if !h.model.log.read.Same(was) {
				t.Error("the day read again was taken in, though the Log's read changed after it was asked")
			}
			h.deliver(next...)
			if m := h.model; !tt.holds(m) || m.log.reading || !m.log.read.Serves(views.LogFilter{}) {
				t.Errorf("read again of its read held, the Log holds what changed it %t, is reading %t, serving every line %t; want it held, every day served, no read under way",
					tt.holds(m), m.log.reading, m.log.read.Serves(views.LogFilter{}))
			}
			servedAsOneRead(t, h.model, l.Ledger, h.clock.now)
		})
	}
}

// The Events shown once the Requests have read back through the failed
// alone, paused at their top: each day held through it is to them a day not
// yet read, as to the cards of their events, so both start at the day after
// the newest such day; each is read again whole, a day a read, the newest
// first, the Events and their cards extending back as it lands; and once
// every one is, they're those of every day held whole. A limit of work's
// on the day before, read whole as the Requests paused, is known from the
// 429 work answered the day before it, a day held through the filter: its
// card tells nothing of it till that day's read again.
func TestTheEventsShowTheDaysHeldWholeAloneTillEachIsReadAgain(t *testing.T) {
	state := t.TempDir()
	answered := ranIn("0e000429", webSession, "~/Code/web", -2*day+2*time.Hour)
	answered.Status = 429
	limit := events.Line{ID: 9, At: past(-day - time.Hour), Kind: status.EventLimit, Account: "work", Run: past(-40 * day)}
	h, l := refilterLog(t, state, []ledger.Held{answered}, limit)
	h.keys(tea.KeyPressMsg{Code: tea.KeyPgUp})
	appliedLog(h, func(f *views.LogFilter) { f.Failed = true })
	if from, unserved := h.model.log.from.Date, h.model.log.read.Unserved(views.LogFilter{}); from != daysBack(4)[0] || !slices.Equal(datesOf(unserved), daysBack(2, 3, 4)) {
		t.Fatalf("paused, through the failed alone, the Log spans from %s, holding through it %v; want from %s, holding the day before whole, %v through it",
			from, datesOf(unserved), daysBack(4)[0], daysBack(2, 3, 4))
	}
	before := len(l.days)

	reads := h.holdingReads(h.press("]")...)
	h.keys(upKey, upKey)
	reads = append(reads, h.holdingReads(h.update(enter)...)...)
	steps := []struct {
		pins  []int
		known string
	}{
		{pins: []int{1}},
		{pins: []int{2, 1}, known: answered.Request},
		{pins: []int{3, 2, 1}, known: answered.Request},
		{pins: []int{4, 3, 2, 1}, known: answered.Request},
	}
	for i, step := range steps {
		m := h.model
		var pins []int
		for _, e := range m.log.page.Events.Rows {
			if e.Kind == status.EventPin {
				pins = append(pins, e.Key.ID)
			}
		}
		on, known := 0, ""
		if card := m.log.card.event; card != nil {
			on = card.Row.Key.ID
			if card.KnownFrom != nil {
				known = card.KnownFrom.Request
			}
		}
		if !slices.Equal(pins, step.pins) || on != limit.ID || known != step.known {
			t.Errorf("with %d days read again, the Events show the pins %v, the card open on the event %d, known from %q; want the pins %v, the card open on the limit, %d, known from %q",
				i, pins, on, known, step.pins, limit.ID, step.known)
		}
		if i < len(steps)-1 {
			if len(reads) == 0 {
				t.Fatalf("with %d days read again, no read is under way, want the next day read again", i)
			}
			reads = landedOne(h, reads)
		}
	}
	if len(reads) > 0 {
		t.Errorf("every day held through the filter read again, %d reads more are under way, want none", len(reads))
	}
	if got, want := l.days[before:], daysBack(2, 3, 4); !slices.Equal(got, want) {
		t.Errorf("the Events shown, the Log read again the days %v, want %v", got, want)
	}
	m := h.model
	src := m.logEventSources(m.log.page.Shows, h.clock.now)
	src.Read = views.LogRead{}
	if whole := views.LogEvents(src); !reflect.DeepEqual(m.log.page.Events.Lines, whole.Lines) {
		t.Errorf("every day read again, the Events show %d lines, want the %d of every day held whole", len(m.log.page.Events.Lines), len(whole.Lines))
	}
	if m.log.page.Shown != dashboard.LogEvents || !m.log.page.Stands[dashboard.LogRequests].Paused() {
		t.Errorf("the Events shown %t, the Requests paused %t; want the Events shown over the Requests paused", m.log.page.Shown == dashboard.LogEvents,
			m.log.page.Stands[dashboard.LogRequests].Paused())
	}
}

// A day read again where the stream tells of a request whose line may be
// filed on it is read whole, whatever the filter served keeps, as ThroughOn
// has it, so the request's row is told of its line: the find's pick of api
// holds the day before through its filter, which hides the request web ran
// then, whose end the stream has told of since; widened by web's while
// paused, the day is read again whole, the request's row shown with its
// line, placed where it was sent.
func TestADayReadAgainIsReadWholeWhereTheStreamTellsOfALineFiledOnIt(t *testing.T) {
	state := t.TempDir()
	ran := took(ranIn("0e000001", webSession, "~/Code/web", -day+25*time.Minute), day-25*time.Minute)
	writeRefilterWorld(t, state, []ledger.Held{ran})
	h := routedHarness(t, routerDocument(three()...))
	l := refilterLogOn(h, state, refilterBack*day)
	pickedAPI(t, h)
	sent := alter(told(router.StreamSent, ran.Request, "work", 0), func(e *router.StreamEvent) { e.At, e.Session = ran.At.Add(time.Second).UTC(), webSession })
	h.hear(sent, alter(sent, func(e *router.StreamEvent) { e.Kind, e.At, e.Status = router.StreamDone, past(0), 200 }))
	yesterday := daysBack(1)[0]
	if unserved := datesOf(h.model.log.read.Unserved(views.LogFilter{})); !slices.Contains(unserved, yesterday) {
		t.Fatalf("the stream told of web's request, the Log holds through api's filter the days %v; want the day before, %s, among them", unserved, yesterday)
	}

	h.deliver(applyingLog(h, func(f *views.LogFilter) { f.Sessions = []string{apiSession, webSession} })...)
	if unserved := datesOf(h.model.log.read.Unserved(views.LogFilter{})); slices.Contains(unserved, yesterday) {
		t.Errorf("widened by web's, the Log holds through a filter the days %v; want the day before, %s, held whole", unserved, yesterday)
	}
	var rows []views.RequestRow
	for _, line := range h.model.log.page.Requests.All() {
		if line.IsRow() && line.Row.Request == ran.Request {
			rows = append(rows, line.Row)
		}
	}
	if len(rows) != 1 || !rows[0].At.Equal(ran.At) {
		t.Errorf("widened by web's, the Requests show %d rows of web's request, %+v; want one, told of its line, arrived at %s", len(rows), rows, ran.At)
	}
	servedAsOneRead(t, h.model, l.Ledger, h.clock.now)
}

// datesOf are the dates of the days given.
func datesOf(days []views.LedgerDay) []string {
	dates := make([]string, len(days))
	for i, d := range days {
		dates[i] = d.Date
	}
	return dates
}
