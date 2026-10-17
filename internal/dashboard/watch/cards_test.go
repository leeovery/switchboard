package watch

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestPOpensTheRoutingCardAndEscClosesIt(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	h.press("p")
	if h.model.open != routingOpen || !strings.Contains(h.view(), "╭─ ROUTING · NEW SESSIONS ─") {
		t.Fatalf("after p, the screen is\n%s\nwant the routing card over it", h.view())
	}
	h.update(escKey)
	if h.model.open != nothingOpen || strings.Contains(h.view(), "ROUTING · ") {
		t.Errorf("after esc, the screen is\n%s\nwant the routing card closed", h.view())
	}
}

func TestTheRoutingCardTakesEveryKeyButCtrlC(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.press("p")
	for _, key := range []string{"w", "u", "g", "b", "down", "t", "j", "s", " ", "tab", "?", "r", "R", "1", "a", "m", "q"} {
		if quits(h.press(key)) {
			t.Errorf("with the routing card open, %q quit, want it taken by the card", key)
		}
	}
	if h.model.open != routingOpen || h.model.sections != (dashboard.Sections{}) || h.model.by != dashboard.ByWindow || h.model.focus != "work" ||
		h.model.view != dashboard.Overview || len(h.source.orders) > 0 || h.model.fetching || h.model.picker.open {
		t.Errorf("with the routing card open, keys acted: open %v, sections %v, by %v, focus %q, showing %q, orders %q, reading %v, picker open %v; want them all taken by the card",
			h.model.open, h.model.sections.Names(), h.model.by, h.model.focus, h.model.view, h.source.orders, h.model.fetching, h.model.picker.open)
	}
	if !quits(h.press("ctrl+c")) {
		t.Error("with the routing card open, ctrl+c didn't quit, want it to")
	}
}

func TestNoCardOpensOverThePicker(t *testing.T) {
	h, _ := settledThemedHarness(t, theme.One("amber"))
	h.typed("t")
	for _, key := range []string{"p", "?"} {
		h.press(key)
		if h.model.open != nothingOpen || !h.model.picker.open {
			t.Errorf("with the picker open, %s opened %v, the picker open %v; want the picker alone", key, h.model.open, h.model.picker.open)
		}
	}
}

func TestThePickerNeverOpensOverACard(t *testing.T) {
	for _, tt := range []struct {
		key  string
		card opened
	}{{key: "p", card: routingOpen}, {key: "?", card: helpOpen}} {
		t.Run(tt.key, func(t *testing.T) {
			h, _ := themedHarness(t, theme.One("amber"))
			h.startRouter(routerDocument(three()...))
			h.start()
			h.answer(darkBackground)

			listing := h.press("t")
			h.press(tt.key)
			h.deliver(listing...)
			if h.model.open != tt.card || h.model.picker.open {
				t.Fatalf("the themes listed once %s opened its card, the card is %v, the picker open %v; want the card alone", tt.key, h.model.open, h.model.picker.open)
			}

			h.update(escKey)
			h.typed("t")
			if !h.model.picker.open {
				t.Error("the card closed, t left the picker shut, want it open")
			}
		})
	}
}

// The page under a card stands still, as the card opened over it, but for
// the heading's clock, which ticks on.
func TestThePageUnderACardStandsStillButForItsClock(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.settle()
	h.press("p")
	opened, under := h.clock.now.Format(time.TimeOnly), h.view()

	moved := routerDocument(three()...)
	moved.Accounts[0].Windows[0].Utilization = 0.9
	h.startRouter(moved)
	h.tickUntil(at(13, 12, 30))
	got := h.view()
	if behindCard(got) != behindCard(under) || !strings.Contains(got, "5-hour 90%") {
		t.Errorf("half a minute on, a look read since, the screen is\n%s\nwant the page as the card opened over it, and the card's figures as read since\n%s", got, under)
	}
	if now := h.clock.now.Format(time.TimeOnly); clockOf(under) != opened || clockOf(got) != now {
		t.Errorf("the clock read %s as the card opened and %s half a minute on, want %s, then ticked on to %s", clockOf(under), clockOf(got), opened, now)
	}
	if _, ok := h.pendingFrame(); ok {
		t.Error("a read eased a bar under the card, a frame armed, want none: the page under it is still")
	}

	h.update(escKey)
	if view := h.view(); clockOf(view) != h.clock.now.Format(time.TimeOnly) || behindCard(view) == behindCard(under) {
		t.Errorf("the card closed, the screen is\n%s\nwant the page as it stands now", view)
	}
}

// clockOf is the time the heading's clock reads on the screen: the title
// row's last word.
func clockOf(view string) string {
	title, _, _ := strings.Cut(view, "\n")
	words := strings.Fields(title)
	return words[len(words)-1]
}

// unclocked is the screen with the heading's clock, which ticks on under a
// card, blanked: the page behind a card, which stands still, alone.
func unclocked(view string) string {
	title, page, _ := strings.Cut(view, "\n")
	clock := strings.LastIndex(title, clockOf(view))
	return title[:clock] + strings.Repeat(" ", len(title)-clock) + "\n" + page
}

// behindCard is the screen with the routing card's box over it blanked, and
// the heading's clock, as unclocked has it: the page behind it alone.
func behindCard(view string) string {
	view = unclocked(view)
	rows := strings.Split(view, "\n")
	top := slices.IndexFunc(rows, func(row string) bool { return strings.Contains(row, "╭─ ROUTING") })
	if top < 0 {
		return view
	}
	x := utf8.RuneCountInString(rows[top][:strings.Index(rows[top], "╭─ ROUTING")])
	right := x + slices.Index([]rune(rows[top])[x:], '╮')
	for y := top; y < len(rows); y++ {
		cells := []rune(rows[y])
		foot := cells[x] == '╰'
		for i := x; i <= right; i++ {
			cells[i] = ' '
		}
		rows[y] = string(cells)
		if foot {
			break
		}
	}
	return strings.Join(rows, "\n")
}

func TestThePageUnderACardFollowsTheTerminalsSize(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.press("?")
	h.clock.now = start.Add(5 * time.Second)
	h.update(tea.WindowSizeMsg{Width: 120, Height: 30})

	rows := strings.Split(h.view(), "\n")
	if len(rows) != 30 || clockOf(h.view()) != "13:12:05" || !strings.Contains(h.view(), "╭─ HELP · OVERVIEW ─") {
		t.Errorf("resized under help, the screen is\n%s\nwant it 30 rows tall, the page as help opened over it, its clock ticking on, help over it", h.view())
	}
}

func TestTheRoutingCardIsBuiltAfreshAtEachRead(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.press("p")

	h.startRouter(four())
	h.tickUntilAsked()
	if rows := h.model.routingCard(h.clock.now).Rows; len(rows) != 5 || rows[4].Account != "client" {
		t.Errorf("the card's rows are %+v, want auto's and the four accounts' the look read", rows)
	}
}
