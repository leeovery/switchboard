package watch

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// Keys as the terminal sends them.
var (
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

// four are three and client.
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

func TestALookThatCantListTheSessionsKeepsThoseListedLast(t *testing.T) {
	h := gridHarness(t)
	listed := h.model.sessions
	h.source.sessionsErr = errors.New("connection reset")
	asked := h.source.listed
	h.tickUntilAsked()

	if h.source.listed != asked+1 || !reflect.DeepEqual(h.model.sessions, listed) {
		t.Errorf("listing failed on a look, the watch asked %d times, holding %+v; want it asked once more, holding those listed last %+v", h.source.listed-asked, h.model.sessions, listed)
	}
	if row := rowWith(h.view(), "1 Work        ● open"); !strings.Contains(row, "2 sessions") {
		t.Errorf("ROUTING's work reads %q, want its 2 sessions still listed", ansi.Strip(row))
	}

	h.stopRouter()
	h.deliver(h.press("R")...)
	if h.model.routed() || h.model.sessions != nil {
		t.Errorf("probing, holding %+v; want no sessions", h.model.sessions)
	}
}

func TestAnotherRouterThatCantListTheSessionsShowsNone(t *testing.T) {
	h := gridHarness(t)
	h.source.sessionsErr = errors.New("connection reset")
	h.source.health = router.Health{PID: 4242, StartedAt: start.UTC()}
	h.tickUntilAsked()

	if h.model.sessions != nil {
		t.Errorf("another router answering, unable to list its sessions, the watch holds %+v; want none of the last router's", h.model.sessions)
	}
}

func TestTheFocusStartsOnTheFirstAccountInTheConfigsOrder(t *testing.T) {
	h := gridHarness(t)
	if h.model.focus != "work" || h.model.frame(h.clock.now).Focus != "work" {
		t.Errorf("the first document read, the focus is on %q, want on work, the first account", h.model.focus)
	}
}

func TestUpAndDownMoveTheFocusInTheConfigsOrderNoFurtherThanTheEnds(t *testing.T) {
	h := gridHarness(t)
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	for _, step := range []struct {
		key  tea.KeyPressMsg
		want string
	}{
		{key: downKey, want: "personal"},
		{key: downKey, want: "side"},
		{key: downKey, want: "client"},
		{key: downKey, want: "client"},
		{key: upKey, want: "side"},
		{key: upKey, want: "personal"},
		{key: upKey, want: "work"},
		{key: upKey, want: "work"},
	} {
		h.keys(step.key)
		if h.model.focus != step.want {
			t.Errorf("after %s, the focus is on %q, want on %q", step.key, h.model.focus, step.want)
		}
	}
	if prefs.updates > 0 {
		t.Errorf("moving the focus kept the preferences %d times, want the focus never kept", prefs.updates)
	}
}

func TestWhileGRAPHSChartsEveryAccountBesideUSAGEThereIsNoFocusToMove(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		typed  []string
		moves  bool
		listed bool
	}{
		{name: "by account, GRAPHS beside USAGE", width: 160, typed: []string{"b", "g"}},
		{name: "by account under 150 columns, GRAPHS charting the focused account", width: 149, typed: []string{"b", "g"}, moves: true, listed: true},
		{name: "by account, GRAPHS hidden", width: 160, typed: []string{"b"}, moves: true, listed: true},
		{name: "by window, GRAPHS shown", width: 160, typed: []string{"g"}, moves: true, listed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := gridHarness(t)
			h.update(tea.WindowSizeMsg{Width: tt.width, Height: 40})
			h.typed(tt.typed...)
			h.keys(downKey)
			if moved := h.model.focus == "personal"; moved != tt.moves {
				t.Errorf("after ↓, the focus is on %q; want it moved to personal: %v", h.model.focus, tt.moves)
			}
			if listed := strings.Contains(h.keyLine(), "↑↓ focus"); listed != tt.listed {
				t.Errorf("the key line reads %q; want ↑↓ focus listed: %v", h.keyLine(), tt.listed)
			}
		})
	}
}

func TestTheFocusGoesToTheFirstAccountOnceItsAccountGoes(t *testing.T) {
	h := gridHarness(t)
	h.keys(downKey, downKey, downKey)
	h.startRouter(routerDocument(three()[1:]...))
	h.tickUntilAsked()

	if h.model.focus != "personal" {
		t.Errorf("once client and work went, the focus is on %q, want on personal, the first account left", h.model.focus)
	}
}

func TestTheFocusStaysOnItsAccountAsTheDocumentChanges(t *testing.T) {
	h := gridHarness(t)
	h.keys(downKey)
	h.startRouter(routerDocument(three()...))
	h.tickUntilAsked()

	if h.model.focus != "personal" {
		t.Errorf("once client went, the focus is on %q, want still on personal", h.model.focus)
	}
}

func TestAClickOnAnAccountsRowMovesTheFocusThere(t *testing.T) {
	h := gridHarness(t)
	h.clickOn(dashboard.Target{On: dashboard.OnAccount, Account: "side"})
	if h.model.focus != "side" {
		t.Errorf("a click on side's row left the focus on %q, want on side", h.model.focus)
	}
	h.clickOn(dashboard.Target{On: dashboard.OnAccount, Account: "gone"})
	if h.model.focus != "side" {
		t.Errorf("a click on the row of an account the document doesn't have moved the focus to %q, want it left on side", h.model.focus)
	}
}

func TestTheOverviewsKeysDoNothingOnAnotherTab(t *testing.T) {
	h := gridHarness(t)
	h.press("tab")
	h.keys(downKey, upKey)
	for _, key := range []string{"u", "g", "r", "s", "c", "l", "b"} {
		h.press(key)
	}
	if h.model.focus != "work" || h.model.sections != (dashboard.Sections{}) || h.model.by != dashboard.ByWindow {
		t.Errorf("showing %q, the Overview's keys moved the focus to %q, showed %v, grouped by %v; want them doing nothing there",
			h.model.view, h.model.focus, h.model.sections.Names(), h.model.by)
	}
}
