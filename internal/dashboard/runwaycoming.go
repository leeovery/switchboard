package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
)

// runwayComingWidth is how wide COMING UP's lines are on Runway from a
// phone's width, from column 0, as the Overview's: its countdowns end at
// column 71. On a phone they take the frame's width.
const runwayComingWidth = 72

// runwayComing draws COMING UP, as laid out: its heading, in text.subtle,
// and under it a line for each thing Runway's COMING UP lists, as
// comingLines draws the Overview's, the window a request starts named; or
// folded, its heading saying what's soonest, as comingFolded has it. Stepped
// back, there's none.
func (f Frame) runwayComing(c *canvas, laid runwayLaid) {
	fit := laid.fit
	if !fit.draws(fit.coming) {
		return
	}
	m := laid.m
	things := comingTeller{moment: m, names: overviewNames(m), window: f.windowNamer(m), namesStarted: true}.happenings(f.Runway.Coming)
	heading := line{{SectionComing.heading(), dimInk}}
	if fit.folded {
		c.line(0, fit.coming, slices.Concat(heading, comingFolded(things, m.Now)).fit(f.Width))
		return
	}
	c.line(0, fit.coming, heading)
	width := runwayComingWidth
	if f.phone() {
		width = f.Width
	}
	comingLines(c, things, m.Now, bounds{y: fit.coming + 1, width: width, rows: fit.lines})
}

// comingFolded is what COMING UP folded says after its heading of the things
// coming up at now, as the Overview's folded heading says it: headingGap
// cells on, the soonest, as its line says it, its time unpadded; and two
// cells on, where more follow it, an ellipsis in text.subtle. It says nothing
// where nothing's coming up.
func comingFolded(things []happening, now time.Time) line {
	if len(things) == 0 {
		return nil
	}
	said := slices.Concat(line{spaces(headingGap)}, things[0].said(now, 0))
	if len(things) > 1 {
		said = append(said, span{"  " + prose.Ellipsis, dimInk})
	}
	return said
}
