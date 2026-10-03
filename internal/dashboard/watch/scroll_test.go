package watch

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/status"
)

// twelve is the probe of twelve accounts, more than a terminal 150 by 50
// shows at once: in six rows of two compact cards, 47 rows of them, of which
// 39 show over RECENT's strip.
func twelve() status.Document {
	accounts := make([]status.Account, 12)
	for i := range accounts {
		accounts[i] = account(string(rune('a'+i)), "Account "+string(rune('A'+i)), session(0.1, 3*time.Hour), week(0.2))
	}
	return document(accounts...)
}

// scrolled is the first row of cards the screen shows, under the heading.
func (h *harness) scrolled() string {
	return strings.Split(h.view(), "\n")[7]
}

func TestScrollKeysScrollTheCardsNoFurtherThanTheyGo(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	if got := h.model.scrolling(); got.Most != 8 || got.Page != 39 {
		t.Fatalf("scrolling() = %+v, want 8 rows at most, a page of 39", got)
	}
	steps := []struct {
		name string
		key  tea.KeyPressMsg
		want int
	}{
		{name: "j, a row down", key: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: 1},
		{name: "j again", key: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: 2},
		{name: "k, a row up", key: tea.KeyPressMsg{Code: 'k', Text: "k"}, want: 1},
		{name: "PgDn, a page, but no further than they go", key: tea.KeyPressMsg{Code: tea.KeyPgDown}, want: 8},
		{name: "j at the foot, nowhere", key: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: 8},
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

func TestScrollingMovesTheCardsUnderTheHeadingWhichStaysPut(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	heading := strings.Split(h.view(), "\n")[:7]

	h.press("j")
	if got := h.scrolled(); !strings.HasPrefix(got, " │  ● open ") {
		t.Errorf("a row down, the first row shown is %q, want the cards' second", got)
	}
	if got := strings.Split(h.view(), "\n")[:7]; strings.Join(got, "\n") != strings.Join(heading, "\n") {
		t.Errorf("the title row and the heading moved, to\n%s", strings.Join(got, "\n"))
	}
	lines := strings.Split(h.view(), "\n")
	if got := strings.TrimSpace(lines[48]); got != "▲ 2 more accounts above · ▼ 2 more accounts below · j/k or wheel to scroll" {
		t.Errorf("the line over the footer says %q, want what's out of view", got)
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
	if h.model.scroll != 8 {
		t.Errorf("three turns down scrolled %d rows, want 8, as far as they go", h.model.scroll)
	}
	h.update(up)
	if h.model.scroll != 8-wheelRows {
		t.Errorf("a turn up scrolled to %d rows, want %d", h.model.scroll, 8-wheelRows)
	}
}

func TestTheScreenAsksForTheWheelOnlyWhileTheCardsScroll(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	if got := h.model.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("while the cards scroll, the mouse mode is %v, want the wheel's", got)
	}
	h.update(tea.WindowSizeMsg{Width: 150, Height: 80})
	if got := h.model.View().MouseMode; got != tea.MouseModeNone {
		t.Errorf("once they fit, the mouse mode is %v, want none, leaving the terminal's selecting to it", got)
	}
}

func TestAScrollTheScreenNoLongerTakesIsDrawnAsFarAsItGoes(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	h.update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	h.update(tea.WindowSizeMsg{Width: 150, Height: 54})
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

func TestTheWheelDoesNothingWhileTheHelpIsOpen(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	h.press("?")
	h.update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	h.press("j")
	if h.model.scroll != 0 {
		t.Errorf("with the help open, the cards scrolled %d rows, want none", h.model.scroll)
	}
}
