package capture

import (
	"context"
	"strings"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

// printout draws the fixture as usage prints it on a terminal as wide as
// given, but for the blank line usage prints first: its router's status
// document, read as usage reads it, in its print look, grouped by and of the
// windows the fixture's flags give, as tall as it is, a line for each of its
// rows, each ending in a newline.
func (f Fixture) printout(width int) string {
	// The fixture's router answers every read, so Read never fails.
	doc, _, _ := f.router.Read(context.Background(), status.Read{Probe: true})
	p := dashboard.Printout{Width: width, Look: f.printLook(), By: f.by, Windows: f.windows, Policy: claude.Policy, Name: claude.WindowName}
	return strings.Join(p.Draw(doc.At(f.now, p.Policy)), "\n") + "\n"
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
