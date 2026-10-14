package watch

import (
	"cmp"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// targets are the targets of the page on screen.
func (h *harness) targets() []dashboard.Target {
	f, doc, at := h.model.screen(h.clock.now)
	return f.Targets(doc, at)
}

// tabTarget is the target of the tab given on the page on screen, failing
// the test where there's none.
func (h *harness) tabTarget(tab dashboard.View) dashboard.Target {
	h.t.Helper()
	i := slices.IndexFunc(h.targets(), func(t dashboard.Target) bool { return t.On == dashboard.OnTab && t.Tab == tab })
	if i < 0 {
		h.t.Fatalf("the page has no target for the %s tab among %+v", tab, h.targets())
	}
	return h.targets()[i]
}

// click clicks the left button on the cell at x along row y, delivering what
// it sends back, and reports whether it quit.
func (h *harness) click(x, y int) bool {
	h.t.Helper()
	msgs := h.update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	h.deliver(msgs...)
	return quits(msgs)
}

// clickOn clicks the target given, as clicked lands on it, delivering what it
// sends back, and reports whether it quit.
func (h *harness) clickOn(t dashboard.Target) bool {
	h.t.Helper()
	next, cmd := h.model.clickedOn(t)
	h.model = next.(Model)
	msgs := h.run(cmd)
	h.deliver(msgs...)
	return quits(msgs)
}

// keyTarget is a key of the key line, as the page's targets give it.
func keyTarget(key string) dashboard.Target {
	return dashboard.Target{On: dashboard.OnKey, Key: key}
}

func TestAClickOnATabShowsItFromItsTopAndKeepsIt(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 150, Height: 50}, {Width: 52, Height: 40}} {
		t.Run(fmt.Sprintf("%dx%d", size.Width, size.Height), func(t *testing.T) {
			h := newHarness(t, twelve())
			prefs := &fakePrefs{}
			h.model.cfg.Prefs = prefs
			h.update(size)
			h.start()

			for _, tab := range dashboard.Views() {
				from := dashboard.Overview
				if tab == from {
					from = dashboard.Log
				}
				h.model.view = from
				target := h.tabTarget(tab)
				for _, x := range []int{target.X, target.X + target.Width - 1} {
					h.model.view, h.model.scroll = from, 1
					h.click(x, target.Y)
					if h.model.view != tab || h.model.scroll != 0 || prefs.kept.Tab != string(tab) {
						t.Errorf("a click at %d,%d, on the %s tab, showed %q scrolled %d, and kept %q; want %q from its top, kept",
							x, target.Y, tab, h.model.view, h.model.scroll, prefs.kept.Tab, tab)
					}
				}
			}
		})
	}
}

func TestAClickOnTheTabShownChangesNothing(t *testing.T) {
	h := runwayHarness(t, runwaySize)
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.press("j")

	target := h.tabTarget(dashboard.Runway)
	h.click(target.X, target.Y)
	if h.model.view != dashboard.Runway || h.model.scroll != 1 || prefs.updates != 0 {
		t.Errorf("a click on the tab shown showed %q scrolled %d, keeping it %d times; want it as it was, a row down, nothing kept", h.model.view, h.model.scroll, prefs.updates)
	}
}

func TestAClickOffEveryTargetDoesNothing(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.settle()
	was := h.model
	targets := h.targets()
	for y := range was.size.Height {
		for x := range was.size.Width {
			if _, on := targetAt(targets, x, y); on {
				continue
			}
			if h.click(x, y) || h.model.view != was.view || h.model.open != nothingOpen || h.model.focus != was.focus ||
				h.model.sections != was.sections || h.model.by != was.by || h.model.picker.open || len(h.source.orders) > 0 {
				t.Fatalf("a click at %d,%d, on no target, acted: showing %q, open %v, focus on %q, sections %v, by %v, orders %q",
					x, y, h.model.view, h.model.open, h.model.focus, h.model.sections.Names(), h.model.by, h.source.orders)
			}
		}
	}
}

func TestATargetTakesTheCellsItSpans(t *testing.T) {
	targets := []dashboard.Target{
		{X: 4, Y: 0, Width: 3, On: dashboard.OnTab, Tab: dashboard.Accounts},
		{X: 7, Y: 0, Width: 2, On: dashboard.OnTab, Tab: dashboard.History},
		{X: 4, Y: 9, Width: 5, On: dashboard.OnKey, Key: "w"},
	}
	tests := []struct {
		x, y int
		want dashboard.Target
		on   bool
	}{
		{x: 3, y: 0},
		{x: 4, y: 0, want: targets[0], on: true},
		{x: 6, y: 0, want: targets[0], on: true},
		{x: 7, y: 0, want: targets[1], on: true},
		{x: 8, y: 0, want: targets[1], on: true},
		{x: 9, y: 0},
		{x: 5, y: 1},
		{x: 8, y: 9, want: targets[2], on: true},
		{x: 9, y: 9},
	}
	for _, tt := range tests {
		if got, on := targetAt(targets, tt.x, tt.y); got != tt.want || on != tt.on {
			t.Errorf("targetAt(%d, %d) = %+v, %v; want %+v, %v", tt.x, tt.y, got, on, tt.want, tt.on)
		}
	}
}

func TestOnlyTheLeftButtonClicks(t *testing.T) {
	h := newHarness(t, calm())
	h.start()
	target := h.tabTarget(dashboard.Runway)
	for _, button := range []tea.MouseButton{tea.MouseRight, tea.MouseMiddle} {
		h.deliver(h.update(tea.MouseClickMsg{X: target.X, Y: target.Y, Button: button})...)
		if h.model.view != dashboard.Overview {
			t.Errorf("button %v on the Runway tab showed %q, want the Overview still", button, h.model.view)
		}
	}
}

func TestAClickOnAKeyOfTheKeyLineActsAsTheKey(t *testing.T) {
	tests := []struct {
		key string
		// acted reports whether the key acted, as the model now stands.
		acted func(m Model) bool
	}{
		{key: "tab", acted: func(m Model) bool { return m.view == dashboard.Accounts }},
		{key: "?", acted: func(m Model) bool { return m.open == helpOpen }},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.start()
			if h.clickOn(keyTarget(tt.key)) || !tt.acted(h.model) {
				t.Errorf("a click on %s didn't act as %s does", tt.key, tt.key)
			}
		})
	}
}

func TestAClickOnTheCellsOfAKeyOnTheKeyLineActsAsTheKey(t *testing.T) {
	wide, phone := tea.WindowSizeMsg{Width: 150, Height: 50}, tea.WindowSizeMsg{Width: 52, Height: 50}
	helpOpened := func(m Model) bool { return m.open == helpOpen }
	tests := []struct {
		size  tea.WindowSizeMsg
		drawn string
		// acted reports whether the key acted, as the model now stands.
		acted func(m Model) bool
	}{
		{size: wide, drawn: "tab views", acted: func(m Model) bool { return m.view == dashboard.Accounts }},
		{size: wide, drawn: "? help", acted: helpOpened},
		{size: phone, drawn: "tab views", acted: func(m Model) bool { return m.view == dashboard.Accounts }},
		{size: phone, drawn: "? keys and toggles", acted: helpOpened},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s at %dx%d", tt.drawn, tt.size.Width, tt.size.Height), func(t *testing.T) {
			for _, at := range []string{"first", "last"} {
				h := routedHarness(t, routerDocument(three()...))
				h.update(tt.size)
				h.start()
				x, y := h.drawnAt(tt.drawn)
				if y != tt.size.Height-1 {
					t.Fatalf("%q is drawn on row %d, want the key line's, %d", tt.drawn, y, tt.size.Height-1)
				}
				if at == "last" {
					x += ansi.StringWidth(tt.drawn) - 1
				}
				if h.click(x, y) || !tt.acted(h.model) {
					t.Errorf("a click on the %s cell of %q, at %d,%d, didn't act as its key does", at, tt.drawn, x, y)
				}
			}
		})
	}
}

func TestAClickOnTheOverviewsKeysActsAsTheKey(t *testing.T) {
	tests := []struct {
		drawn string
		// acted reports whether the key acted, as the model now stands.
		acted func(m Model) bool
	}{
		{drawn: "g graphs", acted: func(m Model) bool { return m.sections.Has(dashboard.SectionGraphs) }},
		{drawn: "l lately", acted: func(m Model) bool { return !m.sections.Has(dashboard.SectionLately) }},
		{drawn: "b by: window", acted: func(m Model) bool { return m.by == dashboard.ByAccount }},
		{drawn: "↑↓ focus", acted: func(m Model) bool { return m.focus == "personal" }},
	}
	for _, tt := range tests {
		t.Run(tt.drawn, func(t *testing.T) {
			for _, at := range []string{"first", "last"} {
				h := routedHarness(t, routerDocument(three()...))
				h.update(tea.WindowSizeMsg{Width: 160, Height: 40})
				h.start()
				x, y := h.drawnAt(tt.drawn)
				if at == "last" {
					x += ansi.StringWidth(tt.drawn) - 1
				}
				if h.click(x, y) || !tt.acted(h.model) {
					t.Errorf("a click on the %s cell of %q, at %d,%d, didn't act as its key does", at, tt.drawn, x, y)
				}
			}
		})
	}
}

func TestAClickOnASectionsHeadingHidesIt(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()

	h.clickOn(dashboard.Target{On: dashboard.OnSection, Section: dashboard.SectionRouting})
	if h.model.sections.Has(dashboard.SectionRouting) || slices.Contains(prefs.kept.Sections, "routing") {
		t.Errorf("a click on ROUTING's heading left it shown, %q, kept %q; want it hidden, and kept so", h.model.sections.Names(), prefs.kept.Sections)
	}
}

func TestAClickOnTheCellsOfPPinOrQQuitDoesNothing(t *testing.T) {
	tests := []struct {
		size  tea.WindowSizeMsg
		drawn []string
	}{
		{size: tea.WindowSizeMsg{Width: 150, Height: 50}, drawn: []string{"p pin", "q quit"}},
		{size: tea.WindowSizeMsg{Width: 52, Height: 50}, drawn: []string{"q quit"}},
	}
	for _, tt := range tests {
		h := routedHarness(t, routerDocument(three()...))
		h.update(tt.size)
		h.start()
		for _, drawn := range tt.drawn {
			x, y := h.drawnAt(drawn)
			for cell := x; cell < x+ansi.StringWidth(drawn); cell++ {
				if h.click(cell, y) || h.model.open != nothingOpen || len(h.source.orders) > 0 {
					t.Errorf("at %d columns, a click at %d,%d, on %q, quit, or opened %v, or ordered %q; want nothing", tt.size.Width, cell, y, drawn, h.model.open, h.source.orders)
				}
			}
		}
	}
}

func TestAClickOnAKeyThePageDoesntTakeDoesNothing(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	for _, key := range []string{"", "pin", "1", "a", "m"} {
		if h.clickOn(keyTarget(key)) || h.model.open != nothingOpen || len(h.source.orders) > 0 {
			t.Errorf("a click on %q quit, or opened %v, or ordered %q; want nothing", key, h.model.open, h.source.orders)
		}
	}
}

func TestNoClickActsWhileACardIsOpen(t *testing.T) {
	for _, card := range []string{"p", "?"} {
		t.Run(card, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.start()
			h.press(card)
			open := h.model.open
			for _, target := range h.targets() {
				if h.click(target.X, target.Y) || h.model.open != open || h.model.view != dashboard.Overview {
					t.Fatalf("with %v open, a click on %+v acted: open %v, showing %q", open, target, h.model.open, h.model.view)
				}
			}
			for y := range h.model.size.Height {
				for x := range h.model.size.Width {
					if h.click(x, y) || h.model.open != open || len(h.source.orders) > 0 || h.model.help.scroll != 0 {
						t.Fatalf("with %v open, a click at %d,%d acted: open %v, orders %q", open, x, y, h.model.open, h.source.orders)
					}
				}
			}
		})
	}
}

func TestWhileThePickerIsOpenAClickActsAsItsKeysAlone(t *testing.T) {
	h, _ := settledThemedHarness(t, theme.One("amber"))
	h.typed("t")

	tab := h.tabTarget(dashboard.Accounts)
	h.click(tab.X, tab.Y)
	if h.model.view != dashboard.Overview || !h.model.picker.open {
		t.Errorf("with the picker open, a click on the Accounts tab showed %q, the picker open %v; want nothing, as tab does nothing then", h.model.view, h.model.picker.open)
	}
	for y := range h.model.size.Height {
		for x := h.model.size.Width - dashboard.PickerWidth(h.model.size.Width); x < h.model.size.Width; x++ {
			if h.click(x, y) || !h.model.picker.open {
				t.Fatalf("a click at %d,%d, on the picker, closed it or quit", x, y)
			}
		}
	}
	h.clickOn(keyTarget("esc"))
	if h.model.picker.open {
		t.Error("a click on the picker's esc left it open, want it closed, as esc does")
	}
}

// pickerOpened is h in colour, its terminal's background dark, with the
// keys given pressed, as keyed presses them, then the theme picker opened and
// moved down a theme, which it shows, failing the test where it doesn't open.
func pickerOpened(t *testing.T, h *harness, keys string) *fakeThemes {
	t.Helper()
	themes := &fakeThemes{listing: listing(), choice: theme.One("nord")}
	h.model.cfg.Themes = themes
	h.answer(darkBackground)
	h.keyed(keys + " t down")
	if !h.model.picker.open {
		t.Fatalf("t opened no theme picker:\n%s", h.view())
	}
	return themes
}

// While the theme picker is open, a click on what the page under it gives
// does nothing, as no key reaches past the picker: each of two on any target
// left of the picker but a key's leaves the screen as it was, colours and
// all, the picker open on the theme it shows, nothing kept, no card open, no
// page opened or closed, and the Overview's focus and what Accounts shows
// where they were. On the Log, two clicks on a line, which choose it and
// open its details as enter does, neither choose it nor open them; on
// Accounts, two on any of its targets, its sub-tabs, Compare's columns, or
// Detail's accounts and sessions, which focus, choose and select what they
// land on, and open it the second time, do none of it. No < or > by a period
// lies left of the picker.
func TestWhileThePickerIsOpenAClickUnderItDoesNothing(t *testing.T) {
	onPage := func(keys string) func(*testing.T) (*harness, *fakeThemes) {
		return func(t *testing.T) (*harness, *fakeThemes) {
			lines := slices.Concat(promptedLines(2, start.Add(-30*time.Minute)), turnsOfRequests(start.Add(-20*time.Minute), 0, 0, 3))
			h, _ := pageHarness(t, lines)
			h.openPage(idD28C)
			return h, pickerOpened(t, h, keys)
		}
	}
	onTheOverview := func(keys string) func(*testing.T) (*harness, *fakeThemes) {
		return func(t *testing.T) (*harness, *fakeThemes) {
			h, _ := pageHarness(t, promptedLines(2, start.Add(-30*time.Minute)))
			return h, pickerOpened(t, h, keys)
		}
	}
	onHistory := func(t *testing.T) (*harness, *fakeThemes) {
		h, _ := historyHarness(t, dashboard.History, dashboard.HistoryPage{YearAsk: views.YearAsk{Account: "work"}})
		h.start()
		return h, pickerOpened(t, h, "")
	}
	onAccounts := func(shown dashboard.AccountsShown) func(*testing.T) (*harness, *fakeThemes) {
		return func(t *testing.T) (*harness, *fakeThemes) {
			h := accountsHarness(t, shown)
			return h, pickerOpened(t, h, "")
		}
	}
	onTheLogsLines := func(t *testing.T) (*harness, *fakeThemes) {
		state := t.TempDir()
		writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "side", -4*time.Minute))
		h := newHarness(t, probedWithoutTheRouter())
		onTheLog(h, state)
		h.start()
		return h, pickerOpened(t, h, "")
	}
	is := func(on dashboard.Clicked, as func(dashboard.Target) bool) func(dashboard.Target) bool {
		return func(t dashboard.Target) bool { return t.On == on && as(t) }
	}
	tests := []struct {
		name   string
		opened func(*testing.T) (*harness, *fakeThemes)
		target func(dashboard.Target) bool
	}{
		{name: "a tab", opened: onTheOverview(""), target: is(dashboard.OnTab, func(t dashboard.Target) bool { return t.Tab == dashboard.Accounts })},
		{name: "a sub-tab", opened: onHistory, target: onSubTab(dashboard.HistoryTokens)},
		{name: "a line of the Log's", opened: onTheLogsLines, target: is(dashboard.OnRow, func(t dashboard.Target) bool { return t.Row == 1 })},
		{name: "a section's heading", opened: onTheOverview(""), target: is(dashboard.OnSection, func(dashboard.Target) bool { return true })},
		{name: "an account's row", opened: onTheOverview(""), target: is(dashboard.OnAccount, func(t dashboard.Target) bool { return t.Account == "personal" })},
		{name: "a session's row", opened: onTheOverview("s c l"), target: is(dashboard.OnSession, func(t dashboard.Target) bool { return t.Session == idD28C })},
		{name: "a turn's row", opened: onPage(""), target: is(dashboard.OnTurn, func(t dashboard.Target) bool { return t.Turn == 2 })},
		{name: "a request's row of the turn open", opened: onPage("enter"), target: is(dashboard.OnRequest, func(t dashboard.Target) bool { return t.Row == 1 })},
		{
			name:   "a sub-tab of Accounts'",
			opened: onAccounts(dashboard.AccountsShown{}),
			target: is(dashboard.OnSubTab, func(t dashboard.Target) bool { return t.SubTab == int(dashboard.AccountsDetail) }),
		},
		{
			name:   "a column of Compare's",
			opened: onAccounts(dashboard.AccountsShown{}),
			target: is(dashboard.OnColumn, func(t dashboard.Target) bool { return t.Account == "personal" }),
		},
		{
			name:   "Compare's focused column",
			opened: onAccounts(dashboard.AccountsShown{Focused: true, Focus: "personal"}),
			target: is(dashboard.OnColumn, func(t dashboard.Target) bool { return t.Account == "personal" }),
		},
		{
			name:   "an account of Detail's list",
			opened: onAccounts(dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work"}),
			target: is(dashboard.OnAccount, func(t dashboard.Target) bool { return t.Account == "personal" }),
		},
		{
			name:   "a session of Detail's SESSIONS",
			opened: onAccounts(dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work"}),
			target: is(dashboard.OnSession, func(t dashboard.Target) bool { return t.Session == id7F3A }),
		},
		{
			name:   "Detail's selected session",
			opened: onAccounts(dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work", InSessions: true, Selected: id9E21}),
			target: is(dashboard.OnSession, func(t dashboard.Target) bool { return t.Session == id9E21 }),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, themes := tt.opened(t)
			i := slices.IndexFunc(h.targets(), func(t dashboard.Target) bool { return tt.target(t) && !h.model.onPicker(t.X) })
			if i < 0 {
				t.Fatalf("the page gives no such target left of the picker among %+v", h.targets())
			}
			target, screen, cursor, focus := h.targets()[i], h.model.View().Content, h.model.picker.cursor, h.model.focus
			page, accounts := h.pageOpen(), h.model.accounts.shown

			for click := range 2 {
				h.click(target.X, target.Y)
				if !h.model.picker.open || h.model.picker.cursor != cursor || len(themes.kept) > 0 || h.model.open != nothingOpen || h.model.focus != focus {
					t.Fatalf("click %d on %+v left the picker open %t, its cursor on %d, kept %v, card %d open, the focus on %q; want the picker open on %d, nothing kept, no card, the focus on %q",
						click+1, target, h.model.picker.open, h.model.picker.cursor, themes.kept, h.model.open, h.model.focus, cursor, focus)
				}
				if h.pageOpen() != page || !reflect.DeepEqual(h.model.accounts.shown, accounts) {
					t.Fatalf("click %d on %+v left %q's page open, Accounts showing %+v; want %q's, Accounts showing %+v", click+1, target, h.pageOpen(), h.model.accounts.shown, page, accounts)
				}
				if got := h.model.View().Content; got != screen {
					t.Fatalf("click %d on %+v changed the screen from\n%s\nto\n%s", click+1, target, ansi.Strip(screen), ansi.Strip(got))
				}
			}
		})
	}
}

func TestShowingSessionsAsAnAnswerStreamsDrawsItsFrames(t *testing.T) {
	h := streamingOn(t, dashboard.Sections{})
	h.settle()
	h.hear(told(router.StreamSent, "r1", "work", 0), told(router.StreamFirst, "r1", "work", 0))

	target := h.tabTarget(dashboard.Sessions)
	h.click(target.X, target.Y)
	if _, ok := h.pendingFrame(); h.model.view != dashboard.Sessions || !ok {
		t.Errorf("a click showing Sessions as d28c's answer streams showed %q, a frame armed %v; want Sessions, a frame armed to draw its shimmer", h.model.view, ok)
	}
}

// TestNoClickOrTurnOfTheWheelRoutesQuitsOrPanics clicks, and turns the
// wheel, on each target the page gives and at cells picked at random, on
// every tab, at sizes from a wide terminal's to a cell's, with nothing open
// over the page, the routing card, help, and the theme picker, drawing the
// screen after each.
func TestNoClickOrTurnOfTheWheelRoutesQuitsOrPanics(t *testing.T) {
	sizes := []tea.WindowSizeMsg{{Width: 160, Height: 40}, {Width: 52, Height: 40}, {Width: 20, Height: 6}, {Width: 1, Height: 1}}
	for i, opening := range []string{"", "p", "?", "t"} {
		t.Run("opened with "+cmp.Or(opening, "nothing"), func(t *testing.T) {
			t.Parallel()
			r := rand.New(rand.NewPCG(6, uint64(i)))
			for _, size := range sizes {
				for _, tab := range dashboard.Views() {
					h := clickedHarness(t, tab, size, opening)
					routing := h.model.open == routingOpen
					for _, cell := range cellsToClick(h.targets(), size, r) {
						x, y := cell[0], cell[1]
						wheel := tea.MouseWheelMsg{X: x, Y: y, Button: []tea.MouseButton{tea.MouseWheelUp, tea.MouseWheelDown}[r.IntN(2)]}
						h.deliver(h.update(wheel)...)
						if h.click(x, y) || len(h.source.orders) > 0 || !routing && h.model.open == routingOpen {
							t.Fatalf("%dx%d on %s, a click at %d,%d quit, ordered %q, or opened the routing card: open %v",
								size.Width, size.Height, tab, x, y, h.source.orders, h.model.open)
						}
						h.model.View()
					}
				}
			}
		})
	}
}

// clickedHarness is a model drawn in colour of the router of three, d28c
// busy on work, showing the tab given in a terminal of the size given,
// started, with the key given pressed, where it's one.
func clickedHarness(t *testing.T, tab dashboard.View, size tea.WindowSizeMsg, opening string) *harness {
	t.Helper()
	h, _ := themedHarness(t, theme.One("amber"))
	doc := routerDocument(three()...)
	h.source.router, h.source.sessions = &doc, []status.Session{sessionOn(idD28C, opus, "work", 10*time.Second)}
	h.model.view = tab
	h.update(size)
	h.start()
	h.answer(darkBackground)
	if opening != "" {
		h.typed(opening)
	}
	return h
}

// cellsToClick are the cells to click on a terminal of the size given: the
// first of each target's, and a few picked by r, a cell beyond it either way
// among them.
func cellsToClick(targets []dashboard.Target, size tea.WindowSizeMsg, r *rand.Rand) [][2]int {
	var cells [][2]int
	for _, t := range targets {
		cells = append(cells, [2]int{t.X, t.Y})
	}
	for range 20 {
		cells = append(cells, [2]int{r.IntN(size.Width+2) - 1, r.IntN(size.Height+2) - 1})
	}
	return cells
}

// A click on a row of the Log's table chooses it, pausing it; on ▼ N new,
// the table follows again; and on a sub-tab, it's shown.
func TestAClickOnTheLogChoosesALineFollowsAgainAndShowsASubTab(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "side", -4*time.Minute))
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, state)
	h.start()
	target := func(on dashboard.Clicked, is func(dashboard.Target) bool) dashboard.Target {
		t.Helper()
		f, doc, at := h.model.screen(h.clock.now)
		targets := f.Targets(doc, at)
		if i := slices.IndexFunc(targets, func(t dashboard.Target) bool { return t.On == on && is(t) }); i >= 0 {
			return targets[i]
		}
		t.Fatalf("the page has no such target among %v", targets)
		return dashboard.Target{}
	}

	row := target(dashboard.OnRow, func(dashboard.Target) bool { return true })
	h.click(row.X, row.Y)
	if r, _, ok := h.model.log.page.Chosen(); !ok || r.Request != "0a000001" || !h.model.log.page.Paused() {
		t.Errorf("a click on the first row chose %q, paused %t; want it chosen, paused", r.Request, h.model.log.page.Paused())
	}

	writeLines(t, state, arrived("0a000003", "work", -3*time.Minute))
	h.read(probedWithoutTheRouter())
	fresh := target(dashboard.OnKey, func(t dashboard.Target) bool { return t.Key == "end" && t.Y == h.model.size.Height-2 })
	h.click(fresh.X, fresh.Y)
	if h.model.log.page.Paused() {
		t.Error("a click on ▼ 1 new left the table paused, want it following")
	}

	events := target(dashboard.OnSubTab, func(t dashboard.Target) bool { return t.SubTab == int(dashboard.LogEvents) })
	h.click(events.X, events.Y)
	if h.model.log.page.Shown != dashboard.LogEvents {
		t.Error("a click on Events didn't show them")
	}
}
