package watch

import (
	"errors"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/status"
)

// The keys of the cards, as the terminal sends them.
var (
	leftKey  = tea.KeyPressMsg{Code: tea.KeyLeft}
	rightKey = tea.KeyPressMsg{Code: tea.KeyRight}
	upKey    = tea.KeyPressMsg{Code: tea.KeyUp}
	downKey  = tea.KeyPressMsg{Code: tea.KeyDown}
	spaceKey = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	escKey   = tea.KeyPressMsg{Code: tea.KeyEscape}
)

// The sample sessions' ids, each a Claude Code session's.
const (
	idD28C = "d28c5e17-3a9b-4f60-8c21-7e4d9b0a6f13"
	id7F3A = "7f3a0c94-5d2e-4b18-a6f7-31c8e9d24b05"
	id9E21 = "9e21d4a8-6b0c-4f35-8e72-a39c5d1f0b47"
)

// sessionOn is a session whose requests of a model go to account, put there
// an hour before the clock starts, and seen seen before it.
func sessionOn(id, model, account string, seen time.Duration) status.Session {
	return status.Session{ID: id, Assignments: []status.Assignment{{
		Model: model, Family: strings.Split(model, "-")[1], Account: account, Reason: "new",
		AssignedAt: start.Add(-time.Hour).UTC(), LastSeen: start.Add(-seen).UTC(),
	}}}
}

// four are three and client, which a terminal 160 columns wide lays out
// three to a row of cards, client under work.
func four() status.Document {
	return routerDocument(append(three(), account("client", "Client", session(0.3, 2*time.Hour), week(0.4)))...)
}

// gridHarness is a model of the router of four, idD28C, busy, and 7f3a, idle,
// on work, and 9e21 on client, in a terminal 160 columns wide and 40 tall,
// started.
func gridHarness(t *testing.T) *harness {
	t.Helper()
	h := routedHarness(t, four())
	h.source.sessions = []status.Session{
		sessionOn(idD28C, "claude-opus-5-5", "work", 10*time.Second),
		sessionOn(id9E21, "claude-sonnet-5-5", "client", 20*time.Second),
		sessionOn(id7F3A, "claude-haiku-4-5", "work", 9*time.Minute),
	}
	h.update(tea.WindowSizeMsg{Width: 160, Height: 40})
	h.start()
	return h
}

// keys presses each key in turn, delivering what each sends back.
func (h *harness) keys(keys ...tea.KeyPressMsg) {
	h.t.Helper()
	for _, k := range keys {
		h.deliver(h.update(k)...)
	}
}

// seatOf is the seat of a session's model, as a card's back lists it.
func seatOf(session, model string) dashboard.Seat {
	return dashboard.Seat{Session: session, Model: model}
}

func TestNoCardHasTheFocusTillAKeyGivesItTheFirstInView(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{leftKey, rightKey, upKey, downKey} {
		t.Run(key.String(), func(t *testing.T) {
			h := gridHarness(t)
			if h.model.focus != "" || strings.Contains(h.view(), "┏") {
				t.Fatalf("before a key, %q has the focus, the screen\n%s\nwant no card heavy-edged", h.model.focus, h.view())
			}
			h.keys(key)
			if h.model.focus != "work" || !strings.Contains(h.view(), "┏━ 1 Work") {
				t.Errorf("after %s, %q has the focus, the screen\n%s\nwant work's card, the first, heavy-edged", key, h.model.focus, h.view())
			}
		})
	}
}

func TestTheArrowsMoveTheFocusAroundTheGrid(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.KeyPressMsg
		want string
	}{
		{name: "along the row", keys: []tea.KeyPressMsg{rightKey, rightKey}, want: "personal"},
		{name: "no further than its end", keys: []tea.KeyPressMsg{rightKey, rightKey, rightKey, rightKey}, want: "side"},
		{name: "back along it, no further than its start", keys: []tea.KeyPressMsg{rightKey, rightKey, leftKey, leftKey}, want: "work"},
		{name: "down a row", keys: []tea.KeyPressMsg{rightKey, downKey}, want: "client"},
		{name: "down to a row too short to reach: its last", keys: []tea.KeyPressMsg{rightKey, rightKey, rightKey, downKey}, want: "client"},
		{name: "up again", keys: []tea.KeyPressMsg{rightKey, downKey, upKey}, want: "work"},
		{name: "no further than the last row", keys: []tea.KeyPressMsg{rightKey, downKey, downKey}, want: "client"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := gridHarness(t)
			h.keys(tt.keys...)
			if h.model.focus != tt.want {
				t.Errorf("the focus is on %q, want %q", h.model.focus, tt.want)
			}
		})
	}
}

func TestUpAndDownGoOverAFlippedCardsSessionsFirst(t *testing.T) {
	h := gridHarness(t)
	h.keys(spaceKey)
	if h.model.focus != "work" || !h.model.flipped["work"] {
		t.Fatalf("space flipped %v with the focus on %q, want work's card, the first, flipped", h.model.flipped, h.model.focus)
	}
	h.keys(rightKey, rightKey, rightKey, downKey, spaceKey)
	steps := []struct {
		key      tea.KeyPressMsg
		focus    string
		selected dashboard.Seat
	}{
		{key: upKey, focus: "client", selected: seatOf(id9E21, "claude-sonnet-5-5")},
		{key: upKey, focus: "work"},
		{key: upKey, focus: "work", selected: seatOf(id7F3A, "claude-haiku-4-5")},
		{key: upKey, focus: "work", selected: seatOf(idD28C, "claude-opus-5-5")},
		{key: upKey, focus: "work", selected: seatOf(idD28C, "claude-opus-5-5")},
		{key: downKey, focus: "work", selected: seatOf(id7F3A, "claude-haiku-4-5")},
		{key: downKey, focus: "client"},
		{key: downKey, focus: "client", selected: seatOf(id9E21, "claude-sonnet-5-5")},
		{key: downKey, focus: "client", selected: seatOf(id9E21, "claude-sonnet-5-5")},
		{key: upKey, focus: "work"},
		{key: downKey, focus: "work", selected: seatOf(idD28C, "claude-opus-5-5")},
		{key: rightKey, focus: "personal"},
	}
	for i, s := range steps {
		h.keys(s.key)
		if h.model.focus != s.focus || h.model.selected != s.selected {
			t.Fatalf("step %d, %s: the focus is on %q, %+v picked out, want %q, %+v", i+1, s.key, h.model.focus, h.model.selected, s.focus, s.selected)
		}
	}
}

func TestSpaceFlipsTheCardWithTheFocusAndSEveryCardOrBack(t *testing.T) {
	h := gridHarness(t)
	steps := []struct {
		keys []tea.KeyPressMsg
		want []string
	}{
		{keys: []tea.KeyPressMsg{spaceKey}, want: []string{"work"}},
		{keys: []tea.KeyPressMsg{spaceKey}},
		{keys: []tea.KeyPressMsg{rightKey, spaceKey, {Code: 's', Text: "s"}}, want: []string{"client", "personal", "side", "work"}},
		{keys: []tea.KeyPressMsg{{Code: 's', Text: "s"}}},
		{keys: []tea.KeyPressMsg{{Code: 'S', Text: "S"}}, want: []string{"client", "personal", "side", "work"}},
	}
	for i, s := range steps {
		h.keys(s.keys...)
		var got []string
		for id, on := range h.model.flipped {
			if on {
				got = append(got, id)
			}
		}
		slices.Sort(got)
		if !slices.Equal(got, s.want) {
			t.Errorf("step %d: flipped %q, want %q", i+1, got, s.want)
		}
	}
	if got := strings.Count(h.view(), " sessions ─╮") + strings.Count(h.view(), " sessions ━┓"); got != 4 {
		t.Errorf("the screen is\n%s\nwant every card's top edge saying sessions, where %d of 4 do", h.view(), got)
	}
}

func TestAFlippedCardsBackListsItsSessions(t *testing.T) {
	h := gridHarness(t)
	h.keys(spaceKey, downKey)
	for _, want := range []string{
		"┏━ 1 Work ━",
		"┃  2 sessions  ·  1 busy",
		"┃  ▸ ● d28c  opus    seen       now",
		"┃      ╰ here since 12:12",
		"┃    ○ 7f3a  haiku   idle       9m",
		"┃  ↑↓ select  1-4 move it  space flip",
	} {
		if !strings.Contains(h.view(), want) {
			t.Errorf("the screen is\n%s\nwant %q", h.view(), want)
		}
	}
}

func TestADigitPinsTheSessionPickedOutAndATakesItsPinOff(t *testing.T) {
	log := logstest.Capture(t)
	h := gridHarness(t)
	h.keys(spaceKey, downKey)
	looks := h.source.looks()

	h.deliver(h.press("3")...)
	if want := []string{"pin " + idD28C + " to side"}; !slices.Equal(h.source.orders, want) {
		t.Errorf("orders = %q, want %q", h.source.orders, want)
	}
	if got, want := h.footer(), "d28c goes to side · Side from its next request · d28c selected on Work"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
	if got := h.source.looks(); got != looks+1 {
		t.Errorf("looked at the router's document %d times after the key, want once", got-looks)
	}
	if !strings.Contains(h.view(), "╰ goes to Side from its next request") {
		t.Errorf("the screen is\n%s\nwant d28c's note to say where it goes", h.view())
	}
	if !h.source.router.Pin.IsZero() {
		t.Errorf("the router's pin is %+v, want none: the session's pin is its own", h.source.router.Pin)
	}
	if want := []string{"level=INFO", `msg="pinned a session"`, "session=d28c5e17", "account=side"}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}

	h.deliver(h.press("a")...)
	if want := []string{"pin " + idD28C + " to side", "unpin " + idD28C}; !slices.Equal(h.source.orders, want) {
		t.Errorf("orders = %q, want %q", h.source.orders, want)
	}
	if got, want := h.footer(), "d28c is routed automatically from its next request · d28c selected on Work"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
	if want := []string{"level=INFO", `msg="unpinned a session"`, "session=d28c5e17"}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
	if strings.Contains(log.String(), idD28C) {
		t.Errorf("log reads\n%s\nwant the session's id cut short, as the router logs it", log)
	}
}

func TestAnOrderOfASessionTheRouterRefusesSaysWhy(t *testing.T) {
	log := logstest.Capture(t)
	h := gridHarness(t)
	h.source.refuse = errors.New("personal has no usable token: switchboard accounts token personal")
	h.keys(spaceKey, downKey)

	h.deliver(h.press("2")...)
	if got, want := h.footer(), "personal has no usable token: switchboard accounts token personal · d28c selected on Work"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
	if want := []string{"level=WARN", `msg="the router didn't take an order"`, "session=d28c5e17", "account=personal", "error="}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
}

func TestTheFooterGivesTheSelectionsKeys(t *testing.T) {
	h := gridHarness(t)
	h.keys(spaceKey, downKey)
	if got, want := h.footer(), "↑↓ select · 1-4 move d28c to that account · space flip back · esc done · d28c selected on Work"; got != want {
		t.Errorf("with d28c picked out, footer = %q, want %q", got, want)
	}
	if got, want := listedKeys(h.model.helpKeys()), []string{"↑↓", "1-4", "a", "space", "esc", "tab", "w", "←→", "s", "m", "r", "?", "q"}; !slices.Equal(got, want) {
		t.Errorf("with d28c picked out, the help lists %q, want %q", got, want)
	}

	h.keys(escKey)
	if h.model.selecting() || !h.model.flipped["work"] || h.model.focus != "work" {
		t.Errorf("after esc, picking out %+v, flipped %v, focus on %q; want nothing picked out, work's card flipped still, with the focus", h.model.selected, h.model.flipped, h.model.focus)
	}
	if got, want := h.footer(), "tab views · w window: auto · ←→ focus · space flip · 1-4 pin · a auto · m move · ? keys · q quit · read 0s ago"; got != want {
		t.Errorf("after esc, footer = %q, want %q", got, want)
	}
}

func TestSpaceFlipsTheCardBackEndingTheSelection(t *testing.T) {
	h := gridHarness(t)
	h.keys(spaceKey, downKey, spaceKey)
	if h.model.selecting() || h.model.flipped["work"] {
		t.Errorf("after space, picking out %+v, flipped %v; want work's card back on its front, nothing picked out", h.model.selected, h.model.flipped)
	}
}

func TestWithoutTheRouterTheKeysOfASessionSaySo(t *testing.T) {
	h := gridHarness(t)
	h.keys(spaceKey, downKey)
	h.stopRouter()
	h.tickUntilAsked()

	if got, want := h.footer(), "↑↓ select · space flip back · esc done · d28c selected on Work"; got != want {
		t.Errorf("without the router, footer = %q, want %q", got, want)
	}
	if !strings.Contains(h.view(), "┃  ↑↓ select  space flip") {
		t.Errorf("the screen is\n%s\nwant work's back listing no keys that move a session", h.view())
	}
	for _, key := range []string{"3", "a"} {
		h.deliver(h.press(key)...)
		if len(h.source.orders) > 0 {
			t.Errorf("%s gave orders %q, want none", key, h.source.orders)
		}
		if got, want := h.footer(), "the router isn't answering · d28c selected on Work"; got != want {
			t.Errorf("after %s, footer = %q, want %q", key, got, want)
		}
	}
}

func TestTheSelectionEndsOnceItsSessionLeavesTheCard(t *testing.T) {
	h := gridHarness(t)
	h.keys(spaceKey, downKey)
	h.source.sessions[0] = sessionOn(idD28C, "claude-opus-5-5", "side", time.Second)
	h.tickUntilAsked()

	if h.model.selecting() || h.model.focus != "work" {
		t.Errorf("once d28c went to side, picking out %+v with the focus on %q, want nothing picked out, the focus on work still", h.model.selected, h.model.focus)
	}
}

func TestALookThatCantListTheSessionsKeepsThoseListedLast(t *testing.T) {
	h := gridHarness(t)
	h.keys(spaceKey, downKey)
	listed := h.model.sessions
	h.source.sessionsErr = errors.New("connection reset")
	asked := h.source.listed
	h.tickUntilAsked()

	if h.source.listed != asked+1 || !reflect.DeepEqual(h.model.sessions, listed) {
		t.Errorf("listing failed on a look, the watch asked %d times, holding %+v; want it asked once more, holding those listed last %+v", h.source.listed-asked, h.model.sessions, listed)
	}
	if h.model.selected != seatOf(idD28C, "claude-opus-5-5") || !strings.Contains(h.view(), "┗━ ● ○ ━") {
		t.Errorf("picking out %+v, the screen\n%s\nwant d28c still picked out, work's dots still lit as listed", h.model.selected, h.view())
	}

	h.stopRouter()
	h.deliver(h.press("r")...)
	if h.model.routed() || h.model.sessions != nil || h.model.selecting() {
		t.Errorf("probing, holding %+v, picking out %+v; want no sessions, and nothing picked out", h.model.sessions, h.model.selected)
	}
}

func TestTheFocusIsGivenUpOnceItsAccountGoes(t *testing.T) {
	h := gridHarness(t)
	h.keys(rightKey, downKey)
	h.startRouter(routerDocument(three()...))
	h.tickUntilAsked()

	if h.model.focus != "" {
		t.Errorf("once client went, the focus is on %q, want on none", h.model.focus)
	}
}

func TestMovingTheFocusToACardOutOfViewScrollsToIt(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	h.keys(downKey)
	if h.model.focus != "a" || h.model.scroll != 0 {
		t.Fatalf("the focus is on %q, scrolled %d, want on a, unscrolled", h.model.focus, h.model.scroll)
	}
	for range 5 {
		h.keys(downKey)
	}
	if h.model.focus != "k" || h.model.scroll == 0 {
		t.Fatalf("the focus is on %q, scrolled %d, want on k, scrolled down to it", h.model.focus, h.model.scroll)
	}
	if !strings.Contains(h.view(), "┏━ 11 Account K") {
		t.Errorf("the screen is\n%s\nwant k's card in view, with the focus", h.view())
	}
	for range 5 {
		h.keys(upKey)
	}
	if h.model.focus != "a" || h.model.scroll != 0 || !strings.Contains(h.view(), "┏━ 1 Account A") {
		t.Errorf("the focus is on %q, scrolled %d, the screen\n%s\nwant on a, scrolled back up to it", h.model.focus, h.model.scroll, h.view())
	}
}

func TestTheFirstFocusIsOnTheFirstCardWhollyInView(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	h.press("j")
	cards := strings.Split(h.view(), "\n")[7:]
	top := slices.IndexFunc(cards, func(row string) bool { return strings.Contains(row, "╭─ ") })
	if top <= 0 {
		t.Fatalf("scrolled a row, the cards start\n%s\nwant a card cut short at the top of the view, and another's top edge under it", strings.Join(cards, "\n"))
	}
	h.keys(rightKey)

	cards = strings.Split(h.view(), "\n")[7:]
	first := topEdge.FindStringSubmatch(cards[top])
	if first == nil || strings.ToLower(first[1]) != h.model.focus {
		t.Fatalf("the focus is on %q, the cards\n%s\nwant it on the first card whose top edge is in view, heavy-edged", h.model.focus, strings.Join(cards, "\n"))
	}
	if !slices.ContainsFunc(cards[top:], func(row string) bool { return strings.HasPrefix(strings.TrimSpace(row), "┗━") }) {
		t.Errorf("the cards are\n%s\nwant the card with the focus whole in view, its bottom edge too", strings.Join(cards, "\n"))
	}
}

// topEdge is the top edge of the leftmost card on a row, heavy-edged, its
// account's name ending in the letter it names.
var topEdge = regexp.MustCompile(`^ ┏━ \d+ Account ([A-Z]) `)

func TestWithOneAccountNoSessionIsPickedOut(t *testing.T) {
	h := routedHarness(t, routerDocument(three()[0]))
	h.source.sessions = []status.Session{sessionOn(idD28C, "claude-opus-5-5", "work", time.Second)}
	h.start()
	h.keys(spaceKey, downKey)

	if !h.model.flipped["work"] || h.model.selecting() {
		t.Errorf("flipped %v, picking out %+v, want work's card flipped, and nothing picked out: there's no other account to move a session to", h.model.flipped, h.model.selected)
	}
	if !strings.Contains(h.view(), "space flip") || strings.Contains(h.view(), "↑↓ select") {
		t.Errorf("the screen is\n%s\nwant the back to list space alone", h.view())
	}
}

func TestAnotherViewEndsTheSelection(t *testing.T) {
	h := gridHarness(t)
	h.keys(spaceKey, downKey)
	h.press("tab")

	if h.model.selecting() {
		t.Errorf("showing %q, picking out %+v, want nothing picked out", h.model.view, h.model.selected)
	}
	h.keys(downKey, spaceKey)
	if h.model.focus != "work" || !h.model.flipped["work"] {
		t.Errorf("showing %q, the cards' keys moved the focus to %q, flipped %v, want them doing nothing there", h.model.view, h.model.focus, h.model.flipped)
	}
}

func TestTheCardKeysDoNothingTillTheDocumentIsRead(t *testing.T) {
	h := routedHarness(t, four())
	h.keys(rightKey, spaceKey, downKey)

	if h.model.focus != "" || h.model.flipped != nil {
		t.Errorf("before a read, the focus is on %q, flipped %v, want neither", h.model.focus, h.model.flipped)
	}
}
