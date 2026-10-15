package views_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// Runway's lanes have, at every moment Runway has to get right, each figure
// the moment wants of them.
func TestRunwaysLanesAtTheMomentsItHasToGetRight(t *testing.T) {
	for _, rm := range viewstest.RunwayMoments() {
		t.Run(rm.Name, func(t *testing.T) {
			from := lanesFrom(rm)
			got := runwayFigures(views.Lanes{}.Look(from), from)
			for id, want := range rm.Want {
				for name, figure := range want {
					if g := got[id][name]; g != figure {
						t.Errorf("%s's %s = %q, want %q", laneNamed(id), name, g, figure)
					}
				}
			}
		})
	}
}

// laneNamed names the lane of the account with the given id, as a test's
// failure says it: the view as a whole, "".
func laneNamed(id string) string {
	if id == "" {
		return "the view"
	}
	return id
}

// lanesFrom are what Runway's lanes are built from at the moment rm.
func lanesFrom(rm viewstest.RunwayMoment) views.LanesFrom {
	return views.LanesFrom{
		Moment: rm.Doc.At(rm.Now, claude.Policy), Readings: rm.Readings, Days: rm.Days, Kept: rm.Kept, Seen: rm.Seen,
		Timeline: views.Stepped(rm.Window, rm.Day, rm.Now, rm.WeekStarts, rm.Columns), Window: rm.Window,
	}
}

// runwayFigures are the figures of the lanes built from what's given, as
// viewstest's Runway figures name and write them.
func runwayFigures(l views.Lanes, from views.LanesFrom) map[string]viewstest.Figures {
	now := from.Moment.Now
	figures := map[string]viewstest.Figures{"": {
		viewstest.FigureNow:    nowColumn(from.Timeline),
		viewstest.FigureOf:     strconv.Itoa(l.Pool.Of),
		viewstest.FigurePool:   poolRuns(l.Pool),
		viewstest.FigureDips:   dipsSaid(l.Pool.Dips, now),
		viewstest.FigureDrawn:  drawnSaid(l.Drawn),
		viewstest.FigureComing: comingSaid(views.RunwayComing(from.Moment, from.Window), now),
	}}
	for _, lane := range l.Accounts {
		figures[lane.ID] = viewstest.Figures{
			viewstest.FigureCells:     cellRuns(lane.Cells),
			viewstest.FigureStretches: stretchesSaid(lane.Stretches, now),
			viewstest.FigureResets:    resetsSaid(lane.Resets, now),
			viewstest.FigureRoom:      map[bool]string{true: "yes", false: "no"}[lane.Room],
			viewstest.FigureUse:       useSaid(lane.Use, now),
		}
	}
	return figures
}

// nowColumn is the timeline's now's column, "none" where it has no now.
func nowColumn(tl views.Timeline) string {
	if c := tl.NowColumn(); c >= 0 {
		return strconv.Itoa(c)
	}
	return "none"
}

// cellWords are each cell's words, as the figures write it.
var cellWords = map[views.Cell]string{
	views.CellUnknown: "blank", views.CellRoom: "room", views.CellRunsOut: "runs out", views.CellCapped: "capped",
	views.CellLimited: "limited", views.CellRefused: "refused", views.CellTokenless: "no token",
}

// cellRuns are the cells as runs of like ones, as the figures write them.
func cellRuns(cells []views.Cell) string {
	words := make([]string, len(cells))
	for i, c := range cells {
		words[i] = cellWords[c]
	}
	return runsOf(words)
}

// poolRuns are the pool's columns as runs of like ones, as the figures write
// them.
func poolRuns(p views.Pool) string {
	words := make([]string, len(p.Columns))
	for i, c := range p.Columns {
		words[i] = "blank"
		if c.Known {
			words[i] = strconv.Itoa(c.With)
		}
	}
	return runsOf(words)
}

// runsOf are the words given, a column's each, as runs of like ones: each
// its words and its columns, ", " between.
func runsOf(words []string) string {
	var runs []string
	for first := 0; first < len(words); {
		last := first
		for last+1 < len(words) && words[last+1] == words[first] {
			last++
		}
		runs = append(runs, words[first]+" "+columnsSaid(first, last))
		first = last + 1
	}
	return strings.Join(runs, ", ")
}

// columnsSaid are the columns from first to last, as "5-20", or "7" for one.
func columnsSaid(first, last int) string {
	if first == last {
		return strconv.Itoa(first)
	}
	return fmt.Sprintf("%d-%d", first, last)
}

// stretchesSaid are the stretches as the figures write them.
func stretchesSaid(stretches []views.LaneStretch, now time.Time) string {
	said := make([]string, len(stretches))
	for i, s := range stretches {
		said[i] = fmt.Sprintf("%s %s – %s", cellWords[s.Cell], edgeSaid(s.Start, s.Projected, now), timeSaid(s.Back, now))
		if !s.Lifts.IsZero() {
			said[i] += ", lifts " + timeSaid(s.Lifts, now)
		}
		if s.AsItResets {
			said[i] += ", as it resets"
		}
		said[i] += " @" + strconv.Itoa(s.Column) + " " + columnsSaid(s.First, s.Last)
		if s.Over {
			said[i] += ", over"
		}
	}
	return strings.Join(said, "; ")
}

// resetsSaid are the resets as the figures write them.
func resetsSaid(resets []views.Reset, now time.Time) string {
	said := make([]string, len(resets))
	for i, r := range resets {
		said[i] = strconv.Itoa(r.Column) + " " + timeSaid(r.At, now)
		if r.Back {
			said[i] += " back"
		}
	}
	return strings.Join(said, ", ")
}

// useSaid is the use as the figures write it.
func useSaid(u views.Use, now time.Time) string {
	share := fmt.Sprintf("%.0f%%", u.Used*100)
	switch {
	case u.Highest && u.Read:
		return u.Key + " highest " + share
	case u.Highest:
		return u.Key + " highest —"
	case u.Read && u.Stale:
		return u.Key + " " + share + " stale, read " + timeSaid(u.ReadAt, now)
	case u.Read:
		return u.Key + " " + share
	case u.Tokenless:
		return "no token"
	case u.Unread:
		return "not read yet"
	}
	return "—"
}

// dipsSaid are the dips as the figures write them.
func dipsSaid(dips []views.Dip, now time.Time) string {
	said := make([]string, len(dips))
	for i, d := range dips {
		steps := make([]string, len(d.Steps))
		for j, s := range d.Steps {
			steps[j] = fmt.Sprintf("%d %s – %s", s.With, edgeSaid(s.From.At, s.From.Projected, now), edgeSaid(s.Until.At, s.Until.Projected, now))
		}
		said[i] = strings.Join(steps, ", ") + " @" + strconv.Itoa(d.Column) + " " + columnsSaid(d.First, d.Last)
		switch {
		case d.Over:
			said[i] += ", over"
		case d.Ahead:
			said[i] += ", ahead"
		}
	}
	return strings.Join(said, "; ")
}

// drawnSaid is what the lanes draw, as the figures write it.
func drawnSaid(d views.Drawn) string {
	said := make([]string, 0, len(d.Cells)+1)
	for _, c := range d.Cells {
		said = append(said, cellWords[c])
	}
	if d.Resets {
		said = append(said, "│")
	}
	return strings.Join(said, ", ")
}

// comingSaid is what COMING UP lists, as the figures write it.
func comingSaid(coming []status.Upcoming, now time.Time) string {
	said := make([]string, len(coming))
	for i, u := range coming {
		said[i] = timeSaid(u.At, now) + " " + u.Account + " " + u.Kind
		if u.Window != "" {
			said[i] += " " + u.Window
		}
		if u.Cap {
			said[i] += " cap"
		}
	}
	return strings.Join(said, "; ")
}

// edgeSaid is a time as the figures write it, "~" before it where it's
// projected.
func edgeSaid(t time.Time, projected bool, now time.Time) string {
	if projected {
		return "~" + timeSaid(t, now)
	}
	return timeSaid(t, now)
}

// timeSaid is t as the figures write it, to the second, in now's time zone:
// "15:04", with ":05" where its seconds aren't none, its weekday before it on
// a day other than now's, and "—" where it's zero.
func timeSaid(t, now time.Time) string {
	if t.IsZero() {
		return "—"
	}
	t = t.In(now.Location()).Round(time.Second)
	layout := "15:04"
	if t.Second() != 0 {
		layout += ":05"
	}
	if y, m, d := t.Date(); y != now.Year() || m != now.Month() || d != now.Day() {
		layout = "Mon " + layout
	}
	return t.Format(layout)
}
