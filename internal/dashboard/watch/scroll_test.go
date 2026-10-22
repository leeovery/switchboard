package watch

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/status"
)

// twelve is the probe of twelve accounts, more than USAGE shows at once in a
// short terminal.
func twelve() status.Document {
	accounts := make([]status.Account, 12)
	for i := range accounts {
		accounts[i] = account(string(rune('a'+i)), "Account "+string(rune('A'+i)), session(0.1, 3*time.Hour), week(0.2))
	}
	return document(accounts...)
}

// yearSize is a terminal History's Year of a month's requests scrolls in, its
// numbers under the key: 31 rows in a view of 17, so 14 rows at most, and a
// page of 16 from the top, where BY MONTH's heading is kept off its foot.
var yearSize = tea.WindowSizeMsg{Width: 150, Height: 26}

// yearHarness is a model of the router of three, showing History's Year of
// the ledger's last 30 days, ten requests each, in a terminal of the size
// given, started.
func yearHarness(t *testing.T, size tea.WindowSizeMsg) *harness {
	t.Helper()
	h, l := historyHarness(t, dashboard.History, dashboard.HistoryPage{})
	for i := range 30 {
		date := dayfile.DateOf(dayfile.DayStart(start.Local(), -i))
		l.days[date] = requested(date, 10)
	}
	h.deliver(h.update(size)...)
	h.start()
	return h
}

// scrolled is the first row of what scrolls the screen shows.
func (h *harness) scrolled() string {
	return strings.Split(h.view(), "\n")[h.overTheView()]
}

func TestScrollKeysScrollTheViewNoFurtherThanItGoes(t *testing.T) {
	h := yearHarness(t, yearSize)
	if got := h.model.scrolling(); got.Most != 14 || got.Page != 16 {
		t.Fatalf("scrolling() = %+v, want 14 rows at most, a page of 16, its last row saying what's out of view", got)
	}
	steps := []struct {
		name string
		key  tea.KeyPressMsg
		want int
	}{
		{name: "j, a row down", key: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: 1},
		{name: "j again", key: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: 2},
		{name: "k, a row up", key: tea.KeyPressMsg{Code: 'k', Text: "k"}, want: 1},
		{name: "PgDn, a page, but no further than it goes", key: tea.KeyPressMsg{Code: tea.KeyPgDown}, want: 14},
		{name: "j at the foot, nowhere", key: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: 14},
		{name: "PgUp, a page, but no further than the top", key: tea.KeyPressMsg{Code: tea.KeyPgUp}, want: 0},
		{name: "k at the top, nowhere", key: tea.KeyPressMsg{Code: 'k', Text: "k"}, want: 0},
	}
	for _, step := range steps {
		h.update(step.key)
		if h.model.scroll != step.want {
			t.Errorf("%s: scrolled %d rows, want %d", step.name, h.model.scroll, step.want)
		}
	}
}

func TestScrollingMovesWhatScrollsUnderTheRowsOverItWhichStayPut(t *testing.T) {
	h := yearHarness(t, yearSize)
	y := h.overTheView()
	above, first := strings.Split(h.view(), "\n")[:y], h.scrolled()

	h.press("j")
	if got := h.scrolled(); got == first {
		t.Errorf("a row down, the first row shown is still %q, want the next", got)
	}
	if got := strings.Split(h.view(), "\n")[:y]; strings.Join(got, "\n") != strings.Join(above, "\n") {
		t.Errorf("the title row, the status line and the heading row moved, to\n%s", strings.Join(got, "\n"))
	}
}

// drawnAt is the cell text is first drawn from on the screen: its column
// and its row, failing the test where it's drawn nowhere.
func (h *harness) drawnAt(text string) (x, y int) {
	h.t.Helper()
	for y, row := range strings.Split(h.view(), "\n") {
		if before, _, ok := strings.Cut(row, text); ok {
			return ansi.StringWidth(before), y
		}
	}
	h.t.Fatalf("the screen is\n%s\nwant %q on it", h.view(), text)
	return 0, 0
}

// wheel turns the wheel, down or up, over the cell at x along row y.
func (h *harness) wheel(button tea.MouseButton, x, y int) {
	h.t.Helper()
	h.deliver(h.update(tea.MouseWheelMsg{X: x, Y: y, Button: button})...)
}

func TestTheWheelScrollsTheViewUnderThePointerAndNothingElse(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{yearSize, {Width: 52, Height: 30}} {
		t.Run(fmt.Sprintf("%dx%d", size.Width, size.Height), func(t *testing.T) {
			h := yearHarness(t, size)
			most := h.model.scrolling().Most
			if most == 0 {
				t.Fatalf("the Year scrolls no rows at %dx%d, want some", size.Width, size.Height)
			}
			yearX, yearY := 10, h.overTheView()

			h.wheel(tea.MouseWheelDown, yearX, yearY)
			if want := min(wheelRows, most); h.model.scroll != want {
				t.Errorf("a turn down over the Year scrolled %d rows, want %d", h.model.scroll, want)
			}
			h.wheel(tea.MouseWheelUp, yearX+1, yearY)
			if h.model.scroll != 0 {
				t.Errorf("a turn up over the Year scrolled to %d, want its top", h.model.scroll)
			}
			for _, text := range []string{" SWITCHBOARD ", " Runway ", "● healthy", "q quit"} {
				x, y := h.drawnAt(text)
				h.wheel(tea.MouseWheelDown, x, y)
				if h.model.scroll != 0 {
					t.Errorf("a turn down over %q, at %d,%d, scrolled the Year %d rows, want none", text, x, y, h.model.scroll)
				}
			}
		})
	}
}

func TestTheWheelScrollsTheViewNoFurtherThanItGoes(t *testing.T) {
	h := yearHarness(t, yearSize)
	x, y := 10, h.overTheView()

	for range 5 {
		h.wheel(tea.MouseWheelDown, x, y)
	}
	if h.model.scroll != 14 {
		t.Errorf("five turns down scrolled %d rows, want 14, as far as the Year goes", h.model.scroll)
	}
	h.wheel(tea.MouseWheelUp, x, y)
	if h.model.scroll != 14-wheelRows {
		t.Errorf("a turn up scrolled to %d rows, want %d", h.model.scroll, 14-wheelRows)
	}
}

// moreAbove is USAGE's line over its accounts while some are scrolled out of
// view above, as in "▲ 2 more accounts"; and rowsAbove its line while it's
// shown whole, as in "▲ 2 more above".
var (
	moreAbove = regexp.MustCompile(`▲ \d+ more accounts?`)
	rowsAbove = regexp.MustCompile(`▲ \d+ more above`)
)

// tenRouted is the router's document of ten accounts, Account A to Account
// J, more than USAGE shows at once in a short terminal.
func tenRouted() status.Document {
	accounts := make([]status.Account, 10)
	for i := range accounts {
		accounts[i] = account(string(rune('a'+i)), "Account "+string(rune('A'+i)), session(0.1, 3*time.Hour), week(0.2))
	}
	return routerDocument(accounts...)
}

func TestUSAGEScrollsAnAccountAtATimeAsTheFocusTheKeysAndTheWheelOverItMoveIt(t *testing.T) {
	h := routedHarness(t, tenRouted())
	h.update(tea.WindowSizeMsg{Width: 160, Height: 22})
	h.start()
	s := h.model.scrolling()
	if s.Most == 0 || s.Page == 0 || s.Page >= 10 {
		t.Fatalf("USAGE scrolls %+v, want some accounts out of view, ten in 22 rows", s)
	}
	scrolled := func(step string, want int) {
		t.Helper()
		if h.model.scroll != want {
			t.Errorf("%s, USAGE is scrolled %d accounts, want %d", step, h.model.scroll, want)
		}
		if above := moreAbove.MatchString(h.view()); above != (want > 0) {
			t.Errorf("%s, the screen is\n%s\nwant ▲ over USAGE's accounts only while some are above", step, h.view())
		}
	}

	for range s.Page - 1 {
		h.keys(downKey)
	}
	scrolled("↓ to the last account in view", 0)
	// Its accounts a row apart, as it gives way at 22 rows, the ▲ come over
	// them takes an account's row, so it scrolls on till the next shows.
	h.keys(downKey)
	scrolled("↓ past it", 2)
	for range s.Page {
		h.keys(upKey)
	}
	scrolled("↑ back to the first", 0)

	h.keys(tea.KeyPressMsg{Code: 'j', Text: "j"})
	scrolled("j", 1)
	h.keys(tea.KeyPressMsg{Code: 'k', Text: "k"})
	scrolled("k", 0)

	x, y := h.drawnAt("Account A")
	h.wheel(tea.MouseWheelDown, x, y)
	scrolled("a turn of the wheel down over USAGE, an account", 1)
	h.wheel(tea.MouseWheelUp, x, y)
	scrolled("a turn up", 0)
	x, y = h.drawnAt("ROUTING ")
	h.wheel(tea.MouseWheelDown, x, y)
	scrolled("a turn down over ROUTING's heading", 0)
}

// Where USAGE scrolls whole, a row at a time, as only the document drawn
// says, a turn of the wheel over it scrolls wheelRows rows, as j scrolls
// one: from 14 rows, as under them USAGE shows no rows to scroll.
func TestTheWheelScrollsUSAGEByRowsWhereItScrollsWhole(t *testing.T) {
	found := 0
	for _, width := range []int{52, 100, 160} {
		for height := 14; height <= 40; height++ {
			h := routedHarness(t, tenRouted())
			h.update(tea.WindowSizeMsg{Width: width, Height: height})
			h.start()
			f, at := h.model.onScreen()
			if f.Shown() != dashboard.Overview || !f.ScrollsRows(h.model.doc, at) || h.model.scrolling().Most <= wheelRows {
				continue
			}
			y := -1
			for row := range height {
				if f.Under(h.model.doc, at, 2, row) == dashboard.UnderView {
					y = row
					break
				}
			}
			if y < 0 {
				t.Errorf("%dx%d: USAGE scrolls by rows, but no cell is over it", width, height)
				continue
			}
			found++
			t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
				h.wheel(tea.MouseWheelDown, 2, y)
				if h.model.scroll != wheelRows {
					t.Errorf("a turn down scrolled USAGE to %d, want %d rows", h.model.scroll, wheelRows)
				}
				h.press("j")
				if h.model.scroll != wheelRows+1 {
					t.Errorf("j then scrolled USAGE to %d, want %d", h.model.scroll, wheelRows+1)
				}
			})
		}
	}
	if found == 0 {
		t.Fatal("no size scrolls USAGE whole, want some")
	}
}

// Where what the view's scroll counts changes, rows where it counted
// accounts or accounts where it counted rows, the view goes back to its top,
// as History's does on any change, so no offset is read in the other's
// unit: USAGE of ten accounts at 120 columns, shown whole or an account at a
// time, as b, a resize, a section's key, w or a read changes it.
func TestAViewWhoseScrollChangesWhatItCountsGoesBackToItsTop(t *testing.T) {
	withFable := func(used float64) status.Document {
		doc := tenRouted()
		for i := range doc.Accounts {
			doc.Accounts[i].Windows = append(doc.Accounts[i].Windows, fableWeek(used))
		}
		return doc
	}
	tests := []struct {
		name    string
		doc     status.Document
		by      dashboard.By
		size    tea.WindowSizeMsg
		changed func(*harness)
	}{
		{name: "w, from accounts to rows", doc: withFable(0.3), size: tea.WindowSizeMsg{Width: 120, Height: 30}, changed: func(h *harness) { h.press("w") }},
		{name: "b, from rows to accounts", size: tea.WindowSizeMsg{Width: 120, Height: 24}, changed: func(h *harness) { h.press("b") }},
		{name: "b, from accounts to rows", by: dashboard.ByAccount, size: tea.WindowSizeMsg{Width: 120, Height: 24}, changed: func(h *harness) { h.press("b") }},
		{name: "a resize, from rows to accounts", size: tea.WindowSizeMsg{Width: 120, Height: 27}, changed: func(h *harness) { h.update(tea.WindowSizeMsg{Width: 120, Height: 28}) }},
		{name: "a section's key, from rows to accounts", size: tea.WindowSizeMsg{Width: 120, Height: 27}, changed: func(h *harness) { h.press("r") }},
		{name: "a read of a model's window nearing its limit on each account, from accounts to rows", size: tea.WindowSizeMsg{Width: 120, Height: 30}, changed: func(h *harness) {
			h.startRouter(withFable(0.6))
			h.tickUntilAsked()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.doc
			if doc.Accounts == nil {
				doc = tenRouted()
			}
			h := routedHarness(t, doc)
			h.model.by = tt.by
			h.update(tt.size)
			h.start()
			f, at := h.model.onScreen()
			wasRows := f.ScrollsRows(h.model.doc, at)
			for range 2 {
				h.press("j")
			}
			if h.model.scroll != 2 {
				t.Fatalf("j twice scrolled USAGE to %d, want 2", h.model.scroll)
			}
			tt.changed(h)
			f, at = h.model.onScreen()
			if f.ScrollsRows(h.model.doc, at) == wasRows {
				t.Fatalf("USAGE scrolls a row at a time %t before and after, want what it counts changed", wasRows)
			}
			if view := h.view(); h.model.scroll != 0 || moreAbove.MatchString(view) || rowsAbove.MatchString(view) {
				t.Errorf("USAGE is scrolled %d, the screen\n%s\nwant it at its top", h.model.scroll, view)
			}
		})
	}
}

func TestAScrollThatMovesNothingLeavesUSAGEScrolledNoFurtherThanItsDrawn(t *testing.T) {
	short, tall := tea.WindowSizeMsg{Width: 160, Height: 22}, tea.WindowSizeMsg{Width: 160, Height: 80}
	h := routedHarness(t, tenRouted())
	h.update(short)
	h.start()
	h.keys(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if h.model.scroll == 0 {
		t.Fatal("PgDn left USAGE unscrolled, want it scrolled down")
	}
	h.update(tall)
	if most := h.model.scrolling().Most; most != 0 {
		t.Fatalf("a tall terminal scrolls USAGE %d accounts, want it to fit", most)
	}
	h.keys(tea.KeyPressMsg{Code: 'j', Text: "j"})
	h.update(short)
	if h.model.scroll != 0 || moreAbove.MatchString(h.view()) {
		t.Errorf("j, where USAGE fit, then a short terminal again: USAGE is scrolled %d accounts, the screen\n%s\nwant it unscrolled, as it was drawn when j moved nothing", h.model.scroll, h.view())
	}
}

func TestTheMouseIsOnWhileTheDashboardRuns(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	for _, size := range []tea.WindowSizeMsg{{Width: 150, Height: 50}, {Width: 150, Height: 80}} {
		h.update(size)
		if got := h.model.View().MouseMode; got != tea.MouseModeCellMotion {
			t.Errorf("at %dx%d, the mouse mode is %v, want clicks and the wheel, whether or not the view scrolls", size.Width, size.Height, got)
		}
	}
}

func TestAScrollTheScreenNoLongerTakesIsDrawnAsFarAsItGoes(t *testing.T) {
	h := yearHarness(t, yearSize)
	h.update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	h.update(tea.WindowSizeMsg{Width: yearSize.Width, Height: yearSize.Height + 1})
	if most := h.model.scrolling().Most; most != 13 {
		t.Fatalf("a taller terminal scrolls %d rows at most, want 13", most)
	}
	at := h.scrolled()
	h.press("k")
	if h.model.scroll != 12 || h.scrolled() == at {
		t.Errorf("k, from as far as the Year goes, scrolled to %d, showing %q; want 12, a row up from where it was drawn", h.model.scroll, h.scrolled())
	}
}

func TestTheViewScrollsAsFarAsItsDrawnWhileTheBarsRise(t *testing.T) {
	accounts := make([]status.Account, 12)
	for i := range accounts {
		accounts[i] = account(string(rune('a'+i)), "Account "+string(rune('A'+i)), session(0.1, 3*time.Hour), week(0.2), fableWeek(0.1))
	}
	h := newHarness(t, document(accounts...))
	h.update(tea.WindowSizeMsg{Width: 150, Height: 24})
	h.start()
	now := h.model.now()
	if got, want := h.model.scrolling(), h.model.frame(now).Scrolling(h.model.doc, now); got != want || got.Most == 0 {
		t.Errorf("as the bars rise from nothing, the Overview scrolls %+v, want %+v, as far as USAGE drawn, Fable's bars among them, goes", got, want)
	}
}

func TestAScrollGoesNoFurtherThanWhatItScrolls(t *testing.T) {
	tall, fits := dashboard.Scrolling{Most: 10, Page: 4}, dashboard.Scrolling{Page: 4}
	tests := []struct {
		name string
		by   scrollStep
		at   int
		s    dashboard.Scrolling
		want int
	}{
		{name: "a row down", by: scrollStep{rows: 1}, at: 3, s: tall, want: 4},
		{name: "a row up", by: scrollStep{rows: -1}, at: 3, s: tall, want: 2},
		{name: "a page down", by: scrollStep{pages: 1}, at: 3, s: tall, want: 7},
		{name: "a page up", by: scrollStep{pages: -1}, at: 7, s: tall, want: 3},
		{name: "a page down, past the foot", by: scrollStep{pages: 1}, at: 8, s: tall, want: 10},
		{name: "a page up, past the top", by: scrollStep{pages: -1}, at: 2, s: tall, want: 0},
		{name: "a row up from further than it now goes", by: scrollStep{rows: -1}, at: 14, s: tall, want: 9},
		{name: "nowhere, where it fits", by: scrollStep{rows: 1}, at: 0, s: fits, want: 0},
	}
	for _, tt := range tests {
		if got := tt.by.from(tt.at, tt.s); got != tt.want {
			t.Errorf("%s: from %d, scrolled to %d, want %d", tt.name, tt.at, got, tt.want)
		}
	}
}

func TestTheWheelScrollsHelpUnderThePointerAndNotThePageAroundIt(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 150, Height: 12}, {Width: 52, Height: 16}} {
		t.Run(fmt.Sprintf("%dx%d", size.Width, size.Height), func(t *testing.T) {
			h := newHarness(t, twelve())
			h.update(size)
			h.start()
			h.press("?")
			f, _, _ := h.model.screen(h.clock.now)
			most := f.CardScrolling().Most
			if most == 0 {
				t.Fatalf("help scrolls no rows at %dx%d, want some", size.Width, size.Height)
			}
			left, top := h.drawnAt("╭─ HELP · OVERVIEW")

			for _, cell := range [][2]int{{0, 0}, {left - 1, top + 2}, {left, top - 1}, {size.Width / 2, size.Height - 1}} {
				h.wheel(tea.MouseWheelDown, cell[0], cell[1])
				if h.model.help.scroll != 0 || h.model.scroll != 0 {
					t.Errorf("a turn down at %d,%d, off help's card, scrolled help %d rows and the page %d; want neither", cell[0], cell[1], h.model.help.scroll, h.model.scroll)
				}
			}
			h.wheel(tea.MouseWheelDown, left+4, top+2)
			if want := min(wheelRows, most); h.model.help.scroll != want || h.model.scroll != 0 {
				t.Errorf("a turn down over help's card scrolled it %d rows and the page %d; want help %d, the page none", h.model.help.scroll, h.model.scroll, want)
			}
			h.wheel(tea.MouseWheelUp, left, top)
			if h.model.help.scroll != 0 {
				t.Errorf("a turn up on help's edge scrolled it to %d, want its top", h.model.help.scroll)
			}
		})
	}
}

func TestHelpScrollsWithinItsCard(t *testing.T) {
	h := newHarness(t, twelve())
	h.update(tea.WindowSizeMsg{Width: 150, Height: 12})
	h.start()
	h.press("?")
	f, _, _ := h.model.screen(h.clock.now)
	s := f.CardScrolling()
	if s.Most <= s.Page {
		t.Fatalf("help scrolls %d rows, a page of %d, want more than a page", s.Most, s.Page)
	}

	steps := []struct {
		name string
		key  tea.KeyPressMsg
		want int
	}{
		{name: "j, a row down", key: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: min(1, s.Most)},
		{name: "PgDn, a page", key: tea.KeyPressMsg{Code: tea.KeyPgDown}, want: min(1+s.Page, s.Most)},
		{name: "PgDn, no further than it goes", key: tea.KeyPressMsg{Code: tea.KeyPgDown}, want: min(1+2*s.Page, s.Most)},
		{name: "k, a row up", key: tea.KeyPressMsg{Code: 'k', Text: "k"}, want: max(min(1+2*s.Page, s.Most)-1, 0)},
		{name: "PgUp, past the top", key: tea.KeyPressMsg{Code: tea.KeyPgUp}, want: max(min(1+2*s.Page, s.Most)-1-s.Page, 0)},
	}
	for _, step := range steps {
		h.update(step.key)
		if h.model.help.scroll != step.want || h.model.scroll != 0 || !h.inHelp() {
			t.Errorf("%s: help scrolled %d rows, the page %d, help open %v; want help %d, the page none, help open", step.name, h.model.help.scroll, h.model.scroll, h.inHelp(), step.want)
		}
	}
	h.press("?")
	h.press("?")
	if h.model.help.scroll != 0 {
		t.Errorf("help opened again scrolled %d rows, want its top", h.model.help.scroll)
	}
}

func TestTheWheelDoesNothingWhileTheRoutingCardIsOpen(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.update(tea.WindowSizeMsg{Width: 150, Height: 20})
	h.start()
	h.press("p")
	before := h.model.routingCard(h.clock.now)
	left, top := h.drawnAt("╭─ ROUTING")
	for _, cell := range [][2]int{{left + 4, top + 3}, {0, top + 3}, {0, 0}} {
		h.wheel(tea.MouseWheelDown, cell[0], cell[1])
		if after := h.model.routingCard(h.clock.now); h.model.scroll != 0 || after.Cursor != before.Cursor || h.model.open != routingOpen {
			t.Errorf("with the routing card open, a turn down at %d,%d scrolled the page %d rows, moved the cursor from %d to %d, open %v; want nothing moved",
				cell[0], cell[1], h.model.scroll, before.Cursor, after.Cursor, h.model.open)
		}
	}
}

func TestOnlyTheWheelScrolls(t *testing.T) {
	h := yearHarness(t, yearSize)
	x, y := 10, h.overTheView()
	for _, button := range []tea.MouseButton{tea.MouseWheelLeft, tea.MouseWheelRight, tea.MouseLeft} {
		h.wheel(button, x, y)
		if h.model.scroll != 0 {
			t.Errorf("button %v scrolled the Year %d rows, want none", button, h.model.scroll)
		}
	}
}

func TestAScrollLaysTheOverviewOutAfreshOnlyWhereItMoves(t *testing.T) {
	over := func(button tea.MouseButton) func(*harness) {
		return func(h *harness) {
			x, y := h.drawnAt("Account A")
			h.wheel(button, x, y)
		}
	}
	tests := []struct {
		name   string
		input  func(*harness)
		scroll int
		afresh bool
	}{
		{name: "k at the top", input: func(h *harness) { h.keys(tea.KeyPressMsg{Code: 'k', Text: "k"}) }},
		{name: "PgUp at the top", input: func(h *harness) { h.keys(tea.KeyPressMsg{Code: tea.KeyPgUp}) }},
		{name: "the wheel turned up over USAGE at the top", input: over(tea.MouseWheelUp)},
		{name: "j", input: func(h *harness) { h.keys(tea.KeyPressMsg{Code: 'j', Text: "j"}) }, scroll: 1, afresh: true},
		{name: "the wheel turned down over USAGE", input: over(tea.MouseWheelDown), scroll: 1, afresh: true},
	}
	var laid []dashboard.Layout
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, tenRouted())
			h.update(tea.WindowSizeMsg{Width: 160, Height: 22})
			h.start()
			h.deliver(h.press("s")...)
			h.source.sessions = []status.Session{
				sessionOn(idD28C, "claude-opus-5-5", "a", 10*time.Second), sessionOn(id7F3A, "claude-sonnet-5-5", "b", time.Minute),
			}
			h.tickUntilAsked()
			kept := h.model.layout
			tt.input(h)
			if h.model.scroll != tt.scroll {
				t.Fatalf("USAGE is scrolled %d accounts, want %d", h.model.scroll, tt.scroll)
			}
			if afresh := !sameLayout(h.model.layout, kept); afresh != tt.afresh {
				t.Errorf("laid out afresh %v, want %v: only a scroll that moves USAGE lets the layout kept go, as sessions started", afresh, tt.afresh)
			}
			if tt.afresh {
				laid = append(laid, h.model.layout)
			}
		})
	}
	if len(laid) == 2 && !sameLayout(laid[0], laid[1]) {
		t.Errorf("the wheel laid the Overview out as %+v, want it as j does, %+v", laid[1], laid[0])
	}
}
