package watch

import (
	"errors"
	"reflect"
	"slices"
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

// The Overview has no focus, GRAPHS gone: where USAGE shows every account,
// ↓ and ↑ change nothing, and neither the key line nor help names them,
// however the windows are grouped, on a phone too.
func TestUpAndDownDoNothingWhereUSAGEShowsEveryAccount(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		typed         []string
	}{
		{name: "by window", width: 160, height: 40},
		{name: "by account, ROUTING beside USAGE", width: 160, height: 40, typed: []string{"b"}},
		{name: "by account under 150 columns", width: 149, height: 40, typed: []string{"b"}},
		{name: "on a phone", width: 73, height: 61},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := gridHarness(t)
			h.update(tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			h.typed(tt.typed...)
			if s := h.model.scrolling(); s.Most != 0 {
				t.Fatalf("USAGE scrolls %+v, want every account shown", s)
			}
			was, layout := h.view(), h.model.layout
			h.keys(downKey, downKey, upKey)
			if h.view() != was || h.model.scroll != 0 || !reflect.DeepEqual(h.model.layout, layout) {
				t.Errorf("↓ and ↑ changed the screen to\n%s\nscrolled %d; want it as it was\n%s", h.view(), h.model.scroll, was)
			}
			if slices.Contains(h.listed(), "↑↓") || slices.Contains(listedKeys(helpOver(h.model)), "↑↓") {
				t.Errorf("the key line lists %q, help %q; want ↑↓ in neither", h.listed(), listedKeys(helpOver(h.model)))
			}
		})
	}
}

// Help names ↑↓ scroll exactly where USAGE scrolls, at every height from
// where it shows none of its rows to where it shows them all, grouped
// either way, wide and on a phone, and the key line never, leaving it to
// help as it leaves j and k; and ↓ moves USAGE exactly there.
func TestUpAndDownAreNamedAndScrollExactlyWhereUSAGEScrolls(t *testing.T) {
	scroll := dashboard.Key{Key: "↑↓", Does: "scroll"}
	found := map[bool]int{}
	for _, width := range []int{73, 160} {
		for _, typed := range [][]string{nil, {"b"}} {
			for height := 12; height <= 40; height++ {
				h := routedHarness(t, tenRouted())
				h.update(tea.WindowSizeMsg{Width: width, Height: height})
				h.start()
				h.typed(typed...)
				scrolls := h.model.scrolling().Most > 0
				found[scrolls]++
				named := slices.ContainsFunc(h.model.live(h.clock.now).KeyLine.Groups, func(g []dashboard.Key) bool { return slices.Contains(g, scroll) })
				if helped := slices.Contains(helpOver(h.model), scroll); named || helped != scrolls {
					t.Errorf("by %q at %d by %d, USAGE scrolls %t, the key line names ↑↓ scroll %t, help %t; want the key line never, help %t", typed, width, height, scrolls, named, helped, scrolls)
				}
				h.keys(downKey)
				if moved := h.model.scroll == 1; moved != scrolls {
					t.Errorf("by %q at %d by %d, USAGE scrolls %t, and ↓ scrolled it to %d", typed, width, height, scrolls, h.model.scroll)
				}
			}
		}
	}
	if found[true] == 0 || found[false] == 0 {
		t.Errorf("of the heights, %d scroll USAGE and %d don't, want some of each", found[true], found[false])
	}
}

// ↓ and ↑ scroll the Overview alone: on another tab, they do that tab's own
// thing, as on Accounts' Detail, where ↓ chooses the next account, and
// scroll nothing.
func TestUpAndDownScrollOnlyOnTheOverview(t *testing.T) {
	h := shortUSAGE(t)
	h.press("tab")
	h.model.accounts.shown = dashboard.AccountsShown{Accounts: threeConfigured, Shown: dashboard.AccountsDetail}
	if h.model.view != dashboard.Accounts || h.model.scroll != 0 {
		t.Fatalf("on %q, scrolled %d; want Accounts, unscrolled", h.model.view, h.model.scroll)
	}
	h.keys(downKey)
	if chosen := h.model.accounts.shown.Chosen; chosen != "work" || h.model.scroll != 0 {
		t.Errorf("↓ on Accounts' Detail chose %q and scrolled %d; want work chosen, as Detail's ↓ does, nothing scrolled", chosen, h.model.scroll)
	}
}

func TestTheOverviewsKeysDoNothingOnAnotherTab(t *testing.T) {
	h := gridHarness(t)
	h.press("tab")
	for _, key := range []string{"u", "r", "s", "c", "l", "b"} {
		h.press(key)
	}
	if h.model.sections != (dashboard.Sections{}) || h.model.by != dashboard.ByWindow {
		t.Errorf("showing %q, the Overview's keys showed %v, grouped by %v; want them doing nothing there",
			h.model.view, h.model.sections.Names(), h.model.by)
	}
}
