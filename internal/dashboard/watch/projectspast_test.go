package watch

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
)

// pastProjectsSize is a terminal short enough that eps's SESSIONS pages
// its lines, the words after its last session among them.
var pastProjectsSize = Size{Width: 160, Height: 24}

// pastProjectsHarness is projectsHarness on Projects over last week, the
// size given, eps chosen and ↑ and ↓ in its SESSIONS: its Friday's five
// sessions, as epsSessions has them, and a session on its Monday, 21 Sep,
// before the five days the ledger keeps, so SESSIONS says after its rows
// that the days before are no longer kept.
func pastProjectsHarness(t *testing.T, size Size) *harness {
	t.Helper()
	h := projectsHarness(t, dashboard.Projects)
	l := h.model.cfg.Ledger.(*projectsLedger)
	l.write(projectAsks("e0000000-0000-4000-8000-000000000000", "/code/eps", start.AddDate(0, 0, -7), 1)...)
	for i, id := range epsSessions {
		l.write(projectAsks(id, "/code/eps", start.AddDate(0, 0, -3).Add(time.Duration(i)*time.Hour), 1)...)
	}
	h.model.cfg.LedgerKeep = 5 * day
	h.start()
	h.model = h.model.learned(beginning{began: dayfile.DayStart(start.AddDate(0, 0, -7).Local(), 0)})
	h.deliver(h.update(tea.WindowSizeMsg{Width: size.Width, Height: size.Height})...)
	h.keyed("<")
	if view := h.view(); !strings.Contains(view, "< last week >") || h.model.projects.shown.Chosen != "/code/eps" {
		t.Fatalf("Projects reads\n%s\nwant last week, eps chosen", view)
	}
	h.keys(rightKey)
	if !h.model.projects.shown.InSessions {
		t.Fatalf("→ left ↑ and ↓ in the list:\n%s", h.view())
	}
	return h
}

// selectLast steps ↓ down eps's SESSIONS to its last session, failing the
// test where it isn't eps's last.
func selectLast(h *harness) {
	h.t.Helper()
	for range len(epsSessions) + 1 {
		h.keys(downKey)
	}
	if got, want := h.model.projects.shown.Selection(), epsSessions[0]; got != want {
		h.t.Fatalf("↓ to SESSIONS' foot selected %s, want %s, the first started last", got, want)
	}
}

// ↓ onto SESSIONS' last session shows the words after it too, as the
// List's step does, from its top and from midway.
func TestAStepOntoAProjectsLastSessionShowsTheWordsAfterIt(t *testing.T) {
	for _, from := range []int{0, 2} {
		h := pastProjectsHarness(t, pastProjectsSize)
		if view := h.view(); strings.Contains(view, unkeptWords) {
			t.Fatalf("SESSIONS from its top reads\n%s\nwant the words after its rows out of view, for the test to hold", view)
		}
		for range from {
			h.keys(downKey)
		}
		selectLast(h)
		if view := h.view(); !strings.Contains(view, unkeptWords) || !onTheList(h, epsSessions[0]) {
			t.Errorf("↓ to the last from %d down, the screen reads\n%s\nwant it and the words after it", from, view)
		}
	}
}

// A row up with k from SESSIONS' foot, the last session selected and still
// in view, holds there, through the looks after it; ↓ then shows the words
// after it again.
func TestARowUpFromAProjectsFootHoldsWithTheLastSessionSelected(t *testing.T) {
	h := pastProjectsHarness(t, pastProjectsSize)
	selectLast(h)
	most := h.model.scroll
	h.keyed("k")
	h.tickUntil(h.clock.now.Add(5 * time.Second))
	if h.model.scroll != most-1 || !onTheList(h, epsSessions[0]) {
		t.Errorf("k from the foot, scrolled %d, %s in view %v; want %d, it still in view", h.model.scroll, epsSessions[0], onTheList(h, epsSessions[0]), most-1)
	}
	standsOnTheLast(h)
}

// A click on the last session, on SESSIONS' bottom row, selects it where it
// is: SESSIONS doesn't scroll from under the pointer to show the words
// after it; ↓ on it then shows them.
func TestAClickOnAProjectsLastSessionKeepsItUnderThePointer(t *testing.T) {
	h := pastProjectsHarness(t, pastProjectsSize)
	selectLast(h)
	h.keys(leftKey)
	h.keyed("k")
	last := epsSessions[0]
	targets := h.targets()
	at := slices.IndexFunc(targets, func(t dashboard.Target) bool { return t.On == dashboard.OnSession && t.Session == last })
	if at < 0 {
		t.Fatalf("a row up from the foot, ↑ and ↓ in the list, the screen reads\n%s\nwant %s on it", h.view(), last)
	}
	bottom := slices.IndexFunc(targets, func(t dashboard.Target) bool { return t.On == dashboard.OnSession && t.Y > targets[at].Y })
	if bottom >= 0 || strings.Contains(h.view(), unkeptWords) {
		t.Fatalf("a row up from the foot, ↑ and ↓ in the list, the screen reads\n%s\nwant %s on SESSIONS' bottom row, the words after it out of view", h.view(), last)
	}
	target, scroll := targets[at], h.model.scroll
	h.click(target.X, target.Y)
	if h.model.projects.shown.Selection() != last || h.model.scroll != scroll {
		t.Fatalf("a click on %s selected %s, scrolled %d; want it selected, scrolled %d", last, h.model.projects.shown.Selection(), h.model.scroll, scroll)
	}
	if !slices.Contains(h.targets(), target) {
		t.Errorf("after the click, %s isn't at %+v; want it under the pointer", last, target)
	}
	standsOnTheLast(h)
}

// standsOnTheLast presses ↓ on SESSIONS' last session, selected: the
// selection stands at the end, and SESSIONS shows the words after it too,
// as a step onto it does.
func standsOnTheLast(h *harness) {
	h.t.Helper()
	last := epsSessions[0]
	h.keys(downKey)
	if h.model.projects.shown.Selection() != last || !strings.Contains(h.view(), unkeptWords) || !onTheList(h, last) {
		h.t.Errorf("↓ on %s, selected at SESSIONS' end, selected %s and the screen reads\n%s\nwant it still selected, in view, and the words after it shown", last, h.model.projects.shown.Selection(), h.view())
	}
}
