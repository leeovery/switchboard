package dashboard

import "github.com/leeovery/switchboard/internal/dashboard/flex"

// runwayComingWidth is how wide COMING UP's lines are on Runway at their
// natural, from column 0, as the Overview's: its countdowns end at column
// 71. A narrower frame has them take its width.
const runwayComingWidth = 72

// comingOut is the form of COMING UP's last place where its lines don't all
// fit, in the inks of what's out of view of Runway's lanes, as in "▼ 3 more".
var comingOut = outForm{inks: subtleOut}

// runwayComing draws COMING UP, as laid out: its heading, in text.subtle,
// and under it, on the rows it's given, the things Runway's COMING UP lists,
// as comingLines draws the Overview's, the window a request starts named,
// the same with the account column collapsed or not, as wide as
// comingColumn has them. Where they don't all fit, the last place says how
// many more, as comingOut has it. Stepped back, there's none.
func (f Frame) runwayComing(c *canvas, laid runwayLaid) {
	fit := laid.fit
	if !fit.draws(fit.coming) {
		return
	}
	m := laid.m
	teller := comingTeller{moment: m, names: overviewNames(m), window: f.windowNamer(m), namesStarted: true}
	things := teller.happenings(f.Runway.Coming)
	c.line(0, fit.coming, line{{SectionComing.heading(), dimInk}})
	width := f.comingColumn()
	shown, more := comingPlaced(len(things), fit.lines)
	comingLines(c, things[:shown], m.Now, bounds{y: fit.coming + 1, width: width, rows: fit.lines})
	c.line(0, fit.coming+1+shown, outOfView(0, more, width, comingOut))
}

// comingPlaced is how many of the things given show in the places given,
// and how many more there are: all where they fit, else all but the last
// place, which says how many more.
func comingPlaced(things, places int) (shown, more int) {
	if things <= places {
		return things, 0
	}
	shown = max(places-1, 0)
	return shown, things - shown
}

// comingColumn is how wide COMING UP's lines are on the frame: at their
// natural, runwayComingWidth, or the frame's width where that's narrower.
func (f Frame) comingColumn() int {
	at, _ := flex.Lay(f.Width, flex.Part{Natural: runwayComingWidth, Most: runwayComingWidth})
	return at[0].Size
}
