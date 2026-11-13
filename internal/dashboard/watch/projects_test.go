package watch

import (
	"encoding/json"
	"fmt"
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

// Till its days are read, Projects says it's loading and draws no page;
// once they land, its page is built of them.
func TestProjectsLoadsTillItsDaysAreRead(t *testing.T) {
	h := projectsHarness(t, dashboard.Projects)
	held := holdingBack[projectsReadMsg](h, h.init()...)
	if len(held) != 1 || !h.model.projects.reading || !h.model.awaiting() || h.model.projects.shown.Page != nil {
		t.Fatalf("its read under way, %d held, reading %v, awaiting %v, its page %v; want one read held, Projects awaiting it, no page", len(held), h.model.projects.reading, h.model.awaiting(), h.model.projects.shown.Page)
	}
	h.deliver(held...)
	if h.model.projects.reading || h.model.awaiting() || h.model.projects.shown.Page == nil {
		t.Errorf("its read landed, reading %v, awaiting %v, its page %v; want its page built", h.model.projects.reading, h.model.awaiting(), h.model.projects.shown.Page)
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
	h.source.sessions = []status.Session{{ID: betaOnly, Assignments: []status.Assignment{
		{Model: opus, Account: "work", Dir: "/code/beta", Project: "/code/beta", AssignedAt: start, LastSeen: h.clock.now},
	}}}
	h.tickUntilAsked()
	built(3, 4, "a session started running")
}

// The project chosen and the session selected are kept, by path and by id,
// across a rebuild; a project that leaves the list leaves the top row
// chosen.
func TestProjectsKeepsTheChoiceAcrossRebuilds(t *testing.T) {
	h := projectsHarness(t, dashboard.Projects)
	h.start()
	h.deliver(h.press(">")...)
	if got := strings.Join(rowsOf(h), " "); got != "/code/alpha /code/beta /code/gamma" {
		t.Fatalf("over 30 days the rows are %s, want alpha, beta and gamma", got)
	}
	h.keys(rightKey, downKey)
	if s := h.model.projects.shown; !s.InSessions || s.Selection() != alphaFirst {
		t.Fatalf("after → ↓, in sessions %v, %s selected; want SESSIONS, alpha's first ended last", s.InSessions, s.Selection())
	}
	h.deliver(h.press("<")...)
	if s := h.model.projects.shown; s.Chosen != "/code/alpha" || s.Selection() != alphaFirst || !s.InSessions {
		t.Errorf("rebuilt over this week, %s chosen, %s selected, in sessions %v; want both kept", s.Chosen, s.Selection(), s.InSessions)
	}
	h.deliver(h.press(">")...)
	h.keys(leftKey, downKey, downKey)
	if got := h.model.projects.shown.Chosen; got != "/code/gamma" {
		t.Fatalf("after ← ↓ ↓, %s chosen, want gamma", got)
	}
	h.deliver(h.press("<")...)
	if got := h.model.projects.shown.Chosen; got != "/code/alpha" {
		t.Errorf("gamma gone from this week's list, %s chosen, want the top row, alpha", got)
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
