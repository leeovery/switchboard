package watch

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// Accounts' Tokens' page is built of the days held over the dial's period,
// as views.Past.TokensOver builds it: never before Tokens shows, nor again
// for a message that changes neither, as a frame or a resize; afresh for
// each look, which reads today's days, and for a period shown anew. The
// frame draws it as built.
func TestAccountsTokensPageIsBuiltWhileTokensShows(t *testing.T) {
	h, _ := historyHarness(t, dashboard.Accounts, dashboard.HistoryPage{})
	h.start()
	h.tickUntilAsked()
	if h.model.accounts.tokens != nil {
		t.Fatalf("on Compare, Tokens' page is %+v, want none built", h.model.accounts.tokens)
	}
	builtOver := func(period views.Period) *views.TokensPage {
		t.Helper()
		page := h.model.accounts.tokens
		want := h.model.past.last.past.Since(nil, h.model.now()).TokensOver(period)
		if page == nil || !reflect.DeepEqual(*page, want) {
			t.Fatalf("Tokens' page is %+v, want it built over %s, %+v", page, period.Name(), want)
		}
		if drawn := h.model.frame(h.model.now()).Accounts.Tokens; drawn != page {
			t.Errorf("the frame draws Tokens' page %p, want the one built, %p", drawn, page)
		}
		return page
	}
	h.keyed("] ]")
	built := builtOver(views.PeriodThisWeek)

	h.update(frameMsg{gen: h.model.frames})
	h.update(tea.WindowSizeMsg{Width: 120, Height: 37})
	if h.model.accounts.tokens != built {
		t.Errorf("a frame and a resize built Tokens' page afresh, want it held")
	}

	h.tickUntilAsked()
	if looked := builtOver(views.PeriodThisWeek); looked == built {
		t.Errorf("a look that read today's days left Tokens' page as it was built, want it built afresh")
	}

	h.keyed("<")
	builtOver(views.PeriodToday)
}

// v cycles Tokens' split, its rows back at their top, and the preferences
// keep it; on Compare and Detail, v does nothing.
func TestVCyclesTokensSplitAndKeepsIt(t *testing.T) {
	h := accountsHarness(t, dashboard.AccountsShown{})
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	for _, view := range []string{"compare", "detail"} {
		was := prefs.updates
		h.keyed("v")
		h.keepWritten()
		if s := h.model.accounts.shown; s.TokensSplit != dashboard.TokensByAccount || prefs.updates != was {
			t.Fatalf("on %s, v split Tokens by %d, the preferences updated %d times more; want nothing", view, s.TokensSplit, prefs.updates-was)
		}
		h.keyed("]")
		h.keepWritten()
	}
	updates := prefs.updates
	for _, split := range []string{"model", "account+model", "none", "account"} {
		h.model.scroll = 2
		h.keyed("v")
		updates++
		h.keepWritten()
		want := theme.AccountsPrefs{View: "tokens", Split: split}
		if got := h.model.accounts.shown.Kept(); got != want || prefs.kept.Accounts != want || prefs.updates != updates || h.model.scroll != 0 {
			t.Fatalf("after v, Accounts shows %+v, the preferences keep %+v, updated %d times, scrolled %d; want %+v kept, %d times, at the top",
				got, prefs.kept.Accounts, prefs.updates, h.model.scroll, want, updates)
		}
	}
}
