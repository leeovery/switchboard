package watch

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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
	h.model.view = dashboard.Sessions
	listed := h.model.sessions
	h.source.sessionsErr = errors.New("connection reset")
	asked := h.source.listed
	h.tickUntilAsked()

	if h.source.listed != asked+1 || !reflect.DeepEqual(h.model.sessions, listed) {
		t.Errorf("listing failed on a look, the watch asked %d times, holding %+v; want it asked once more, holding those listed last %+v", h.source.listed-asked, h.model.sessions, listed)
	}
	if !strings.Contains(h.view(), "d28c  opus") {
		t.Errorf("the screen is\n%s\nwant d28c still listed", h.view())
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

func TestTheFocusIsGivenUpOnceItsAccountGoes(t *testing.T) {
	h := gridHarness(t)
	h.model.focus = "client"
	h.startRouter(routerDocument(three()...))
	h.tickUntilAsked()

	if h.model.focus != "" {
		t.Errorf("once client went, the focus is on %q, want on none", h.model.focus)
	}
}

func TestTheOverviewsKeysDoNothingOnAnotherTab(t *testing.T) {
	h := gridHarness(t)
	h.press("tab")
	h.keys(downKey, upKey)
	for _, key := range []string{"u", "g", "r", "s", "c", "l", "b"} {
		h.press(key)
	}
	if h.model.focus != "" || h.model.sections != (dashboard.Sections{}) || h.model.by != dashboard.ByWindow {
		t.Errorf("showing %q, the Overview's keys moved the focus to %q, showed %v, grouped by %v; want them doing nothing there",
			h.model.view, h.model.focus, h.model.sections.Names(), h.model.by)
	}
}
