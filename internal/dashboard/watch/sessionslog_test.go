package watch

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// listHarness is projectsHarness opening on Sessions' List, started, its
// period the dial's last 30 days, and the size given.
func listHarness(t *testing.T, size Size) *harness {
	t.Helper()
	h := projectsHarness(t, dashboard.Sessions)
	h.start()
	h.deliver(h.update(tea.WindowSizeMsg{Width: size.Width, Height: size.Height})...)
	h.keyed("] >")
	return h
}

// loggedIDs are the ids of the List's log's sessions, in its order.
func loggedIDs(h *harness) []string {
	var ids []string
	if log := h.model.sessionsTab.shown.Log; log != nil {
		for _, s := range log.Sessions {
			ids = append(ids, s.Session)
		}
	}
	return ids
}

// The List lists every session of the dial's period, each ended under the
// day it ended, the latest ended first: over 30 days, today's three, then
// gamma's of yesterday; today, today's alone, as today's List lists them.
func TestTheListLogsThePeriodsSessions(t *testing.T) {
	h := listHarness(t, Size{Width: 160, Height: 50})
	if got, want := loggedIDs(h), []string{betaOnly, alphaSecond, alphaFirst, gammaOnly}; !slices.Equal(got, want) {
		t.Fatalf("over 30 days, the List lists %v, want %v", got, want)
	}
	if view := h.view(); !strings.Contains(view, "ended today") || !strings.Contains(view, "ended yesterday") || !strings.Contains(view, "< 30 days >") {
		t.Errorf("over 30 days, the screen reads\n%s\nwant ended today and ended yesterday ruled, and the dial at 30 days", view)
	}
	h.keyed("< <")
	if got, want := loggedIDs(h), []string{betaOnly, alphaSecond, alphaFirst}; h.model.period != views.PeriodToday || !slices.Equal(got, want) {
		t.Fatalf("today, the List lists %v, want %v", got, want)
	}
	var today []string
	for _, s := range heldList(h).Sessions {
		today = append(today, s.Session)
	}
	if got := loggedIDs(h); !slices.Equal(got, today) {
		t.Errorf("today, the List lists %v, want today's List as it was, %v", got, today)
	}
}

// One read of the period's days serves Projects and the List alike: shown
// one after the other, either way round, the days are read once.
func TestOneReadServesProjectsAndTheList(t *testing.T) {
	h := projectsHarness(t, dashboard.Projects)
	h.start()
	if h.model.sessionDays.asked != 1 || h.model.projects.shown.Page == nil {
		t.Fatalf("on Projects, the days were read %d times, its page %v; want once, built", h.model.sessionDays.asked, h.model.projects.shown.Page)
	}
	h.keys(tabKey, tabKey)
	h.keyed("]")
	if h.model.sessionDays.asked != 1 || h.model.sessionsTab.shown.Log == nil {
		t.Errorf("on the List after Projects, the days were read %d times, its log %v; want none again, the log built", h.model.sessionDays.asked, h.model.sessionsTab.shown.Log)
	}

	h = listHarness(t, Size{Width: 160, Height: 50})
	asked := h.model.sessionDays.asked
	h.keys(shiftTabKey, shiftTabKey)
	if h.model.view != dashboard.Projects || h.model.sessionDays.asked != asked || h.model.projects.shown.Page == nil {
		t.Errorf("on %s after the List, the days were read %d times, its page %v; want Projects, none read again, its page built", h.model.view, h.model.sessionDays.asked-asked, h.model.projects.shown.Page)
	}
}

// Till the days are read, the List draws no log, its heading saying it's
// loading, as Projects draws it; once they land, it draws its log.
func TestTheListLoadsAsProjectsDoes(t *testing.T) {
	h := projectsHarness(t, dashboard.Sessions)
	h.start()
	held := holdingBack[daysReadMsg](h, h.press("]")...)
	if len(held) != 1 || h.model.sessionsTab.shown.Log != nil || !h.model.awaiting() {
		t.Fatalf("its read under way, %d held, its log %v, awaiting %v; want one read held, no log, the List awaiting it", len(held), h.model.sessionsTab.shown.Log, h.model.awaiting())
	}
	h.deliver(held...)
	if h.model.sessionsTab.shown.Log == nil || h.model.awaiting() {
		t.Errorf("its read landed, its log %v, awaiting %v; want its log built", h.model.sessionsTab.shown.Log, h.model.awaiting())
	}
}

// The dial shows on the List alone: there < and > step the shared period,
// and the key line lists them; on the Switchboard, they step nothing, and
// the key line leaves them out.
func TestTheDialStepsOnTheListAlone(t *testing.T) {
	h := projectsHarness(t, dashboard.Sessions)
	h.start()
	h.keyed(">")
	if line := h.keyLine(); h.model.period != views.PeriodThisWeek || strings.Contains(line, "< >") {
		t.Errorf("on the Switchboard, > left %q, the key line reading %q; want this week, no < >", h.model.period.Name(), line)
	}
	h.keyed("] >")
	if line := h.keyLine(); h.model.period != views.PeriodThirtyDays || !strings.Contains(line, "< > period: 30 days") {
		t.Errorf("on the List, > left %q, the key line reading %q; want 30 days, < > period: 30 days", h.model.period.Name(), line)
	}
}

// ↑ and ↓ choose a session down the whole log, past the day rules, the List
// scrolling to keep the one chosen in view; enter opens its page, ended on a
// day gone as much as running.
func TestTheListsChoiceStaysInViewAndOpensAnEndedSession(t *testing.T) {
	h := withDelta(projectsHarness(t, dashboard.Sessions))
	h.start()
	h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: 16})...)
	h.keyed("] >")
	ids := loggedIDs(h)
	if len(ids) != 10 {
		t.Fatalf("over 30 days, the List lists %v, want ten sessions", ids)
	}
	inView := func() bool {
		chosen := h.model.sessionsTab.shown.Chosen.Session
		return slices.ContainsFunc(h.targets(), func(t dashboard.Target) bool { return t.On == dashboard.OnSession && t.Session == chosen })
	}
	for i, id := range ids {
		h.keys(downKey)
		if got := h.model.sessionsTab.shown.Chosen.Session; got != id || !inView() {
			t.Fatalf("after %d ↓, %s chosen, in view %v; want %s, in view", i+1, got, inView(), id)
		}
	}
	if h.model.scroll == 0 {
		t.Errorf("the last chosen, the List is scrolled %d, want it scrolled down to it", h.model.scroll)
	}
	last, opening := ids[len(ids)-1], h.model.opening
	h.keys(enterKey)
	if h.model.opening != opening+1 || h.model.session == nil || h.model.session.id != last {
		t.Errorf("enter on %s, ended ten days back, asked %d pages, opened %+v; want its page", last, h.model.opening-opening, h.model.session)
	}
	h.keys(escKey)
	for range ids {
		h.keys(upKey)
	}
	if h.model.scroll != 0 || !inView() {
		t.Errorf("back at the first, the List is scrolled %d, the choice in view %v; want it at its top, in view", h.model.scroll, inView())
	}
}

// The List's log is built afresh only as what it's built of changes: never
// at a look that changes none of it, and once as the period steps.
func TestTheListsLogIsBuiltOnlyAsWhatItsBuiltOfChanges(t *testing.T) {
	h := listHarness(t, Size{Width: 160, Height: 50})
	logs := h.model.sessionsTab.logs
	h.clock.now = h.clock.now.Add(time.Second)
	h.read(routerDocument(three()...))
	if got := h.model.sessionsTab.logs; got != logs {
		t.Errorf("a look on, nothing changed, the log was built %d times more, want none", got-logs)
	}
	h.keyed("<")
	if got := h.model.sessionsTab.logs; got != logs+1 {
		t.Errorf("the period stepped, the log was built %d times more, want once", got-logs)
	}
}
