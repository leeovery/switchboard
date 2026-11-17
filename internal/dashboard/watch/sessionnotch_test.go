package watch

import (
	"runtime"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/ledger"
)

// A turn of the wheel over a session's page, the screen drawn after it, as
// Bubble Tea draws it after every message, costs what's on screen, never
// the session's history: over a page of a hundred times the turns, or a
// turn open of a hundred times the requests, it allocates no more than
// historyAllowance more than over a small one, as the page's story is laid
// once a width, not once a frame, and its turns' rows are made only as they
// show. It counts the bytes allocated, not the time taken, so a busy machine
// never fails it.
func TestANotchOnASessionsPageCostsWhatsOnScreenNotItsHistory(t *testing.T) {
	tests := []struct {
		name  string
		lines func(n int) []ledger.Line
		open  bool
	}{
		{name: "over its turns", lines: func(n int) []ledger.Line { return promptedLines(n, start.Add(-time.Duration(n)*time.Minute)) }},
		{name: "over its turn open's requests", lines: oneTurnLines, open: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			small, big := notchCost(t, tt.lines(50), tt.open), notchCost(t, tt.lines(5000), tt.open)
			if big > small+historyAllowance {
				t.Errorf("a notch allocates %d bytes over a page of 50, and %d over one of 5,000; want no more than %d more", small, big, historyAllowance)
			}
		})
	}
}

// historyAllowance is the most bytes a notch may allocate over a page of a
// hundred times the history beyond what it allocates over a small one: the
// wider numbers it says, a few kilobytes, measured 17 Nov 2026. Laying the
// story once a frame added 46 KB at a hundred times the history, and making
// every turn's row before drawing those in view 300 KB.
const historyAllowance = 16 << 10

// oneTurnLines are idD28C's lines of one turn of n requests, a minute apart,
// the last a minute before the clock starts.
func oneTurnLines(n int) []ledger.Line {
	lines := promptedLines(n, start.Add(-time.Duration(n)*time.Minute))
	for i := range lines {
		lines[i].Prompt = "prompt-1"
	}
	return lines
}

// notchCost is the bytes a turn of the wheel allocates, on average, over
// the page of idD28C of the lines given, its turns or, where open says, its
// newest turn opened, its requests, the screen drawn after each, as
// Bubble Tea draws it: ten turns down, then ten up, once the page has been
// drawn once.
func notchCost(t *testing.T, lines []ledger.Line, open bool) uint64 {
	t.Helper()
	h, _ := pageHarness(t, lines)
	h.openPage(idD28C)
	h.settle()
	if open {
		h.keys(enterKey)
		h.settle()
	}
	_ = h.model.View()
	notch := func(button tea.MouseButton) {
		h.timers = nil
		h.deliver(h.update(tea.MouseWheelMsg{X: 10, Y: 34, Button: button})...)
		_ = h.model.View()
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range 10 {
		notch(tea.MouseWheelDown)
	}
	scrolled := h.shownPage()
	for range 10 {
		notch(tea.MouseWheelUp)
	}
	runtime.ReadMemStats(&after)
	if scrolled.Scroll == 0 && scrolled.RequestsScroll == 0 {
		t.Fatalf("ten notches down scrolled nothing, want the page of %d lines scrolled", len(lines))
	}
	return (after.TotalAlloc - before.TotalAlloc) / 20
}
