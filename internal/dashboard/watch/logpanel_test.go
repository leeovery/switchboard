package watch

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
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
