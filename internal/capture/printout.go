package capture

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/score"
)

// printout draws the fixture as usage prints it on a terminal as wide as
// given: from what its router gives, read directly, in its print look, as
// tall as it is, a line for each of its rows, each ending in a newline.
func (f Fixture) printout(width int) string {
	p := dashboard.Printout{Width: width, Look: f.printLook(), Policy: claude.Policy}
	return strings.Join(p.Draw(f.router.snapshot(), f.now), "\n") + "\n"
}

// printLook is the look usage prints the fixture in: its theme's, painting
// no background, its blends worked out against the theme's canvas, as on a
// terminal whose background it is; or, colourless, without colour.
func (f Fixture) printLook() dashboard.Look {
	if f.colourless {
		return dashboard.NoColour()
	}
	return dashboard.Print(f.theme, nil)
}

// snapshot is what usage reads of the router: its document, as Read gives
// it, the sessions it lists, and the history of its accounts' windows, each
// as it was read.
func (s source) snapshot() dashboard.Snapshot {
	history := make(dashboard.History, len(s.trails))
	for _, t := range s.trails {
		readings := make([]score.Reading, len(t.readings))
		for i, p := range t.readings {
			readings[i] = score.Reading{At: p.At, Utilization: p.Utilization}
		}
		history[dashboard.Ref{Account: t.account, Window: t.window}] = dashboard.Trail{Start: t.start, Readings: readings}
	}
	return dashboard.Snapshot{Doc: s.doc.WorkedOut(claude.Policy), Sessions: slices.Clone(s.sessions), History: history}
}
