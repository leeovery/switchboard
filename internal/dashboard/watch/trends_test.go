package watch

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// trendsHarness is pastHarness opening on Trends, with the sections
// named shown, its preferences kept by the fake it returns, its ledger
// holding yesterday's requests on work, in a terminal 173 cells wide and 53
// tall. It has started.
func trendsHarness(t *testing.T, sections ...string) (*harness, *fakePrefs) {
	t.Helper()
	h, l := pastHarness(t, dashboard.Graphs)
	l.days[yesterday] = requested(yesterday, 40)
	prefs := &fakePrefs{}
	h.model.cfg.Prefs, h.model.graphs = prefs, dashboard.GraphsSectionsNamed(sections)
	h.start()
	h.update(tea.WindowSizeMsg{Width: 173, Height: 53})
	return h, prefs
}

// Opening in place of a tab kept that's gone, Trends keeps itself as the tab
// shown and its sections shown, as its keys keep them, so the next watch
// opens on them though the tab kept is gone; else it keeps nothing as it
// opens.
func TestTrendsInPlaceOfAGoneTabKeepsItselfAsItOpens(t *testing.T) {
	for _, keep := range []bool{true, false} {
		h := &harness{t: t, clock: &fakeClock{now: start}, source: &fakeSource{doc: calm()}, notifier: &fakeNotifier{}}
		prefs := &fakePrefs{kept: theme.Prefs{Tab: "history"}}
		cfg := h.config(Size{Width: 160, Height: 37})
		cfg.Prefs, cfg.View, cfg.KeepGraphs = prefs, dashboard.Graphs, keep
		cfg.Graphs = dashboard.GraphsSectionsNamed(nil).Toggled(dashboard.GraphsYear)
		h.model = New(t.Context(), cfg)
		h.start()

		want, updates := theme.Prefs{Tab: "history"}, 0
		if keep {
			want, updates = theme.Prefs{Tab: "graphs", GraphsTab: theme.GraphsPrefs{Sections: []string{"week", "year"}}}, 1
		}
		if prefs.updates != updates || prefs.kept.Tab != want.Tab || !slices.Equal(prefs.kept.GraphsTab.Sections, want.GraphsTab.Sections) {
			t.Errorf("to keep %t, opening kept %+v in %d writes; want %+v in %d", keep, prefs.kept, prefs.updates, want, updates)
		}
	}
}

// onPage reports whether the heading given is on the page, as the screen
// shows it scrolled a page at a time from its top to its foot; it's left at
// its top.
func onPage(h *harness, heading string) bool {
	h.t.Helper()
	pgDown, pgUp := tea.KeyPressMsg{Code: tea.KeyPgDown}, tea.KeyPressMsg{Code: tea.KeyPgUp}
	found := false
	for range 6 {
		found = found || strings.Contains(h.view(), "\n"+heading+" ")
		h.keys(pgDown)
	}
	for range 6 {
		h.keys(pgUp)
	}
	return found
}

// y, m and d each show their section where it's hidden and hide it where it
// shows, as Graphs' own keys do, and the sections shown are kept by name.
func TestTrendsYearMonthsAndModelsKeysShowOrHideThemAndKeepThem(t *testing.T) {
	h, prefs := trendsHarness(t, "week")
	for _, step := range []struct {
		key, heading string
		want         []string
		shown        bool
	}{
		{key: "y", heading: "YEAR", want: []string{"week", "year"}, shown: true},
		{key: "m", heading: "BY MONTH", want: []string{"week", "year", "months"}, shown: true},
		{key: "d", heading: "MODELS BY DAY", want: []string{"week", "year", "months", "models"}, shown: true},
		{key: "w", heading: "WEEK", want: []string{"year", "months", "models"}},
		{key: "y", heading: "YEAR", want: []string{"months", "models"}},
		{key: "m", heading: "BY MONTH", want: []string{"models"}},
		{key: "d", heading: "MODELS BY DAY", want: []string{}},
	} {
		h.typed(step.key)
		if got := h.model.graphs.Names(); !slices.Equal(got, step.want) || !slices.Equal(prefs.kept.GraphsTab.Sections, step.want) {
			t.Errorf("after %s, showing %q, kept %#v; want %q shown and kept", step.key, got, prefs.kept.GraphsTab.Sections, step.want)
		}
		if got := onPage(h, step.heading); got != step.shown {
			t.Errorf("after %s, %s's heading on the page %t, want %t", step.key, step.heading, got, step.shown)
		}
	}
}

// Trends builds YEAR's and BY MONTH's page, and MODELS BY DAY's, of the
// days the watch's reads hold while one of them shows, and none while none
// does: hidden again, a look reads no days, and what's built is left as it
// was.
func TestTrendsBuildsItsPagesOfTheDaysReadWhileTheyShow(t *testing.T) {
	h, _ := trendsHarness(t, "week")
	if h.model.trends.year != nil || h.model.trends.days != nil {
		t.Fatalf("with WEEK alone, Trends built YEAR's page %v and MODELS BY DAY's %v, want neither", h.model.trends.year, h.model.trends.days)
	}
	h.typed("m", "d")
	g := h.model.frame(h.clock.now).Graphs
	if g.Year == nil || g.Days == nil || !g.Read {
		t.Fatalf("with BY MONTH and MODELS BY DAY shown, the frame's pages are %v and %v, read %t; want both built", g.Year, g.Days, g.Read)
	}
	if got := g.Year.Numbers.Requests; got != 40 {
		t.Errorf("YEAR's page counts %d requests, want the 40 the ledger holds", got)
	}

	h.typed("m", "d")
	l, built := h.model.cfg.Ledger.(*daysLedger), h.model.trends
	reads := len(l.reads)
	today := dayfile.DateOf(start)
	l.days[today] = requested(today, 7)
	h.lookAt(start.Add(interval))
	h.update(frameMsg{gen: h.model.frames})
	if len(l.reads) != reads || h.model.trends.year != built.year || h.model.trends.days != built.days {
		t.Errorf("BY MONTH and MODELS BY DAY hidden again, a look read the ledger %d times more and built YEAR's page afresh %t, MODELS BY DAY's %t; want no read and neither built",
			len(l.reads)-reads, h.model.trends.year != built.year, h.model.trends.days != built.days)
	}
}

// A frame with nothing YEAR is built of changed builds nothing and asks the
// days for none of their versions: a version v shows that the days didn't
// use is left as it is, where a check would show all use; once the days
// held change, it's checked, and YEAR shows all use.
func TestTrendsChecksWhatYearShowsOnlyWhereItsDaysChange(t *testing.T) {
	h, _ := trendsHarness(t, "year")
	unused := views.YearAsk{Version: "never-used"}
	h.model.trends.split, h.model.trends.yearOf.split = unused, unused
	built := h.model.trends.year
	h.update(frameMsg{gen: h.model.frames})
	if h.model.trends.split != unused || h.model.trends.year != built {
		t.Fatalf("a frame with nothing changed left YEAR showing %+v, built afresh %t; want %+v left as it was, nothing built", h.model.trends.split, h.model.trends.year != built, unused)
	}
	h.model.past.held++
	h.update(frameMsg{gen: h.model.frames})
	if h.model.trends.split != (views.YearAsk{}) || h.model.trends.year == built {
		t.Errorf("the days held changed, YEAR shows %+v, built afresh %t; want all use, built afresh", h.model.trends.split, h.model.trends.year != built)
	}
}

// v cycles what YEAR shows, all use, each version, then each account, the
// key line saying what it shows; with one account shown, ← and → step
// between the accounts, round, and the key line lists them. Without YEAR or
// BY MONTH, v does nothing.
func TestTrendsVCyclesWhatYearShowsAndArrowsStepTheAccounts(t *testing.T) {
	h, _ := trendsHarness(t, "week")
	h.typed("v")
	if h.model.trends.split != (views.YearAsk{}) {
		t.Fatalf("with YEAR hidden, v showed %+v, want all use still", h.model.trends.split)
	}
	h.typed("y")
	choices := []views.YearAsk{{}}
	for _, v := range h.model.trends.year.Versions {
		choices = append(choices, views.YearAsk{Version: v})
	}
	choices = append(choices, views.YearAsk{Account: h.model.trends.year.Accounts[0]})
	for _, want := range choices[1:] {
		h.typed("v")
		if h.model.trends.split != want {
			t.Fatalf("after v, YEAR shows %+v, want %+v", h.model.trends.split, want)
		}
	}
	keyLine := func() string {
		rows := strings.Split(h.view(), "\n")
		return rows[len(rows)-1]
	}
	if line := keyLine(); !strings.Contains(line, "v split: Work") || !strings.Contains(line, "←→ account") {
		t.Errorf("YEAR shown for work, the key line reads %q, want v split: Work and ←→ account", line)
	}
	for _, step := range []struct {
		key  tea.KeyPressMsg
		want string
	}{{rightKey, "personal"}, {rightKey, "side"}, {rightKey, "work"}, {leftKey, "side"}} {
		h.keys(step.key)
		if got := h.model.trends.split.Account; got != step.want {
			t.Errorf("after %s, YEAR shows %q, want %q", step.key, got, step.want)
		}
	}
}
