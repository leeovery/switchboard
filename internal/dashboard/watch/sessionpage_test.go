package watch

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// pageLedger is a request ledger of the lines given, by session, which notes
// the ids each read of its sessions asked for.
type pageLedger struct {
	ledger.Empty
	lines map[string][]ledger.Line
	asked [][]string
}

func (l *pageLedger) Sessions(ids []string, _ func(string, ledger.Line)) map[string][]ledger.Line {
	l.asked = append(l.asked, slices.Clone(ids))
	return l.lines
}

// promptedLines are idD28C's lines of n turns on work, a prompt each, the
// first coming at from and each a minute after the one before, its own
// conversation's request answering it.
func promptedLines(n int, from time.Time) []ledger.Line {
	lines := make([]ledger.Line, n)
	for i := range lines {
		lines[i] = ledger.Line{
			At: from.Add(time.Duration(i) * time.Minute).UTC(), Kind: ledger.KindMessage, Session: idD28C, Model: opus, Account: "work",
			Reason: "new", Status: 200, Attempts: 1, TotalMS: 1000, Prompt: "prompt-" + strconv.Itoa(i+1), Class: ledger.ClassMain,
			Answer: ledger.Answer{Stop: "end_turn"},
		}
	}
	return lines
}

// pageHarness is gridHarness, idD28C's lines as given in its ledger, priced
// at the price table's prices.
func pageHarness(t *testing.T, lines []ledger.Line) (*harness, *pageLedger) {
	t.Helper()
	h := gridHarness(t)
	return h, h.withLines(lines)
}

// withLines has the model read the lines given in its ledger, each its
// session's, priced at the price table's prices.
func (h *harness) withLines(lines []ledger.Line) *pageLedger {
	l := &pageLedger{Empty: h.emptyLedger(), lines: make(map[string][]ledger.Line)}
	for _, line := range lines {
		l.lines[line.Session] = append(l.lines[line.Session], line)
	}
	h.model.cfg.Ledger, h.model.cfg.Prices = l, ledger.Pricing
	return l
}

// openPage opens the page of the session with the given id, as a click on
// its row does, its read landing.
func (h *harness) openPage(id string) {
	h.t.Helper()
	next, cmd := h.model.openSession(id)
	h.model = next
	h.deliver(h.run(cmd)...)
	if h.model.session == nil {
		h.t.Fatalf("%s's page didn't open", id)
	}
}

// shownPage is the session's page as the frame shows it, failing the test
// where none is open.
func (h *harness) shownPage() dashboard.SessionShown {
	h.t.Helper()
	if h.model.session == nil {
		h.t.Fatal("no session's page is open")
	}
	return h.model.session.shown
}

// A click on a session's row in SESSIONS opens its page once its lines are
// read, by a command, never in Update; the tab it opened over stays lit, and
// the page opens on its newest turn, its directory shown under the home the
// watch is given. A read asked for before the last opens nothing as it
// lands.
func TestAClickOnASessionInSESSIONSOpensItsPageOnceItsLinesAreRead(t *testing.T) {
	h, l := pageHarness(t, promptedLines(2, start.Add(-30*time.Minute)))
	h.model.cfg.Home = "/home/someone"
	h.keyed("s c l")
	click := h.sessionClick(idD28C)

	next, stale := h.model.Update(click)
	h.model = next.(Model)
	next, read := h.model.Update(click)
	h.model = next.(Model)
	if len(l.asked) != 0 || h.model.session != nil {
		t.Fatalf("clicked, the ledger was read %d times and a page is open %v; want neither till the read runs", len(l.asked), h.model.session != nil)
	}
	h.deliver(h.run(stale)...)
	if h.model.session != nil {
		t.Fatal("the read asked for before the last landed and opened its page, want it dropped")
	}
	h.deliver(h.run(read)...)
	if !slices.EqualFunc(l.asked, [][]string{{idD28C}, {idD28C}}, slices.Equal) {
		t.Errorf("the reads asked the ledger for %q, want d28c's lines alone each time", l.asked)
	}
	page := h.shownPage()
	if page.Page.Session != idD28C || page.Turn != 2 || page.Home != "/home/someone" || h.model.view != dashboard.Overview {
		t.Errorf("the read landed, the page is %s's, turn %d chosen, under %q, over %s; want d28c's, its newest, turn 2, under the home given, over the Overview",
			page.Page.Session, page.Turn, page.Home, h.model.view)
	}
}

// sessionClick is a click on the row of the session with the given id in
// SESSIONS, failing the test where it gives none.
func (h *harness) sessionClick(id string) tea.MouseClickMsg {
	h.t.Helper()
	i := slices.IndexFunc(h.targets(), func(t dashboard.Target) bool { return t.On == dashboard.OnSession && t.Session == id })
	if i < 0 {
		h.t.Fatalf("SESSIONS gives %s no target among %+v", id, h.targets())
	}
	return tea.MouseClickMsg{X: h.targets()[i].X, Y: h.targets()[i].Y, Button: tea.MouseLeft}
}

// A page asked to open opens nothing once its read lands where the user has
// left the tab it was asked over, or covered it with a card, before then: by
// esc, tab or shift-tab, a click on a tab, the one shown among them, p or ?.
func TestAPageAskedToOpenOpensNothingOnceLeftBeforeItsLinesAreRead(t *testing.T) {
	press := func(key tea.KeyPressMsg) func(*harness) {
		return func(h *harness) { h.deliver(h.update(key)...) }
	}
	click := func(tab dashboard.View) func(*harness) {
		return func(h *harness) {
			target := h.tabTarget(tab)
			h.click(target.X, target.Y)
		}
	}
	tests := []struct {
		name  string
		leave func(*harness)
		open  opened
	}{
		{name: "esc", leave: press(escKey)},
		{name: "tab", leave: press(tea.KeyPressMsg{Code: tea.KeyTab})},
		{name: "shift-tab", leave: press(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})},
		{name: "a click on Accounts' tab", leave: click(dashboard.Accounts)},
		{name: "a click on the Overview's tab, shown", leave: click(dashboard.Overview)},
		{name: "p", leave: press(tea.KeyPressMsg{Code: 'p', Text: "p"}), open: routingOpen},
		{name: "?", leave: press(tea.KeyPressMsg{Code: '?', Text: "?"}), open: helpOpen},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := pageHarness(t, promptedLines(2, start.Add(-30*time.Minute)))
			h.keyed("s c l")
			next, read := h.model.Update(h.sessionClick(idD28C))
			h.model = next.(Model)
			tt.leave(h)
			view := h.model.view
			h.deliver(h.run(read)...)
			if h.model.session != nil || h.model.open != tt.open {
				t.Errorf("left on %s, the read landed: a page open %v, card %d open; want no page, card %d as it was", view, h.model.session != nil, h.model.open, tt.open)
			}
		})
	}
}

// On the Log, esc acts as the Log has it, the newest first: its find, then
// a pause, then a filter; and drops a session's page asked to open all the
// same, as s asks one, so its read opens nothing as it lands. Left alone,
// the read opens the page over the Log, paused as it was.
func TestEscOnTheLogDropsAPageAskedToOpenAsTheLogTakesIt(t *testing.T) {
	tests := []struct {
		name                            string
		filtered, paused, finding, esc  bool
		wantFiltered, wantPaused, opens bool
	}{
		{name: "paused, esc: following again", paused: true, esc: true},
		{name: "paused and filtered, esc: following again, the filter kept", filtered: true, paused: true, esc: true, wantFiltered: true},
		{name: "filtered, esc: the filter let go", filtered: true, esc: true},
		{name: "finding, paused and filtered, esc: the find closed alone", filtered: true, paused: true, finding: true, esc: true, wantFiltered: true, wantPaused: true},
		{name: "following, esc", esc: true},
		{name: "paused, no esc: the page opens", paused: true, wantPaused: true, opens: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "side", -4*time.Minute))
			h := routedHarness(t, routerDocument(three()...))
			onTheLog(h, state)
			h.start()
			if tt.filtered {
				h.model.log.page = h.model.log.page.FoundOnly(idD28C)
			}
			if tt.paused {
				h.keys(upKey)
			}
			next, read := h.model.openSession(idD28C)
			h.model = next
			if tt.finding {
				h.typed("/")
			}
			if tt.esc {
				h.keys(escKey)
			}
			h.deliver(h.run(read)...)
			p := h.model.log.page
			if opened := h.model.session != nil; opened != tt.opens || p.Filtered() != tt.wantFiltered || p.Paused() != tt.wantPaused || p.Finding.Open {
				t.Errorf("the read landed: a page open %t, the Log filtered %t, paused %t, finding %t; want a page open %t, filtered %t, paused %t, not finding",
					opened, p.Filtered(), p.Paused(), p.Finding.Open, tt.opens, tt.wantFiltered, tt.wantPaused)
			}
		})
	}
}

// A session's page leaves as its keys and clicks have it: esc goes back to
// the tab it was opened over, as it was left; tab and shift-tab, and a click
// on a tab, go to that tab from its top.
func TestASessionsPageLeavesForTheTabItWasOpenedOverOrAnother(t *testing.T) {
	press := func(key tea.KeyPressMsg) func(Model) (tea.Model, tea.Cmd) {
		return func(m Model) (tea.Model, tea.Cmd) { return m.pressed(key) }
	}
	click := func(target dashboard.Target) func(Model) (tea.Model, tea.Cmd) {
		return func(m Model) (tea.Model, tea.Cmd) { return m.clickedOn(target) }
	}
	tests := []struct {
		name   string
		leave  func(Model) (tea.Model, tea.Cmd)
		view   dashboard.View
		scroll int
	}{
		{name: "esc", leave: press(escKey), view: dashboard.Overview, scroll: 2},
		{name: "esc on the key line", leave: click(keyTarget("esc")), view: dashboard.Overview, scroll: 2},
		{name: "tab", leave: press(tea.KeyPressMsg{Code: tea.KeyTab}), view: dashboard.Accounts},
		{name: "shift-tab", leave: press(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}), view: dashboard.Log},
		{name: "a click on History's tab", leave: click(dashboard.Target{On: dashboard.OnTab, Tab: dashboard.History}), view: dashboard.History},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := turnsModel(20)
			m.scroll, m.focus = 2, "side"
			next, _ := tt.leave(m)
			got := next.(Model)
			if got.session != nil || got.view != tt.view || got.scroll != tt.scroll || got.focus != "side" {
				t.Errorf("a page open %v, on %s scrolled %d, the focus on %q; want the page closed, on %s scrolled %d, the focus kept",
					got.session != nil, got.view, got.scroll, got.focus, tt.view, tt.scroll)
			}
		})
	}
}

// turnsModel is a model on the Overview, at 160×37, showing idD28C's page
// of n turns on one account, its newest chosen: its turns' rows hold
// eleven.
func turnsModel(n int) Model {
	turns := make([]views.Turn, n)
	for i := range turns {
		turns[i].Turn = n - i
	}
	page := views.SessionPage{Session: idD28C, Accounts: []views.SessionAccount{{Account: "work"}}, Turns: turns}
	return Model{
		cfg: Config{Now: (&fakeClock{now: start}).Now}, size: Size{Width: 160, Height: 37}, view: dashboard.Overview,
		session: &sessionShown{id: idD28C, shown: dashboard.SessionShown{Page: page, Turn: n}},
	}
}

// ↑ and ↓ move the turn chosen, no further than the first and the last,
// scrolling no further than keeps it in view; j and k, PgDn and PgUp, and
// the wheel over the turns scroll them, the turn chosen kept in view;
// whatever tab the page is open over, the Log, whose table scrolls, among
// them.
func TestASessionsKeysAndTheWheelMoveThroughItsTurns(t *testing.T) {
	press := func(key tea.KeyPressMsg, times int) func(Model) Model {
		return func(m Model) Model {
			for range times {
				next, _ := m.pressed(key)
				m = next.(Model)
			}
			return m
		}
	}
	wheel := func(button tea.MouseButton, y int) func(Model) Model {
		return func(m Model) Model { return m.wheeled(tea.MouseWheelMsg{X: 10, Y: y, Button: button}) }
	}
	steps := []struct {
		name         string
		act          func(Model) Model
		turn, scroll int
	}{
		{name: "↓ eleven times", act: press(downKey, 11), turn: 9, scroll: 1},
		{name: "↑ twelve times, past the newest", act: press(upKey, 12), turn: 20, scroll: 0},
		{name: "j three times", act: press(tea.KeyPressMsg{Code: 'j', Text: "j"}, 3), turn: 17, scroll: 3},
		{name: "k", act: press(tea.KeyPressMsg{Code: 'k', Text: "k"}, 1), turn: 17, scroll: 2},
		{name: "PgDn, past the last page", act: press(tea.KeyPressMsg{Code: tea.KeyPgDown}, 1), turn: 11, scroll: 9},
		{name: "the wheel up over the story", act: wheel(tea.MouseWheelUp, 12), turn: 11, scroll: 9},
		{name: "the wheel up over the turns", act: wheel(tea.MouseWheelUp, 25), turn: 11, scroll: 6},
		{name: "PgUp", act: press(tea.KeyPressMsg{Code: tea.KeyPgUp}, 1), turn: 11, scroll: 0},
		{name: "↓ twenty times, past the oldest", act: press(downKey, 20), turn: 1, scroll: 9},
	}
	for _, over := range []dashboard.View{dashboard.Overview, dashboard.Log} {
		t.Run(string(over), func(t *testing.T) {
			m := turnsModel(20)
			m.view = over
			for _, step := range steps {
				m = step.act(m)
				if got := m.session.shown; got.Turn != step.turn || got.Scroll != step.scroll {
					t.Fatalf("after %s, turn %d is chosen, the turns scrolled %d; want turn %d, scrolled %d", step.name, got.Turn, got.Scroll, step.turn, step.scroll)
				}
			}
		})
	}
}

// A click on a turn's row chooses it.
func TestAClickOnATurnChoosesIt(t *testing.T) {
	next, _ := turnsModel(20).clickedOn(dashboard.Target{On: dashboard.OnTurn, Turn: 15})
	if got := next.(Model).session.shown; got.Turn != 15 || got.Scroll != 0 {
		t.Errorf("a click on turn 15's row chose turn %d, the turns scrolled %d; want 15, unscrolled", got.Turn, got.Scroll)
	}
}

// While its page is open, the watch reads the request stream, whatever tab
// it's open over, while the router answers.
func TestASessionsPageReadsTheStreamWhileTheRouterAnswers(t *testing.T) {
	tests := []struct {
		name       string
		page, gone bool
		want       bool
	}{
		{name: "its page open over History", page: true, want: true},
		{name: "History alone"},
		{name: "its page open, the router gone", page: true, gone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{view: dashboard.History, asRead: asRead{routed: true}}
			if tt.page {
				m.session = &sessionShown{}
			}
			if tt.gone {
				m.lost = start
			}
			if got := m.wantsStream(); got != tt.want {
				t.Errorf("wants the stream %t, want %t", got, tt.want)
			}
		})
	}
}

// Open, a session's page is built afresh at each look, of the router's
// listing then, and at each tick, its story running to the clock's time;
// one the router lists no longer stays open, saying it ended, its story
// ending at its last request.
func TestASessionsPageFollowsEachLookAndTickAndStaysOnceItEnds(t *testing.T) {
	lines := promptedLines(3, start.Add(-30*time.Minute))
	h, _ := pageHarness(t, lines)
	h.openPage(idD28C)

	h.fire(h.lastTick())
	if story := h.shownPage().Story; story.Ended || !story.End.Equal(h.clock.now) {
		t.Errorf("a tick on, the story ends at %s, over %t; want it running to the tick, %s", story.End, story.Ended, h.clock.now)
	}

	h.source.sessions[0] = sessionOn(idD28C, opus, "side", time.Second)
	h.tickUntilAsked()
	if goes := h.shownPage().Page.Goes; len(goes) != 1 || goes[0].Account != "side" {
		t.Errorf("the router listing its opus on side, row 7 has it going to %+v, want side", goes)
	}

	h.source.sessions = slices.Delete(h.source.sessions, 0, 1)
	h.tickUntilAsked()
	page := h.shownPage()
	last := lines[len(lines)-1].At
	if page.Page.Runs() || page.Page.Activity.Doing != views.Ended || !page.Story.Ended || !page.Story.End.Equal(last) {
		t.Errorf("listed no longer, the page runs %t, doing %v, its story over %t at %s; want it ended, its story ending at %s",
			page.Page.Runs(), page.Page.Activity.Doing, page.Story.Ended, page.Story.End, last)
	}
	h.fire(h.lastTick())
	if end := h.shownPage().Story.End; !end.Equal(last) {
		t.Errorf("ended, a tick on, the story ends at %s, want still at %s", end, last)
	}
}

// A session's page reads [ledger] keep as the Log's oldest day does: a
// Config that doesn't say keeps a day's lines config.DefaultLedgerKeep, as
// the router keeps them, so an ended session's lines are kept until that
// long after its first day's, never for good.
func TestAnEndedSessionsPageReadsTheKeepAsTheLogDoes(t *testing.T) {
	lines := promptedLines(2, start.Add(-30*time.Minute))
	h, _ := pageHarness(t, lines)
	h.source.sessions = slices.DeleteFunc(h.source.sessions, func(s status.Session) bool { return s.ID == idD28C })
	h.tickUntilAsked()
	h.openPage(idD28C)

	want, _ := dayfile.KeptUntil(views.Today(lines[0].At).Date, config.DefaultLedgerKeep)
	if page := h.shownPage().Page; page.Runs() || page.KeptUntil != want {
		t.Errorf("its keep not said, the page runs %t, kept until %q; want it ended, kept until %q", page.Runs(), page.KeptUntil, want)
	}
}

// While the router doesn't answer, a session's page keeps its own pin as
// the router last listed it: its card, open, still picks it, naming no
// account auto would choose, as the router can't say; and clearing it says
// the router isn't answering, as new sessions' card does, sending nothing.
func TestASessionsCardKeepsItsOwnPinWhileTheRouterDoesntAnswer(t *testing.T) {
	h, _ := pageHarness(t, promptedLines(2, start.Add(-30*time.Minute)))
	h.source.sessions[0].Pin = "side"
	h.source.sessions[0].Assignments[0].Account, h.source.sessions[0].Assignments[0].Auto = "side", "work"
	h.tickUntilAsked()
	h.openPage(idD28C)
	h.keyed("p")
	if card := h.card(); !slices.Equal(card.Picked, []string{"side"}) || card.Session.Auto != "work" {
		t.Fatalf("the card picks %q, auto naming %q; want side, its own pin, and work", card.Picked, card.Session.Auto)
	}

	h.stopRouter()
	h.tickUntilAsked()
	if card := h.card(); h.model.open != routingOpen || !slices.Equal(card.Picked, []string{"side"}) || card.Session.Auto != "" {
		t.Fatalf("the router gone, the card is open %t, picking %q, auto naming %q; want it open, picking side, auto naming none",
			h.model.open == routingOpen, card.Picked, card.Session.Auto)
	}
	h.keyed("0 enter")
	if len(h.source.orders) > 0 || h.model.open != routingOpen || h.card().Refused != status.ErrNoRouter.Error() {
		t.Errorf("0 enter sent %q, the card open %t, saying %q; want nothing sent, the card open, saying %q",
			h.source.orders, h.model.open == routingOpen, h.card().Refused, status.ErrNoRouter)
	}
}

// With one account, a session's page lists p while its own pin stands, as
// the router last listed it: faint while the router doesn't answer.
func TestASessionsPageListsPForItsOwnPinWhileTheRouterDoesntAnswer(t *testing.T) {
	one := account("work", "Work", session(0.25, 3*time.Hour), week(0.5))
	h := routedHarness(t, routerDocument(one))
	h.source.doc = document(one)
	pinned := sessionOn(idD28C, opus, "work", 10*time.Second)
	pinned.Pin = "work"
	h.source.sessions = []status.Session{pinned}
	h.update(tea.WindowSizeMsg{Width: 160, Height: 40})
	h.start()
	h.withLines(promptedLines(2, start.Add(-30*time.Minute)))
	h.openPage(idD28C)

	h.stopRouter()
	h.tickUntilAsked()
	if pin, listed := h.model.pinKey(); !listed || !pin.Off {
		t.Errorf("the router gone, p is listed %t, faint %t; want it listed, faint", listed, pin.Off)
	}
}

// While a page follows its newest turn, a turn new since takes the choice;
// once another is chosen, it keeps it as more come.
func TestASessionsPageFollowsItsNewestTurnTillAnotherIsChosen(t *testing.T) {
	m := turnsModel(3)
	grown := func(m Model) dashboard.SessionShown {
		was := m.session.shown
		is := was
		is.Page.Turns = slices.Insert(slices.Clone(was.Page.Turns), 0, views.Turn{Turn: len(was.Page.Turns) + 1})
		return following(was, is, m.turnsScrolling(is, start))
	}
	if got := grown(m).Turn; got != 4 {
		t.Errorf("its newest chosen, turn 4 came and turn %d is chosen, want 4", got)
	}
	m = m.moveTurn(1)
	if got := grown(m).Turn; got != 2 {
		t.Errorf("turn 2 chosen, turn 4 came and turn %d is chosen, want 2", got)
	}
}

// On a session's page, a look brightens its account's windows as they
// change, and what the stream tells brightens nothing: its counters move
// with the traffic.
func TestOnASessionsPageALookBrightensItsWindowsAndTheStreamNothing(t *testing.T) {
	h, _ := pageHarness(t, promptedLines(2, start.Add(-30*time.Minute)))
	h.openPage(idD28C)
	h.settle()

	doc := four()
	doc.Accounts[0].Windows[0].Utilization = 0.3
	h.source.router = &doc
	h.tickUntilAsked()
	fiveHour := dashboard.Number{Kind: dashboard.NumberUsed, Account: "work", Of: "5h"}
	if got := h.brightened()[fiveHour]; got != 1 {
		t.Errorf("work's 5-hour window read at 30%%, it's %v bright, want 1", got)
	}

	was := slices.Collect(maps.Keys(h.brightened()))
	h.hear(told(router.StreamSent, "r1", "work", 0))
	for n := range h.brightened() {
		if !slices.Contains(was, n) {
			t.Errorf("the stream told of d28c's request going out, and %+v brightened, want nothing", n)
		}
	}
}

// The key line of a session's page leads with esc back, in tab views' place,
// four cells after it; a phone's with enter's word its own. It's the page's
// whatever tab it's open over, the Log, whose phone's line keeps keys of its
// own, among them.
func TestASessionsKeyLineLeadsWithEscBack(t *testing.T) {
	tests := []struct {
		over          dashboard.View
		width, height int
		want          string
	}{
		{over: dashboard.Overview, width: 160, height: 37, want: "esc back    ↑↓ turn  enter its requests    p pin    ? help  q quit"},
		{over: dashboard.Overview, width: 52, height: 32, want: "esc back   ↑↓ turn   enter open   ? keys   q quit"},
		{over: dashboard.Log, width: 160, height: 37, want: "esc back    ↑↓ turn  enter its requests    p pin    ? help  q quit"},
		{over: dashboard.Log, width: 52, height: 32, want: "esc back   ↑↓ turn   enter open   ? keys   q quit"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("over %s at %d", tt.over, tt.width), func(t *testing.T) {
			h := sessionPaged(t, "")
			h.model.view = tt.over
			h.deliver(h.update(tea.WindowSizeMsg{Width: tt.width, Height: tt.height})...)
			rows := strings.Split(h.view(), "\n")
			if got := strings.TrimSpace(rows[len(rows)-1]); got != tt.want {
				t.Errorf("the key line reads\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}
