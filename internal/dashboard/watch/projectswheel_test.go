package watch

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/swipe"
)

// scrollings are each way the Mac scrolls, with the turn of the wheel the
// terminal is told of as the fingers move down, and up, on the trackpad:
// under natural scrolling, the content following the fingers, fingers down
// turn the wheel up.
var scrollings = []struct {
	name      string
	scrolling swipe.Direction
	down, up  tea.MouseButton
}{
	{name: "natural", scrolling: swipe.Natural, down: tea.MouseWheelUp, up: tea.MouseWheelDown},
	{name: "traditional", scrolling: swipe.Traditional, down: tea.MouseWheelDown, up: tea.MouseWheelUp},
}

// wheelHarness is Projects shown in a terminal of the size given, the Mac
// scrolling as given, over
// projectsHarness's ledger and twenty projects more, a request each, so the
// list runs past its rows; and alpha's twenty sessions more, so its SESSIONS
// does too, alpha the top project still.
func wheelHarness(t *testing.T, size tea.WindowSizeMsg, scrolling swipe.Direction) *harness {
	t.Helper()
	h := projectsHarness(t, dashboard.Projects)
	h.model.cfg.Scrolling = scrolling
	l := h.model.cfg.Ledger.(*projectsLedger)
	for i := range 20 {
		l.write(projectAsks(fmt.Sprintf("e%07d-0000-4000-8000-000000000000", i), fmt.Sprintf("/code/p%02d", i), start.Add(-time.Duration(i+1)*time.Minute), 1)...)
		l.write(projectAsks(fmt.Sprintf("f%07d-0000-4000-8000-000000000000", i), "/code/alpha", start.Add(-4*time.Hour).Add(time.Duration(i)*time.Minute), 1)...)
	}
	h.start()
	h.update(size)
	return h
}

// projectsOnScreen are the list's projects shown, as a click would choose
// them, failing the test where the list shows them all.
func (h *harness) projectsOnScreen() []dashboard.Target {
	h.t.Helper()
	var shown []dashboard.Target
	for _, t := range h.targets() {
		if t.On == dashboard.OnProject {
			shown = append(shown, t)
		}
	}
	if len(shown) == 0 || len(shown) >= len(rowsOf(h)) {
		h.t.Fatalf("the list shows %d of its %d projects, want some but not all, on\n%s", len(shown), len(rowsOf(h)), h.view())
	}
	return shown
}

// Fingers moved down over Projects' list choose the project after the one
// chosen, and up the one before, as ↓ and ↑ do there, whichever way the Mac
// scrolls, scrolling the list with the choice, and nothing past its top or
// its bottom; fingers moved over it from SESSIONS bring ↑ and ↓ back to the
// list, as a click on it does.
func TestTheWheelOverProjectsListChoosesAProjectATimeAsTheFingersMove(t *testing.T) {
	for _, sc := range scrollings {
		for _, size := range []tea.WindowSizeMsg{{Width: 73, Height: 61}, {Width: 160, Height: 37}} {
			t.Run(fmt.Sprintf("%s/%dx%d", sc.name, size.Width, size.Height), func(t *testing.T) {
				h := wheelHarness(t, size, sc.scrolling)
				rows := rowsOf(h)
				shown := h.projectsOnScreen()
				x, y := size.Width/2, shown[len(shown)-1].Y
				chosen := func() string { return h.model.projects.shown.Chosen }

				h.wheel(sc.up, x, y)
				if chosen() != rows[0] || h.model.projects.shown.ListScroll != 0 || h.model.projects.chose {
					t.Errorf("fingers up at the list's top chose %s, scrolled %d, held as chosen %v; want %s still, unscrolled, the top row not held", chosen(), h.model.projects.shown.ListScroll, h.model.projects.chose, rows[0])
				}
				h.wheel(sc.down, x, y)
				if chosen() != rows[1] || h.model.projects.shown.InSessions {
					t.Errorf("fingers down over the list chose %s, in sessions %v; want %s, in the list", chosen(), h.model.projects.shown.InSessions, rows[1])
				}
				for range rows {
					h.wheel(sc.down, x, h.projectsOnScreen()[0].Y)
				}
				if last := rows[len(rows)-1]; chosen() != last || h.model.projects.shown.ListScroll == 0 {
					t.Errorf("fingers down to the list's bottom chose %s, scrolled %d; want %s, the list scrolled to it", chosen(), h.model.projects.shown.ListScroll, last)
				}
				if !slices.ContainsFunc(h.projectsOnScreen(), func(t dashboard.Target) bool { return t.Project == chosen() }) {
					t.Errorf("the project chosen, %s, is out of view, want the list scrolled to show it", chosen())
				}
				h.wheel(sc.up, x, h.projectsOnScreen()[0].Y)
				if chosen() != rows[len(rows)-2] {
					t.Errorf("fingers up from the list's bottom chose %s, want %s", chosen(), rows[len(rows)-2])
				}

				for range rows {
					h.wheel(sc.up, x, h.projectsOnScreen()[0].Y)
				}
				h.keys(rightKey)
				h.wheel(sc.down, x, h.projectsOnScreen()[0].Y)
				if s := h.model.projects.shown; s.Chosen != rows[1] || s.InSessions {
					t.Errorf("fingers down over the list from SESSIONS chose %s, in sessions %v; want %s, back in the list", s.Chosen, s.InSessions, rows[1])
				}
			})
		}
	}
}

// Fingers moved over Projects' list from SESSIONS, past either of the list's
// ends, whichever way the Mac scrolls, bring ↑ and ↓ back to the list, the
// project chosen kept, as a click on its row does.
func TestTheWheelAtProjectsListsEndsFromSessionsComesBackToTheList(t *testing.T) {
	for _, sc := range scrollings {
		for _, size := range []tea.WindowSizeMsg{{Width: 73, Height: 61}, {Width: 160, Height: 37}} {
			t.Run(fmt.Sprintf("%s/%dx%d", sc.name, size.Width, size.Height), func(t *testing.T) {
				h := wheelHarness(t, size, sc.scrolling)
				rows := rowsOf(h)
				x := size.Width / 2
				ends := []struct {
					name string
					way  tea.MouseButton
					to   func()
					at   string
				}{
					{name: "top", way: sc.up, to: func() {}, at: rows[0]},
					{name: "bottom", way: sc.down, to: func() {
						for range rows {
							h.keys(downKey)
						}
					}, at: rows[len(rows)-1]},
				}
				for _, end := range ends {
					end.to()
					if got := h.model.projects.shown.Chosen; got != end.at {
						t.Fatalf("at the list's %s, %s chosen, want %s", end.name, got, end.at)
					}
					h.keys(rightKey)
					if !h.model.projects.shown.InSessions {
						t.Fatalf("→ at the list's %s left ↑ and ↓ in the list, want SESSIONS", end.name)
					}
					h.wheel(end.way, x, h.projectsOnScreen()[0].Y)
					if s := h.model.projects.shown; s.Chosen != end.at || s.InSessions {
						t.Errorf("fingers past the list's %s from SESSIONS chose %s, in sessions %v; want %s, back in the list", end.name, s.Chosen, s.InSessions, end.at)
					}
				}
			})
		}
	}
}

// Whichever way the Mac scrolls, a turn of the wheel over SESSIONS scrolls
// it wheelRows rows the way the wheel turns, the project chosen kept: macOS
// already has content follow the fingers. A turn over anything else of
// Projects moves nothing.
func TestTheWheelOverProjectsSessionsScrollsThemAndElsewhereNothing(t *testing.T) {
	for _, sc := range scrollings {
		for _, size := range []tea.WindowSizeMsg{{Width: 73, Height: 61}, {Width: 160, Height: 37}} {
			t.Run(fmt.Sprintf("%s/%dx%d", sc.name, size.Width, size.Height), func(t *testing.T) {
				h := wheelHarness(t, size, sc.scrolling)
				most := h.model.scrolling().Most
				if most == 0 {
					t.Fatalf("SESSIONS scrolls no rows at %dx%d, want some, on\n%s", size.Width, size.Height, h.view())
				}
				chosen := h.model.projects.shown.Chosen
				i := slices.IndexFunc(h.targets(), func(t dashboard.Target) bool { return t.On == dashboard.OnSession })
				if i < 0 {
					t.Fatalf("SESSIONS shows no session on\n%s", h.view())
				}
				x, y := size.Width/2, h.targets()[i].Y

				h.wheel(tea.MouseWheelDown, x, y)
				if want := min(wheelRows, most); h.model.scroll != want || h.model.projects.shown.Chosen != chosen {
					t.Errorf("a turn down over SESSIONS scrolled %d rows, chose %s; want %d, %s still", h.model.scroll, h.model.projects.shown.Chosen, want, chosen)
				}
				h.wheel(tea.MouseWheelUp, x, y)
				if h.model.scroll != 0 {
					t.Errorf("a turn up over SESSIONS left it scrolled %d rows, want none", h.model.scroll)
				}

				_, every := h.drawnAt("all projects")
				_, head := h.drawnAt("SESSIONS")
				for _, y := range []int{0, every, head} {
					before := h.view()
					h.wheel(tea.MouseWheelDown, x, y)
					if s := h.model.projects.shown; s.Chosen != chosen || h.model.scroll != 0 || s.ListScroll != 0 || h.view() != before {
						t.Errorf("a turn down over row %d, reading %q, chose %s, scrolled SESSIONS %d and the list %d; want nothing moved", y, strings.Split(before, "\n")[y], s.Chosen, h.model.scroll, s.ListScroll)
					}
				}
			})
		}
	}
}

// Whichever way the Mac scrolls, the wheel over the Log's table scrolls it
// the way the wheel turns: a turn down, following, moves nothing, and a turn
// up pauses it.
func TestTheWheelOverTheLogScrollsItTheWayTheWheelTurns(t *testing.T) {
	for _, sc := range scrollings {
		t.Run(sc.name, func(t *testing.T) {
			state := t.TempDir()
			writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "side", -4*time.Minute))
			h := routedHarness(t, routerDocument(three()...))
			onTheLog(h, state)
			h.model.cfg.Scrolling = sc.scrolling
			h.start()
			_, y := h.drawnAt("Side")

			h.wheel(tea.MouseWheelDown, 10, y)
			if got := h.logState(); !strings.HasPrefix(got, "● following") {
				t.Errorf("a turn down over the Log's table left its state line reading %q, want it following", got)
			}
			h.wheel(tea.MouseWheelUp, 10, y)
			if got := h.logState(); !strings.HasPrefix(got, "‖ paused at ") {
				t.Errorf("a turn up over the Log's table left its state line reading %q, want it paused", got)
			}
		})
	}
}
