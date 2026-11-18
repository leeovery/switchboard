package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// newLane is work's lane, new in the view since the time given over the
// columns given, nothing known of them, with room after them, and the
// stretches given.
func newLane(since time.Time, columns int, stretches ...views.LaneStretch) laidLane {
	ln := laneWith(stretches...)
	ln.Cells = slices.Concat(make([]views.Cell, columns), slices.Repeat([]views.Cell{views.CellRoom}, 136-columns))
	ln.New = views.LaneNew{Since: since, Columns: columns}
	return ln
}

// A lane new in the view is dotted before its account's first reading, ┈ in
// text.faint along its row from the lanes' first column, and the same
// without colour, as its dots tell it from ━ alone; its first column read is
// drawn as known; a lane that isn't new is blank there.
func TestALaneNewInTheViewIsDottedFaintBeforeItsFirstReading(t *testing.T) {
	for _, tt := range []struct {
		name string
		look Look
	}{
		{name: "in colour", look: Screen(builtin(t, "nord"))},
		{name: "without colour", look: NoColour()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := runwayShowing(runwayLive())
			f.Look = tt.look
			c := drawnLane(f, newLane(routedClock(1, 9, 0), 5))
			for col := range 5 {
				if got := c.at(framesLanesFrom+col, 10); got.glyph != "┈" || got.ink != faintInk {
					t.Errorf("column %d is %q in %+v, want ┈ in text.faint", col, got.glyph, got.ink)
				}
			}
			if got := c.at(framesLanesFrom+5, 10).glyph; got != roomCell {
				t.Errorf("the first column read is %q, want %q", got, roomCell)
			}
			unread := laneWith()
			if got := drawnLane(f, unread).at(framesLanesFrom, 10).glyph; got != " " {
				t.Errorf("a lane not new is %q in its first column, want blank", got)
			}
		})
	}
	if got := Screen(builtin(t, "nord")).laneNewMark(); got.ink.token != theme.TextFaint {
		t.Errorf("the dots are in %v, want text.faint", got.ink.token)
	}
}

// What's said of a new stretch, in text.faint from its first column, is
// "new since" its first reading, as a stretch's start is told against now,
// its weekday on a day not today's; "new" where the stretch is too narrow for
// that; and nothing where it's too narrow even for "new".
func TestANewStretchSaysSinceWhenNarrowingToFit(t *testing.T) {
	tests := []struct {
		name    string
		since   time.Time
		columns int
		want    string
	}{
		{name: "today", since: routedClock(1, 9, 0), columns: 19, want: "new since 09:00"},
		{name: "a day before", since: time.Date(2026, 9, 30, 20, 0, 0, 0, time.UTC), columns: 19, want: "new since Wed 20:00"},
		{name: "just wide enough", since: routedClock(1, 9, 0), columns: 15, want: "new since 09:00"},
		{name: "a cell too narrow, new", since: routedClock(1, 9, 0), columns: 14, want: "new"},
		{name: "as wide as new", since: routedClock(1, 9, 0), columns: 3, want: "new"},
		{name: "too narrow even for new", since: routedClock(1, 9, 0), columns: 2, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := runwayShowing(runwayLive())
			c := newCanvas(f.Width, 20)
			f.runwayWords(c, runwayLaid{m: status.Moment{Now: routedAt}, x: framesLanesFrom, lanes: []laidLane{newLane(tt.since, tt.columns)}})
			if got := strings.TrimSpace(textAt(c, framesLanesFrom, 11, tt.columns)); got != tt.want {
				t.Errorf("the row under the stretch reads %q, want %q", got, tt.want)
			}
			if tt.want != "" {
				if got := c.at(framesLanesFrom, 11).ink; got != faintInk {
					t.Errorf("its words are in %+v, want text.faint", got)
				}
			}
		})
	}
}

// The words of a new stretch never meet the lane's other words: those keep
// their place, and what's said of the new stretch gives way, ending
// phraseGap before them, "new" where "new since" doesn't, nothing where
// neither does. With the account column collapsed, they start phraseGap
// after the use, as the lane's other words do.
func TestANewStretchsWordsGiveWayToTheLanesOthers(t *testing.T) {
	tl := runwayLive().Timeline
	out := func(first int) views.LaneStretch {
		return stretchOf(tl, views.CellLimited, tl.Moment(first), routedClock(2, 3, 0), first, first+3)
	}
	tests := []struct {
		name      string
		lane      laidLane
		collapsed bool
		want      string
	}{
		{name: "words far after it", lane: newLane(routedClock(1, 9, 0), 18, out(40)), want: "new since 09:00" + strings.Repeat(" ", 25) + "out Fri 16:41  ·  back Fri 03:00"},
		{name: "words shifted left over it, new", lane: newLane(routedClock(1, 9, 0), 18, out(8)), want: "new     out 01:09  ·  back Fri 03:00"},
		{name: "words too near for new, nothing", lane: newLane(routedClock(1, 9, 0), 18, out(4)), want: "    out Wed 20:13  ·  back Fri 03:00"},
		{name: "collapsed, after the use", lane: newLane(routedClock(1, 9, 0), 20), collapsed: true, want: "34%  new since 09:00"},
		{name: "collapsed, too narrow after the use", lane: newLane(routedClock(1, 9, 0), 19), collapsed: true, want: "34%  new"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runwayLive()
			r.Collapsed = tt.collapsed
			f := runwayShowing(r)
			ln := tt.lane
			ln.Use = views.Use{Key: "7d", Used: 0.34, Read: true}
			x := r.lanesFrom(f.Width)
			c := newCanvas(f.Width, 20)
			laid := runwayLaid{m: status.Moment{Now: routedAt}, x: x, lanes: []laidLane{ln}}
			if tt.collapsed {
				f.runwayLanes(c, laid)
			}
			f.runwayWords(c, laid)
			if got := strings.TrimRight(textAt(c, x, 11, f.Width-x), " "); got != tt.want {
				t.Errorf("the row under the lane reads\n%q, want\n%q", got, tt.want)
			}
		})
	}
}
