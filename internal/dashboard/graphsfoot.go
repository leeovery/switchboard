package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// footApart are the cells between the items of v0.1's legend, and
// between the windows on the pool's line.
const footApart = 3

// burnLegend is v0.1's legend under Graphs' burn-down cards, width cells
// wide, its items footApart apart, flowed onto as many rows as it
// takes, an item never broken: each mark in its ink, a lasting window's, and
// what it means in text.muted. The room left, ▆▆, as a card fills it; where
// it's heading, ⠂⠄⡀; where it runs out, ✕; and where reserved says an account
// has a reserve, the reserve's ⠠ ⠠.
func burnLegend(width int, reserved bool) []line {
	items := []line{
		{{"▆▆", ink{token: lastingInk.token, fade: roomFade}}, {" room left", mutedInk}},
		{{"⠂⠄⡀", lastingInk}, {" where it's heading", mutedInk}},
		{{outMark, outInk}, {" runs out", mutedInk}},
	}
	if reserved {
		items = append(items, line{{"⠠ ⠠", capDotInk}, {" reserve", mutedInk}})
	}
	return wrapped(spaces(footApart), width, items)
}

// poolLine is the pool's line of the moment m, width cells wide: Σ in
// text.subtle and pool in text.secondary; then, footApart on, the room
// of the window of each of Runway's views, the week's first, as its pool
// lane words it, the view's name in text.subtle before it, footApart
// apart; and where a pin holds the pool, " · pinned" after the first, as
// usage has it. As many of those parts as fit, whole, from the first, so the
// 5-hour's goes first, then the pin.
func (f Frame) poolLine(m status.Moment, width int) line {
	parts := []line{{{"Σ ", dimInk}, {"pool", noteInk}}}
	pinned := m.Pool().Pinned
	for _, w := range runwayWindows {
		key := w.KeyIn(m.Policy())
		r, ok := runwayRoom(m, key)
		if !ok {
			continue
		}
		said := poolWords(r.room, r.accounts, false).full
		said[0] = said[0].numbered(Number{Kind: NumberRoom, Of: key})
		parts = append(parts, slices.Concat(line{spaces(footApart), {runwayNames[w] + " ", dimInk}}, said))
		if pinned {
			parts, pinned = append(parts, line{pinnedSaid}), false
		}
	}
	return wholeParts(parts, width)
}

// lastingInk is the ink of a window whose room lasts to its reset, the
// ramp's first stop, as Runway's lanes colour one.
var lastingInk = ink{token: theme.VizRamp1}
