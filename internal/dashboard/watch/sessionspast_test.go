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

// epsSessions are the sessions of last week's Friday, 25 Sep, on the List
// of pastListHarness, a request each, an hour apart.
var epsSessions = []string{
	"e1000000-0000-4000-8000-000000000001", "e2000000-0000-4000-8000-000000000002", "e3000000-0000-4000-8000-000000000003",
	"e4000000-0000-4000-8000-000000000004", "e5000000-0000-4000-8000-000000000005",
}

// unkeptWords are what the List says of last week's days no longer kept.
const unkeptWords = "no longer kept"

// pastListHarness is projectsHarness on Sessions' List of last week, the
// size given: its Friday's five sessions, as epsSessions has them, and a
// session on its Monday, 21 Sep, before the five days the ledger keeps, so
// the List says after its rows that the days before are no longer kept.
func pastListHarness(t *testing.T, size Size) *harness {
	t.Helper()
	h := projectsHarness(t, dashboard.Sessions)
	l := h.model.cfg.Ledger.(*projectsLedger)
	l.write(projectAsks("e0000000-0000-4000-8000-000000000000", "/code/eps", start.AddDate(0, 0, -7), 1)...)
	for i, id := range epsSessions {
		l.write(projectAsks(id, "/code/eps", start.AddDate(0, 0, -3).Add(time.Duration(i)*time.Hour), 1)...)
	}
	h.model.cfg.LedgerKeep = 5 * day
	h.start()
	// projectsLedger tells no first day, so where the work began is told as a
	// read of Accounts' past would learn it: the start of the first line's day.
	h.model = h.model.learned(beginning{began: dayfile.DayStart(start.AddDate(0, 0, -7).Local(), 0)})
	h.deliver(h.update(tea.WindowSizeMsg{Width: size.Width, Height: size.Height})...)
	h.keyed("] <")
	if !strings.Contains(h.view(), "< last week >") {
		t.Fatalf("the List reads\n%s\nwant last week", h.view())
	}
	return h
}

// chooseLast steps ↓ down the List to its last session, failing the test
// where it isn't eps's last.
func chooseLast(h *harness) {
	h.t.Helper()
	for range len(epsSessions) + 1 {
		h.keys(downKey)
	}
	if got, want := h.model.sessionsTab.shown.Chosen.Session, epsSessions[0]; got != want {
		h.t.Fatalf("↓ to the List's foot chose %s, want %s, the first started last", got, want)
	}
}

// ↓ onto the last session shows the words after it too, from the List's
// top and from midway.
func TestAStepOntoTheLastSessionShowsTheWordsAfterIt(t *testing.T) {
	for _, from := range []int{0, 2} {
		h := pastListHarness(t, Size{Width: 160, Height: 14})
		for range from {
			h.keys(downKey)
		}
		chooseLast(h)
		if view := h.view(); !strings.Contains(view, unkeptWords) {
			t.Errorf("↓ to the last from %d down, the screen reads\n%s\nwant the words after it", from, view)
		}
	}
}

// A row up with k from the List's foot, the last session chosen and still
// in view, holds there, through the looks after it: no reveal of the words
// undoes it.
func TestARowUpFromTheFootHoldsWithTheLastSessionChosen(t *testing.T) {
	h := pastListHarness(t, Size{Width: 160, Height: 14})
	chooseLast(h)
	most := h.model.scroll
	h.keyed("k")
	h.tickUntil(h.clock.now.Add(5 * time.Second))
	if h.model.scroll != most-1 || !onTheList(h, epsSessions[0]) {
		t.Errorf("k from the foot, scrolled %d, %s in view %v; want %d, it still in view", h.model.scroll, epsSessions[0], onTheList(h, epsSessions[0]), most-1)
	}
}

// A click on the last session, on the List's bottom row, chooses it where
// it is: the List doesn't scroll from under the pointer to show the words
// after it.
func TestAClickOnTheLastSessionKeepsItUnderThePointer(t *testing.T) {
	h := pastListHarness(t, Size{Width: 160, Height: 14})
	chooseLast(h)
	h.keys(escKey)
	h.keyed("k")
	last := epsSessions[0]
	targets := h.targets()
	at := slices.IndexFunc(targets, func(t dashboard.Target) bool { return t.On == dashboard.OnSession && t.Session == last })
	if at < 0 {
		t.Fatalf("a row up from the foot, none chosen, the screen reads\n%s\nwant %s on it", h.view(), last)
	}
	bottom := slices.IndexFunc(targets, func(t dashboard.Target) bool { return t.On == dashboard.OnSession && t.Y > targets[at].Y })
	if bottom >= 0 || strings.Contains(h.view(), unkeptWords) {
		t.Fatalf("a row up from the foot, none chosen, the screen reads\n%s\nwant %s on the List's bottom row, the words after it out of view", h.view(), last)
	}
	target, scroll := targets[at], h.model.scroll
	h.click(target.X, target.Y)
	if h.model.sessionsTab.shown.Chosen.Session != last || h.model.scroll != scroll {
		t.Fatalf("a click on %s chose %s, scrolled %d; want it chosen, scrolled %d", last, h.model.sessionsTab.shown.Chosen.Session, h.model.scroll, scroll)
	}
	if !slices.Contains(h.targets(), target) {
		t.Errorf("after the click, %s isn't at %+v; want it under the pointer", last, target)
	}
	h.keys(downKey)
	if h.model.sessionsTab.shown.Chosen.Session != last || !strings.Contains(h.view(), unkeptWords) || !onTheList(h, last) {
		t.Errorf("↓ on %s, chosen at the List's end, chose %s and the screen reads\n%s\nwant it still chosen, in view, and the words after it shown", last, h.model.sessionsTab.shown.Chosen.Session, h.view())
	}
}
