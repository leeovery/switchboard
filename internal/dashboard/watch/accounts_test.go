package watch

import (
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
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

// Keys Accounts takes as the terminal sends them, beside those focus_test
// names.
var (
	leftKey  = tea.KeyPressMsg{Code: tea.KeyLeft}
	rightKey = tea.KeyPressMsg{Code: tea.KeyRight}
	enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}
)

// runningOnWork are three sessions on work, as SESSIONS lists them, the one
// seen last first: d28c, busy, then 9e21 and 7f3a.
func runningOnWork() []status.Session {
	return []status.Session{
		sessionOn(idD28C, opus, "work", 10*time.Second),
		sessionOn(id9E21, "claude-sonnet-5-5", "work", 20*time.Second),
		sessionOn(id7F3A, "claude-haiku-4-5", "work", 9*time.Minute),
	}
}

// accountsHarness is a model of the router of three, which says which it is,
// configured as threeConfigured, runningOnWork running on work, showing
// Accounts as given, in a terminal 160 columns wide and 37 tall, started.
func accountsHarness(t *testing.T, shown dashboard.AccountsShown) *harness {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
	h.source.health = router.Health{PID: 7, StartedAt: start.Add(-time.Hour).UTC()}
	h.source.sessions = runningOnWork()
	shown.Accounts = threeConfigured
	h.model.view, h.model.accounts.shown = dashboard.Accounts, shown
	h.update(tea.WindowSizeMsg{Width: 160, Height: 37})
	h.start()
	return h
}

// pageOpen is the id of the session whose page is open, "" where none is.
func (h *harness) pageOpen() string {
	if h.model.session == nil {
		return ""
	}
	return h.model.session.id
}

// [ and ] turn Accounts' sub-tab and < and > step its period, each shown from
// the view's top, and the preferences keep each change; at either end, < and
// > change nothing, nor keep anything.
func TestAccountsKeysTurnTheSubTabAndStepThePeriodKeptAsTheyChange(t *testing.T) {
	h := accountsHarness(t, dashboard.AccountsShown{})
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	steps := []struct {
		key    string
		kept   theme.AccountsPrefs
		scroll int
	}{
		{key: "]", kept: theme.AccountsPrefs{View: "detail"}},
		{key: "<", kept: theme.AccountsPrefs{View: "detail", Period: "today"}},
		{key: "<", kept: theme.AccountsPrefs{View: "detail", Period: "today"}, scroll: 2},
		{key: ">", kept: theme.AccountsPrefs{View: "detail"}},
		{key: "[", kept: theme.AccountsPrefs{View: "compare"}},
	}
	updates := 0
	for _, step := range steps {
		h.model.scroll = 2
		h.keyed(step.key)
		if step.scroll == 0 {
			updates++
		}
		if got := h.model.accounts.shown.Kept(); got != step.kept || prefs.kept.Accounts != step.kept || prefs.updates != updates || h.model.scroll != step.scroll {
			t.Fatalf("after %s, Accounts shows %+v, the preferences keep %+v, updated %d times, the view scrolled %d; want %+v kept, %d times, scrolled %d",
				step.key, got, prefs.kept.Accounts, prefs.updates, h.model.scroll, step.kept, updates, step.scroll)
		}
	}
}

// On Compare, enter does nothing till → focuses a column, the first, and
// moves the focus along them, the columns scrolling to keep it in view; then
// enter opens the account focused in Detail, from its top.
func TestComparesFocusMovesAlongItsColumnsAndEnterOpensItInDetail(t *testing.T) {
	h := accountsHarness(t, dashboard.AccountsShown{})
	ten := make(config.Accounts, 10)
	for i := range ten {
		ten[i] = config.Account{ID: "acct-" + strconv.Itoa(i+1)}
	}
	h.model.accounts.shown.Accounts = ten
	h.keys(enterKey)
	if s := h.model.accounts.shown; s.View() != dashboard.AccountsCompare || s.Focused {
		t.Fatalf("enter with nothing focused showed %d, focused %v; want Compare, unfocused", s.View(), s.Focused)
	}

	h.keys(slices.Repeat([]tea.KeyPressMsg{rightKey}, 7)...)
	if s := h.model.accounts.shown; s.Focus != "acct-7" || s.First != 1 {
		t.Fatalf("→ 7 times focused %q, the columns from %d; want acct-7, from the 2nd, six showing", s.Focus, s.First)
	}
	h.model.scroll = 2
	h.keys(enterKey)
	if s := h.model.accounts.shown; s.View() != dashboard.AccountsDetail || s.Chosen != "acct-7" || h.model.scroll != 0 {
		t.Errorf("enter showed %d of %q, scrolled %d; want Detail of acct-7, from its top", s.View(), s.Chosen, h.model.scroll)
	}
}

// On Detail, ↓ chooses an account in its list, and enter opens the page of
// the session selected, the first, from the list; → takes ↑↓ into its
// SESSIONS, where ↓ selects the next, whose page enter opens; and ← takes
// them back. esc goes back to Detail as it was left, and what's chosen holds
// across the tabs, for the run.
func TestDetailsKeysChooseAnAccountAndOpenTheSelectedSessionsPage(t *testing.T) {
	h := accountsHarness(t, dashboard.AccountsShown{Shown: dashboard.AccountsDetail})
	steps := []struct {
		name   string
		keys   []tea.KeyPressMsg
		opened string
		want   dashboard.AccountsShown
	}{
		{name: "↓ enter", keys: []tea.KeyPressMsg{downKey, enterKey}, opened: idD28C, want: dashboard.AccountsShown{Chosen: "work"}},
		{name: "→ ↓ enter", keys: []tea.KeyPressMsg{rightKey, downKey, enterKey}, opened: id9E21, want: dashboard.AccountsShown{Chosen: "work", InSessions: true, Selected: id9E21}},
		{name: "tab shift-tab ←", keys: []tea.KeyPressMsg{tabKey, shiftTabKey, leftKey}, want: dashboard.AccountsShown{Chosen: "work", Selected: id9E21}},
	}
	for _, step := range steps {
		h.model.scroll = 1
		h.keys(step.keys...)
		step.want.Shown, step.want.Accounts = dashboard.AccountsDetail, threeConfigured
		if got := h.model.accounts.shown; h.pageOpen() != step.opened || !reflect.DeepEqual(got, step.want) {
			t.Fatalf("after %s, %q's page is open, Accounts shows %+v; want %q's, over %+v", step.name, h.pageOpen(), got, step.opened, step.want)
		}
		if step.opened == "" {
			continue
		}
		h.keys(escKey)
		if got := h.model.accounts.shown; h.model.session != nil || h.model.view != dashboard.Accounts || !reflect.DeepEqual(got, step.want) || h.model.scroll != 1 {
			t.Fatalf("esc left %q's page open, %s showing %+v, scrolled %d; want Accounts as left, %+v, scrolled 1", h.pageOpen(), h.model.view, got, h.model.scroll, step.want)
		}
	}
}

// The session selected in Detail's SESSIONS ending gives the selection to the
// one in its place, else the last; with none left, ↑↓ go back to the list.
func TestTheSessionSelectedEndingSelectsTheOneInItsPlace(t *testing.T) {
	h := accountsHarness(t, dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work"})
	h.keys(rightKey, downKey)
	steps := []struct {
		name       string
		running    []status.Session
		selected   string
		inSessions bool
	}{
		{name: "9e21 ended", running: []status.Session{runningOnWork()[0], runningOnWork()[2]}, selected: id7F3A, inSessions: true},
		{name: "every session ended"},
	}
	for _, step := range steps {
		h.source.sessions = step.running
		h.deliver(h.press("R")...)
		if s := h.model.accounts.shown; s.Selected != step.selected || s.InSessions != step.inSessions {
			t.Fatalf("after %s, %q is selected, ↑↓ in SESSIONS %v; want %q, %v", step.name, s.Selected, s.InSessions, step.selected, step.inSessions)
		}
	}
}

// A click on Accounts acts as its keys do: on a sub-tab, it shows it; on an
// account in Detail's list, it chooses it; on a session, it selects it, and
// on the one selected, opens its page; on a Compare column's heads, it
// focuses it, and on the one focused, opens it in Detail.
func TestAClickOnAccountsChoosesWhatItLandsOnAndOpensWhatsChosen(t *testing.T) {
	h := accountsHarness(t, dashboard.AccountsShown{})
	onSubTab := func(v dashboard.AccountsView) func(dashboard.Target) bool {
		return func(t dashboard.Target) bool { return t.On == dashboard.OnSubTab && t.SubTab == int(v) }
	}
	onSide := func(t dashboard.Target) bool { return t.On == dashboard.OnColumn && t.Account == "side" }
	selected := dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work", InSessions: true, Selected: id9E21}
	steps := []struct {
		name   string
		click  func()
		opened string
		want   dashboard.AccountsShown
	}{
		{name: "Detail", click: func() { h.clickWhere(onSubTab(dashboard.AccountsDetail)) }, want: dashboard.AccountsShown{Shown: dashboard.AccountsDetail}},
		{name: "work", click: func() { h.clickOn(dashboard.Target{On: dashboard.OnAccount, Account: "work"}) }, want: dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work"}},
		{name: "9e21", click: func() { h.clickOn(dashboard.Target{On: dashboard.OnSession, Session: id9E21}) }, want: selected},
		{name: "9e21 again", click: func() { h.clickOn(dashboard.Target{On: dashboard.OnSession, Session: id9E21}) }, opened: id9E21, want: selected},
		{name: "Compare", click: func() { h.keys(escKey); h.clickWhere(onSubTab(dashboard.AccountsCompare)) }, want: dashboard.AccountsShown{Chosen: "work", InSessions: true, Selected: id9E21}},
		{name: "side's heads", click: func() { h.clickWhere(onSide) }, want: dashboard.AccountsShown{Focused: true, Focus: "side", Chosen: "work", InSessions: true, Selected: id9E21}},
		{name: "side's heads again", click: func() { h.clickWhere(onSide) }, want: dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Focused: true, Focus: "side", Chosen: "side"}},
	}
	for _, step := range steps {
		step.click()
		step.want.Accounts = threeConfigured
		if got := h.model.accounts.shown; h.pageOpen() != step.opened || !reflect.DeepEqual(got, step.want) {
			t.Fatalf("after a click on %s, %q's page is open, Accounts shows %+v; want %q's, over %+v", step.name, h.pageOpen(), got, step.opened, step.want)
		}
	}
}

// A look on Accounts reads today's days for its page alone, History's page
// left behind them, unbuilt, till History shows, which builds it of them.
func TestALookOnAccountsLeavesHistorysPageToBeBuiltOnceItShows(t *testing.T) {
	h, l := historyHarness(t, dashboard.Accounts, dashboard.HistoryPage{})
	h.start()
	year := h.model.past.page.Year
	today := dayfile.DateOf(start)
	l.days[today] = requested(today, 3)
	h.tickUntilAsked()
	if h.model.past.page.Year != year {
		t.Fatal("a look on Accounts built History's Year afresh, want it left till History shows")
	}
	h.keys(tabKey)
	built := h.model.past.page.Year
	if got, ok := gridDay(*built, today); h.model.view != dashboard.History || !ok || got.Requests != 3 {
		t.Fatalf("on %s, today is %+v on the Year, want History, today's 3 requests drawn", h.model.view, got)
	}
	h.update(frameMsg{gen: h.model.frames})
	if h.model.past.page.Year != built {
		t.Error("a frame built History's Year afresh, want it held as built")
	}
}

// A look on Accounts brightens each number it reads that it changed, as on
// every page: on Compare, the week of the account whose use rose, and all
// accounts', its mean; never another's that stood still.
func TestALookOnAccountsBrightensTheNumbersItChanged(t *testing.T) {
	h := accountsHarness(t, dashboard.AccountsShown{})
	work := dashboard.Number{Kind: dashboard.NumberUsed, Account: "work", Of: "7d"}
	side := dashboard.Number{Kind: dashboard.NumberUsed, Account: "side", Of: "7d"}
	all := dashboard.Number{Kind: dashboard.NumberAllUsed, Of: "7d"}
	if h.drawnNumber(work) == "" || h.drawnNumber(all) == "" {
		t.Fatalf("Compare draws work's week %q and all accounts' %q, want both", h.drawnNumber(work), h.drawnNumber(all))
	}
	doc := routerDocument(three()...)
	doc.Accounts[0].Windows[1].Utilization = 0.8
	h.startRouter(doc)
	h.deliver(h.press("R")...)
	if got := h.brightened(); got[work] != 1 || got[all] != 1 || got[side] != 0 {
		t.Errorf("a look that read work's week at 80%% brightened %v, want work's week and all accounts' at 1, side's not", got)
	}
}

// Detail's SESSIONS list the sessions as the request stream has seen them,
// the one seen last first, and enter opens the first as they're listed.
func TestEnterOpensTheSessionDetailListsFirstAsTheStreamHasSeenThem(t *testing.T) {
	h := accountsHarness(t, dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work"})
	h.hear(alter(told(router.StreamSent, "r1", "work", 0), func(e *router.StreamEvent) { e.Session, e.Model = id7F3A, "claude-haiku-4-5" }))
	h.keys(enterKey)
	if got := h.pageOpen(); got != id7F3A {
		t.Errorf("enter opened %q's page, want 7f3a's, seen last as the stream told", got)
	}
}
