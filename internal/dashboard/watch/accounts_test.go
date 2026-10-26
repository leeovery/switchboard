package watch

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
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

// Accounts' page is stood afresh only as what it stands at changes, for
// every frame to read: frames read the one stood, as does a message of the
// same moment; one that builds it afresh stands it afresh, as < does.
func TestAccountsPageIsStoodAfreshAsItsBuiltAfresh(t *testing.T) {
	h, _ := historyHarness(t, dashboard.Accounts, dashboard.HistoryPage{})
	h.start()
	now := h.clock.now
	stood := h.model.frame(now).Accounts.Page
	if stood == nil || h.model.frame(now).Accounts.Page != stood {
		t.Fatalf("two frames drew Accounts' page stood at %p then at %p, want both the one stood", stood, h.model.frame(now).Accounts.Page)
	}

	h.update(frameMsg{gen: h.model.frames})
	if h.model.frame(now).Accounts.Page != stood {
		t.Errorf("a message of the same moment stood Accounts' page afresh, want it held")
	}

	h.keyed("<")
	if got := h.model.frame(now).Accounts.Page.Period; got != views.AccountsToday {
		t.Errorf("< pressed, Accounts' page drawn is over %q, want it stood afresh over today", got)
	}
}

// Accounts' page, and History's Weeks, are stood afresh only as what they
// stand at changes, counted over a second of frames: never by a frame of the
// one document; afresh as a look reads the document afresh; and as the time
// passes work's week's reset, when each stands otherwise, the week closed.
func TestPagesAreStoodAfreshOnlyAsWhatTheyStandAtChanges(t *testing.T) {
	tests := []struct {
		view  dashboard.View
		stood func(m Model) any
		weeks func(m Model) []views.WeekPeak
	}{
		{
			view: dashboard.Accounts, stood: func(m Model) any { return m.accounts.stood },
			weeks: func(m Model) []views.WeekPeak { return m.accounts.stood.Rows[0].Peaks },
		},
		{
			view: dashboard.History, stood: func(m Model) any { return m.past.page.Weeks },
			weeks: func(m Model) []views.WeekPeak { return m.past.page.Weeks.Accounts[0].Peaks },
		},
	}
	for _, tt := range tests {
		t.Run(tt.view.Title(), func(t *testing.T) {
			mo := accountsMoment(t, "a week at its limit at 20:59:59, a second before it resets")
			h := momentHarness(t, mo, tt.view)
			h.start()
			stood := []any{tt.stood(h.model)}
			stands := func() {
				if s := tt.stood(h.model); s != stood[len(stood)-1] {
					stood = append(stood, s)
				}
			}
			for range 10 {
				h.clock.now = h.clock.now.Add(50 * time.Millisecond)
				h.update(frameMsg{gen: h.model.frames})
				stands()
			}
			if len(stood) != 1 {
				t.Errorf("over 10 frames of the one document, the page was stood %d times, want once", len(stood))
			}

			before := peaksOf(tt.weeks(h.model))
			h.clock.now = mo.Now.Add(1500 * time.Millisecond)
			h.update(frameMsg{gen: h.model.frames})
			stands()
			if after := peaksOf(tt.weeks(h.model)); len(stood) != 2 || slices.Equal(after, before) {
				t.Errorf("past work's week's reset, the page was stood %d times since, work's peaks %v, want once, the week closed, as against %v", len(stood)-1, after, before)
			}

			read := mo.Doc
			read.GeneratedAt = read.GeneratedAt.Add(time.Millisecond)
			h.source.router = &read
			h.tickUntilAsked()
			stands()
			if len(stood) == 2 {
				t.Errorf("a look reading the document afresh left the page stood as it was, want it stood afresh")
			}
		})
	}
}

// accountsMoment is the moment of AccountsMoments with the given name.
func accountsMoment(t *testing.T, name string) viewstest.AccountsMoment {
	t.Helper()
	moments := viewstest.AccountsMoments()
	i := slices.IndexFunc(moments, func(mo viewstest.AccountsMoment) bool { return mo.Name == name })
	if i < 0 {
		t.Fatalf("no moment %q", name)
	}
	return moments[i]
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

// The frame holds the configured accounts' plans at the most each has cost,
// as the config prices them, so turned Compare's plans' column at 40
// columns holds them whole though the period's page prices the plans
// lower: max20x cost $4,000 a month before it fell to $200.
func TestTurnedComparesPlansAreSizedByTheConfigsPlansAtTheirMost(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	cfg := h.config(Size{Width: 40, Height: 60})
	cfg.Accounts = config.Accounts{{ID: "work", Plan: "max20x"}, {ID: "side", Plan: "max20x"}}
	cfg.Prices.Plans = []ledger.Plan{{ID: "max20x", Name: "Max 20x", Size: 20, Prices: []ledger.PlanPrice{
		{From: "2024-01-01", Monthly: 4000 * ledger.Dollar}, {From: "2025-04-09", Monthly: 200 * ledger.Dollar},
	}}}
	m := New(t.Context(), cfg)
	priced := func(account string, all bool, monthly ledger.Picodollars) views.AccountsRow {
		return views.AccountsRow{Totals: views.AccountTotals{Account: account, All: all}, Plan: views.PlanCost{Account: account, All: all, Price: &monthly}}
	}
	page := views.AccountsPage{
		Period: views.AccountsThisWeek, Recorded: true,
		Rows: []views.AccountsRow{priced("work", false, 200*ledger.Dollar), priced("side", false, 200*ledger.Dollar)},
		All:  priced("", true, 400*ledger.Dollar),
	}
	f := m.frame(h.clock.now)
	f.Width, f.Height, f.View, f.Accounts.Page = 40, 60, dashboard.Accounts, &page
	rows := f.Draw(probedWithoutTheRouter(), h.clock.now)
	i := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(ansi.Strip(row), "VALUE") })
	if i < 0 {
		t.Fatalf("no VALUE in\n%s", ansi.Strip(strings.Join(rows, "\n")))
	}
	value := ansi.Strip(rows[i])
	if got, want := strings.Index(value, "worth")-strings.Index(value, "plan"), len("$8,000/mo")+1; got != want {
		t.Errorf("the plans' column is %d cells, want %d, all accounts' $8,000/mo a month at its most, in %q", got, want, value)
	}
}

// Each look stands Accounts' page at its moment: a week reset before the
// router reads it again closes that week in the page the frame draws, as
// views.AccountsPage.At has it, while the page as built of the days, which
// don't hold the reset yet, stands as it was.
func TestEachLookStandsAccountsPageAtItsMoment(t *testing.T) {
	mo := resetBeforeTheReread(t)
	h := momentHarness(t, mo, dashboard.Accounts)
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

// History's Weeks is stood at each look's moment, as Accounts' page is: a
// week reset before the router reads it again closes that week in both, so
// each account's peaks over Accounts' six weeks, and its verdict, and every
// account's, are drawn alike on each.
func TestHistorysWeeksTellTheAccountsWeeksAsAccountsAtTheMoment(t *testing.T) {
	mo := resetBeforeTheReread(t)
	h := momentHarness(t, mo, dashboard.History)
	h.start()
	weeks := h.model.frame(mo.Now).History.Weeks
	h.keys(shiftTabKey)
	page := h.model.frame(mo.Now).Accounts.Page
	if weeks == nil || page == nil {
		t.Fatalf("Weeks is %v drawn, Accounts' page %v; want both", weeks, page)
	}
	shown := len(weeks.Weeks) - len(page.Weeks)
	rows := append(slices.Clone(weeks.Accounts), weeks.All)
	for i, row := range append(slices.Clone(page.Rows), page.All) {
		if got, want := rows[i].Peaks[shown:], row.Peaks; !reflect.DeepEqual(got, want) {
			t.Errorf("row %d's peaks are drawn %v on Weeks, %v on Accounts", i, peaksOf(got), peaksOf(want))
		}
		if got, want := rows[i].AccountVerdict, row.Verdict; !reflect.DeepEqual(got, want) {
			t.Errorf("row %d's verdict is drawn %s on Weeks, %s on Accounts", i, verdictSaid(got), verdictSaid(want))
		}
	}
}

// Accounts first shown after History read the days, its page built later,
// before the days are read again, folds the reset of a run its document
// reads begun since they were read, as a page built as they landed does.
func TestAccountsShownAfterHistoryReadItsDaysFoldsAResetSinceTheRead(t *testing.T) {
	mo := accountsMoment(t, "a week reset at 21:00, reread at 21:00:30, the days read before it")
	h := momentHarness(t, mo, dashboard.History)
	h.clock.now = mo.DaysRead
	h.start()
	reads := len(h.model.cfg.Ledger.(*daysLedger).reads)
	h.clock.now = mo.Now
	h.keys(shiftTabKey)

	if got := len(h.model.cfg.Ledger.(*daysLedger).reads); got != reads {
		t.Fatalf("Accounts shown read the days %d times more, want it shown of those History read", got-reads)
	}
	page := h.model.frame(mo.Now).Accounts.Page
	if page == nil {
		t.Fatal("Accounts' page isn't drawn")
	}
	if got, want := leftOf(page.Rows[0]), mo.Want["work"][viewstest.FigureLeft]; got != want {
		t.Errorf("work's left at its last reset is drawn %s, want %s: its week closed at the reset", got, want)
	}
}

// The days read are read as the read was asked, not as they land: asked a
// moment before a reset, landing after it, they fold the reset of the run
// begun between, which they can't hold.
func TestDaysReadAcrossAResetFoldItAsReadWhenAsked(t *testing.T) {
	mo := accountsMoment(t, "a week reset at 21:00, reread at 21:00:30, the days read before it")
	h := momentHarness(t, mo, dashboard.Accounts)
	h.clock.now = mo.DaysRead
	h.start()
	h.clock.now = mo.Now
	h.deliver(sinceMsg{asked: h.model.past.asked, days: mo.Days[len(mo.Days)-1:], at: time.Date(2026, time.October, 5, 20, 59, 59, 0, time.Local)})

	page := h.model.frame(mo.Now).Accounts.Page
	if page == nil {
		t.Fatal("Accounts' page isn't drawn")
	}
	if got, want := leftOf(page.Rows[0]), mo.Want["work"][viewstest.FigureLeft]; got != want {
		t.Errorf("work's left at its last reset is drawn %s, want %s: its week closed at the reset", got, want)
	}
}

// resetBeforeTheReread is the moment of a week reset at 21:00, at 21:00:01,
// before the router reads it again.
func resetBeforeTheReread(t *testing.T) viewstest.AccountsMoment {
	t.Helper()
	return accountsMoment(t, "a week reset at 21:00, at 21:00:01, before the router reads it again")
}

// momentHarness is a harness of a watch opening on the view given, at the
// moment mo, of its document, its accounts and its days, History on Weeks.
func momentHarness(t *testing.T, mo viewstest.AccountsMoment, opening dashboard.View) *harness {
	t.Helper()
	h := &harness{t: t, clock: &fakeClock{now: mo.Now}, source: &fakeSource{doc: probedWithoutTheRouter()}, notifier: &fakeNotifier{}}
	h.source.router = &mo.Doc
	days := make(map[string]ledger.Summary)
	for _, d := range mo.Days {
		days[d.Day] = d
	}
	l := &daysLedger{Empty: h.emptyLedger(), clock: h.clock, first: mo.Days[0].Day, days: days}
	cfg := h.config(Size{Width: 160, Height: 37})
	cfg.Ledger, cfg.Accounts, cfg.WeekStarts, cfg.WeekWindow, cfg.Policy = l, mo.Accounts, mo.WeekStarts, claude.WeekWindow, claude.Policy()
	cfg.View, cfg.History = opening, dashboard.HistoryPage{Shown: dashboard.HistoryWeeks}
	h.model = New(t.Context(), cfg)
	return h
}

// peaksOf are peaks as whole percentages, "—" where one isn't known, and …
// after a week's still running.
func peaksOf(peaks []views.WeekPeak) []string {
	read := make([]string, len(peaks))
	for i, p := range peaks {
		read[i] = "—"
		if p.Share != nil {
			read[i] = views.Peak(*p.Share, p.Open)
		}
	}
	return read
}

// verdictSaid says a verdict, its limits over its weeks, the share left on
// average and the weeks short without its account, where known, as "hits
// its limit most weeks 3/7 19% short 2".
func verdictSaid(v views.AccountVerdict) string {
	said := fmt.Sprintf("%s %d/%d", v.Verdict, v.Limits, v.Weeks)
	if v.LeftAtReset != nil {
		said += " " + prose.Percent(*v.LeftAtReset)
	}
	if v.WithoutIt != nil {
		said += fmt.Sprintf(" short %d", *v.WithoutIt)
	}
	return said
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
		{name: "Compare, with the account sessions run on chosen in Detail", shown: dashboard.AccountsShown{Accounts: threeConfigured, Chosen: "work"}},
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

// Keys Accounts takes as the terminal sends them, beside those arrows_test
// and sessionturn_test name.
var (
	leftKey  = tea.KeyPressMsg{Code: tea.KeyLeft}
	rightKey = tea.KeyPressMsg{Code: tea.KeyRight}
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
	h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: 37})...)
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
	h.model.accounts.shown.Accounts = tenAccounts()
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

// esc ends Compare's focus, which enter then has nothing to open of, the
// columns staying where they are; and on Detail it does nothing.
func TestEscEndsComparesFocus(t *testing.T) {
	h := accountsHarness(t, dashboard.AccountsShown{})
	h.keys(rightKey, escKey, enterKey)
	if s := h.model.accounts.shown; s.Focused || s.View() != dashboard.AccountsCompare {
		t.Fatalf("→ esc enter left the focus %v on %q, showing %d; want no focus, Compare", s.Focused, s.Focus, s.View())
	}
	h.keys(rightKey, enterKey, escKey)
	if s := h.model.accounts.shown; s.View() != dashboard.AccountsDetail || !s.Focused || s.Focus != "work" {
		t.Errorf("→ enter esc showed %d, Compare focused %v on %q; want Detail, Compare's focus held on work", s.View(), s.Focused, s.Focus)
	}
}

// esc closes what's open over Compare before it ends Compare's focus: help
// and the routing card each close, the focus held; the next esc ends it.
func TestEscClosesWhatsOpenOverCompareBeforeItEndsItsFocus(t *testing.T) {
	for _, key := range []string{"?", "p"} {
		t.Run(key, func(t *testing.T) {
			h := accountsHarness(t, dashboard.AccountsShown{})
			h.keys(rightKey)
			h.deliver(h.press(key)...)
			if h.model.open == nothingOpen {
				t.Fatalf("%s opened nothing over Compare", key)
			}
			h.keys(escKey)
			if s := h.model.accounts.shown; h.model.open != nothingOpen || !s.Focused || s.Focus != "work" {
				t.Fatalf("%s then esc left %v open, Compare focused %v on %q; want it closed, the focus held on work", key, h.model.open, s.Focused, s.Focus)
			}
			h.keys(escKey)
			if s := h.model.accounts.shown; s.Focused {
				t.Errorf("the next esc left Compare focused on %q, want its focus ended", s.Focus)
			}
		})
	}
}

// A frame too narrow for the columns Compare showed its focus among scrolls
// them on to show it: ten accounts focused on acct-7 at 160 columns, six
// showing, show acct-4 to acct-7 at 122, four showing, as the heading says.
func TestAResizeKeepsComparesFocusInView(t *testing.T) {
	h := accountsHarness(t, dashboard.AccountsShown{})
	h.model.accounts.shown.Accounts = tenAccounts()
	h.keys(slices.Repeat([]tea.KeyPressMsg{rightKey}, 7)...)
	h.update(tea.WindowSizeMsg{Width: 122, Height: 37})
	var columns []string
	for _, t := range h.targets() {
		if t.On == dashboard.OnColumn && !slices.Contains(columns, t.Account) {
			columns = append(columns, t.Account)
		}
	}
	heading := strings.Split(h.view(), "\n")[4]
	if want := []string{"acct-4", "acct-5", "acct-6", "acct-7", ""}; !slices.Equal(columns, want) || !strings.Contains(heading, "accounts 4–7 of 10") {
		t.Errorf("at 122 columns, focused on acct-7, Compare draws the columns %q under %q; want %q, accounts 4–7 of 10", columns, heading, want)
	}
}

// tenAccounts are ten accounts, acct-1 to acct-10, in the config's order.
func tenAccounts() config.Accounts {
	ten := make(config.Accounts, 10)
	for i := range ten {
		ten[i] = config.Account{ID: "acct-" + strconv.Itoa(i+1)}
	}
	return ten
}

// Detail's list scrolls to show the account chosen no further than it must,
// from where it was left: of ten accounts at 24 rows, acct-10 chosen then
// acct-9, the list shows the accounts it showed with acct-10 chosen.
func TestDetailsListStaysScrolledWhereItWasLeft(t *testing.T) {
	h := accountsHarness(t, dashboard.AccountsShown{Shown: dashboard.AccountsDetail})
	h.model.accounts.shown.Accounts = tenAccounts()
	h.update(tea.WindowSizeMsg{Width: 160, Height: 24})
	listed := func() []string {
		var ids []string
		for _, t := range h.targets() {
			if t.On == dashboard.OnAccount && t.Account != "" {
				ids = append(ids, t.Account)
			}
		}
		return ids
	}
	h.keys(slices.Repeat([]tea.KeyPressMsg{downKey}, 10)...)
	last := listed()
	h.keys(upKey)
	if got := listed(); h.model.accounts.shown.Chosen != "acct-9" || !slices.Equal(got, last) || !slices.Contains(last, "acct-10") || len(last) == 10 {
		t.Errorf("acct-9 chosen after acct-10, chosen %q, the list shows %q; want the accounts shown with acct-10 chosen, %q, fewer than ten", h.model.accounts.shown.Chosen, got, last)
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

// enter on a session in Detail opens its page as a session's page opens
// anywhere: drawn in Detail's place once its lines are read, those waiting
// on a read of the ledger's tail under way, Detail showing and acting as it
// is till then, esc dropping it. esc comes back to Detail as it was left,
// drawn as it was.
func TestEnterOnASessionInDetailOpensItsPageAndEscComesBackToDetailAsItWas(t *testing.T) {
	tests := []struct {
		name             string
		tailing, dropped bool
	}{
		{name: "its lines read at once"},
		{name: "its lines read once the tail's read lands", tailing: true},
		{name: "dropped by esc before the tail's read lands", tailing: true, dropped: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "work", -4*time.Minute))
			h := accountsHarness(t, dashboard.AccountsShown{Shown: dashboard.AccountsDetail})
			h.model.cfg.Ledger = ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, views.DigestVersion, logs.For("watch"))
			h.keys(downKey)
			left, detail := h.model.accounts.shown, h.view()
			asLeft := func(after string) {
				t.Helper()
				if got := h.model.accounts.shown; h.model.session != nil || !reflect.DeepEqual(got, left) || h.view() != detail {
					t.Fatalf("%s, %q's page is open, Accounts showing %+v, drawn\n%s\nwant Detail as it was left, %+v, drawn\n%s", after, h.pageOpen(), got, h.view(), left, detail)
				}
			}

			var tail tea.Cmd
			if tt.tailing {
				h.keys(enterKey)
				tail = h.readingTheTail()
				h.keys(escKey)
			}
			opened, read := h.model.Update(enterKey)
			h.model = opened.(Model)
			if tt.tailing {
				if read != nil {
					t.Fatal("enter on d28c read its lines beside the tail's read, want them read once it lands")
				}
				asLeft("enter on d28c while the tail is read")
				if tt.dropped {
					h.keys(escKey)
				}
				read = tail
			}
			h.deliver(h.run(read)...)
			if tt.dropped {
				asLeft("esc before the tail's read landed")
				return
			}

			rows := strings.Split(h.view(), "\n")
			if h.pageOpen() != idD28C || !strings.HasPrefix(rows[4], "SESSION") || !strings.Contains(rows[4], status.ShownID(idD28C)) || !strings.HasPrefix(h.keyLine(), "esc back") {
				t.Fatalf("enter on d28c opened %q's page, drawing\n%s\nwant d28c's page drawn in Detail's place, its key line leading with esc back", h.pageOpen(), h.view())
			}
			if got := h.shownPage().Page.Totals.Requests; got != 2 {
				t.Errorf("d28c's page counts %d requests, want its 2 lines", got)
			}
			h.keys(escKey)
			asLeft("esc on d28c's page")
		})
	}
}

// Compare's focus keys take a focus at every width where the frame drawn
// draws Compare wide, as its CompareWide has it, and nowhere it's turned,
// with short names and long.
func TestComparesFocusKeysActWhereTheFrameDrawsItWide(t *testing.T) {
	long := config.Accounts{{ID: "work", Label: "research-consulting-0001"}, {ID: "personal"}, {ID: "side"}}
	for _, configured := range []config.Accounts{threeConfigured, long} {
		h := accountsHarness(t, dashboard.AccountsShown{})
		h.model.accounts.shown.Accounts = configured
		for width := 40; width <= 220; width++ {
			h.update(tea.WindowSizeMsg{Width: width, Height: 37})
			h.keys(rightKey)
			wide := h.model.frame(h.clock.now).CompareWide()
			if got := h.model.accounts.shown.Focused; got != wide {
				t.Errorf("at %d columns, → focused %v, the frame drawn wide %v", width, got, wide)
			}
			h.keys(escKey)
		}
	}
}

// Where Compare is turned, drawing no focus, its focus keys, ←→, enter and
// esc, do nothing there, nor say anything, though a wider frame left a
// column focused; Detail takes its keys as ever.
func TestTurnedComparesFocusKeysDoNothing(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{leftKey, rightKey, enterKey, escKey} {
		h := accountsHarness(t, dashboard.AccountsShown{})
		h.keys(rightKey, rightKey)
		h.update(tea.WindowSizeMsg{Width: 52, Height: 37})
		was := h.model.accounts.shown
		h.keys(key)
		if s := h.model.accounts.shown; !reflect.DeepEqual(s, was) || h.model.note != "" {
			t.Errorf("turned, %s left the focus %v on %q, showing %d, noting %q; want nothing done", key, s.Focused, s.Focus, s.View(), h.model.note)
		}
	}

	h := accountsHarness(t, dashboard.AccountsShown{Shown: dashboard.AccountsDetail})
	h.update(tea.WindowSizeMsg{Width: 52, Height: 37})
	h.keys(downKey, enterKey)
	if h.pageOpen() != idD28C {
		t.Errorf("stacked, ↓ enter on Detail opened %q's page, want %s's", h.pageOpen(), idD28C)
	}
}

// enter, drawn faint as it has nothing to open, says why in the key line's
// place, as p does; with one account none runs on, whose key line has no
// enter, it says nothing.
func TestAFaintEnterSaysWhyInTheKeyLinesPlace(t *testing.T) {
	tests := []struct {
		name  string
		shown dashboard.AccountsShown
		alone bool
		want  string
	}{
		{name: "Compare, no column focused", want: "no account is focused"},
		{name: "Detail of all accounts", shown: dashboard.AccountsShown{Shown: dashboard.AccountsDetail}, want: "all accounts lists no sessions"},
		{name: "Detail of an account none runs on", shown: dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "side"}, want: "no session is running on this account"},
		{name: "the one account there is, none running on it", alone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := accountsHarness(t, tt.shown)
			if tt.alone {
				h.model.accounts.shown.Accounts = config.Accounts{{ID: "side"}}
			}
			h.keys(enterKey)
			if h.model.note != tt.want || h.model.session != nil {
				t.Errorf("enter noted %q, opening %q's page; want %q noted, nothing opened", h.model.note, h.pageOpen(), tt.want)
			}
		})
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

// A frame takes nothing in, so the selection follows no session ending at
// it, as the clock ends 7f3a, idle past the hour; the next message that
// takes anything in follows it.
func TestAFrameLeavesTheSelectionForTheNextMessageToFollow(t *testing.T) {
	h := accountsHarness(t, dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work"})
	h.keys(rightKey, downKey, downKey)
	h.clock.now = start.Add(52 * time.Minute)
	h.update(frameMsg{gen: h.model.frames})
	if got := h.model.accounts.shown.Selected; got != id7F3A {
		t.Fatalf("a frame moved the selection to %q, want it on 7f3a still", got)
	}
	h.redraw()
	if got := h.model.accounts.shown.Selected; got != id9E21 {
		t.Errorf("the message after the frame selected %q, want 9e21, in 7f3a's place", got)
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

// A click on the heading's < or > steps the period Compare and Detail share,
// as the key does: < to a shorter one and > to a longer, no further than
// today or the last 12 weeks.
func TestAClickOnTheHeadingsLessOrMoreStepsAccountsPeriod(t *testing.T) {
	for name, shown := range map[string]dashboard.AccountsView{"Compare": dashboard.AccountsCompare, "Detail": dashboard.AccountsDetail} {
		t.Run(name, func(t *testing.T) {
			h := accountsHarness(t, dashboard.AccountsShown{Shown: shown})
			for i, step := range []struct {
				way  int
				want views.AccountsPeriod
			}{
				{-1, views.AccountsToday}, {-1, views.AccountsToday}, {1, views.AccountsThisWeek},
				{1, views.AccountsThirtyDays}, {1, views.AccountsTwelveWeeks}, {1, views.AccountsTwelveWeeks},
			} {
				h.clickWhere(onStep(step.way))
				if got := h.model.accounts.shown; got.Period != step.want || got.Shown != shown {
					t.Fatalf("after click %d, on the step %+d, Accounts shows %d over %q, want %d over %q", i+1, step.way, got.Shown, got.Period, shown, step.want)
				}
			}
		})
	}
}

// A look that reads today's days afresh brightens the period's figures on
// Accounts it changes, drawn before and after at the one moment, as History's
// do: work's requests and all accounts', on Compare and on Detail; never
// another's that stood still, nor any where the look found nothing new.
func TestALookThatAddsRequestsTodayBrightensAccountsPeriodFigures(t *testing.T) {
	for name, shown := range map[string]dashboard.AccountsView{"Compare": dashboard.AccountsCompare, "Detail": dashboard.AccountsDetail} {
		t.Run(name, func(t *testing.T) {
			h, l := historyHarness(t, dashboard.Accounts, dashboard.HistoryPage{})
			h.model.accounts.shown.Shown = shown
			today := dayfile.DateOf(start)
			l.days[today] = requested(today, 2)
			h.start()
			h.tickUntilAsked()
			if got := h.brightened(); len(got) > 0 {
				t.Fatalf("a look that found nothing new brightened %v, want none", got)
			}

			l.days[today] = requested(today, 5)
			h.tickUntilAsked()
			work := dashboard.Number{Kind: dashboard.NumberPeriod, Account: "work", Of: "requests"}
			all := dashboard.Number{Kind: dashboard.NumberPeriod, Of: "requests"}
			side := dashboard.Number{Kind: dashboard.NumberPeriod, Account: "side", Of: "requests"}
			got := h.brightened()
			if shown == dashboard.AccountsCompare && (got[work] != 1 || got[all] != 1 || got[side] != 0) {
				t.Errorf("on Compare, a look that found 3 more requests on work today brightened %v, want work's requests and all accounts' at 1, side's not", got)
			}
			if shown == dashboard.AccountsDetail && got[all] != 1 {
				t.Errorf("on Detail of all accounts, a look that found 3 more requests today brightened %v, want its requests at 1", got)
			}
		})
	}
}

// → puts the selection on the session it lands on, the first, and there it
// stays as the request stream reorders SESSIONS, the one seen last first:
// enter opens it, whatever's listed first since.
func TestTheSelectionStaysWithItsSessionAsTheStreamReordersSESSIONS(t *testing.T) {
	h := accountsHarness(t, dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work"})
	h.keys(rightKey)
	h.hear(alter(told(router.StreamSent, "r1", "work", 0), func(e *router.StreamEvent) { e.Session, e.Model = id7F3A, "claude-haiku-4-5" }))
	h.keys(enterKey)
	if got := h.pageOpen(); got != idD28C {
		t.Errorf("enter opened %q's page, want d28c's, selected as → landed, though 7f3a was seen since", got)
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
