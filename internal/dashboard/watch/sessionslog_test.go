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

// onTheList reports whether the List shows the session with the given id,
// where a click would land on it.
func onTheList(h *harness, id string) bool {
	return slices.ContainsFunc(h.targets(), func(t dashboard.Target) bool { return t.On == dashboard.OnSession && t.Session == id })
}

// steppedWaiting is a List of 30 days at 160 by 16, the clock at the time
// given, its first session chosen and scrolled out of view, six rows down,
// that id.
func steppedWaiting(t *testing.T, at time.Time) (*harness, string) {
	t.Helper()
	h := withDelta(projectsHarness(t, dashboard.Sessions))
	h.clock.now = at
	h.start()
	h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: 16})...)
	h.keyed("] >")
	h.keys(downKey)
	first := h.model.sessionsTab.shown.Chosen.Session
	h.keyed("j j j j j j")
	if first == "" || onTheList(h, first) {
		t.Fatalf("scrolled six rows, %q chosen, in view %v; want the first chosen, out of view", first, onTheList(h, first))
	}
	return h, first
}

// A step whose log waits on a read keeps the session chosen in view once
// the read lands: to 12 weeks, the days read reaching 30 days back, the log
// is let go for its read, and built as it lands, the session revealed.
func TestAStepAwaitingTheDaysReadRevealsTheChosenSessionAsItLands(t *testing.T) {
	h, first := steppedWaiting(t, start)
	held := holdingBack[daysReadMsg](h, h.press(">")...)
	if len(held) != 1 || h.model.sessionsTab.shown.Log != nil {
		t.Fatalf("stepped to 12 weeks, %d reads held, its log %v; want the read held, no log", len(held), h.model.sessionsTab.shown.Log)
	}
	h.deliver(held...)
	if h.model.period != views.PeriodTwelveWeeks || h.model.sessionsTab.shown.Chosen.Session != first || !onTheList(h, first) {
		t.Errorf("the read landed, %q shown, %s chosen, in view %v; want 12 weeks, %s chosen and in view", h.model.period.Name(), h.model.sessionsTab.shown.Chosen.Session, onTheList(h, first), first)
	}
}

// A step back to the period the log was last built of, before the step's
// read lands, builds it as it was, the List where it was left.
func TestAStepBackBeforeTheReadLandsLeavesTheListWhereItWas(t *testing.T) {
	h, first := steppedWaiting(t, start)
	scroll := h.model.scroll
	holdingBack[daysReadMsg](h, h.press(">")...)
	h.keyed("<")
	if h.model.period != views.PeriodThirtyDays || h.model.sessionsTab.shown.Log == nil || h.model.scroll != scroll || h.model.sessionsTab.shown.Chosen.Session != first {
		t.Errorf("stepped back before the read landed, %q shown, log %v, scroll %d, %s chosen; want 30 days built, scroll %d, %s chosen", h.model.period.Name(), h.model.sessionsTab.shown.Log != nil, h.model.scroll, h.model.sessionsTab.shown.Chosen.Session, scroll, first)
	}
}

// Two steps taken while the first's read is out keep the session chosen in
// view once the log is built of the second.
func TestTwoStepsAwaitingTheirReadsRevealTheChosenSessionAsTheLogIsBuilt(t *testing.T) {
	h, first := steppedWaiting(t, start)
	held := holdingBack[daysReadMsg](h, h.press(">")...)
	held = append(held, holdingBack[daysReadMsg](h, h.press(">")...)...)
	if h.model.sessionsTab.shown.Log != nil {
		t.Fatalf("stepped twice, its log %v; want no log while its reads are out", h.model.sessionsTab.shown.Log)
	}
	h.deliver(held...)
	if h.model.period != views.PeriodTwelveMonths || h.model.sessionsTab.shown.Chosen.Session != first || !onTheList(h, first) {
		t.Errorf("the reads landed, %q shown, %s chosen, in view %v; want 12 months, %s chosen and in view", h.model.period.Name(), h.model.sessionsTab.shown.Chosen.Session, onTheList(h, first), first)
	}
}

// A step past midnight, the tail yet to be read on the day, keeps the
// session chosen in view once the tail's read lands: the log is let go for
// it, and built as it lands, the session revealed.
func TestAStepPastMidnightRevealsTheChosenSessionAsTheTailsReadLands(t *testing.T) {
	h, first := steppedWaiting(t, onTheDay(23, 59, 30))
	h.clock.now = onTheDay(24, 0, 5)
	held := holdingBack[tailedMsg](h, h.press(">")...)
	if len(held) != 1 || h.model.sessionsTab.shown.Log != nil {
		t.Fatalf("stepped past midnight, %d reads of the tail held, its log %v; want the read held, no log", len(held), h.model.sessionsTab.shown.Log)
	}
	h.deliver(held...)
	if h.model.period != views.PeriodTwelveWeeks || h.model.sessionsTab.shown.Chosen.Session != first || !onTheList(h, first) {
		t.Errorf("the tail's read landed, %q shown, %s chosen, in view %v; want 12 weeks, %s chosen and in view", h.model.period.Name(), h.model.sessionsTab.shown.Chosen.Session, onTheList(h, first), first)
	}
}

// A period stepped on the List keeps the session chosen in view, where the
// new period's log lists it, as Projects keeps its place; and where it
// doesn't, the List shows from its top, nothing chosen.
func TestASteppedPeriodKeepsTheChosenSessionInView(t *testing.T) {
	h := withDelta(projectsHarness(t, dashboard.Sessions))
	h.start()
	h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: 16})...)
	h.keyed("] >")
	ids := loggedIDs(h)
	for range ids {
		h.keys(downKey)
	}
	last := ids[len(ids)-1]
	if h.model.scroll == 0 || h.model.sessionsTab.shown.Chosen.Session != last {
		t.Fatalf("the last chosen, %s, the List scrolled %d; want it chosen, scrolled to it", h.model.sessionsTab.shown.Chosen.Session, h.model.scroll)
	}
	inView := func() bool {
		return slices.ContainsFunc(h.targets(), func(t dashboard.Target) bool { return t.On == dashboard.OnSession && t.Session == last })
	}
	h.keyed(">")
	if h.model.period != views.PeriodTwelveWeeks || h.model.sessionsTab.shown.Chosen.Session != last || !inView() {
		t.Errorf("stepped to %q, %s chosen, in view %v; want 12 weeks, %s chosen and in view", h.model.period.Name(), h.model.sessionsTab.shown.Chosen.Session, inView(), last)
	}
	h.keyed("< < < <")
	if h.model.period != views.PeriodToday || h.model.sessionsTab.shown.Chosen != (dashboard.Plug{}) || h.model.scroll != 0 {
		t.Errorf("stepped to %q, %+v chosen, scrolled %d; want today, none chosen, the List from its top", h.model.period.Name(), h.model.sessionsTab.shown.Chosen, h.model.scroll)
	}
}
