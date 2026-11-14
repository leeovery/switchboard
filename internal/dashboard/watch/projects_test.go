package watch

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// projectsLedger reads as a request ledger holding the lines given, each on
// the local day it arrived on.
type projectsLedger struct {
	ledger.Empty
	lines map[string][]ledger.Held
}

func (l *projectsLedger) Day(date string) []ledger.Held {
	return l.lines[date]
}

// The sessions of Projects' tests: two of alpha's, the one worth most; one
// of beta's; and one of gamma's, yesterday alone.
const (
	alphaFirst  = "a1000000-0000-4000-8000-000000000001"
	alphaSecond = "a2000000-0000-4000-8000-000000000002"
	betaOnly    = "b1000000-0000-4000-8000-000000000003"
	gammaOnly   = "c1000000-0000-4000-8000-000000000004"
)

// deltaSessions are delta's six sessions, ten days back: worth more
// together than alpha's, so top over 30 days, and gone from this week's
// list.
var deltaSessions = []string{
	"d1000000-0000-4000-8000-000000000001", "d2000000-0000-4000-8000-000000000002", "d3000000-0000-4000-8000-000000000003",
	"d4000000-0000-4000-8000-000000000004", "d5000000-0000-4000-8000-000000000005", "d6000000-0000-4000-8000-000000000006",
}

// projectAsks are n requests of the session given, in the project given, on
// work, a minute apart from at.
func projectAsks(session, project string, at time.Time, n int) []ledger.Held {
	lines := make([]ledger.Held, n)
	for i := range lines {
		t := at.Add(time.Duration(i) * time.Minute).UTC()
		lines[i] = ledger.Held{
			At: t, Request: fmt.Sprintf("%s-%d", session, t.UnixNano()), Session: session, Kind: ledger.KindMessage, Dir: project, Project: project,
			Account: "work", Model: opus, Status: 200, Class: ledger.ClassMain, Usage: json.RawMessage(hourUsage),
		}
	}
	return lines
}

// projectsHarness is a model of the router of three, weeks starting on
// Monday, the day the clock starts on, opening on the tab given in a
// terminal 160 cells wide and 50 tall, over a ledger holding alpha's four
// requests and beta's two today, and gamma's one yesterday. It hasn't
// started.
func projectsHarness(t *testing.T, opening dashboard.View) *harness {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
	l := &projectsLedger{Empty: h.emptyLedger(), lines: map[string][]ledger.Held{}}
	for _, lines := range [][]ledger.Held{
		projectAsks(alphaFirst, "/code/alpha", start.Add(-3*time.Hour), 3),
		projectAsks(alphaSecond, "/code/alpha", start.Add(-2*time.Hour), 1),
		projectAsks(betaOnly, "/code/beta", start.Add(-time.Hour), 2),
		projectAsks(gammaOnly, "/code/gamma", start.AddDate(0, 0, -1), 1),
	} {
		date := dayfile.DateOf(lines[0].At)
		l.lines[date] = append(l.lines[date], lines...)
	}
	cfg := h.config(Size{Width: 160, Height: 50})
	cfg.Ledger, cfg.Accounts, cfg.WeekStarts, cfg.View = l, threeConfigured, time.Monday, opening
	h.model = New(t.Context(), cfg)
	return h
}

// withDelta is h with delta's sessions on its ledger, as deltaSessions has
// them, a request each, an hour apart. It's given before h starts.
func withDelta(h *harness) *harness {
	l := h.model.cfg.Ledger.(*projectsLedger)
	for i, id := range deltaSessions {
		lines := projectAsks(id, "/code/delta", start.AddDate(0, 0, -10).Add(time.Duration(i)*time.Hour), 1)
		date := dayfile.DateOf(lines[0].At)
		l.lines[date] = append(l.lines[date], lines...)
	}
	return h
}

// running is the router listing the session given running in the project
// given, from now, its look at it taken in.
func running(h *harness, id, project string) {
	h.t.Helper()
	h.source.sessions = []status.Session{{ID: id, Assignments: []status.Assignment{
		{Model: opus, Account: "work", Dir: project, Project: project, AssignedAt: h.clock.now, LastSeen: h.clock.now},
	}}}
	h.tickUntilAsked()
}

// selectionInView reports whether SESSIONS shows the session selected,
// where a click would land on it.
func selectionInView(h *harness) bool {
	selected := h.model.projects.shown.Selection()
	return slices.ContainsFunc(h.targets(), func(t dashboard.Target) bool { return t.On == dashboard.OnSession && t.Session == selected })
}

// rowsOf are the paths of the list's rows.
func rowsOf(h *harness) []string {
	var paths []string
	for _, r := range h.model.projects.shown.Page.Rows {
		paths = append(paths, r.Path)
	}
	return paths
}

// Projects reads the sessions' days once, as it's first shown, from the
// start of the period's first day, and again only where the dial reaches
// further back than the days held: never while another tab shows, nor
// shown again, nor for a period within them.
func TestProjectsReadsItsDaysOnceAndAgainOnlyFurtherBack(t *testing.T) {
	h := projectsHarness(t, dashboard.Overview)
	h.start()
	if h.model.projects.asked != 0 {
		t.Fatalf("on the Overview, Projects' days were read %d times, want none", h.model.projects.asked)
	}
	h.keys(tabKey, tabKey)
	now := h.clock.now
	from := func(p views.Period) time.Time { return p.From(now, time.Monday) }
	if h.model.view != dashboard.Projects || h.model.projects.asked != 1 || !h.model.projects.from.Equal(from(views.PeriodThisWeek)) {
		t.Fatalf("on %s, the days were read %d times, from %v; want Projects, read once from %v", h.model.view, h.model.projects.asked, h.model.projects.from, from(views.PeriodThisWeek))
	}
	steps := []struct {
		key    string
		period views.Period
		asked  int
	}{
		{"<", views.PeriodToday, 1},
		{">", views.PeriodThisWeek, 1},
		{">", views.PeriodThirtyDays, 2},
		{"<", views.PeriodThisWeek, 2},
		{">", views.PeriodThirtyDays, 2},
		{">", views.PeriodTwelveWeeks, 3},
		{">", views.PeriodTwelveMonths, 4},
		{"<", views.PeriodTwelveWeeks, 4},
	}
	for _, s := range steps {
		h.deliver(h.press(s.key)...)
		if h.model.period != s.period || h.model.projects.asked != s.asked {
			t.Fatalf("after %s, %q shown, the days were read %d times; want %q, read %d", s.key, h.model.period.Name(), h.model.projects.asked, s.period.Name(), s.asked)
		}
	}
	if want := from(views.PeriodTwelveMonths); !h.model.projects.from.Equal(want) {
		t.Errorf("the days held are from %v, want the furthest back read, %v", h.model.projects.from, want)
	}
	h.keys(tabKey, shiftTabKey)
	if h.model.projects.asked != 4 {
		t.Errorf("shown again, the days were read %d times, want 4: none again", h.model.projects.asked)
	}
}

// Till its days are read, Projects says it's loading and draws no page,
// as it's first shown, and as the dial steps further back than the days
// held, never the new period's name over the old period's figures; once
// they land, its page is built of them.
func TestProjectsLoadsTillItsDaysAreRead(t *testing.T) {
	h := projectsHarness(t, dashboard.Projects)
	reads := []struct {
		name  string
		msgs  func() []tea.Msg
		built string
	}{
		{"as it's first shown", h.init, "/code/alpha /code/beta"},
		{"after a step further back", func() []tea.Msg { return h.press(">") }, "/code/alpha /code/beta /code/gamma"},
	}
	for _, r := range reads {
		held := holdingBack[projectsReadMsg](h, r.msgs()...)
		if s := h.model.projects.shown; len(held) != 1 || !h.model.projects.reading || !h.model.awaiting() || s.Page != nil || s.Project != nil {
			t.Fatalf("%s, its read under way, %d held, reading %v, awaiting %v, its page %v; want one read held, Projects awaiting it, no page", r.name, len(held), h.model.projects.reading, h.model.awaiting(), s.Page)
		}
		if strings.Contains(h.view(), "alpha") {
			t.Errorf("%s, its read under way, the screen draws alpha's row:\n%s\nwant no page", r.name, h.view())
		}
		h.deliver(held...)
		if h.model.projects.reading || h.model.awaiting() || h.model.projects.shown.Page == nil {
			t.Fatalf("%s, its read landed, reading %v, awaiting %v, its page %v; want its page built", r.name, h.model.projects.reading, h.model.awaiting(), h.model.projects.shown.Page)
		}
		if got := strings.Join(rowsOf(h), " "); got != r.built {
			t.Errorf("%s, its read landed, the rows are %s, want %s", r.name, got, r.built)
		}
	}
	held := holdingBack[projectsReadMsg](h, h.press(">")...)
	h.deliver(h.press("<")...)
	if !h.model.projects.reading || h.model.projects.shown.Page == nil || h.model.awaiting() {
		t.Errorf("a step back within the days held as a read further back runs, reading %v, its page %v, awaiting %v; want its page built and drawn, not loading", h.model.projects.reading, h.model.projects.shown.Page, h.model.awaiting())
	}
	h.deliver(held...)
}

// A step further back returns, once its read lands, to where the user was
// before it: the project chosen, ↑ and ↓ in SESSIONS, the session selected
// and in view, though the read draws no page while it's under way. Only
// where the project chosen left the list does the list take the choice, as
// a rebuild that loses it gives it.
func TestProjectsReturnsToWhereTheUserWasOnceAStepBacksReadLands(t *testing.T) {
	t.Run("the project still listed", func(t *testing.T) {
		h := withDelta(projectsHarness(t, dashboard.Projects))
		h.start()
		h.update(tea.WindowSizeMsg{Width: 108, Height: 24})
		h.keys(rightKey, downKey)
		held := holdingBack[projectsReadMsg](h, h.press(">")...)
		if s := h.model.projects.shown; len(held) != 1 || s.Page != nil || s.InSessions {
			t.Fatalf("its read under way, %d held, its page %v, in sessions %v; want no page, as the first read draws", len(held), s.Page, s.InSessions)
		}
		h.deliver(held...)
		if s := h.model.projects.shown; s.Chosen != "/code/alpha" || !s.InSessions || s.Selection() != alphaFirst || !selectionInView(h) {
			t.Errorf("its read landed, %s chosen, in sessions %v, %s selected, in view %v; want alpha, its SESSIONS, %s, in view", s.Chosen, s.InSessions, s.Selection(), selectionInView(h), alphaFirst)
		}
	})
	t.Run("the project gone", func(t *testing.T) {
		h := projectsHarness(t, dashboard.Projects)
		h.start()
		const epsilon = "e1000000-0000-4000-8000-000000000005"
		running(h, epsilon, "/code/epsilon")
		h.keys(downKey, downKey, rightKey)
		if s := h.model.projects.shown; s.Chosen != "/code/epsilon" || !s.InSessions {
			t.Fatalf("after ↓ ↓ →, %s chosen, in sessions %v; want epsilon, its SESSIONS", s.Chosen, s.InSessions)
		}
		held := holdingBack[projectsReadMsg](h, h.press(">")...)
		h.source.sessions = nil
		h.tickUntilAsked()
		h.deliver(held...)
		if s := h.model.projects.shown; s.Chosen != "/code/alpha" || s.InSessions || s.Selected != "" || h.model.scroll != 0 {
			t.Errorf("its read landed, epsilon gone, %s chosen, in sessions %v, %q selected, scrolled %d; want the top row, alpha, the list with the choice, none selected, from its top", s.Chosen, s.InSessions, s.Selected, h.model.scroll)
		}
	})
}

// A step of the dial keeps the session selected where the user was in
// SESSIONS, scrolled as far as it was, past what showing it alone takes, and
// in view: within the days held, as the page is built at once, and further
// back, as the read lands.
func TestProjectsDialStepKeepsTheSelectionScrolledInView(t *testing.T) {
	steps := []struct {
		name string
		step func(h *harness)
	}{
		{"within the days held", func(h *harness) {
			h.deliver(h.press(">")...)
			if h.model.projects.reading {
				h.t.Fatal("the step to 12 weeks asked for a read, want the days held to reach it")
			}
		}},
		{"further back", func(h *harness) {
			held := holdingBack[projectsReadMsg](h, h.press(">")...)
			if len(held) != 1 {
				h.t.Fatalf("the step to 12 weeks held %d reads, want one", len(held))
			}
			h.deliver(held...)
		}},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			h := withDelta(projectsHarness(t, dashboard.Projects))
			h.start()
			h.update(tea.WindowSizeMsg{Width: 108, Height: 24})
			h.deliver(h.press(">")...)
			if st.name == "within the days held" {
				h.deliver(h.press(">")...)
				h.deliver(h.press("<")...)
			}
			h.keys(rightKey)
			for range deltaSessions {
				h.keys(downKey)
			}
			scrolled := h.model.scroll
			h.keys(upKey)
			if s := h.model.projects.shown; s.Chosen != "/code/delta" || s.Selection() != deltaSessions[1] || scrolled == 0 || h.model.scroll != scrolled || !selectionInView(h) {
				t.Fatalf("over 30 days, → and ↓ to the last, then ↑, %s chosen, %s selected, scrolled %d, in view %v; want delta's last but one, still scrolled %d", s.Chosen, s.Selection(), h.model.scroll, selectionInView(h), scrolled)
			}
			st.step(h)
			if s := h.model.projects.shown; h.model.period != views.PeriodTwelveWeeks || s.Chosen != "/code/delta" || !s.InSessions || s.Selection() != deltaSessions[1] || h.model.scroll != scrolled || !selectionInView(h) {
				t.Errorf("over %s, %s chosen, in sessions %v, %s selected, scrolled %d, in view %v; want 12 weeks, delta's SESSIONS, %s, scrolled %d, in view", h.model.period.Name(), s.Chosen, s.InSessions, s.Selection(), h.model.scroll, selectionInView(h), deltaSessions[1], scrolled)
			}
		})
	}
}

// A rebuild scrolls SESSIONS no further than it now goes: a step to a
// period with fewer of its lines leaves it no further down than they go, so
// the step back never brings back the offset it had.
func TestProjectsRebuiltScrollsSessionsNoFurtherThanItGoes(t *testing.T) {
	h := withDelta(projectsHarness(t, dashboard.Projects))
	l := h.model.cfg.Ledger.(*projectsLedger)
	today := projectAsks("d8000000-0000-4000-8000-000000000008", "/code/delta", start.Add(-30*time.Minute), 1)
	l.lines[dayfile.DateOf(start)] = append(l.lines[dayfile.DateOf(start)], today...)
	h.start()
	h.update(tea.WindowSizeMsg{Width: 108, Height: 24})
	h.deliver(h.press(">")...)
	h.keys(downKey, upKey)
	for range deltaSessions {
		h.keys(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	if s := h.model.projects.shown; s.Chosen != "/code/delta" || s.InSessions || h.model.scroll == 0 {
		t.Fatalf("over 30 days, delta chosen by ↓ ↑ and j to SESSIONS' foot, %s chosen, in sessions %v, scrolled %d; want delta, the list with the choice, scrolled", s.Chosen, s.InSessions, h.model.scroll)
	}
	h.deliver(h.press("<")...)
	if s := h.model.projects.shown; s.Chosen != "/code/delta" || h.model.scroll != 0 {
		t.Fatalf("over this week, its one session today, %s chosen, scrolled %d; want delta, scrolled no further than its one line goes, 0", s.Chosen, h.model.scroll)
	}
	h.deliver(h.press(">")...)
	if h.model.scroll != 0 {
		t.Errorf("back over 30 days, SESSIONS scrolled %d, want 0: the offset gone as this week's lines went no further", h.model.scroll)
	}
}

// Without a ledger, Projects reads nothing and awaits nothing: its page is
// built of the sessions running alone, as the List's are, and says there
// are no sessions where none runs.
func TestProjectsWithoutALedgerShowsTheSessionsRunning(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	cfg := h.config(Size{Width: 160, Height: 50})
	cfg.Ledger, cfg.Accounts, cfg.WeekStarts, cfg.View = nil, threeConfigured, time.Monday, dashboard.Projects
	h.model = New(t.Context(), cfg)
	h.start()
	if h.model.projects.asked != 0 || h.model.awaiting() || !strings.Contains(h.view(), "no sessions") {
		t.Fatalf("without a ledger, read %d times, awaiting %v, the screen:\n%s\nwant no read, nothing awaited, and no sessions said", h.model.projects.asked, h.model.awaiting(), h.view())
	}
	running(h, betaOnly, "/code/beta")
	if got := strings.Join(rowsOf(h), " "); got != "/code/beta" || !strings.Contains(h.view(), "beta") {
		t.Errorf("without a ledger, beta's session running, the rows are %q; want beta's alone, drawn", got)
	}
}

// Projects' page is built only as what it's built of changes: the days
// read, the period, or the sessions running; never for a frame, a resize or
// a look that changes none of them. The chosen project's page is built as
// the page is, or the project chosen changes.
func TestProjectsPagesAreBuiltOnlyAsWhatTheyreBuiltOfChanges(t *testing.T) {
	h := projectsHarness(t, dashboard.Projects)
	h.start()
	built := func(pages, projects int, after string) {
		t.Helper()
		if got := h.model.projects; got.pages != pages || got.projects != projects {
			t.Errorf("after %s, the page was built %d times and the project's %d, want %d and %d", after, got.pages, got.projects, pages, projects)
		}
	}
	built(1, 1, "it opened")
	h.update(frameMsg{gen: h.model.frames})
	h.update(tea.WindowSizeMsg{Width: 120, Height: 50})
	h.tickUntilAsked()
	built(1, 1, "a frame, a resize and a look")
	h.keys(downKey)
	built(1, 2, "↓")
	h.deliver(h.press("<")...)
	built(2, 3, "<")
	running(h, betaOnly, "/code/beta")
	built(3, 4, "a session started running")
}

// The project chosen and the session selected are kept, by path and by id,
// across a rebuild that ranks the list afresh; a project that leaves the
// list leaves the top row chosen, the list with the choice, SESSIONS'
// selection let go and SESSIONS from its top.
func TestProjectsKeepsTheChoiceAcrossRebuilds(t *testing.T) {
	h := withDelta(projectsHarness(t, dashboard.Projects))
	h.start()
	h.update(tea.WindowSizeMsg{Width: 108, Height: 24})
	h.deliver(h.press(">")...)
	if got := strings.Join(rowsOf(h), " "); got != "/code/delta /code/alpha /code/beta /code/gamma" {
		t.Fatalf("over 30 days the rows are %s, want delta, alpha, beta and gamma", got)
	}
	h.keys(downKey, rightKey, downKey)
	if s := h.model.projects.shown; s.Chosen != "/code/alpha" || !s.InSessions || s.Selection() != alphaFirst {
		t.Fatalf("after ↓ → ↓, %s chosen, in sessions %v, %s selected; want alpha, second, its SESSIONS, its first ended last", s.Chosen, s.InSessions, s.Selection())
	}
	h.deliver(h.press("<")...)
	if s := h.model.projects.shown; s.Chosen != "/code/alpha" || s.Selection() != alphaFirst || !s.InSessions {
		t.Errorf("rebuilt over this week, alpha first, %s chosen, %s selected, in sessions %v; want both kept", s.Chosen, s.Selection(), s.InSessions)
	}
	h.deliver(h.press(">")...)
	h.keys(leftKey, upKey, rightKey)
	for range deltaSessions {
		h.keys(downKey)
	}
	if s := h.model.projects.shown; s.Chosen != "/code/delta" || s.Selection() != deltaSessions[0] || h.model.scroll == 0 {
		t.Fatalf("after ← ↑ → and ↓ to its last, %s chosen, %s selected, SESSIONS scrolled %d; want delta, its first, listed last, scrolled", s.Chosen, s.Selection(), h.model.scroll)
	}
	h.deliver(h.press("<")...)
	if s := h.model.projects.shown; s.Chosen != "/code/alpha" || s.InSessions || s.Selected != "" || h.model.scroll != 0 {
		t.Errorf("delta gone from this week's list, %s chosen, in sessions %v, %q selected, SESSIONS scrolled %d; want the top row, alpha, the list with the choice, none selected, from its top", s.Chosen, s.InSessions, s.Selected, h.model.scroll)
	}
}

// The project shown is held as chosen however it came to be shown, the top
// row's taken into its SESSIONS as one chosen: a list ranked afresh, as the
// dial steps within the days held, keeps it, never handing SESSIONS to
// another.
func TestProjectsHoldsTheTopRowOnceItsSessionsAreTaken(t *testing.T) {
	takings := []struct {
		name string
		take func(h *harness)
	}{
		{"→", func(h *harness) { h.keys(rightKey) }},
		{"a click on its session", func(h *harness) {
			h.t.Helper()
			i := slices.IndexFunc(h.targets(), func(t dashboard.Target) bool { return t.On == dashboard.OnSession && t.Session == alphaSecond })
			if i < 0 {
				h.t.Fatalf("no target for alpha's second session among %+v", h.targets())
			}
			h.clickOn(h.targets()[i])
		}},
	}
	for _, tk := range takings {
		t.Run(tk.name, func(t *testing.T) {
			h := withDelta(projectsHarness(t, dashboard.Projects))
			h.start()
			h.deliver(h.press(">")...)
			h.deliver(h.press("<")...)
			tk.take(h)
			h.deliver(h.press(">")...)
			if s := h.model.projects.shown; s.Chosen != "/code/alpha" || !s.InSessions {
				t.Errorf("after %s, then over 30 days, delta top, %s chosen, in sessions %v; want alpha kept, its SESSIONS", tk.name, s.Chosen, s.InSessions)
			}
		})
	}
}

// A rebuild that moves the session selected keeps it in view, where it
// showed: a session starting running in its project, listed first, never
// pushes it out of SESSIONS.
func TestProjectsRebuiltKeepsTheSelectionInView(t *testing.T) {
	h := withDelta(projectsHarness(t, dashboard.Projects))
	h.start()
	h.update(tea.WindowSizeMsg{Width: 108, Height: 24})
	h.deliver(h.press(">")...)
	h.keys(rightKey)
	var inView []string
	for _, tg := range h.targets() {
		if tg.On == dashboard.OnSession {
			inView = append(inView, tg.Session)
		}
	}
	if len(inView) == 0 || len(inView) == len(deltaSessions) {
		t.Fatalf("SESSIONS shows %d of delta's %d sessions at 108×24, want some, not all", len(inView), len(deltaSessions))
	}
	for range len(inView) - 1 {
		h.keys(downKey)
	}
	selected := h.model.projects.shown.Selection()
	if selected != inView[len(inView)-1] || h.model.scroll != 0 {
		t.Fatalf("%s selected, scrolled %d; want the last in view, %s, unscrolled", selected, h.model.scroll, inView[len(inView)-1])
	}
	running(h, "d7000000-0000-4000-8000-000000000007", "/code/delta")
	if got := h.model.projects.shown.Selection(); got != selected || !selectionInView(h) {
		t.Errorf("a session of delta's started running, %s selected, in view %v; want %s kept in view", got, selectionInView(h), selected)
	}
}

// ↑ and ↓ choose a project in the list, → takes them into its SESSIONS and
// ← back; there they select a session, and enter opens its page, as it
// doesn't in the list. The key line says what they do where they act.
func TestProjectsKeysMoveTheChoiceBetweenTheListAndSessions(t *testing.T) {
	h := projectsHarness(t, dashboard.Projects)
	h.start()
	if line := h.keyLine(); !strings.Contains(line, "↑↓ project") || !strings.Contains(line, "→ its sessions") || strings.Contains(line, "enter") {
		t.Errorf("in the list, the key line reads %q, want ↑↓ project and → its sessions, no enter", line)
	}
	opening := h.model.opening
	h.keys(enterKey)
	if h.model.opening != opening {
		t.Errorf("enter in the list opened a session's page, want it to do nothing")
	}
	h.keys(downKey)
	if got := h.model.projects.shown.Chosen; got != "/code/beta" {
		t.Fatalf("after ↓, %s chosen, want beta", got)
	}
	h.keys(upKey, rightKey)
	if s := h.model.projects.shown; !s.InSessions || s.Selection() != alphaSecond {
		t.Fatalf("after ↑ →, in sessions %v, %s selected; want SESSIONS, alpha's second, ended last, first", s.InSessions, s.Selection())
	}
	if line := h.keyLine(); !strings.Contains(line, "↑↓ session") || !strings.Contains(line, "← projects") || !strings.Contains(line, "enter its page") {
		t.Errorf("in SESSIONS, the key line reads %q, want ↑↓ session, ← projects and enter its page", line)
	}
	h.keys(downKey)
	if got := h.model.projects.shown.Selection(); got != alphaFirst {
		t.Errorf("after ↓ in SESSIONS, %s selected, want alpha's first", got)
	}
	h.keys(leftKey)
	if s := h.model.projects.shown; s.InSessions || s.Chosen != "/code/alpha" {
		t.Errorf("after ←, in sessions %v, %s chosen; want the list, alpha still chosen", s.InSessions, s.Chosen)
	}
	h.keys(rightKey, enterKey)
	if h.model.opening != opening+1 {
		t.Errorf("enter in SESSIONS asked to open no session's page, want %s's", alphaFirst)
	}
}
