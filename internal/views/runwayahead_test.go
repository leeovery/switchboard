package views_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// What a lane draws ahead of now is what it draws of the same columns once a
// later look has them in the past, the document and the readings unchanged:
// at every hard moment, and at every moment Runway has to get right, in
// either view, of today and of this week, each column after now's that a
// look later, now in the timeline's last column, has before it; and the
// stretch holding now's column begins where the later look has it begin.
// The readings hold what the document read, as the router keeps them; a
// moment whose readings read an account later than its document did, so
// that the two tell it otherwise, is left out. A column a window's run-out
// projects held is a guess no reading confirms, so has room once past, as
// one running out does; and a column a hold no event tells covers is left
// out, as nothing keeps such a hold once it lifts.
func TestALaneDrawsAheadWhatItDrawsOnceThoseColumnsArePast(t *testing.T) {
	for _, c := range laterCases() {
		t.Run(c.name, func(t *testing.T) {
			ahead, past := views.Lanes{}.Look(c.now), views.Lanes{}.Look(c.later)
			tl, m := c.now.Timeline, c.now.Moment
			for i, lane := range ahead.Accounts {
				a := m.Accounts[i]
				var differ []int
				for col := tl.NowColumn() + 1; col < c.later.Timeline.NowColumn(); col++ {
					if at := tl.Moment(col); !projected(a, at) && !untold(m, a.ID, at) && past.Accounts[i].Cells[col] != asPast(lane.Cells[col]) {
						differ = append(differ, col)
					}
				}
				if len(differ) > 0 {
					col := differ[0]
					t.Errorf("%s's column %d, at %s, is drawn %s ahead and %s once past, and %d more columns differ",
						a.ID, col, tl.Moment(col).Format("Mon 15:04"), cellWords[lane.Cells[col]], cellWords[past.Accounts[i].Cells[col]], len(differ)-1)
				}
				now := tl.NowColumn()
				if s, ok := holding(lane.Stretches, now); ok && !s.Projected && !untold(m, a.ID, tl.Moment(now)) {
					if later, ok := holding(past.Accounts[i].Stretches, now); ok && later.Cell == s.Cell && !later.Start.Equal(s.Start) {
						t.Errorf("%s's stretch holding now, %s, begins %v drawn now and %v once past", a.ID, cellWords[s.Cell], s.Start, later.Start)
					}
				}
			}
		})
	}
}

// holding is the stretch of those given that holds the column col,
// reporting false where none does.
func holding(stretches []views.LaneStretch, col int) (views.LaneStretch, bool) {
	i := slices.IndexFunc(stretches, func(s views.LaneStretch) bool { return s.First <= col && col <= s.Last })
	if i < 0 {
		return views.LaneStretch{}, false
	}
	return stretches[i], true
}

// laterCase is what the lanes at a moment are built from, named, and what a
// later look's are: of the same timeline, its now in its last column, the
// moment's document and readings unchanged.
type laterCase struct {
	name       string
	now, later views.LanesFrom
}

// laterCases are the lanes the test builds, now and later: of each hard
// moment, its readings those its document read; and of each moment Runway
// has to get right whose readings read no account later than its document,
// its readings and what its document read; in either view, of the day and
// of the week its now falls in.
func laterCases() []laterCase {
	var cases []laterCase
	for _, hm := range statustest.HardMoments(claude.Policy) {
		cases = append(cases, laterOf(hm.Name, hm.Doc, hm.Now, docReadings(hm.Doc))...)
	}
	for _, rm := range viewstest.RunwayMoments() {
		if !readSince(rm.Readings, rm.Doc) {
			read := slices.Concat(rm.Readings, docReadings(rm.Doc))
			slices.SortStableFunc(read, func(a, b readings.Reading) int { return a.At.Compare(b.At) })
			cases = append(cases, laterOf(rm.Name, rm.Doc, rm.Now, read)...)
		}
	}
	return cases
}

// readSince reports whether the readings given read an account later than
// the document doc last did.
func readSince(read []readings.Reading, doc status.Document) bool {
	return slices.ContainsFunc(read, func(r readings.Reading) bool {
		a, _ := doc.Account(r.Account)
		return r.At.After(a.FetchedAt)
	})
}

// laterOf are the cases of the document doc read at now, the readings given,
// in either view, of now's day and now's week, weeks starting on Monday.
func laterOf(name string, doc status.Document, now time.Time, read []readings.Reading) []laterCase {
	y, mo, d := now.Date()
	day := time.Date(y, mo, d, 0, 0, 0, 0, time.UTC)
	var cases []laterCase
	for _, w := range []views.RunwayWindow{views.RunwayFiveHour, views.RunwayWeek} {
		at := func(t time.Time) views.LanesFrom {
			return views.LanesFrom{Moment: doc.At(t, claude.Policy), Readings: read, Timeline: views.Stepped(w, day, t, time.Monday, 136), Window: w}
		}
		first := at(now)
		later, _ := first.Timeline.Bounds(first.Timeline.Columns - 1)
		cases = append(cases, laterCase{name: fmt.Sprintf("%s, the %s view", name, viewKeys[w]), now: first, later: at(later)})
	}
	return cases
}

// projected reports whether a window of the account a every model shares
// has run out by t, before it resets, as the moment projects it.
func projected(a status.AccountNow, t time.Time) bool {
	return slices.ContainsFunc(a.Windows, func(w status.WindowNow) bool {
		return w.Shared && w.RunsOutAhead && !w.Out.At.IsZero() && !t.Before(w.Out.At) && (w.Next.IsZero() || t.Before(w.Next))
	})
}

// untold reports whether a hold of the account with the given id that no
// event tells holds at t, as the moment's Told has it, from before anything.
func untold(m status.Moment, id string, t time.Time) bool {
	return slices.ContainsFunc(m.Told(id), func(s status.Span) bool { return s.From.IsZero() && t.Before(s.Until) })
}

// asPast is the cell c drawn ahead as it's drawn once past: running out, with
// room, as nothing past runs out.
func asPast(c views.Cell) views.Cell {
	if c == views.CellRunsOut {
		return views.CellRoom
	}
	return c
}
