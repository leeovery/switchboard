package watch

import (
	"cmp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/theme"
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

// historyHarness is a model of the router of three, configured as
// threeConfigured, weeks starting on Monday, its History reading the ledger
// it returns, which holds days from 2024, and opening as kept, on the tab
// given, in a terminal 160 cells wide and 37 tall. It hasn't started.
func historyHarness(t *testing.T, opening dashboard.View, kept dashboard.HistoryPage) (*harness, *daysLedger) {
	t.Helper()
	h := &harness{t: t, clock: &fakeClock{now: start}, source: &fakeSource{doc: probedWithoutTheRouter()}, notifier: &fakeNotifier{}}
	doc := routerDocument(three()...)
	h.source.router = &doc
	l := &daysLedger{Empty: h.emptyLedger(), clock: h.clock, first: "2024-06-03", days: map[string]ledger.Summary{}}
	cfg := h.config(Size{Width: 160, Height: 37})
	cfg.Ledger, cfg.Prices, cfg.Accounts, cfg.WeekStarts, cfg.WeekWindow = l, ledger.Pricing, threeConfigured, time.Monday, "7d"
	cfg.View, cfg.History = opening, kept
	h.model = New(t.Context(), cfg)
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
// the first day of the Year of the last 12 months then, weeks starting on
// Monday; and yesterday is the date of the day before.
var (
	todayStart = dayfile.DayStart(start.Local(), 0)
	yearFrom   = views.YearFrom(views.YearAsk{}, start, time.Monday)
	yesterday  = dayfile.DateOf(dayfile.DayStart(start.Local(), -1))
)

// History reads the year once, from the Year's first day, as YearFrom has
// it, then at each look today alone, from its start.
func TestHistoryReadsTheYearOnceThenTodayAloneAtEachLook(t *testing.T) {
	h, l := historyHarness(t, dashboard.History, dashboard.HistoryPage{})
	h.start()
	if !slices.EqualFunc(l.reads, []time.Time{yearFrom}, time.Time.Equal) || h.model.past.page.Year == nil {
		t.Fatalf("History opened, the ledger was read from %v, the Year %v; want the year read once, from %s, and drawn", l.reads, h.model.past.page.Year, yearFrom)
	}
	for range 3 {
		h.tickUntilAsked()
	}
	if want := []time.Time{yearFrom, todayStart, todayStart, todayStart}; !slices.EqualFunc(l.reads, want, time.Time.Equal) {
		t.Errorf("after three looks, the ledger was read from %v, want %v: today alone at each", l.reads, want)
	}
}

// While History isn't shown, nothing of it is read. Shown, its year is
// read, the once; and shown again, the days since a look while it wasn't.
func TestHistoryReadsNothingWhileItIsntShown(t *testing.T) {
	h, l := historyHarness(t, dashboard.Overview, dashboard.HistoryPage{})
	h.start()
	h.tickUntilAsked()
	if len(l.reads) > 0 {
		t.Fatalf("on the Overview, the ledger was read for History from %v, want never", l.reads)
	}
	h.keys(tabKey, tabKey)
	if h.model.view != dashboard.History || !slices.EqualFunc(l.reads, []time.Time{yearFrom}, time.Time.Equal) {
		t.Fatalf("on %s, the ledger was read from %v, want History, its year read from %s", h.model.view, l.reads, yearFrom)
	}

	h.keys(tabKey)
	h.tickUntilAsked()
	if len(l.reads) != 1 {
		t.Errorf("on %s, the ledger was read from %v, want nothing more", h.model.view, l.reads)
	}
	h.keys(shiftTabKey)
	if want := []time.Time{yearFrom, todayStart}; !slices.EqualFunc(l.reads, want, time.Time.Equal) {
		t.Errorf("History shown again, the ledger was read from %v, want %v: today's, for the look while it wasn't", l.reads, want)
	}
}

// The first look on a new day reads from the start of the day before it,
// whose end the last look didn't see, and the Year moves on to the new day.
func TestTheFirstLookOnANewDayReadsTheDayBeforeItToo(t *testing.T) {
	h, l := historyHarness(t, dashboard.History, dashboard.HistoryPage{})
	h.start()
	tomorrow := dayfile.DayStart(start.Local(), 1)
	l.days[dayfile.DateOf(tomorrow)] = requested(dayfile.DateOf(tomorrow), 4)

	h.lookAt(tomorrow.Add(time.Minute))
	h.lookAt(tomorrow.Add(time.Hour))
	if want := []time.Time{yearFrom, todayStart, tomorrow}; !slices.EqualFunc(l.reads, want, time.Time.Equal) {
		t.Errorf("past midnight, the ledger was read from %v, want %v", l.reads, want)
	}
	if got, ok := gridDay(*h.model.past.page.Year, dayfile.DateOf(tomorrow)); !ok || got.LeftOut || got.Requests != 4 {
		t.Errorf("the new day is %+v on the Year, want its 4 requests drawn", got)
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

// < steps the Year back through the calendar years: this one, which the
// last 12 months reach, reads nothing; each before it is read once, as it's
// first stepped back to; and the steps stop at the year the ledger began,
// > stepping forward again over years read already.
func TestTheYearReadsEachCalendarYearOnceAndStopsWhereTheLedgerBegan(t *testing.T) {
	h, l := historyHarness(t, dashboard.History, dashboard.HistoryPage{})
	h.start()
	read := func(year int) time.Time { return views.YearFrom(views.YearAsk{Year: year}, start, time.Monday) }
	tests := []struct {
		key    string
		period string
		reads  []time.Time
	}{
		{key: "<", period: "2026"},
		{key: "<", period: "2025", reads: []time.Time{read(2025)}},
		{key: "<", period: "2024", reads: []time.Time{read(2025), read(2024)}},
		{key: "<", period: "2024", reads: []time.Time{read(2025), read(2024)}},
		{key: ">", period: "2025", reads: []time.Time{read(2025), read(2024)}},
		{key: ">", period: "2026", reads: []time.Time{read(2025), read(2024)}},
		{key: ">", period: "last 12 months", reads: []time.Time{read(2025), read(2024)}},
	}
	for i, tt := range tests {
		h.typed(tt.key)
		p := h.model.past.page
		if p.Period() != tt.period || p.Year == nil || !slices.EqualFunc(l.reads[1:], tt.reads, time.Time.Equal) {
			t.Fatalf("after %d presses, the Year shows %q, drawn %v, the ledger read since the year from %v; want %q, drawn, read from %v",
				i+1, p.Period(), p.Year != nil, l.reads[1:], tt.period, tt.reads)
		}
	}
}

// Of a ledger that doesn't tell where it begins, < steps back to this
// calendar year alone, which the last 12 months reach.
func TestTheYearStepsBackToThisYearAloneWhereTheLedgerDoesntSayWhereItBegan(t *testing.T) {
	h, l := historyHarness(t, dashboard.History, dashboard.HistoryPage{})
	h.model.cfg.Ledger = struct{ views.Ledger }{l}
	h.start()
	h.typed("<", "<")
	if got := h.model.past.page.Period(); got != "2026" || len(l.reads) != 1 {
		t.Errorf("< twice, the Year shows %q, the ledger read from %v; want 2026, the year alone", got, l.reads)
	}
}

// A version kept from a run before that the last 12 months didn't use is
// let go once they're read, the Year showing all use; one they used stays.
func TestAVersionKeptThatTheYearDidntUseShowsAllUse(t *testing.T) {
	for _, tt := range []struct{ kept, want string }{{kept: "Opus 4", want: ""}, {kept: "Opus 5.5", want: "Opus 5.5"}} {
		h, l := historyHarness(t, dashboard.History, dashboard.HistoryPage{YearAsk: views.YearAsk{Version: tt.kept}})
		l.days[yesterday] = requested(yesterday, 10)
		h.start()
		if got := h.model.past.page.YearAsk.Version; got != tt.want {
			t.Errorf("kept showing %q, the Year shows %q, want %q", tt.kept, got, tt.want)
		}
	}
}

// A look that changes today's requests brightens the Year's figures it
// changes, drawn before and after at the one moment.
func TestALookThatAddsRequestsTodayBrightensTheYearsFigures(t *testing.T) {
	h, l := historyHarness(t, dashboard.History, dashboard.HistoryPage{})
	today := dayfile.DateOf(start)
	l.days[yesterday] = requested(yesterday, 10)
	h.start()
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

// History's keys change what it shows, from its top, and the preferences
// keep each change, but a calendar year stepped back to; none reads the
// ledger again.
func TestHistorysKeysChangeWhatItShowsAndThePreferencesKeepIt(t *testing.T) {
	h, l := historyHarness(t, dashboard.History, dashboard.HistoryPage{})
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()
	tests := []struct {
		key  string
		kept theme.HistoryPrefs
	}{
		{key: "]", kept: theme.HistoryPrefs{View: "weeks", Year: "all", Weeks: "table", Tokens: theme.TokensPrefs{Period: "week", Split: "account"}}},
		{key: "v", kept: theme.HistoryPrefs{View: "weeks", Year: "all", Weeks: "columns", Tokens: theme.TokensPrefs{Period: "week", Split: "account"}}},
		{key: "]", kept: theme.HistoryPrefs{View: "days", Year: "all", Weeks: "columns", Tokens: theme.TokensPrefs{Period: "week", Split: "account"}}},
		{key: "]", kept: theme.HistoryPrefs{View: "tokens", Year: "all", Weeks: "columns", Tokens: theme.TokensPrefs{Period: "week", Split: "account"}}},
		{key: "<", kept: theme.HistoryPrefs{View: "tokens", Year: "all", Weeks: "columns", Tokens: theme.TokensPrefs{Period: "day", Split: "account"}}},
		{key: "v", kept: theme.HistoryPrefs{View: "tokens", Year: "all", Weeks: "columns", Tokens: theme.TokensPrefs{Period: "day", Split: "model"}}},
		{key: "]", kept: theme.HistoryPrefs{View: "year", Year: "all", Weeks: "columns", Tokens: theme.TokensPrefs{Period: "day", Split: "model"}}},
		{key: "<", kept: theme.HistoryPrefs{View: "year", Year: "all", Weeks: "columns", Tokens: theme.TokensPrefs{Period: "day", Split: "model"}}},
		{key: "v", kept: theme.HistoryPrefs{View: "year", Year: "work", Weeks: "columns", Tokens: theme.TokensPrefs{Period: "day", Split: "model"}}},
		{key: "right", kept: theme.HistoryPrefs{View: "year", Year: "personal", Weeks: "columns", Tokens: theme.TokensPrefs{Period: "day", Split: "model"}}},
	}
	updates := 0
	for _, tt := range tests {
		h.model.scroll = 2
		h.keyed(tt.key)
		if tt.key != "<" || tt.kept.View != "year" {
			updates++
		}
		if prefs.kept.History != tt.kept || prefs.updates != updates || h.model.scroll != 0 {
			t.Fatalf("after %s, the preferences keep %+v, updated %d times, the view scrolled %d; want %+v, %d times, from its top",
				tt.key, prefs.kept.History, prefs.updates, h.model.scroll, tt.kept, updates)
		}
	}
	if len(l.reads) != 1 {
		t.Errorf("the ledger was read from %v, want the year alone, the once: this calendar year it reaches", l.reads)
	}
}

// History's key line names its keys, the period and what v shows, as the
// design's key lines do: ←→ account joining v while the Year shows an
// account alone; p pin left out with one account; and a phone's keeping [ ]
// view alone.
func TestHistorysKeyLineIsTheDesigns(t *testing.T) {
	const ends = "    p pin    ? help  q quit"
	tests := []struct {
		name   string
		kept   dashboard.HistoryPage
		one    bool
		width  int
		want   string
		listed []string
	}{
		{name: "the Year", want: "tab views   [ ] view    < > period: last 12 months    v split: all" + ends},
		{
			name: "the Year of Opus 5.5 in 2026", kept: dashboard.HistoryPage{YearAsk: views.YearAsk{Version: "Opus 5.5", Year: 2026}},
			want: "tab views   [ ] view    < > period: 2026    v split: Opus 5.5" + ends,
		},
		{
			name: "the Year of an account", kept: dashboard.HistoryPage{YearAsk: views.YearAsk{Account: "work"}},
			want: "tab views   [ ] view    < > period: last 12 months    v split: Work  ←→ account" + ends,
		},
		{name: "Weeks", kept: dashboard.HistoryPage{Shown: dashboard.HistoryWeeks}, want: "tab views   [ ] view    v split: table" + ends},
		{name: "Weeks in columns", kept: dashboard.HistoryPage{Shown: dashboard.HistoryWeeks, WeeksShape: dashboard.WeeksColumns}, want: "tab views   [ ] view    v split: columns" + ends},
		{name: "Days", kept: dashboard.HistoryPage{Shown: dashboard.HistoryDays}, want: "tab views   [ ] view" + ends},
		{name: "Tokens", kept: dashboard.HistoryPage{Shown: dashboard.HistoryTokens}, want: "tab views   [ ] view    < > period: by week    v split: by account" + ends},
		{name: "one account", one: true, want: "tab views   [ ] view    < > period: last 12 months    v split: all    ? help  q quit"},
		{name: "a phone", width: 52, want: "tab views   [ ] view   ? keys   q quit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, l := historyHarness(t, dashboard.History, tt.kept)
			l.days[yesterday] = requested(yesterday, 10)
			if tt.one {
				doc := routerDocument(three()[0])
				h.source.router = &doc
			}
			h.deliver(h.update(tea.WindowSizeMsg{Width: cmp.Or(tt.width, 160), Height: 37})...)
			h.start()
			rows := strings.Split(h.view(), "\n")
			if got := strings.TrimRight(rows[len(rows)-1], " "); got != tt.want {
				t.Errorf("the key line reads\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// A click on a sub-tab shows it; on < or > by a period, steps it; and on [
// ], < >, v and ←→ on the key line presses ], >, v and →.
func TestAClickOnHistoryActsAsItsKeyDoes(t *testing.T) {
	h, _ := historyHarness(t, dashboard.History, dashboard.HistoryPage{YearAsk: views.YearAsk{Account: "work"}})
	h.start()
	steps := []struct {
		name string
		on   func(dashboard.Target) bool
		want func(dashboard.HistoryPage) bool
	}{
		{
			name: "←→ on the key line", on: onKey("←→"),
			want: func(p dashboard.HistoryPage) bool { return p.YearAsk.Account == "personal" },
		},
		{
			name: "< by the period", on: onStep(-1),
			want: func(p dashboard.HistoryPage) bool { return p.YearAsk.Year == 2026 },
		},
		{
			name: "> by the period", on: onStep(1),
			want: func(p dashboard.HistoryPage) bool { return p.YearAsk.Year == 0 },
		},
		{
			name: "Tokens", on: onSubTab(dashboard.HistoryTokens),
			want: func(p dashboard.HistoryPage) bool { return p.Shown == dashboard.HistoryTokens },
		},
		{
			name: "< > on the key line", on: onKey("< >"),
			want: func(p dashboard.HistoryPage) bool { return p.TokensAsk.Period == views.TokensByMonth },
		},
		{
			name: "v on the key line", on: onKey("v"),
			want: func(p dashboard.HistoryPage) bool { return p.TokensSplit == dashboard.TokensByModel },
		},
		{
			name: "[ ] on the key line", on: onKey("[ ]"),
			want: func(p dashboard.HistoryPage) bool { return p.Shown == dashboard.HistoryYear },
		},
	}
	for _, step := range steps {
		h.clickWhere(step.on)
		if !step.want(h.model.past.page) {
			t.Fatalf("after a click on %s, History shows %+v", step.name, h.model.past.page)
		}
	}
}

// onKey, onStep and onSubTab report whether a target is the key given on
// the key line, the heading's < or > stepping the way given, or the sub-tab
// given.
func onKey(key string) func(dashboard.Target) bool {
	return func(t dashboard.Target) bool { return t.On == dashboard.OnKey && t.Key == key }
}

func onStep(way int) func(dashboard.Target) bool {
	return func(t dashboard.Target) bool { return t.On == dashboard.OnStep && t.Step == way }
}

func onSubTab(v dashboard.HistoryView) func(dashboard.Target) bool {
	return func(t dashboard.Target) bool { return t.On == dashboard.OnSubTab && t.SubTab == int(v) }
}

// A turn of the wheel over what of History scrolls scrolls the Year three
// rows, and Tokens a period, as j does.
func TestTheWheelScrollsHistorysYearByRowsAndTokensAPeriodAtATime(t *testing.T) {
	tests := []struct {
		name   string
		kept   dashboard.HistoryPage
		height int
		want   int
	}{
		{name: "the Year", height: 30, want: wheelRows},
		{name: "Tokens", kept: dashboard.HistoryPage{Shown: dashboard.HistoryTokens, TokensAsk: views.TokensAsk{Period: views.TokensByDay}}, height: 37, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, l := historyHarness(t, dashboard.History, tt.kept)
			for i := range 30 {
				date := dayfile.DateOf(dayfile.DayStart(start.Local(), -i))
				l.days[date] = requested(date, 10)
			}
			h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: tt.height})...)
			h.start()
			if h.model.scrolling().Most < tt.want {
				t.Fatalf("History scrolls %+v, want it to scroll", h.model.scrolling())
			}
			h.wheel(tea.MouseWheelDown, 10, h.overTheView())
			if h.model.scroll != tt.want {
				t.Errorf("a turn of the wheel scrolled History %d, want %d", h.model.scroll, tt.want)
			}
		})
	}
}

// overTheView is a row of what of the view on screen scrolls, at column 10,
// failing the test where there's none.
func (h *harness) overTheView() int {
	h.t.Helper()
	f, at := h.model.onScreen()
	for y := range f.Height {
		if f.Under(h.model.doc, at, 10, y) == dashboard.UnderView {
			return y
		}
	}
	h.t.Fatal("nothing on screen scrolls")
	return 0
}

// ? opens help over History, which lists its keys after tab.
func TestHelpOpensOverHistoryListingItsKeys(t *testing.T) {
	h, _ := historyHarness(t, dashboard.History, dashboard.HistoryPage{})
	h.start()
	h.typed("?")
	if !h.inHelp() || !strings.Contains(h.view(), "─ HELP · HISTORY ") {
		t.Fatalf("after ?, the screen is\n%s\nwant help over History", h.view())
	}
	if got, want := listedKeys(h.model.help.keys), []string{"tab", "[ ]", "< >", "v", "p", "R", "?", "q"}; !slices.Equal(got, want) {
		t.Errorf("help lists %q, want %q", got, want)
	}
}
