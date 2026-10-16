package watch

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/views"
)

// enterKey is enter, as the keyboard sends it.
var enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}

// turnsOfRequests are idD28C's lines of a turn each count given, on work,
// the first turn's first request coming at from and each request ten
// seconds after the one before: its own conversation's, each calling a tool
// but the last, which ends the turn.
func turnsOfRequests(from time.Time, counts ...int) []ledger.Line {
	var lines []ledger.Line
	for turn, n := range counts {
		for i := range n {
			stop := "tool_use"
			if i == n-1 {
				stop = "end_turn"
			}
			lines = append(lines, ledger.Line{
				At: from.Add(time.Duration(len(lines)) * 10 * time.Second).UTC(), Kind: ledger.KindMessage, Session: idD28C, Model: opus,
				Account: "work", Reason: "new", Status: 200, Attempts: 1, TotalMS: 1000, Prompt: "prompt-" + string(rune('1'+turn)),
				Class: ledger.ClassMain, Answer: ledger.Answer{Stop: stop},
			})
		}
	}
	return lines
}

// openTurnOne is a harness of idD28C's page, its first turn of eight
// requests, its second of one, at 160×40, its first turn chosen and opened.
func openTurnOne(t *testing.T) (*harness, []ledger.Line) {
	t.Helper()
	lines := turnsOfRequests(start.Add(-30*time.Minute), 8, 1)
	h, _ := pageHarness(t, lines)
	h.openPage(idD28C)
	h.keys(downKey, enterKey)
	return h, lines
}

// enter opens the turn chosen into its requests, in place, its first
// chosen; ↑ and ↓ move through them, no further than the first and the
// last, and j, k, PgDn and PgUp scroll them, five shown at a time, the
// request chosen kept in view; esc closes the turn, then goes back. The key
// line says so, open and closed.
func TestATurnOpensIntoItsRequestsAndItsKeysMoveThroughThem(t *testing.T) {
	h, _ := openTurnOne(t)
	keyLine := func(want ...[]string) {
		t.Helper()
		if got := h.grouped(); !slices.EqualFunc(got, want, slices.Equal) {
			t.Errorf("the key line is %q, want %q", got, want)
		}
	}
	keyLine([]string{"esc close"}, []string{"↑↓ request", "enter open"}, []string{"p pin"}, []string{"? help", "q quit"})
	steps := []struct {
		name                  string
		keys                  []tea.KeyPressMsg
		open, request, scroll int
		foot                  string
	}{
		{name: "opened", open: 1, foot: "▼ 3 more of its requests"},
		{name: "↓ six times", keys: slices.Repeat([]tea.KeyPressMsg{downKey}, 6), open: 1, request: 6, scroll: 2,
			foot: "▲ 2 more of its requests   ·   ▼ 1 more of its requests"},
		{name: "↓ twice, past the last", keys: []tea.KeyPressMsg{downKey, downKey}, open: 1, request: 7, scroll: 3},
		{name: "k three times", keys: slices.Repeat([]tea.KeyPressMsg{{Code: 'k', Text: "k"}}, 3), open: 1, request: 4},
		{name: "PgDn", keys: []tea.KeyPressMsg{{Code: tea.KeyPgDown}}, open: 1, request: 4, scroll: 3},
		{name: "↑ five times, past the first", keys: slices.Repeat([]tea.KeyPressMsg{upKey}, 5), open: 1},
		{name: "esc, closing it", keys: []tea.KeyPressMsg{escKey}},
	}
	for _, step := range steps {
		h.keys(step.keys...)
		got := h.shownPage()
		if got.Open != step.open || got.Request != step.request || got.RequestsScroll != step.scroll || got.Turn != 1 {
			t.Fatalf("after %s, turn %d open, request %d chosen, scrolled %d, turn %d chosen; want turn %d open, request %d, scrolled %d, turn 1 chosen",
				step.name, got.Open, got.Request, got.RequestsScroll, got.Turn, step.open, step.request, step.scroll)
		}
		if step.foot != "" && !strings.Contains(h.view(), step.foot) {
			t.Errorf("after %s, the turn open's foot doesn't say %q:\n%s", step.name, step.foot, h.view())
		}
	}
	if requests := len(h.shownPage().Requests); requests != 0 {
		t.Errorf("closed, the page holds %d of its requests' rows, want none", requests)
	}
	keyLine([]string{"esc back"}, []string{"↑↓ turn", "enter its requests"}, []string{"p pin"}, []string{"? help", "q quit"})
	h.keys(escKey)
	if h.model.session != nil {
		t.Error("esc again left the page open, want it gone back where it was opened from")
	}
}

// openingTurn opens the turn chosen, as enter does, and returns its read,
// held back undelivered as one still under way, and the timer armed for it
// to say it's loading.
func (h *harness) openingTurn() ([]tea.Msg, *timer) {
	h.t.Helper()
	read := holdingBack[turnReadMsg](h, h.update(enterKey)...)
	tm, ok := h.pending(turnLoadingMsg{opened: h.model.session.opened()})
	if len(read) != 1 || !ok {
		h.t.Fatalf("a turn opened, its read under way %d times, its loading armed %t; want it read once, its loading armed", len(read), ok)
	}
	return read, tm
}

// A turn opened reads its requests' lines from the ledger, no row of them
// shown till they're read: its foot saying nothing of it short of
// LoadingAfter, then saying it's loading till they're read; then shows
// them. A read for a turn closed since, or opened again since, opens
// nothing, the turn open the later's; and closed, the page holds none of
// its lines.
func TestATurnOpenedShowsItsLoadingTillItsLinesAreRead(t *testing.T) {
	lines := turnsOfRequests(start.Add(-30*time.Minute), 8, 1)
	h, _ := pageHarness(t, lines)
	h.openPage(idD28C)
	h.keys(downKey)
	read, loading := h.openingTurn()
	if shown := h.shownPage(); shown.RequestsLoading || len(shown.Requests) != 0 || footSays(h.view(), "loading…") {
		t.Fatalf("turn 1 opened, its read under way short of %s, loading %t, showing %d rows; want no rows, saying nothing of loading", LoadingAfter, shown.RequestsLoading, len(shown.Requests))
	}
	h.fire(loading)
	if shown := h.shownPage(); !shown.RequestsLoading || len(shown.Requests) != 0 || !footSays(h.view(), "loading…") {
		t.Fatalf("turn 1 opened, its read under way %s on, loading %t, showing %d rows, its foot saying loading %t; want it loading, no rows shown, saying so",
			LoadingAfter, shown.RequestsLoading, len(shown.Requests), footSays(h.view(), "loading…"))
	}
	h.deliver(read...)
	if shown := h.shownPage(); shown.RequestsLoading || len(shown.Requests) != 8 {
		t.Errorf("its read landed, loading %t, showing %d rows; want its 8 requests shown", shown.RequestsLoading, len(shown.Requests))
	}

	h.keys(escKey)
	stale, staleLoading := h.openingTurn()
	h.keys(escKey)
	again, againLoading := h.openingTurn()
	h.fire(staleLoading)
	if shown := h.shownPage(); shown.RequestsLoading {
		t.Error("the loading of a turn opened before it was closed and opened again came due, the turn says it's loading; want nothing said till its own comes due")
	}
	h.fire(againLoading)
	h.deliver(stale...)
	if shown := h.shownPage(); !shown.RequestsLoading || len(shown.Requests) != 0 {
		t.Errorf("the read of a turn opened before it was closed and opened again landed, loading %t, showing %d rows; want it loading still, no rows",
			shown.RequestsLoading, len(shown.Requests))
	}
	h.deliver(again...)
	if shown := h.shownPage(); shown.RequestsLoading || len(shown.Requests) != 8 {
		t.Errorf("the read of the turn opened again landed, loading %t, showing %d rows; want its 8 requests shown", shown.RequestsLoading, len(shown.Requests))
	}
	h.keys(escKey)
	if turn := reflect.ValueOf(h.model.session.held).FieldByName("turn"); !turn.IsNil() {
		t.Error("its turn closed, the page holds its lines, want none")
	}
}

// A turn opened while a read of the ledger's tail is under way, the read
// growing a copy of the page from before it opened, stays open as the read
// lands, its requests shown.
func TestATurnOpenedWhileTheTailIsReadStaysOpenAsTheReadLands(t *testing.T) {
	h, _ := pageHarness(t, turnsOfRequests(start.Add(-30*time.Minute), 8, 1))
	h.openPage(idD28C)
	h.settle()
	h.model.tail.due = true
	tailed := holdingBack[tailedMsg](h, h.update(tea.WindowSizeMsg{Width: h.model.size.Width, Height: h.model.size.Height})...)
	if len(tailed) != 1 {
		t.Fatalf("a look read the tail %d times, want once", len(tailed))
	}
	h.keys(downKey, enterKey)
	h.deliver(tailed...)
	if turn := reflect.ValueOf(h.model.session.held).FieldByName("turn"); turn.IsNil() {
		t.Fatal("the tail's read landed, turn 1 opened as it ran, the page holds none of its lines; want them held")
	}
	if shown := h.shownPage(); shown.RequestsLoading || len(shown.Requests) != 8 {
		t.Errorf("the tail's read landed, turn 1 opened as it ran, loading %t, showing %d rows; want its 8 requests shown", shown.RequestsLoading, len(shown.Requests))
	}
}

// footSays reports whether the foot of the turn open the view draws says
// what's given.
func footSays(view, said string) bool {
	return slices.ContainsFunc(strings.Split(view, "\n"), func(row string) bool {
		return strings.HasPrefix(strings.TrimSpace(row), "└") && strings.Contains(row, said)
	})
}

// A turn open's request in flight counts on as it's drawn, as the Log's row
// of it does: a frame, which builds the page afresh not at all, draws its
// took to the moment, and its characters as the stream last told them.
func TestATurnOpensRequestInFlightCountsOnAsItsDrawn(t *testing.T) {
	h, _ := pageHarness(t, promptedLines(2, start.Add(-30*time.Minute)))
	h.openPage(idD28C)
	h.settle()
	h.keys(enterKey)
	serving := func(e router.StreamEvent) router.StreamEvent {
		e.Prompt, e.Class = "prompt-2", ledger.ClassMain
		return e
	}
	sent := h.clock.now.Sub(start)
	h.hear(serving(told(router.StreamSent, "r3", "work", sent)), serving(told(router.StreamFirst, "r3", "work", sent+time.Second)))
	h.clock.now = past(sent + 1250*time.Millisecond)
	h.hear(alter(serving(told(router.StreamProgress, "r3", "work", sent+1250*time.Millisecond)), func(e *router.StreamEvent) { e.Chars = 500 }))

	for _, took := range []time.Duration{2 * time.Second, 3500 * time.Millisecond, 75 * time.Second} {
		h.clock.now = past(sent + took)
		h.deliver(frameMsg{gen: h.model.frames})
		rows := strings.Split(h.view(), "\n")
		i := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(row, "  │") && strings.Contains(row, "answering") })
		if i < 0 {
			t.Fatalf("%s after it was sent, the turn open has no row of r3 answering:\n%s", took, strings.Join(rows, "\n"))
		}
		for _, want := range []string{prose.Seconds(took), "answering · " + prose.Characters(500) + " characters"} {
			if !strings.Contains(rows[i], want) {
				t.Errorf("a frame %s after it was sent, the turn open's row of r3 doesn't read %q:\n%s", took, want, rows[i])
			}
		}
	}
}

// The Log's card of a turn open's request in flight counts its characters
// as the stream tells them as it's drawn, as the Log's card does, whatever
// the page last built the request's row of; the page behind it standing
// still as the card opened.
func TestATurnOpensCardInFlightCountsItsCharactersAsTheStreamTellsThem(t *testing.T) {
	h, _ := pageHarness(t, promptedLines(2, start.Add(-30*time.Minute)))
	h.openPage(idD28C)
	h.settle()
	h.keys(enterKey)
	serving := func(e router.StreamEvent) router.StreamEvent {
		e.Prompt, e.Class = "prompt-2", ledger.ClassMain
		return e
	}
	sent := h.clock.now.Sub(start)
	streamed := func(chars int) {
		h.hear(alter(serving(told(router.StreamProgress, "r3", "work", h.clock.now.Sub(start))), func(e *router.StreamEvent) { e.Chars = chars }))
	}
	h.hear(serving(told(router.StreamSent, "r3", "work", sent)), serving(told(router.StreamFirst, "r3", "work", sent+time.Second)))
	h.clock.now = past(sent + 1250*time.Millisecond)
	streamed(500)
	h.keys(downKey, enterKey)
	if card := h.model.turnCard(); h.model.open != turnCardOpen || card.Request == nil || card.Request.Row.Request != "r3" {
		t.Fatalf("↓ and enter opened card %d of %+v, want the Log's card of r3", h.model.open, card.Request)
	}

	answering := func(chars int) string { return "answering · " + prose.Characters(chars) + " characters" }
	for i, chars := range []int{1500, 4200, 9800} {
		h.clock.now = past(sent + time.Second + time.Duration(i+2)*250*time.Millisecond)
		streamed(chars)
		built := *h.model.session
		built.shown.Requests = slices.Clone(built.shown.Requests)
		built.shown.Requests[built.shown.Request].Row.Chars = 500
		h.model.session = &built
		rows := strings.Split(h.view(), "\n")
		card := slices.IndexFunc(rows, func(row string) bool { return strings.Contains(row, "├ stopped") })
		row := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(row, "  │") && strings.Contains(row, "answering") })
		if card < 0 || !strings.Contains(rows[card], answering(chars)) {
			t.Errorf("%d characters streamed, the card doesn't read %q:\n%s", chars, answering(chars), strings.Join(rows, "\n"))
		}
		if row < 0 || !strings.Contains(rows[row], answering(500)) {
			t.Errorf("%d characters streamed, the row behind the card doesn't read %q, as it stood as the card opened:\n%s", chars, answering(500), strings.Join(rows, "\n"))
		}
	}
}

// enter on a request of the turn open opens the Log's details card of it
// over the page, the turn open behind, with no s, as its session's page is
// the one it's over; ↑ and ↓ step the card through the turn's requests, the
// request chosen following; and esc closes it, the turn left open at the
// request chosen.
func TestEnterOnARequestOfATurnOpenOpensTheLogsCardOfIt(t *testing.T) {
	h, lines := openTurnOne(t)
	h.keys(downKey, enterKey)
	if h.model.open != turnCardOpen || !h.model.turnCard().Request.Row.At.Equal(lines[1].At) {
		t.Fatalf("enter on the second request opened card %d, of %+v; want the Log's details of the request at %s", h.model.open, h.model.turnCard().Request, lines[1].At)
	}
	if view := h.view(); !strings.Contains(view, "↑↓ previous/next request   esc close") {
		t.Errorf("the card's keys aren't ↑↓ and esc alone:\n%s", view)
	}
	opening := h.model.opening
	h.typed("s")
	if h.model.open != turnCardOpen || h.model.opening != opening || h.model.pending != "" || h.shownPage().Request != 1 {
		t.Errorf("s on the card left card %d open, %d pages asked, %q pending, request %d chosen; want nothing done",
			h.model.open, h.model.opening-opening, h.model.pending, h.shownPage().Request)
	}

	h.keys(downKey)
	if got := h.shownPage().Request; got != 2 || h.model.open != turnCardOpen || !h.model.turnCard().Request.Row.At.Equal(lines[2].At) {
		t.Errorf("↓ in the card chose request %d, the card of %s; want the third, its card", got, h.model.turnCard().Request.Row.At)
	}
	h.keys(escKey)
	if got := h.shownPage(); h.model.open != nothingOpen || got.Open != 1 || got.Request != 2 {
		t.Errorf("esc left card %d open, turn %d open at request %d; want no card, turn 1 open at the third", h.model.open, got.Open, got.Request)
	}
}

// A click on a request of the turn open chooses it, and on the one chosen
// opens its details, as enter does; a click on another turn's row closes
// the turn open and chooses that one.
func TestAClickOnATurnOpensRequestsChoosesOrOpensIt(t *testing.T) {
	h, _ := openTurnOne(t)
	target := func(on dashboard.Clicked, turn, row int) dashboard.Target {
		h.t.Helper()
		i := slices.IndexFunc(h.targets(), func(t dashboard.Target) bool { return t.On == on && t.Turn == turn && t.Row == row })
		if i < 0 {
			h.t.Fatalf("the page gives no target of %v, turn %d, row %d", on, turn, row)
		}
		return h.targets()[i]
	}
	third := target(dashboard.OnRequest, 0, 2)
	h.click(third.X, third.Y)
	if got := h.shownPage().Request; got != 2 || h.model.open != nothingOpen {
		t.Fatalf("a click on the third request chose request %d, card %d open; want the third, no card", got, h.model.open)
	}
	h.click(third.X, third.Y)
	if h.model.open != turnCardOpen {
		t.Fatalf("a click on the request chosen opened card %d, want its details", h.model.open)
	}
	h.keys(escKey)
	second := target(dashboard.OnTurn, 2, 0)
	h.click(second.X, second.Y)
	if got := h.shownPage(); got.Open != 0 || got.Turn != 2 {
		t.Errorf("a click on turn 2's row left turn %d open, turn %d chosen; want none open, turn 2 chosen", got.Open, got.Turn)
	}
}

// A turn opened keeps its own row and its first request in view as the
// turns scroll (call 57): at the least height, its row, its heads and that
// request alone. A turn chosen, none open, keeps its own row alone.
func TestATurnOpenedKeepsItsRowAndItsFirstRequestInView(t *testing.T) {
	tests := []struct {
		name   string
		height int
		open   bool
		scroll int
	}{
		{name: "open, in eleven rows", height: 37, open: true, scroll: 2},
		{name: "chosen, in eleven rows", height: 37},
		{name: "open, in three rows", height: 29, open: true, scroll: 10},
		{name: "chosen, in three rows", height: 29, scroll: 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := turnsModel(20)
			m.size.Height = tt.height
			shown := m.session.shown
			if tt.open {
				shown.Turn, shown.Open, shown.Requests = 10, 10, make([]views.LogLine, 8)
			}
			if got := choosing(shown, 10, m.turnsScrolling(shown)).Scroll; got != tt.scroll {
				t.Errorf("the turn at 10 chosen, the turns scroll %d, want %d", got, tt.scroll)
			}
		})
	}
}

// With a turn open, ↑ and ↓ keep the request chosen in view, the turns
// scrolling as far as they must: of twenty turns in eleven rows, the tenth at
// their foot, opened, its second request chosen comes up into view, and
// enter places its card by its row, over it, a row between.
func TestATurnOpenKeepsItsRequestChosenInViewAsTheTurnsScroll(t *testing.T) {
	counts := slices.Repeat([]int{1}, 20)
	counts[9] = 6
	h, _ := pageHarness(t, turnsOfRequests(start.Add(-30*time.Minute), counts...))
	h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: 37})...)
	h.openPage(idD28C)
	h.keys(slices.Repeat([]tea.KeyPressMsg{downKey}, 10)...)
	if got := h.shownPage(); got.Turn != 10 || got.Scroll != 0 {
		t.Fatalf("↓ ten times chose turn %d, the turns scrolled %d; want turn 10 at their foot, unscrolled", got.Turn, got.Scroll)
	}

	h.keys(enterKey, downKey)
	got := h.shownPage()
	if got.Open != 10 || got.Request != 1 || got.Scroll != 3 {
		t.Errorf("enter, ↓ left turn %d open, request %d chosen, the turns scrolled %d; want turn 10 open, its second request, scrolled 3 to show it",
			got.Open, got.Request, got.Scroll)
	}
	i := slices.IndexFunc(h.targets(), func(t dashboard.Target) bool { return t.On == dashboard.OnRequest && t.Row == 1 })
	if i < 0 {
		t.Fatalf("the request chosen has no row in view:\n%s", h.view())
	}
	lit := h.targets()[i].Y

	h.keys(enterKey)
	_, top := h.drawnAt("╭─ REQUEST")
	rows := strings.Split(h.view(), "\n")
	bottom := slices.IndexFunc(rows[top:], func(row string) bool { return strings.Contains(row, "╰") }) + top
	if bottom != lit-2 {
		t.Errorf("enter placed the card on rows %d to %d, want it over the request's row %d, a row between:\n%s", top+1, bottom+1, lit+1, h.view())
	}
}

// With a turn open whose requests have scrolled, ↓ scrolls the turns no
// further than keeps the request chosen in view: they keep the rows down to
// it from the first request shown, not from the turn's first. Of twenty
// turns in eleven rows, the tenth of eight requests at their foot, opened,
// its seventh request chosen has its requests scrolled 2 and the turns 6.
func TestATurnOpenScrollsItsTurnsNoFurtherThanItsRequestChosen(t *testing.T) {
	counts := slices.Repeat([]int{1}, 20)
	counts[9] = 8
	h, _ := pageHarness(t, turnsOfRequests(start.Add(-30*time.Minute), counts...))
	h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: 37})...)
	h.openPage(idD28C)
	h.keys(slices.Repeat([]tea.KeyPressMsg{downKey}, 10)...)
	h.keys(enterKey)
	h.keys(slices.Repeat([]tea.KeyPressMsg{downKey}, 6)...)
	got := h.shownPage()
	if got.Open != 10 || got.Request != 6 || got.RequestsScroll != 2 || got.Scroll != 6 {
		t.Errorf("enter, ↓ six times left turn %d open, request %d chosen, its requests scrolled %d, the turns %d; want turn 10, request 6, scrolled 2, the turns 6",
			got.Open, got.Request, got.RequestsScroll, got.Scroll)
	}
}

// How far a page's turns scroll, which each key, look and stream event on
// the page asks, is laid out of the page alone at the size drawn at, as
// TurnsScrolling has it: nothing more of the frame is built for it.
func TestHowFarAPagesTurnsScrollIsLaidOutOfThePageAlone(t *testing.T) {
	h, _ := pageHarness(t, turnsOfRequests(start.Add(-30*time.Minute), slices.Repeat([]int{1}, 20)...))
	h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: 37})...)
	h.openPage(idD28C)
	shown := h.shownPage()
	bare := dashboard.Frame{Width: 160, Height: 37, Session: &shown}
	f, doc, at := h.model.screen(h.clock.now)
	if got, want := h.model.turnsScrolling(shown), f.Scrolling(doc, at); got != want || want.Most == 0 {
		t.Fatalf("the page alone lays its turns out scrolling %+v, want %+v, as the frame drawn does, scrolling", got, want)
	}
	least := testing.AllocsPerRun(10, func() { bare.TurnsScrolling() })
	if got := testing.AllocsPerRun(10, func() { h.model.turnsScrolling(shown) }); got > least {
		t.Errorf("how far the turns scroll allocates %.0f times, want no more than the page alone laid out does, %.0f", got, least)
	}
}

// The read of a turn opened on a page closed before it landed opens nothing
// on the page opened again since, its same turn open: that waits on its own.
func TestATurnsReadForAPageClosedSinceOpensNothingOnItOpenedAgain(t *testing.T) {
	h, _ := pageHarness(t, turnsOfRequests(start.Add(-30*time.Minute), 8, 1))
	h.openPage(idD28C)
	h.keys(downKey)
	stale, _ := h.openingTurn()
	h.keys(escKey, escKey)
	h.openPage(idD28C)
	h.keys(downKey)
	again, _ := h.openingTurn()
	h.deliver(stale...)
	if read := reflect.ValueOf(h.model.session.held).FieldByName("turn").Elem().FieldByName("read"); read.Bool() {
		t.Error("the read of a turn opened on the page closed since landed, the page opened again holds it as its turn's; want it waiting on its own")
	}
	h.deliver(again...)
	if shown := h.shownPage(); shown.RequestsLoading || len(shown.Requests) != 8 {
		t.Errorf("the read of the turn opened again landed, loading %t, showing %d rows; want its 8 requests shown", shown.RequestsLoading, len(shown.Requests))
	}
}
