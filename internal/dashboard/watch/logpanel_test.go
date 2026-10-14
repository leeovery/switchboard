package watch

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// f opens the panel over the Log: what it would show follows each tick, the
// page behind it unchanged; esc closes it, changing nothing; enter applies
// what it picks, and the preferences keep the columns it shows, not the
// filter.
func TestTheLogsPanelPicksWhatItShowsAndEnterAppliesIt(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "side", -4*time.Minute))
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, state)
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()

	h.keyed("f right space")
	if p := h.model.log.panel; h.model.open != logPanelOpen || p.Lines.Shown != 1 || p.Lines.Lines() != 2 || len(shownRequests(h)) != 2 {
		t.Errorf("work unticked, the panel would show %d of %d lines, the page %d; want 1 of 2, the page 2 still", p.Lines.Shown, p.Lines.Lines(), len(shownRequests(h)))
	}
	h.keyed("esc")
	if h.model.open != nothingOpen || h.model.log.page.Filter.Accounts != nil || prefs.updates != 0 {
		t.Errorf("esc left the filter %+v and kept the preferences %d times, want it closed, nothing changed", h.model.log.page.Filter, prefs.updates)
	}

	h.keyed("f space right space enter")
	page := h.model.log.page
	if got := shownRequests(h); h.model.open != nothingOpen || !slices.Equal(got, []string{"0a000002"}) || page.Columns.Shows(dashboard.LogSession) {
		t.Errorf("applied, the Requests show %v, session's column showing %v; want side's alone, no session column", got, page.Columns.Shows(dashboard.LogSession))
	}
	if kept := prefs.kept.Log.Columns; prefs.updates != 1 || len(kept) == 0 || slices.Contains(kept, "session") {
		t.Errorf("the preferences kept the columns %v, %d times, want them kept once, without session", kept, prefs.updates)
	}

	h.keyed("f space /")
	if h.model.open != nothingOpen || h.model.log.page.Columns.Shows(dashboard.LogSession) {
		t.Errorf("/ left the panel open %v, or applied its tick, want it closed for the find, nothing changed", h.model.open == logPanelOpen)
	}

	h.keyed("esc f right down space enter")
	if got := h.model.log.page.Filter.Accounts; !slices.Equal(got, []string{"side"}) || prefs.updates != 1 {
		t.Errorf("personal unticked and applied, the filter keeps %v and the preferences were kept %d times; want side's, kept once still, as no column changed", got, prefs.updates)
	}
}

// A click on one of the panel's ticks ticks it.
func TestAClickOnTheLogsPanelTicksATick(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute))
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, state)
	h.start()

	h.keyed("f")
	x, y := -1, -1
	for i, row := range strings.Split(h.view(), "\n") {
		if before, _, found := strings.Cut(row, "□ worth"); found {
			x, y = len([]rune(before)), i
		}
	}
	h.deliver(h.update(tea.MouseClickMsg{X: x + 3, Y: y, Button: tea.MouseLeft})...)
	if !h.model.log.panel.Columns.Shows(dashboard.LogWorth) || h.model.log.page.Columns.Shows(dashboard.LogWorth) {
		t.Errorf("clicked on worth at %d, %d, the panel shows it %v, the page %v; want the panel's ticked, the page's not till applied", x, y,
			h.model.log.panel.Columns.Shows(dashboard.LogWorth), h.model.log.page.Columns.Shows(dashboard.LogWorth))
	}
}

// A session and a model the filter ticks before midnight, of which no line
// comes after it, are listed on the panel past midnight, ticked, the
// session as the Log's read holds its lines, so either can be let go.
func TestThePanelListsWhatItsFilterTicksPastMidnight(t *testing.T) {
	const sonnet = "claude-sonnet-4-5"
	state := t.TempDir()
	before := ranIn("0b000001", apiSession, "~/Code/api", 0)
	before.At, before.Model = onTheDay(23, 30, 0), sonnet
	writeLines(t, state, before)
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, state)
	h.clock.now = onTheDay(23, 40, 0)
	h.start()
	h.model.log.page.Filter = views.LogFilter{Sessions: []string{apiSession}, Models: []string{sonnet}, Kinds: []views.RowKind{views.RowRequest}}

	after := ranIn("0c000001", webSession, "~/Code/web", 0)
	after.At = onTheDay(24, 2, 0)
	writeLines(t, state, after)
	h.clock.now = onTheDay(24, 5, 0)
	h.read(probedWithoutTheRouter())
	h.keyed("f")

	panel := h.model.log.panel
	if i := slices.IndexFunc(panel.Sessions, func(s views.LogSession) bool { return s.ID == apiSession }); i < 0 || panel.Sessions[i].Dir != "~/Code/api" {
		t.Errorf("past midnight, the panel lists %+v; want api among them, in ~/Code/api", panel.Sessions)
	}
	view := h.view()
	for _, want := range []string{"■ " + status.ShownID(apiSession), "■ " + ledger.ShortModelID(sonnet), "□ " + ledger.ShortModelID(opus)} {
		if !strings.Contains(view, want) {
			t.Errorf("past midnight, the panel has no %q:\n%s", want, view)
		}
	}
}
