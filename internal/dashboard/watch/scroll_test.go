package watch

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

// twelve is the probe of twelve accounts, more than a terminal 150 by 50
// shows at once: in six rows of two compact cards, 47 rows of them, of which
// 42 show over RECENT's strip.
func twelve() status.Document {
	accounts := make([]status.Account, 12)
	for i := range accounts {
		accounts[i] = account(string(rune('a'+i)), "Account "+string(rune('A'+i)), session(0.1, 3*time.Hour), week(0.2))
	}
	return document(accounts...)
}

// scrolled is the first row of cards the screen shows, under the status line.
func (h *harness) scrolled() string {
	return strings.Split(h.view(), "\n")[4]
}

func TestScrollKeysScrollTheCardsNoFurtherThanTheyGo(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	if got := h.model.scrolling(); got.Most != 5 || got.Page != 42 {
		t.Fatalf("scrolling() = %+v, want 5 rows at most, a page of 42", got)
	}
	steps := []struct {
		name string
		key  tea.KeyPressMsg
		want int
	}{
		{name: "j, a row down", key: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: 1},
		{name: "j again", key: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: 2},
		{name: "k, a row up", key: tea.KeyPressMsg{Code: 'k', Text: "k"}, want: 1},
		{name: "PgDn, a page, but no further than they go", key: tea.KeyPressMsg{Code: tea.KeyPgDown}, want: 5},
		{name: "j at the foot, nowhere", key: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: 5},
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

func TestScrollingMovesTheCardsUnderTheStatusLineWhichStaysPut(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	above := strings.Split(h.view(), "\n")[:4]

	h.press("j")
	if got := h.scrolled(); !strings.HasPrefix(got, " │  ● open ") {
		t.Errorf("a row down, the first row shown is %q, want the cards' second", got)
	}
	if got := strings.Split(h.view(), "\n")[:4]; strings.Join(got, "\n") != strings.Join(above, "\n") {
		t.Errorf("the title row and the status line moved, to\n%s", strings.Join(got, "\n"))
	}
	lines := strings.Split(h.view(), "\n")
	if got := strings.TrimSpace(lines[48]); got != "▲ 2 more accounts above · ▼ 2 more accounts below · j/k or wheel to scroll" {
		t.Errorf("the line over the key line says %q, want what's out of view", got)
	}
}

func TestTheWheelScrollsTheCards(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	down, up := tea.MouseWheelMsg{Button: tea.MouseWheelDown}, tea.MouseWheelMsg{Button: tea.MouseWheelUp}

	h.update(down)
	if h.model.scroll != wheelRows {
		t.Errorf("a turn down scrolled %d rows, want %d", h.model.scroll, wheelRows)
	}
	h.update(down)
	h.update(down)
	if h.model.scroll != 5 {
		t.Errorf("three turns down scrolled %d rows, want 5, as far as they go", h.model.scroll)
	}
	h.update(up)
	if h.model.scroll != 5-wheelRows {
		t.Errorf("a turn up scrolled to %d rows, want %d", h.model.scroll, 5-wheelRows)
	}
}

func TestTheMouseIsOnWhileTheDashboardRuns(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	for _, size := range []tea.WindowSizeMsg{{Width: 150, Height: 50}, {Width: 150, Height: 80}} {
		h.update(size)
		if got := h.model.View().MouseMode; got != tea.MouseModeCellMotion {
			t.Errorf("at %dx%d, the mouse mode is %v, want clicks and the wheel, whether or not the cards scroll", size.Width, size.Height, got)
		}
	}
}

func TestAScrollTheScreenNoLongerTakesIsDrawnAsFarAsItGoes(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	h.update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	h.update(tea.WindowSizeMsg{Width: 150, Height: 51})
	if most := h.model.scrolling().Most; most != 4 {
		t.Fatalf("a taller terminal scrolls %d rows at most, want 4", most)
	}
	at := h.scrolled()
	h.press("k")
	if h.model.scroll != 3 || h.scrolled() == at {
		t.Errorf("k, from as far as the cards go, scrolled to %d, showing %q; want 3, a row up from where they were drawn", h.model.scroll, h.scrolled())
	}
}

func TestTheCardsScrollAsFarAsTheyreDrawnWhileTheBarsRise(t *testing.T) {
	accounts := make([]status.Account, 12)
	for i := range accounts {
		accounts[i] = account(string(rune('a'+i)), "Account "+string(rune('A'+i)), session(0.1, 3*time.Hour), week(0.2), fableWeek(0.1))
	}
	h := newHarness(t, document(accounts...))
	h.start()
	now := h.model.now()
	if got, want := h.model.scrolling(), h.model.frame(now).Scrolling(h.model.doc, now); got != want {
		t.Errorf("as the bars rise from nothing, the cards scroll %+v, want %+v, as far as the cards drawn, Fable's bars among them, go", got, want)
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

func TestTheWheelScrollsHelpWhileItsOpenAndNotThePageUnderIt(t *testing.T) {
	h := newHarness(t, twelve())
	h.update(tea.WindowSizeMsg{Width: 150, Height: 20})
	h.start()
	h.press("?")
	f, _, _ := h.model.screen(h.clock.now)
	most := f.CardScrolling().Most

	h.update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if want := min(wheelRows, most); h.model.help.scroll != want || h.model.scroll != 0 {
		t.Errorf("with help open, a turn down scrolled help %d rows and the cards %d; want help %d, the cards none", h.model.help.scroll, h.model.scroll, want)
	}
	h.update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if h.model.help.scroll != 0 {
		t.Errorf("a turn up scrolled help to %d, want its top", h.model.help.scroll)
	}
}

func TestHelpScrollsWithinItsCard(t *testing.T) {
	h := newHarness(t, twelve())
	h.update(tea.WindowSizeMsg{Width: 150, Height: 20})
	h.start()
	h.press("?")
	f, _, _ := h.model.screen(h.clock.now)
	s := f.CardScrolling()

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
			t.Errorf("%s: help scrolled %d rows, the cards %d, help open %v; want help %d, the cards none, help open", step.name, h.model.help.scroll, h.model.scroll, h.inHelp(), step.want)
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
	h.update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if after := h.model.routingCard(h.clock.now); h.model.scroll != 0 || after.Cursor != before.Cursor || h.model.open != routingOpen {
		t.Errorf("with the routing card open, a turn down scrolled the page %d rows, moved the cursor from %d to %d, open %v; want nothing moved", h.model.scroll, before.Cursor, after.Cursor, h.model.open)
	}
}

func TestOnlyTheWheelScrolls(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	for _, button := range []tea.MouseButton{tea.MouseWheelLeft, tea.MouseWheelRight, tea.MouseLeft} {
		h.update(tea.MouseWheelMsg{Button: button})
		if h.model.scroll != 0 {
			t.Errorf("button %v scrolled the cards %d rows, want none", button, h.model.scroll)
		}
	}
}
