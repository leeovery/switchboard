package watch

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/views"
)

// periodKeyed is m as the dial's key given steps it, as periodStepped has it.
func periodKeyed(m Model, key string) Model {
	stepped, _ := m.periodStepped(key)
	return stepped
}

// dialTabs are the tabs the dial shows on, each as a harness shows it.
var dialTabs = []struct {
	name string
	show func(*harness)
}{
	{"Compare", func(h *harness) {
		h.model.view, h.model.accounts.shown.Shown = dashboard.Accounts, dashboard.AccountsCompare
	}},
	{"Detail", func(h *harness) {
		h.model.view, h.model.accounts.shown.Shown = dashboard.Accounts, dashboard.AccountsDetail
	}},
	{"Tokens", func(h *harness) {
		h.model.view, h.model.accounts.shown.Shown = dashboard.Accounts, dashboard.AccountsTokens
	}},
	{"Projects", func(h *harness) { h.model.view = dashboard.Projects }},
	{"the List", func(h *harness) {
		h.model.view, h.model.sessionsTab.shown.Shown = dashboard.Sessions, dashboard.SessionsList
	}},
}

// dialHarness is a model of the router of three accounts, a ledger of the
// projects' sessions under it, weeks starting on Monday, showing the tab
// given, its preferences kept as given. It has started.
func dialHarness(t *testing.T, show func(*harness), prefs *fakePrefs) *harness {
	t.Helper()
	h := projectsHarness(t, dashboard.Overview)
	h.model.cfg.Prefs = prefs
	show(h)
	h.start()
	return h
}

// Each of the dial's keys steps its period on every tab the dial shows on,
// as views' rule has it at now: < back, never past the ledger's first day; >
// on; - shorter and + longer, keeping the last day, = as +; n back to now.
// The preferences keep the length alone, as - + and = change it, never a
// step in time.
func TestTheDialsKeysStepItsPeriodOnEveryTabItShowsOn(t *testing.T) {
	for _, tab := range dialTabs {
		t.Run(tab.name, func(t *testing.T) {
			prefs := &fakePrefs{}
			h := dialHarness(t, tab.show, prefs)
			now, starts := h.clock.now, h.model.cfg.WeekStarts
			began := views.LengthThirtyDays.Now().From(now, starts)
			h.model.begun = begun{began: began}
			back := func(p views.Period) views.Period { stepped, _ := p.Back(now, starts, began); return stepped }
			on := func(p views.Period) views.Period { stepped, _ := p.On(now, starts); return stepped }
			shorter := func(p views.Period) views.Period { stepped, _ := p.Shorter(now, starts); return stepped }
			longer := func(p views.Period) views.Period { stepped, _ := p.Longer(now, starts); return stepped }
			steps := []struct {
				key  string
				want func(views.Period) views.Period
				kept string
			}{
				{key: "<", want: back, kept: ""},
				{key: "+", want: longer, kept: "30-days"},
				{key: "-", want: shorter, kept: ""},
				{key: "-", want: shorter, kept: "today"},
				{key: "=", want: longer, kept: ""},
				{key: ">", want: on, kept: ""},
				{key: "<", want: back, kept: ""},
				{key: "n", want: func(p views.Period) views.Period { return p.Length().Now() }, kept: ""},
			}
			updates := 0
			for i, step := range steps {
				was := h.model.period
				want := step.want(was)
				if want == was {
					t.Fatalf("step %d, %s from %+v steps nowhere, want a step that moves", i+1, step.key, was)
				}
				h.keyed(step.key)
				if strings.ContainsAny(step.key, "-+=") {
					updates++
				}
				if h.model.period != want || prefs.kept.Period != step.kept || prefs.updates != updates {
					t.Fatalf("step %d, %s from %q gives %q, the preferences keeping %q, updated %d times; want %q, keeping %q, %d times",
						i+1, step.key, views.DialOf(was, now, starts, began).Name(), views.DialOf(h.model.period, now, starts, began).Name(),
						prefs.kept.Period, prefs.updates, views.DialOf(want, now, starts, began).Name(), step.kept, updates)
				}
				if dial := h.model.frame(now).Dial; dial != h.model.dial(now) {
					t.Errorf("after %s, the frame's dial is %+v, want the period's, %+v", step.key, dial, h.model.dial(now))
				}
			}
		})
	}
}

// At either end a key of the dial's does nothing, keeps nothing and leaves
// the view scrolled as it was: > and n at now; < on a period holding the
// ledger's first day; - at today and + at 12 months.
func TestTheDialsKeysStopAtEitherEnd(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		period func(h *harness) views.Period
	}{
		{name: "> at now", key: ">", period: func(*harness) views.Period { return views.LengthThisWeek.Now() }},
		{name: "n at now", key: "n", period: func(*harness) views.Period { return views.LengthThirtyDays.Now() }},
		{name: "< on the period holding the ledger's first day", key: "<", period: func(h *harness) views.Period {
			last, _ := views.LengthThisWeek.Now().Back(h.clock.now, h.model.cfg.WeekStarts, time.Time{})
			h.model.begun = begun{began: last.From(h.clock.now, h.model.cfg.WeekStarts)}
			return last
		}},
		{name: "- at today", key: "-", period: func(*harness) views.Period { return views.LengthToday.Now() }},
		{name: "+ at 12 months", key: "+", period: func(*harness) views.Period { return views.LengthTwelveMonths.Now() }},
	}
	for _, tab := range dialTabs {
		for _, tt := range tests {
			t.Run(tab.name+", "+tt.name, func(t *testing.T) {
				prefs := &fakePrefs{}
				h := dialHarness(t, tab.show, prefs)
				period := tt.period(h)
				h.model.period = period
				h.keyed(tt.key)
				if h.model.period != period || prefs.updates != 0 {
					t.Errorf("%s left the period %+v, the preferences updated %d times; want %+v, never updated", tt.key, h.model.period, prefs.updates, period)
				}
				scrolled := h.model
				scrolled.scroll = 2
				if refused, took := scrolled.periodStepped(tt.key); !took || refused.period != period || refused.scroll != 2 {
					t.Errorf("%s, scrolled 2 rows, took it: %v, leaving %+v scrolled %d; want it taken, %+v left scrolled 2", tt.key, took, refused.period, refused.scroll, period)
				}
			})
		}
	}
}

// Till the ledger's first day is known, < steps back freely; once it is, <
// stops at the period holding it.
func TestLessStepsFreelyTillTheLedgersFirstDayIsKnown(t *testing.T) {
	h := dialHarness(t, dialTabs[3].show, &fakePrefs{})
	week := views.LengthThisWeek.Now()
	last, _ := week.Back(h.clock.now, h.model.cfg.WeekStarts, time.Time{})
	learned := begun{began: last.From(h.clock.now, h.model.cfg.WeekStarts)}
	want := week
	for range 4 {
		want, _ = want.Back(h.clock.now, h.model.cfg.WeekStarts, time.Time{})
		h.press("<")
	}
	if h.model.period != want {
		t.Fatalf("where the ledger's first day isn't known, four < give %+v, want %+v, stepping back freely", h.model.period, want)
	}
	h.model.begun, h.model.period = learned, week
	h.keyed("< < < <")
	if got := h.model.dial(h.clock.now); h.model.period != last || !got.Earliest() {
		t.Errorf("four < from this week stopped at %q, want last week, holding the ledger's first day, %s", got.Name(), dayfile.DateOf(learned.began))
	}
}

// The ledger's first day one tab's read learns is the dial's floor on every
// other: learned by Projects' read of its days, it stops < on Accounts, and
// no read asks for it again.
func TestTheLedgersFirstDayOneTabLearnsStopsLessOnAnother(t *testing.T) {
	h := projectsHarness(t, dashboard.Projects)
	week := views.LengthThisWeek.Now()
	last, _ := week.Back(h.clock.now, time.Monday, time.Time{})
	l := &beginsFlaky{projectsLedger: h.model.cfg.Ledger.(*projectsLedger), began: last.From(h.clock.now, time.Monday)}
	h.model.cfg.Ledger = l
	h.start()
	if h.model.begun.began.IsZero() || l.asked != 1 {
		t.Fatalf("Projects read its days, the ledger asked %d times, and its first day is %v; want it asked once, and known", l.asked, h.model.begun.began)
	}
	h.keys(shiftTabKey)
	h.tickUntilAsked()
	h.keyed("< < <")
	if got := h.model.dial(h.clock.now); h.model.view != dashboard.Accounts || h.model.period != last || !got.Earliest() || l.asked != 1 {
		t.Errorf("on %s, three < stopped at %q, the ledger asked %d times; want Accounts, last week, holding the ledger's first day, asked once", h.model.view, got.Name(), l.asked)
	}
}

// A past period keeps its dates as midnight passes, so what's shown doesn't
// move; now rolls on to the new day.
func TestAPastPeriodKeepsItsDatesAcrossMidnight(t *testing.T) {
	for _, tab := range dialTabs {
		t.Run(tab.name, func(t *testing.T) {
			h := dialHarness(t, tab.show, &fakePrefs{})
			h.keyed("- <")
			yesterday, starts := h.model.period, h.model.cfg.WeekStarts
			until, gone := yesterday.Until(h.clock.now, starts)
			if dial := h.model.dial(h.clock.now); !gone || dial.Name() != "yesterday" {
				t.Fatalf("- then < show %q, gone: %v; want yesterday", dial.Name(), gone)
			}
			h.clock.now = dayfile.DayStart(h.clock.now.Local(), 1).Add(time.Minute)
			h.update(tea.WindowSizeMsg{Width: 160, Height: 50})
			if got, stays := h.model.period.Until(h.clock.now, starts); h.model.period != yesterday || !stays || !got.Equal(until) {
				t.Errorf("past midnight, the period ends %v, gone: %v, want it as it was, ending %v", got, stays, until)
			}
			if name := h.model.frame(h.clock.now).Dial.Name(); name == "yesterday" {
				t.Errorf("past midnight, the dial names the day before %q, want it by its date", name)
			}
			h.keyed("n")
			if _, gone := h.model.period.Until(h.clock.now, starts); gone || h.model.period != views.LengthToday.Now() {
				t.Errorf("n past midnight gives %+v, want today, the new day", h.model.period)
			}
		})
	}
}

// todayHour is the hour now falls in where the period is today at now, and
// none of a day gone or another length.
func TestTodayHourIsNowsOfTodayAlone(t *testing.T) {
	now := start
	yesterday, _ := views.LengthToday.Now().Back(now, time.Monday, time.Time{})
	tests := []struct {
		name   string
		period views.Period
		want   int
	}{
		{name: "today", period: views.LengthToday.Now(), want: now.Local().Hour()},
		{name: "a day gone", period: yesterday, want: -1},
		{name: "this week", period: views.LengthThisWeek.Now(), want: -1},
	}
	for _, tt := range tests {
		if got := todayHour(tt.period, now, time.Monday); got != tt.want {
			t.Errorf("%s: todayHour = %d, want %d", tt.name, got, tt.want)
		}
	}
}

// A P turn keeps the period shown, past or now, as the pool turned to shows
// it from the same dates.
func TestAPTurnKeepsAPastPeriod(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, pooledProjects...)
	h := newHarness(t, probedWithoutTheRouter())
	digestedAsTheRouter(t, h, state, "b2n8")
	pools := newPoolTurns(t, testPools, slices.Concat(defaultAccounts, acmeAccounts), map[string]Source{"b2n8": h.source, "q7mt": h.source})
	h.model.cfg = pools.opened(h.model.cfg, "b2n8")
	followedOn(h, state)
	h.model.view = dashboard.Projects
	h.start()
	h.keyed("- <")
	past := h.model.period
	if _, gone := past.Until(h.clock.now, h.model.cfg.WeekStarts); !gone {
		t.Fatalf("- then < show %+v, want a day gone", past)
	}
	h.deliver(h.press("P")...)
	if h.model.cfg.Pool.ID != "q7mt" || h.model.period != past {
		t.Errorf("P turned to %q over %+v, want Acme, over %+v as it was", h.model.cfg.Pool.ID, h.model.period, past)
	}
}

// The tabs without the dial take none of its keys: the period stays as it
// is on the Overview, Sessions' Switchboard, Runway, Trends and the Log.
func TestTheDialsKeysDoNothingWhereItDoesntShow(t *testing.T) {
	shows := map[string]func(*harness){
		"the Overview":          func(h *harness) { h.model.view = dashboard.Overview },
		"Sessions' Switchboard": func(h *harness) { h.model.view = dashboard.Sessions },
		"Runway":                func(h *harness) { h.model.view = dashboard.Runway },
		"Trends":                func(h *harness) { h.model.view = dashboard.Graphs },
		"the Log":               func(h *harness) { h.model.view = dashboard.Log },
	}
	for _, name := range slices.Sorted(maps.Keys(shows)) {
		t.Run(name, func(t *testing.T) {
			prefs := &fakePrefs{}
			h := dialHarness(t, shows[name], prefs)
			h.keyed("< - n + = >")
			if h.model.period != views.LengthThisWeek.Now() || prefs.kept.Period != "" {
				t.Errorf("the dial's keys left %+v, the preferences keeping %q; want this week, nothing kept", h.model.period, prefs.kept.Period)
			}
		})
	}
}

// A click on the heading's readout steps the period as its sign's key
// does: - shorter, + longer, < back and > on.
func TestAClickOnTheReadoutsSignsStepsItAsTheirKeysDo(t *testing.T) {
	for _, tab := range dialTabs {
		t.Run(tab.name, func(t *testing.T) {
			h := dialHarness(t, tab.show, &fakePrefs{})
			now, starts, began := h.clock.now, h.model.cfg.WeekStarts, h.model.begun.began
			week := views.LengthThisWeek.Now()
			last, _ := week.Back(now, starts, began)
			lastThirty, _ := last.Longer(now, starts)
			steps := []struct {
				name  string
				click func()
				want  views.Period
			}{
				{name: "<", click: func() { h.clickWhere(onStep(-1)) }, want: last},
				{name: ">", click: func() { h.clickWhere(onStep(1)) }, want: week},
				{name: "< again", click: func() { h.clickWhere(onStep(-1)) }, want: last},
				{name: "+", click: func() { h.clickWhere(onPress("+")) }, want: lastThirty},
				{name: "-", click: func() { h.clickWhere(onPress("-")) }, want: last},
			}
			for _, step := range steps {
				step.click()
				if h.model.period != step.want {
					t.Fatalf("a click on the heading's %s gives %+v, want %+v", step.name, h.model.period, step.want)
				}
			}
		})
	}
}

// The key line names none of the dial's keys, at any width, now or past:
// no - + and no < >, which the heading's readout names; n now alone while
// a past period shows, in a group of its own after the page's first keys,
// and on a phone's line kept to the last, after the page's other keys, P
// pool among them. Where the wide line doesn't fit, p pin goes before a
// phone's line takes its place.
func TestTheKeyLineNamesNoneOfTheDialsKeysAtEveryWidth(t *testing.T) {
	const (
		ends     = "    p pin    ? help  q quit"
		detail   = "↑↓ account  enter its page"
		project  = "↑↓ project   → its sessions   "
		sessions = "↑↓ session   enter its page   "
	)
	tests := []struct {
		tab   string
		past  bool
		pools bool
		width int
		want  string
	}{
		{tab: "Detail", width: 173, want: "tab views   [ ] view    " + detail + ends},
		{tab: "Detail", width: 160, want: "tab views   [ ] view    " + detail + ends},
		{tab: "Detail", width: 138, want: "tab views   [ ] view    " + detail + ends},
		{tab: "Detail", width: 108, want: "tab views   [ ] view    " + detail + ends},
		{tab: "Detail", width: 104, want: "tab views   [ ] view    " + detail + ends},
		{tab: "Detail", width: 73, want: "tab views   [ ] view   ? keys   q quit"},
		{tab: "Detail", width: 52, want: "tab views   [ ] view   ? keys   q quit"},
		{tab: "Detail", past: true, width: 173, want: "tab views   [ ] view    n now    " + detail + ends},
		{tab: "Detail", past: true, width: 160, want: "tab views   [ ] view    n now    " + detail + ends},
		{tab: "Detail", past: true, width: 138, want: "tab views   [ ] view    n now    " + detail + ends},
		{tab: "Detail", past: true, width: 108, want: "tab views   [ ] view    n now    " + detail + ends},
		{tab: "Detail", past: true, width: 104, want: "tab views   [ ] view    n now    " + detail + ends},
		{tab: "Detail", past: true, width: 73, want: "tab views   [ ] view   n now   ? keys   q quit"},
		{tab: "Detail", past: true, width: 52, want: "tab views   [ ] view   n now   ? keys   q quit"},
		{tab: "Detail", past: true, pools: true, width: 90, want: "tab views   [ ] view   P pool   n now   ? keys   q quit"},
		{tab: "Detail", past: true, pools: true, width: 73, want: "tab views   [ ] view   P pool   n now   ? keys   q quit"},
		{tab: "Projects", width: 173, want: "tab views   " + project + "p pin   ? help  q quit"},
		{tab: "Projects", width: 138, want: "tab views   " + project + "p pin   ? help  q quit"},
		{tab: "Projects", width: 104, want: "tab views   " + project + "p pin   ? help  q quit"},
		{tab: "Projects", width: 73, want: "tab views   ↑↓ project   → sessions   ? keys   q quit"},
		{tab: "Projects", width: 52, want: "↑↓ project   → sessions   ? keys   q quit"},
		{tab: "Projects", past: true, width: 173, want: "tab views   " + project + "n now   p pin   ? help  q quit"},
		{tab: "Projects", past: true, width: 160, want: "tab views   " + project + "n now   p pin   ? help  q quit"},
		{tab: "Projects", past: true, width: 108, want: "tab views   " + project + "n now   p pin   ? help  q quit"},
		{tab: "Projects", past: true, width: 104, want: "tab views   " + project + "n now   p pin   ? help  q quit"},
		{tab: "Projects", past: true, width: 73, want: "tab views   ↑↓ project   → sessions   n now   ? keys   q quit"},
		{tab: "Projects", past: true, width: 52, want: "tab views   ↑↓ project   n now   ? keys   q quit"},
		{tab: "Projects", past: true, pools: true, width: 90, want: "tab views   ↑↓ project   → sessions   P pool   n now   ? keys   q quit"},
		{tab: "Projects", past: true, pools: true, width: 73, want: "tab views   ↑↓ project   → sessions   P pool   n now   ? keys   q quit"},
		{tab: "the List", width: 160, want: "tab views   [ ] view   " + sessions + "p pin   ? help  q quit"},
		{tab: "the List", width: 108, want: "tab views   [ ] view   " + sessions + "p pin   ? help  q quit"},
		{tab: "the List", width: 104, want: "tab views   [ ] view   " + sessions + "p pin   ? help  q quit"},
		{tab: "the List", width: 73, want: "tab views   ↑↓ session   [ ] view   ? keys   q quit"},
		{tab: "the List", width: 52, want: "tab views   ↑↓ session   [ ] view   ? keys   q quit"},
		{tab: "the List", past: true, width: 173, want: "tab views   [ ] view   n now   " + sessions + "p pin   ? help  q quit"},
		{tab: "the List", past: true, width: 138, want: "tab views   [ ] view   n now   " + sessions + "p pin   ? help  q quit"},
		{tab: "the List", past: true, width: 108, want: "tab views   [ ] view   n now   " + sessions + "p pin   ? help  q quit"},
		{tab: "the List", past: true, width: 104, want: "tab views   [ ] view   n now   " + sessions + "p pin   ? help  q quit"},
		{tab: "the List", past: true, width: 73, want: "tab views   ↑↓ session   [ ] view   n now   ? keys   q quit"},
		{tab: "the List", past: true, width: 52, want: "tab views   ↑↓ session   n now   ? keys   q quit"},
		{tab: "the List", past: true, pools: true, width: 90, want: "tab views   ↑↓ session   [ ] view   P pool   n now   ? keys   q quit"},
		{tab: "the List", past: true, pools: true, width: 73, want: "tab views   ↑↓ session   [ ] view   P pool   n now   ? keys   q quit"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s at %d, past: %t, pools: %t", tt.tab, tt.width, tt.past, tt.pools), func(t *testing.T) {
			i := slices.IndexFunc(dialTabs, func(tab struct {
				name string
				show func(*harness)
			}) bool {
				return tab.name == tt.tab
			})
			h := dialHarness(t, dialTabs[i].show, &fakePrefs{})
			if tt.pools {
				h.model.cfg.Pool.Pools = 2
				h.model.cfg.Turn = func(context.Context, string, PoolAsk) (Pooled, error) { return Pooled{}, nil }
			}
			h.update(tea.WindowSizeMsg{Width: tt.width, Height: 50})
			if tt.past {
				h.keyed("<")
			}
			rows := strings.Split(h.view(), "\n")
			if got := strings.TrimRight(rows[len(rows)-1], " "); got != tt.want {
				t.Errorf("the key line reads\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// A dial tab's wide line that doesn't fit leaves p pin out, every other key
// kept in its group, before a phone's line takes its place.
func TestADialTabsWideLineLeavesPPinOutFirst(t *testing.T) {
	groups := [][]dashboard.Key{{tabViews}, {{Key: "[ ]", Does: "view"}}, {{Key: "n", Does: "now"}}, {pinWord, poolKey}, {helpKey, dashboard.Quit}}
	want := [][]dashboard.Key{{tabViews}, {{Key: "[ ]", Does: "view"}}, {{Key: "n", Does: "now"}}, {poolKey}, {helpKey, dashboard.Quit}}
	if got := dialNarrower(groups); len(got) != 1 || !slices.EqualFunc(got[0], want, slices.Equal) {
		t.Errorf("the narrower forms are %+v, want one, %+v", got, want)
	}
	if len(groups[3]) != 2 {
		t.Errorf("dialNarrower changed the groups given, %+v", groups)
	}
}
