package capture

import (
	"context"
	"strings"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/status"
)

// printout draws the fixture as usage prints it on a terminal as wide as
// given: its router read as usage reads one, through watch.Once, in its print
// look, as tall as it is, a line for each of its rows, each ending in a
// newline.
func (f Fixture) printout(width int) string {
	// The fixture's router answers every read, so Once never fails.
	snap, _ := watch.Once(context.Background(), f.router, status.Read{Probe: true})
	p := dashboard.Printout{Width: width, Look: f.printLook(), Policy: claude.Policy}
	return strings.Join(p.Draw(snap, f.now), "\n") + "\n"
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
