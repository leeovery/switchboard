package dashboard

import "github.com/leeovery/switchboard/internal/dashboard/flex"

// runwayComingWidth is how wide a column of COMING UP's lines is on Runway at
// its natural, as the Overview's lines are: its countdowns end at its 72nd
// cell. A narrower frame has one column take its width.
const runwayComingWidth = 72

// comingApart is the cells between COMING UP's two columns.
const comingApart = 4

// comingOut is the form of COMING UP's last place where its lines don't all
// fit, in the inks of what's out of view of Runway's lanes, as in "▼ 3 more".
var comingOut = outForm{inks: subtleOut}

// runwayComing draws COMING UP, as laid out: its heading, in text.subtle,
// and under it, on the rows it's given, the things Runway's COMING UP lists,
// as comingLines draws the Overview's, the window a request starts named,
// the same with the account column collapsed or not, in the columns
// comingColumns lays, in time order down the first, then across into the
// next. Where they don't all fit, the last place says how many more, as
// comingOut has it. Stepped back, there's none.
func (f Frame) runwayComing(c *canvas, laid runwayLaid) {
	fit := laid.fit
	if !fit.draws(fit.coming) {
		return
	}
	c.line(0, fit.coming, line{{SectionComing.heading(), dimInk}})
	if fit.lines == 0 {
		return
	}
	m := laid.m
	teller := comingTeller{moment: m, names: overviewNames(m), window: f.windowNamer(m), namesStarted: true}
	things := teller.happenings(f.Runway.Coming)
	columns, top := f.comingColumns(), fit.coming+1
	places := fit.lines * len(columns)
	shown, more := comingPlaced(len(things), places)
	for i, column := range columns {
		from := min(i*fit.lines, shown)
		comingLines(c, things[from:min(from+fit.lines, shown)], m.Now, bounds{x: column.At, y: top, width: column.Size, rows: fit.lines})
	}
	last := columns[(places-1)/fit.lines]
	c.line(last.At, top+(places-1)%fit.lines, outOfView(0, more, last.Size, comingOut))
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

// comingColumns are the columns COMING UP's lines take across the frame, as
// flex lays them: two, comingApart apart, sharing the width, the odd cell
// the second's, so it ends at the frame's edge, where both fit at their
// natural; else one, at its natural, or the frame's width where that's
// narrower.
func (f Frame) comingColumns() []flex.Span {
	column := flex.Part{Least: runwayComingWidth, Natural: runwayComingWidth, Grows: true}
	second := column
	second.Gap, second.Odd = comingApart, true
	if two, fits := flex.Lay(f.Width, column, second); fits {
		return two
	}
	one, _ := flex.Lay(f.Width, flex.Part{Natural: runwayComingWidth, Most: runwayComingWidth})
	return one
}

// comingRows are the rows COMING UP's lines take in the columns given, as
// many as the lines given fill down the first, then across.
func comingRows(lines, columns int) int {
	return (lines + columns - 1) / columns
}
