package views_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status/statustest"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// What Runway's lanes say is the same whatever span of the readings history
// Runway's tail reads, so long as it covers the timeline: at every hard
// moment, live in either view, and at every moment Runway has to get right,
// each figure of the lanes judged by what a tail reads, as the watch's does,
// from the timeline's start, from the start of its day, and from three days
// before, each window's run before it carried in from a week before, is that
// of the lanes judged by every reading up to the tail's end, a week on from
// now's day.
func TestRunwaysLanesAreTheSameWhateverSpanTheTailReads(t *testing.T) {
	for _, c := range spanCases() {
		t.Run(c.name, func(t *testing.T) {
			tl, now := c.from.Timeline, c.from.Moment.Now
			end := dayfile.DayStart(now, 7)
			every := c.from
			every.Readings = tailRead(c.read, time.Time{}, end)
			want := runwayFigures(views.Lanes{}.Look(every), every)
			for _, start := range slices.CompactFunc([]time.Time{tl.Start, dayfile.DayStart(tl.Start, 0), dayfile.DayStart(tl.Start, -3)}, time.Time.Equal) {
				cut := c.from
				cut.Readings = tailRead(c.read, start, end)
				got := runwayFigures(views.Lanes{}.Look(cut), cut)
				for id, figures := range want {
					for name, figure := range figures {
						if g := got[id][name]; g != figure {
							t.Errorf("read from %s, %s's %s = %q, want %q, as every reading has it", start.Format("Mon 2 Jan 15:04"), laneNamed(id), name, g, figure)
						}
					}
				}
			}
		})
	}
}

// spanCase is what the lanes at a moment are built from, named, and every
// reading of the readings history then, oldest first.
type spanCase struct {
	name string
	from views.LanesFrom
	read []readings.Reading
}

// spanCases are the lanes the test builds: of each hard moment, live in
// either view, its readings those its document read; and of each moment
// Runway has to get right, as it's shown.
func spanCases() []spanCase {
	var cases []spanCase
	for _, hm := range statustest.HardMoments(claude.Policy) {
		for _, w := range []views.RunwayWindow{views.RunwayFiveHour, views.RunwayWeek} {
			from := views.LanesFrom{Moment: hm.Doc.At(hm.Now, claude.Policy), Timeline: views.LiveTimeline(w, hm.Now, 136), Window: w}
			cases = append(cases, spanCase{name: fmt.Sprintf("%s, the %s view", hm.Name, viewKeys[w]), from: from, read: docReadings(hm.Doc)})
		}
	}
	for _, rm := range viewstest.RunwayMoments() {
		cases = append(cases, spanCase{name: rm.Name, from: lanesFrom(rm), read: rm.Readings})
	}
	return cases
}

// tailRead is what Runway's tail reads of the readings given, from start up
// to end, each window's run from a week before start carried in, as the
// watch's tail reads them: every one before end where start is zero.
func tailRead(read []readings.Reading, start, end time.Time) []readings.Reading {
	from := dayfile.DayStart(start, -7)
	if start.IsZero() {
		from = start
	}
	return readings.CarriedRuns(slices.Values(read), from, start, end)
}
