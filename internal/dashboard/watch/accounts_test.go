package watch

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// Accounts and History share one read of the ledger's days and the days it
// holds: Accounts shown first reads the year, the once, and History shown
// after it draws the year without reading it again; each look on either
// reads today alone.
func TestAccountsAndHistoryShareOneReadOfTheYear(t *testing.T) {
	h, l := historyHarness(t, dashboard.Accounts, dashboard.HistoryPage{})
	h.start()
	if !slices.EqualFunc(l.reads, []time.Time{yearFrom}, time.Time.Equal) || h.model.accounts.page == nil {
		t.Fatalf("Accounts opened, the ledger was read from %v, its page %v; want the year read once, from %s, and the page built", l.reads, h.model.accounts.page, yearFrom)
	}
	h.keys(tabKey)
	if h.model.view != dashboard.History || len(l.reads) != 1 || h.model.past.page.Year == nil {
		t.Fatalf("on %s, the ledger was read from %v, the Year %v; want History, its Year drawn of the year read once", h.model.view, l.reads, h.model.past.page.Year)
	}
	h.tickUntilAsked()
	h.keys(shiftTabKey)
	h.tickUntilAsked()
	if want := []time.Time{yearFrom, todayStart, todayStart}; !slices.EqualFunc(l.reads, want, time.Time.Equal) {
		t.Errorf("after a look on each, the ledger was read from %v, want %v: today alone at each", l.reads, want)
	}
}

// A calendar year History's Year was stepped back to is read while History
// shows alone: Accounts, which counts no further back than 12 weeks, never
// reads it.
func TestAccountsNeverReadsACalendarYearTheYearSteppedBackTo(t *testing.T) {
	h, l := historyHarness(t, dashboard.Accounts, dashboard.HistoryPage{})
	h.start()
	h.model.past.page.YearAsk.Year = 2024
	h.update(tea.WindowSizeMsg{Width: 160, Height: 37})
	if len(l.reads) != 1 {
		t.Errorf("on Accounts, History's Year stepped back to 2024, the ledger was read from %v, want the last 12 months alone", l.reads)
	}
}

// Accounts' page is built of the days held over the period shown: never
// before Accounts shows, though History holds the days, nor again for a
// message that changes neither, as a frame or a resize; afresh for each
// look, which reads today's days, and for a period shown anew.
func TestAccountsPageIsBuiltOnceForTheDaysHeldAndThePeriod(t *testing.T) {
	h, _ := historyHarness(t, dashboard.History, dashboard.HistoryPage{})
	h.start()
	h.tickUntilAsked()
	if h.model.accounts.page != nil {
		t.Fatalf("on History, its days read, Accounts' page is %+v, want none built", h.model.accounts.page)
	}
	h.keys(shiftTabKey)
	built := h.model.accounts.page
	if built == nil || built.Period != views.AccountsThisWeek {
		t.Fatalf("Accounts shown, its page is %+v, want it built over this week", built)
	}

	h.update(frameMsg{gen: h.model.frames})
	h.update(tea.WindowSizeMsg{Width: 120, Height: 37})
	if h.model.accounts.page != built {
		t.Errorf("a frame and a resize built Accounts' page afresh, want it held")
	}

	h.tickUntilAsked()
	looked := h.model.accounts.page
	if looked == built {
		t.Errorf("a look that read today's days left Accounts' page as it was built, want it built afresh")
	}

	shown := h.model.accounts.shown
	shown.Period = views.AccountsToday
	h.model = h.model.showAccounts(shown)
	h.update(frameMsg{gen: h.model.frames})
	today := h.model.accounts.page
	if today == looked || today.Period != views.AccountsToday {
		t.Errorf("today shown, Accounts' page is over %q, want it built afresh over today", today.Period)
	}

	h.clock.now = dayfile.DayStart(start.Local(), 1).Add(30 * time.Second)
	h.update(tea.WindowSizeMsg{Width: 160, Height: 37})
	if page := h.model.accounts.page; page == today || page.ToCome != today.ToCome-1 {
		t.Errorf("past midnight, before a look, Accounts' page has %d days of the week to come, want it built afresh with %d", page.ToCome, today.ToCome-1)
	}
}

// What Accounts shows that the preferences keep, the sub-tab and the period,
// is kept as it changes; what's chosen in it, never.
func TestAccountsSubTabAndPeriodAreKeptAsTheyChange(t *testing.T) {
	h, _ := historyHarness(t, dashboard.Accounts, dashboard.HistoryPage{})
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()
	shown := h.model.accounts.shown
	steps := []struct {
		name    string
		change  func(*dashboard.AccountsShown)
		kept    theme.AccountsPrefs
		updates int
	}{
		{name: "the period", change: func(s *dashboard.AccountsShown) { s.Period = views.AccountsThirtyDays }, kept: theme.AccountsPrefs{View: "compare", Period: "30-days"}, updates: 1},
		{name: "the sub-tab", change: func(s *dashboard.AccountsShown) { s.Shown = dashboard.AccountsDetail }, kept: theme.AccountsPrefs{View: "detail", Period: "30-days"}, updates: 2},
		{name: "the account chosen", change: func(s *dashboard.AccountsShown) { s.Chosen = "work" }, kept: theme.AccountsPrefs{View: "detail", Period: "30-days"}, updates: 2},
		{name: "Compare's focus", change: func(s *dashboard.AccountsShown) { s.Focused, s.Focus = true, "side" }, kept: theme.AccountsPrefs{View: "detail", Period: "30-days"}, updates: 2},
	}
	for _, step := range steps {
		step.change(&shown)
		h.model = h.model.showAccounts(shown)
		if prefs.kept.Accounts != step.kept || prefs.updates != step.updates {
			t.Fatalf("after %s changed, the preferences keep %+v, updated %d times; want %+v, %d times", step.name, prefs.kept.Accounts, prefs.updates, step.kept, step.updates)
		}
	}
}

// A watch opens Accounts as the preferences kept it, over the configured
// accounts.
func TestAccountsOpensAsKept(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	cfg := h.config(Size{Width: 160, Height: 37})
	cfg.Accounts, cfg.AccountsShown = threeConfigured, dashboard.AccountsKept(theme.AccountsPrefs{View: "detail", Period: "12-weeks"})
	m := New(t.Context(), cfg)
	if s := m.accounts.shown; s.Shown != dashboard.AccountsDetail || s.Period != views.AccountsTwelveWeeks || !slices.Equal(s.Accounts.IDs(), threeConfigured.IDs()) {
		t.Errorf("Accounts opens as %+v, want Detail over the last 12 weeks, of the accounts configured", s)
	}
}

// The frame names the accounts' own week as the config does, the one key
// Accounts' page and its WEEK USED read it by.
func TestTheFrameNamesTheAccountsWeekAsTheConfigDoes(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	cfg := h.config(Size{Width: 160, Height: 37})
	cfg.WeekWindow = "7d_test"
	m := New(t.Context(), cfg)
	if got := m.frame(h.clock.now).WeekWindow; got != "7d_test" {
		t.Errorf("the frame's week is %q, want the config's, 7d_test", got)
	}
}

// Each look stands Accounts' page at its moment: a week reset before the
// router reads it again closes that week in the page the frame draws, as
// views.AccountsPage.At has it, while the page as built of the days, which
// don't hold the reset yet, stands as it was.
func TestEachLookStandsAccountsPageAtItsMoment(t *testing.T) {
	moments := viewstest.AccountsMoments()
	i := slices.IndexFunc(moments, func(mo viewstest.AccountsMoment) bool {
		return mo.Name == "a week reset at 21:00, at 21:00:01, before the router reads it again"
	})
	if i < 0 {
		t.Fatal("no moment of a week reset before the reread")
	}
	mo := moments[i]
	h := &harness{t: t, clock: &fakeClock{now: mo.Now}, source: &fakeSource{doc: probedWithoutTheRouter()}, notifier: &fakeNotifier{}}
	h.source.router = &mo.Doc
	days := make(map[string]ledger.Summary)
	for _, d := range mo.Days {
		days[d.Day] = d
	}
	l := &daysLedger{Empty: h.emptyLedger(), clock: h.clock, first: mo.Days[0].Day, days: days}
	cfg := h.config(Size{Width: 160, Height: 37})
	cfg.Ledger, cfg.Prices, cfg.Accounts, cfg.WeekStarts, cfg.WeekWindow, cfg.Policy = l, ledger.Pricing, mo.Accounts, mo.WeekStarts, claude.WeekWindow, claude.Policy
	cfg.View = dashboard.Accounts
	h.model = New(t.Context(), cfg)
	h.start()

	built := h.model.accounts.page
	drawn := h.model.frame(mo.Now).Accounts.Page
	if built == nil || drawn == nil {
		t.Fatalf("Accounts' page is %v built, %v drawn; want both", built, drawn)
	}
	if got, want := leftOf(drawn.Rows[0]), mo.Want["work"][viewstest.FigureLeft]; got != want {
		t.Errorf("work's left at its last reset is drawn %s, want %s: its week closed at the reset", got, want)
	}
	if leftOf(built.Rows[0]) == leftOf(drawn.Rows[0]) {
		t.Errorf("work's left at its last reset is built %s, as drawn, want the page as built of the days to stand", leftOf(built.Rows[0]))
	}
}

// leftOf is what a row's week left at its last reset, as a whole percentage,
// "—" where none ended.
func leftOf(row views.AccountsRow) string {
	if row.WeekLeft == nil {
		return "—"
	}
	return prose.Percent(*row.WeekLeft)
}

// The request stream is read on Detail while it shows an account sessions
// run on, whose SESSIONS say what each is doing; not on Compare, nor of all
// accounts, nor of an account none runs on.
func TestDetailReadsTheStreamWhileItShowsAnAccountSessionsRunOn(t *testing.T) {
	tests := []struct {
		name  string
		shown dashboard.AccountsShown
		want  bool
	}{
		{name: "Compare", shown: dashboard.AccountsShown{}},
		{name: "Detail of all accounts", shown: dashboard.AccountsShown{Shown: dashboard.AccountsDetail}},
		{name: "Detail of an account sessions run on", shown: dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work"}, want: true},
		{name: "Detail of an account none runs on", shown: dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "side"}},
		{name: "the one account there is, sessions running on it", shown: dashboard.AccountsShown{Accounts: config.Accounts{{ID: "work"}}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := streamingHarness(t)
			h.model.view, h.model.accounts.shown = dashboard.Accounts, tt.shown
			h.update(tea.WindowSizeMsg{Width: 160, Height: 40})
			if open := h.opened() > 0 && !h.closed(h.opened()); open != tt.want {
				t.Errorf("the stream is open %v, want %v", open, tt.want)
			}
		})
	}
}
