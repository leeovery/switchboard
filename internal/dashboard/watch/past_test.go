package watch

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/views"
)

// daysLedger reads as a request ledger holding the days from first on to
// today, by the test's clock, each as days holds it by its date, else of no
// requests, and tells that it begins on first; and notes the day each read
// of its days was from.
type daysLedger struct {
	ledger.Empty
	clock *fakeClock
	first string
	days  map[string]ledger.Summary
	reads []time.Time
}

func (l *daysLedger) Days(from time.Time) []ledger.Summary {
	l.reads = append(l.reads, from)
	var days []ledger.Summary
	for _, date := range dayfile.Span(from, l.clock.Now()) {
		if date < l.first {
			continue
		}
		d, ok := l.days[date]
		if !ok {
			d = ledger.Summary{Day: date}
		}
		days = append(days, d)
	}
	return days
}

func (l *daysLedger) FirstDay(time.Time) (time.Time, error) {
	start, _, _ := dayfile.Day(l.first)
	return start, nil
}

// requested is a day of n requests of Opus 5.5 on work, in one session.
func requested(date string, n int) ledger.Summary {
	return ledger.Summary{Day: date, Accounts: []ledger.AccountDay{{
		Account: "work", Sessions: 1, SessionIDs: []string{idD28C},
		Models: []ledger.ModelDay{{Model: "claude-opus-5-5", Upstream: n, Sessions: 1, SessionIDs: []string{idD28C}}},
	}}}
}

// threeConfigured are the router of three's accounts as configured, work
// labelled apart from its id.
var threeConfigured = config.Accounts{{ID: "work", Label: "Work"}, {ID: "personal"}, {ID: "side"}}

// pastHarness is a model of the router of three, configured as
// threeConfigured, weeks starting on Monday, reading the ledger it returns,
// which holds days from 2024, and opening on the tab given, in a terminal 160
// cells wide and 37 tall. It hasn't started.
func pastHarness(t *testing.T, opening dashboard.View) (*harness, *daysLedger) {
	t.Helper()
	h := &harness{t: t, clock: &fakeClock{now: start}, source: &fakeSource{doc: probedWithoutTheRouter()}, notifier: &fakeNotifier{}}
	doc := routerDocument(three()...)
	h.source.router = &doc
	l := &daysLedger{Empty: h.emptyLedger(), clock: h.clock, first: "2024-06-03", days: map[string]ledger.Summary{}}
	cfg := h.config(Size{Width: 160, Height: 37})
	cfg.Ledger, cfg.Accounts, cfg.WeekStarts, cfg.WeekWindow = l, threeConfigured, time.Monday, "7d"
	cfg.View = opening
	h.model = New(t.Context(), cfg)
	return h, l
}

// readingHarness is pastHarness opening on Trends, YEAR and BY MONTH shown,
// whose page the watch builds of the last 12 months' days it reads. It
// hasn't started.
func readingHarness(t *testing.T) (*harness, *daysLedger) {
	t.Helper()
	h, l := pastHarness(t, dashboard.Graphs)
	h.model.graphs = dashboard.GraphsSectionsNamed([]string{"year", "months"})
	return h, l
}

// lookAt moves the clock to at, as after a sleep, and fires the tick that
// was pending, which looks at the router's document there.
func (h *harness) lookAt(at time.Time) {
	h.t.Helper()
	pending, asked := h.lastTick(), len(h.source.asked)
	h.clock.now, pending.fired = at, true
	h.deliver(h.update(pending.msg)...)
	if len(h.source.asked) == asked {
		h.t.Fatalf("the tick at %s asked the router for nothing", at)
	}
}

// todayStart and yearFrom are when today starts, as the clock starts, and
// the first day read for the last 12 months then, as DaysFrom has it,
// weeks starting on Monday; and yesterday is the date of the day before.
var (
	todayStart = dayfile.DayStart(start.Local(), 0)
	yearFrom   = views.DaysFrom(views.YearAsk{}, start, time.Monday)
	yesterday  = dayfile.DateOf(dayfile.DayStart(start.Local(), -1))
)

// The last 12 months are read once, from their first day read, as DaysFrom
// has it, then at each look today alone, from its start.
func TestTheYearIsReadOnceThenTodayAloneAtEachLook(t *testing.T) {
	h, l := readingHarness(t)
	h.start()
	if !slices.EqualFunc(l.reads, []time.Time{yearFrom}, time.Time.Equal) || h.model.trends.year == nil {
		t.Fatalf("Trends opened, the ledger was read from %v, YEAR %v; want the year read once, from %s, and built", l.reads, h.model.trends.year, yearFrom)
	}
	for range 3 {
		h.tickUntilAsked()
	}
	if want := []time.Time{yearFrom, todayStart, todayStart, todayStart}; !slices.EqualFunc(l.reads, want, time.Time.Equal) {
		t.Errorf("after three looks, the ledger was read from %v, want %v: today alone at each", l.reads, want)
	}
}

// While nothing drawn of the ledger's days shows, none is read. Accounts
// shown, the year is read, the once; and shown again, the days since a look
// while it wasn't.
func TestTheDaysAreReadForNothingThatIsntShown(t *testing.T) {
	h, l := pastHarness(t, dashboard.Overview)
	h.start()
	h.tickUntilAsked()
	if len(l.reads) > 0 {
		t.Fatalf("on the Overview, the ledger's days were read from %v, want never", l.reads)
	}
	h.keys(tabKey)
	if h.model.view != dashboard.Accounts || !slices.EqualFunc(l.reads, []time.Time{yearFrom}, time.Time.Equal) {
		t.Fatalf("on %s, the ledger was read from %v, want Accounts, the year read from %s", h.model.view, l.reads, yearFrom)
	}

	h.keys(shiftTabKey)
	h.tickUntilAsked()
	if len(l.reads) != 1 {
		t.Errorf("on %s, the ledger was read from %v, want nothing more", h.model.view, l.reads)
	}
	h.keys(tabKey)
	if want := []time.Time{yearFrom, todayStart}; !slices.EqualFunc(l.reads, want, time.Time.Equal) {
		t.Errorf("Accounts shown again, the ledger was read from %v, want %v: today's, for the look while it wasn't", l.reads, want)
	}
}

// The first look on a new day reads from the start of the day before it,
// whose end the last look didn't see, and YEAR moves on to the new day.
func TestTheFirstLookOnANewDayReadsTheDayBeforeItToo(t *testing.T) {
	h, l := readingHarness(t)
	h.start()
	tomorrow := dayfile.DayStart(start.Local(), 1)
	l.days[dayfile.DateOf(tomorrow)] = requested(dayfile.DateOf(tomorrow), 4)

	h.lookAt(tomorrow.Add(time.Minute))
	h.lookAt(tomorrow.Add(time.Hour))
	if want := []time.Time{yearFrom, todayStart, tomorrow}; !slices.EqualFunc(l.reads, want, time.Time.Equal) {
		t.Errorf("past midnight, the ledger was read from %v, want %v", l.reads, want)
	}
	if got, ok := gridDay(*h.model.trends.year, dayfile.DateOf(tomorrow)); !ok || got.LeftOut || got.Requests != 4 {
		t.Errorf("the new day is %+v on YEAR, want its 4 requests drawn", got)
	}
}

// gridDay is the day with the given date of the Year's grid, reporting false
// where it has none.
func gridDay(page views.YearPage, date string) (views.GridDay, bool) {
	for _, w := range page.Weeks {
		if i := slices.IndexFunc(w.Days, func(d views.GridDay) bool { return d.Day == date }); i >= 0 {
			return w.Days[i], true
		}
	}
	return views.GridDay{}, false
}

// begunOn reads as the ledger it holds, but tells that it began on the day
// with the date given, or fails to with the error given.
type begunOn struct {
	*daysLedger
	began string
	err   error
}

func (l begunOn) FirstDay(time.Time) (time.Time, error) {
	if l.err != nil {
		return time.Time{}, l.err
	}
	start, _, _ := dayfile.Day(l.began)
	return start, nil
}

// YEAR takes the day the ledger began as the ledger tells it, never the
// first day it holds: of a ledger begun on 15 September that holds nothing
// before the 20th, BY MONTH's September is from the 15th.
func TestTheYearTakesTheDayTheLedgerBeganAsTheLedgerTellsIt(t *testing.T) {
	h, l := readingHarness(t)
	l.first = "2026-09-20"
	l.days["2026-09-20"] = requested("2026-09-20", 10)
	h.model.cfg.Ledger = begunOn{daysLedger: l, began: "2026-09-15"}
	h.start()
	months := h.model.trends.year.ByMonth
	if i := slices.IndexFunc(months, func(m views.MonthRow) bool { return m.Month.Month == "2026-09" }); i < 0 || months[i].From != "2026-09-15" {
		t.Errorf("BY MONTH is %+v, want September from the 15th", months)
	}
}

// freshLedger reads as the ledger it holds, but tells that it begins on the
// first day it holds requests on, failing as one that holds nothing does
// till then.
type freshLedger struct {
	*daysLedger
}

func (l freshLedger) FirstDay(time.Time) (time.Time, error) {
	if len(l.days) == 0 {
		return time.Time{}, fs.ErrNotExist
	}
	start, _, _ := dayfile.Day(slices.Min(slices.Collect(maps.Keys(l.days))))
	return start, nil
}

// Trends opened on an empty ledger, as on a fresh install, takes it to begin
// today, as a read of its days does, so YEAR and Accounts price the plans
// from today, as history does, not over the 12 months or the week; and,
// asking again while the ledger holds no day, learns where it began from the
// read that first finds a request recorded, the day after: BY MONTH's month
// says it's from that day, and the plans are priced from it.
func TestTrendsOpenedOnAnEmptyLedgerBeginsTodayTillARequestLands(t *testing.T) {
	h, l := readingHarness(t)
	today := dayfile.DateOf(start)
	l.first = today
	h.model.cfg.Ledger = freshLedger{l}
	h.model.cfg.Accounts = config.Accounts{{ID: "work", Plan: "ultra"}, {ID: "personal", Plan: "max"}}
	h.start()
	if h.model.trends.year == nil || !h.model.past.began.Equal(todayStart) {
		t.Fatalf("Trends opened on an empty ledger, YEAR built %v, the ledger begun %s; want it built, begun today",
			h.model.trends.year != nil, h.model.past.began)
	}
	pricedFromItsBeginning(t, h, h.clock.now, todayStart, []ledger.Summary{{Day: today}})

	tomorrowStart := dayfile.DayStart(start.Local(), 1)
	tomorrow := dayfile.DateOf(tomorrowStart)
	l.first, l.days[tomorrow] = tomorrow, requested(tomorrow, 10)
	h.lastTick().due = tomorrowStart.Add(time.Hour)
	now := h.tickUntilAsked()
	if !h.model.past.began.Equal(tomorrowStart) {
		t.Fatalf("once a request lands the day after, the ledger is taken to begin %s, want %s", h.model.past.began, tomorrowStart)
	}
	months := h.model.trends.year.ByMonth
	if i := slices.IndexFunc(months, func(m views.MonthRow) bool { return m.Month.Month == tomorrow[:7] }); i < 0 || months[i].From != tomorrow {
		t.Errorf("BY MONTH is %+v, want %s from the %s", months, tomorrow[:7], tomorrow)
	}
	pricedFromItsBeginning(t, h, now, tomorrowStart, []ledger.Summary{requested(tomorrow, 10)})
}

// pricedFromItsBeginning fails the test where YEAR's numbers and Accounts'
// this week the watch holds don't price the plans as the days given, read at
// now, do of a ledger begun at began.
func pricedFromItsBeginning(t *testing.T, h *harness, now, began time.Time, days []ledger.Summary) {
	t.Helper()
	in := views.HistoryInput{
		From: yearFrom, Now: now, Began: began, Prices: ledger.Pricing, Accounts: h.model.cfg.Accounts,
		WeekStarts: time.Monday, WeekWindow: "7d",
	}
	past := views.NewPast(in, days)
	want := past.Year(views.YearAsk{}).Numbers
	if got := h.model.trends.year.Numbers; got.Cost == nil || want.Cost == nil || *got.Cost != *want.Cost {
		t.Errorf("YEAR's plans cost %v, want %v, priced from %s", deref(got.Cost), deref(want.Cost), began)
	}
	got, wantWeek := h.model.past.last.past.Accounts(views.PeriodThisWeek).All.Plan.Cost, past.Accounts(views.PeriodThisWeek).All.Plan.Cost
	if got == nil || wantWeek == nil || *got != *wantWeek {
		t.Errorf("Accounts' plans cost %v this week, want %v, priced from %s", deref(got), deref(wantWeek), began)
	}
}

// deref is what p points at, nil where it's nil.
func deref[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// A ledger that fails to say where it begins, but for holding nothing, has
// it logged, the once, though the looks after it ask again; and the year
// read as it would be.
func TestALedgerThatFailsToSayWhereItBeginsIsLogged(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		logged int
	}{
		{name: "failing", err: errors.New("permission denied"), logged: 1},
		{name: "holding nothing", err: fs.ErrNotExist},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			h, l := readingHarness(t)
			l.days[yesterday] = requested(yesterday, 10)
			h.model.cfg.Ledger = begunOn{daysLedger: l, err: tt.err}
			h.start()
			h.tickUntilAsked()
			h.tickUntilAsked()

			if y := h.model.trends.year; y == nil || y.Numbers.Requests != 10 {
				t.Errorf("YEAR is %v; want it built, its 10 requests drawn", y)
			}
			if got := beginningsUntold(log); got != tt.logged || tt.logged > 0 && !log.Has("level=WARN", "component=watch", `error="permission denied"`) {
				t.Errorf("the log reads\n%s\nwant %d warning that the ledger couldn't say where it begins", log, tt.logged)
			}
		})
	}
}

// beginningsUntold is how many times the log warns that the ledger couldn't
// say where it begins.
func beginningsUntold(log *logstest.Log) int {
	n := 0
	for _, line := range log.Lines() {
		if strings.Contains(line, `msg="couldn't read where the request ledger begins"`) {
			n++
		}
	}
	return n
}

// flakyStart reads as the ledger it holds, but fails to say where it begins
// the first fails times it's asked, as of too many files open, then tells it
// as the ledger it holds does; and counts the times it's asked.
type flakyStart struct {
	*daysLedger
	fails, asked int
}

func (l *flakyStart) FirstDay(now time.Time) (time.Time, error) {
	l.asked++
	if l.asked <= l.fails {
		return time.Time{}, errors.New("too many open files")
	}
	return l.daysLedger.FirstDay(now)
}

// A failure to say where the ledger begins is passing: each look after it
// asks again, till the ledger says, the failures logged the once; then no
// look's read asks again.
func TestALedgerThatFailsToSayWhereItBeginsIsAskedAgainTillItSays(t *testing.T) {
	log := logstest.Capture(t)
	h, l := readingHarness(t)
	flaky := &flakyStart{daysLedger: l, fails: 2}
	h.model.cfg.Ledger = flaky
	h.start()
	h.tickUntilAsked()
	if !h.model.past.began.IsZero() {
		t.Fatalf("the ledger failed to say where it began twice, but it's taken as %s", h.model.past.began)
	}

	h.tickUntilAsked()
	if began, _, _ := dayfile.Day(l.first); !h.model.past.began.Equal(began) || flaky.asked != 3 {
		t.Errorf("at the third read, the ledger is taken to begin %s, asked %d times; want %s, at each read", h.model.past.began, flaky.asked, began)
	}
	if got := beginningsUntold(log); got != 1 {
		t.Errorf("the log reads\n%s\nwant one warning that the ledger couldn't say where it begins, of two failures", log)
	}
	h.tickUntilAsked()
	if flaky.asked != 3 {
		t.Errorf("once the ledger said where it began, it was asked %d times more, want none", flaky.asked-3)
	}
}

// Learning where the ledger began brightens nothing a look's days didn't
// change: a look that learns it, of a ledger begun yesterday, and finds
// nothing new, prices the plans from yesterday, its worth against them
// rising, but brightens nothing.
func TestLearningWhereTheLedgerBeganBrightensNothing(t *testing.T) {
	h, l := readingHarness(t)
	l.first = yesterday
	day := requested(yesterday, 10)
	day.Accounts[0].Models[0].Usage = json.RawMessage(`{"input_tokens":10000000}`)
	l.days[yesterday] = day
	h.model.cfg.Ledger = &flakyStart{daysLedger: l, fails: 1}
	h.model.cfg.Accounts = config.Accounts{{ID: "work", Plan: "ultra"}, {ID: "personal", Plan: "max"}}
	h.start()
	against := dashboard.Number{Kind: dashboard.NumberYear, Of: "against it"}
	was := h.drawnNumber(against)
	h.tickUntilAsked()
	if is := h.drawnNumber(against); h.model.past.began.IsZero() || is == was {
		t.Fatalf("the look learned the ledger began %s, the worth against the plans drawn %q, as before; want it learned, and the plans priced from it",
			h.model.past.began, is)
	}
	if got := h.brightened(); len(got) > 0 {
		t.Errorf("a look that learned where the ledger began, and found nothing new, brightened %v, want none", got)
	}
}

// A look that changes today's requests brightens YEAR's figures it changes,
// drawn before and after at the one moment, in a terminal wide enough for
// its numbers beside the grid, and BY MONTH's months in view.
func TestALookThatAddsRequestsTodayBrightensTheYearsFigures(t *testing.T) {
	h, l := readingHarness(t)
	today := dayfile.DateOf(start)
	l.days[yesterday] = requested(yesterday, 10)
	h.start()
	h.update(tea.WindowSizeMsg{Width: 173, Height: 53})
	h.tickUntilAsked()
	if got := h.brightened(); len(got) > 0 {
		t.Fatalf("a look that found nothing new brightened %v, want none", got)
	}

	l.days[today] = requested(today, 3)
	h.tickUntilAsked()
	requests := dashboard.Number{Kind: dashboard.NumberYear, Of: "requests"}
	month := dashboard.Number{Kind: dashboard.NumberYear, Of: today[:7] + " requests"}
	if got := h.brightened(); got[requests] != 1 || got[month] != 1 {
		t.Errorf("a look that found 3 requests today brightened %v, want the 12 months' requests and the month's at 1", got)
	}
}
