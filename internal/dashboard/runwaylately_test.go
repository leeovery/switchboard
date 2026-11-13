package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayLatelyOf are the events of the document m is a moment of, held as
// one router told them, as Runway's LATELY tells them.
func runwayLatelyOf(m status.Moment) []views.Sentence {
	return views.RunwayLately(m, views.EventsSince(time.Time{}, m, time.Time{}, nil))
}

// LATELY goes beside COMING UP where both columns hold a whole line, COMING
// UP its natural 72 in the Overview's COMING UP column and LATELY its least,
// 46, in the Overview's LATELY column: from 159 columns. Else it goes under
// COMING UP where one column holds its least, from 46; else nowhere.
func TestLatelyGoesBesideComingUpFrom159ColumnsElseUnderItElseNowhere(t *testing.T) {
	if got := runwayLatelyLeast(); got != 46 {
		t.Fatalf("LATELY's least is %d cells, want 46: a time of 12, two blanks, a name of 8, ` open again` and two blanks before `23h 59m ago`", got)
	}
	tests := []struct {
		width int
		want  latelyPlace
	}{
		{width: 216, want: latelySideBySide},
		{width: 173, want: latelySideBySide},
		{width: 159, want: latelySideBySide},
		{width: 158, want: latelyStacked},
		{width: 108, want: latelyStacked},
		{width: 73, want: latelyStacked},
		{width: 46, want: latelyStacked},
		{width: 45, want: comingAlone},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.width), func(t *testing.T) {
			if got := (Frame{Width: tt.width}).latelyPlaceOf(); got != tt.want {
				t.Errorf("at %d columns, LATELY's place is %d, want %d", tt.width, got, tt.want)
			}
		})
	}
}

// Side by side, LATELY takes the Overview's LATELY column's width from column
// 0, w − ⌊w/2⌋ − 2, and COMING UP the Overview's COMING UP column's,
// round(w × 0.45), ending at the frame's edge; stacked, COMING UP keeps the
// columns it takes alone, LATELY under them from column 0, as wide as they
// span, and where COMING UP takes one column, both share it, 72 where their
// lines read whole in it, or the frame's width where that's narrower; and
// alone, COMING UP takes comingColumns' columns.
func TestLatelysColumnIsTheOverviewsBesideComingUpElseOneItShares(t *testing.T) {
	m := latelyMoment(slices.Clone(runwayEvents)...)
	tests := []struct {
		width  int
		place  latelyPlace
		lately flex.Span
		coming []flex.Span
	}{
		{width: 173, place: latelySideBySide, lately: flex.Span{Size: 85}, coming: []flex.Span{{At: 95, Size: 78}}},
		{width: 159, place: latelySideBySide, lately: flex.Span{Size: 78}, coming: []flex.Span{{At: 87, Size: 72}}},
		{width: 158, place: latelyStacked, lately: flex.Span{Size: 158}, coming: []flex.Span{{Size: 77}, {At: 81, Size: 77}}},
		{width: 150, place: latelyStacked, lately: flex.Span{Size: 150}, coming: []flex.Span{{Size: 73}, {At: 77, Size: 73}}},
		{width: 108, place: latelyStacked, lately: flex.Span{Size: 72}, coming: []flex.Span{{Size: 72}}},
		{width: 52, place: latelyStacked, lately: flex.Span{Size: 52}, coming: []flex.Span{{Size: 52}}},
		{width: 160, place: comingAlone, coming: []flex.Span{{Size: 78}, {At: 82, Size: 78}}},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.width), func(t *testing.T) {
			f := latelyFrame(tt.width, 53, 4, 0)
			f.Runway.Lately = runwayLatelyOf(m)
			coming, lately := f.runwayColumns(m, runwayFit{place: tt.place, latelyLines: 10})
			if lately != tt.lately || !slices.Equal(coming, tt.coming) {
				t.Errorf("at %d columns, LATELY laid %d, its column is %+v and COMING UP's %+v, want %+v and %+v", tt.width, tt.place, lately, coming, tt.lately, tt.coming)
			}
		})
	}
}

// Stacked, COMING UP and LATELY share one width: what their lines need to
// read whole, a long name among them, never under 72, nor over the frame's
// width, so a name is cut only where the frame is too narrow for it.
func TestStackedComingUpAndLatelyWidenTogetherForTheirLinesToReadWhole(t *testing.T) {
	long := latelyMomentOf(func(doc *status.Document) {
		doc.Accounts[0].Label, doc.Accounts[2].Label = "work-team-plan", "side-hustle-consulting"
	}, slices.Clone(runwayEvents)...)
	stacked := runwayFit{place: latelyStacked, latelyLines: 10}
	f := latelyFrame(108, 53, 0, 0)
	f.Runway.Coming = []status.Upcoming{{At: routedClock(2, 16, 20), Account: "side", Kind: status.UpcomingRunsOut, Window: "7d"}}
	f.Runway.Lately = runwayLatelyOf(long)
	coming, lately := f.runwayColumns(long, stacked)
	if lately.Size <= runwayComingWidth || lately.Size >= f.Width || !slices.Equal(coming, []flex.Span{lately}) {
		t.Fatalf("with a long name at 108, LATELY's column is %+v and COMING UP's %+v; want one they share, wider than 72, narrower than the frame", lately, coming)
	}
	cut := func(cells int) bool {
		said := slices.ContainsFunc(runwayLatelyLines(long, f.Runway.Lately, cells), func(l line) bool { return strings.Contains(l.plain(), "…") })
		things := f.runwayHappenings(long)
		for _, h := range things {
			said = said || strings.Contains(h.said(long.Now, whenColumn(things, long.Now)).fit(cells-comingTail).plain(), "…")
		}
		return said
	}
	if cut(lately.Size) || !cut(lately.Size-1) {
		t.Errorf("at %d cells, a line is cut %t, and a cell narrower %t; want every line whole, and one cut a cell narrower", lately.Size, cut(lately.Size), cut(lately.Size-1))
	}
	narrow := f
	narrow.Width = 80
	if _, lately := narrow.runwayColumns(long, stacked); lately.Size != 80 {
		t.Errorf("with a long name at 80, LATELY's column is %+v, want the frame's width", lately)
	}
}

// latelyFrame is a frame showing Runway's live view at routedAt, of the
// size given, its lanes of three accounts, COMING UP's lines as many as
// given and LATELY's as many as given.
func latelyFrame(width, height, coming, lately int) Frame {
	shown := RunwayShown{Lanes: lanesOf(3), Coming: thingsComing(coming), Lately: make([]views.Sentence, lately)}
	shown.Timeline = shown.TimelineAt(routedAt, time.Monday, width)
	return Frame{Width: width, Height: height, View: Runway, Policy: claudeLike, Runway: shown}
}

// Side by side, from 159 columns, LATELY's heading is on COMING UP's row, and
// both run down to the blank row over the legend; at 158, LATELY is stacked
// under COMING UP's every line, in the two columns it takes alone, a blank
// row between, down to the same row.
func TestLatelyRunsFromItsHeadingToTheBlankRowOverTheLegend(t *testing.T) {
	tests := []struct {
		width int
		place latelyPlace
		row   int
	}{
		{width: 159, place: latelySideBySide, row: 23},
		{width: 158, place: latelyStacked, row: 27},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.width), func(t *testing.T) {
			f := latelyFrame(tt.width, 53, 4, 30)
			fit := f.runwayFitted(f.top())
			if fit.place != tt.place || fit.lately != tt.row {
				t.Fatalf("LATELY is laid %d on row %d, want %d on row %d", fit.place, fit.lately, tt.place, tt.row)
			}
			if over := fit.legend - 2; fit.lately+fit.latelyLines != over {
				t.Errorf("LATELY's lines end on row %d, want the row before the blank one over the legend, %d", fit.lately+fit.latelyLines, over)
			}
			if tt.place == latelySideBySide && fit.lately != fit.coming {
				t.Errorf("side by side, LATELY's heading is on row %d, COMING UP's on %d; want them on one", fit.lately, fit.coming)
			}
			if tt.place == latelyStacked && fit.lately != fit.coming+1+fit.lines+1 {
				t.Errorf("stacked, LATELY's heading is on row %d, want a blank row after COMING UP's last, %d", fit.lately, fit.coming+fit.lines+1)
			}
		})
	}
}

// Stacked, LATELY shows where its heading and two lines fit in the rows the
// lanes and COMING UP leave, COMING UP's every line in its column over it; a
// row short of that, it goes first, before any part compacts, and COMING UP
// is alone, as it was before LATELY.
func TestStackedLatelyShowsWhereItsHeadingAndTwoLinesFitElseGoesFirst(t *testing.T) {
	const least = 35
	fits := latelyFrame(108, least, 4, 9)
	fit := fits.runwayFitted(fits.top())
	if fit.place != latelyStacked || !fit.draws(fit.lately) || fit.latelyLines != latelyLeastLines || fit.step != runwayRoomy {
		t.Fatalf("at 108 by %d, LATELY is laid %d, drawn %t with %d lines, at step %d; want stacked, its heading and two lines, at the roomiest", least, fit.place, fit.draws(fit.lately), fit.latelyLines, fit.step)
	}
	short := latelyFrame(108, least-1, 4, 9)
	fit = short.runwayFitted(short.top())
	if fit.place != comingAlone || fit.draws(fit.lately) || fit.step != runwayRoomy || fit.lines != 4 {
		t.Errorf("at 108 by %d, LATELY is laid %d, drawn %t, at step %d, COMING UP's %d lines; want COMING UP alone at the roomiest, every line", least-1, fit.place, fit.draws(fit.lately), fit.step, fit.lines)
	}
}

// LATELY never costs the lanes or COMING UP a step, nor COMING UP its
// columns: Runway takes the step and COMING UP's columns it takes without
// it, and LATELY goes in the rows left. Four accounts and twelve lines at 37
// rows keep the roomiest step, COMING UP in two columns, at 150, 158 and 160
// alike, with no rows left for LATELY beside or under it; given rows enough,
// at 150 and 158 it's under COMING UP's two columns, and at 160 beside it,
// every line in one column, the step the same.
func TestLatelyNeverCostsTheLanesOrComingUpAStep(t *testing.T) {
	for _, width := range []int{150, 158, 160} {
		f := latelyFrame(width, 37, 12, 9)
		f.Runway.Lanes = lanesOf(4)
		fit := f.runwayFitted(f.top())
		if fit.step != runwayRoomy || fit.place != comingAlone || fit.draws(fit.lately) || fit.lines != 6 {
			t.Errorf("at %d by 37, Runway fits at step %d, LATELY laid %d, drawn %t, COMING UP %d rows; want the roomiest, no LATELY, COMING UP's 12 lines in two columns of 6", width, fit.step, fit.place, fit.draws(fit.lately), fit.lines)
		}
	}
	for _, width := range []int{150, 158} {
		f := latelyFrame(width, 45, 12, 9)
		f.Runway.Lanes = lanesOf(4)
		if fit := f.runwayFitted(f.top()); fit.step != runwayRoomy || fit.place != latelyStacked || fit.lines != 6 {
			t.Errorf("at %d by 45, Runway fits at step %d, LATELY laid %d, COMING UP %d rows; want the roomiest, LATELY under COMING UP's 12 lines in two columns of 6", width, fit.step, fit.place, fit.lines)
		}
	}
	f := latelyFrame(160, 45, 12, 9)
	f.Runway.Lanes = lanesOf(4)
	if fit := f.runwayFitted(f.top()); fit.step != runwayRoomy || fit.place != latelySideBySide || fit.lines != 12 {
		t.Errorf("at 160 by 45, Runway fits at step %d, LATELY laid %d, COMING UP %d rows; want the roomiest, LATELY beside COMING UP's 12 lines in one column", fit.step, fit.place, fit.lines)
	}
}

// latelyMoment is the frames' moment at routedAt, Thursday 14:42:07, with the
// router's events given, as latelyMomentOf has it.
func latelyMoment(events ...status.Event) status.Moment {
	return latelyMomentOf(func(*status.Document) {}, events...)
}

// latelyMomentOf is the frames' moment at routedAt, Thursday 14:42:07, its
// windows labelled as Claude labels them, its document changed as change
// has it, with the router's events given, the newest first.
func latelyMomentOf(change func(*status.Document), events ...status.Event) status.Moment {
	doc := framesDoc()
	change(&doc)
	doc = doc.WorkedOut(claudeLike)
	for _, a := range doc.Accounts {
		for i, w := range a.Windows {
			a.Windows[i].Label = claude.WindowLabel(w.Key)
		}
	}
	for i := range events {
		events[i].ID = len(events) - i
	}
	doc.Events = events
	return doc.At(routedAt, claudeLike)
}

// runwayEvents are events of the frames' world, as Runway's signed-off frames
// tell them: work's cap at 14:31, three sessions moved to side; personal
// under pressure at 13:04, its 5-hour running out ~13:50; a session started
// at 12:47, which LATELY never tells; and personal open again at Wednesday
// 23:58.
var runwayEvents = []status.Event{
	{Kind: status.EventCap, At: routedClock(1, 14, 31), Account: "work", Windows: []string{"5h"}, Count: 3, To: "side", Reserve: 0.05},
	{Kind: status.EventPressure, At: routedClock(1, 13, 4), Account: "personal", Windows: []string{"5h"}, Until: routedClock(1, 13, 50)},
	{Kind: status.EventStarted, At: routedClock(1, 12, 47), Account: "personal", Session: "41e0"},
	{Kind: status.EventRoom, At: time.Date(2026, 9, 30, 23, 58, 0, 0, time.UTC), Account: "personal", Windows: []string{"5h"}},
}

// LATELY's lines are the account events alone, newest first, no session's
// start among them: each its time, padded to the longest shown, two blanks,
// its words in full where they fit whole beside how long ago it was, else
// short, and how long ago, right-aligned at the column's end.
func TestLatelysLinesAreItsAccountEventsWholeOrShortTheirAgoAtTheEnd(t *testing.T) {
	m := latelyMoment(slices.Clone(runwayEvents)...)
	tests := []struct {
		cells int
		want  []string
	}{
		{
			cells: 85, want: []string{
				"14:31      work reached its cap: 3 sessions moved to side                     11m ago",
				"13:04      personal came under pressure: its 5-hour runs out ~13:50        1h 38m ago",
				"Wed 23:58  personal open again: its 5-hour window reset                   14h 44m ago",
			},
		},
		{
			cells: 72, want: []string{
				"14:31      work reached its cap: 3 sessions moved to side        11m ago",
				"13:04      personal under pressure: out ~13:50                1h 38m ago",
				"Wed 23:58  personal open again: its 5-hour window reset      14h 44m ago",
			},
		},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.cells), func(t *testing.T) {
			var got []string
			for _, l := range runwayLatelyLines(m, runwayLatelyOf(m), tt.cells) {
				got = append(got, l.plain())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("LATELY reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

// A LATELY line from a week back, on today's weekday, says its date in place
// of its weekday, as that alone would read as today's; the times' column as
// wide as the widest shown.
func TestLatelysLineFromAWeekBackSaysItsDate(t *testing.T) {
	weekBack := status.Event{Kind: status.EventLimit, At: time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC), Account: "side", Windows: []string{"5h"}}
	m := latelyMoment(slices.Concat(runwayEvents[:1], runwayEvents[3:], []status.Event{weekBack})...)
	want := []string{
		"14:31         work reached its cap: 3 sessions moved to side        11m ago",
		"Wed 23:58     personal open again: its 5-hour window reset      14h 44m ago",
		"24 Sep 15:00  side reached its 5-hour limit                      6d 23h ago",
	}
	var got []string
	for _, l := range runwayLatelyLines(m, runwayLatelyOf(m), 75) {
		got = append(got, l.plain())
	}
	if !slices.Equal(got, want) {
		t.Errorf("LATELY reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A LATELY line's time is in text.secondary, its account bold text.primary,
// what befell it text.muted, its object bold text.secondary, and how long
// ago text.subtle, with no mark before its words.
func TestLatelysLinesAreInkedAsTheDesignSays(t *testing.T) {
	m := latelyMoment(slices.Clone(runwayEvents[:1])...)
	c := newCanvas(72, 1)
	c.line(0, 0, runwayLatelyLines(m, runwayLatelyOf(m), 72)[0])
	for _, tt := range []struct {
		name string
		x    int
		want ink
	}{
		{name: "its time", x: 0, want: secondaryInk},
		{name: "its account", x: 7, want: titleInk},
		{name: "what befell it", x: 12, want: mutedInk},
		{name: "its object", x: 29, want: strongInk},
		{name: "how long ago", x: 71, want: dimInk},
	} {
		if got := c.at(tt.x, 0).ink; got != tt.want {
			t.Errorf("%s, in column %d, %q, is in %+v, want %+v", tt.name, tt.x, c.at(tt.x, 0).glyph, got, tt.want)
		}
	}
}

// Drawn, LATELY's heading is in text.subtle over its lines, in its column;
// with no events, it says why, as the Overview's LATELY does.
func TestRunwayDrawsLatelyInItsColumnOrWhyItHasNone(t *testing.T) {
	m := latelyMoment(slices.Clone(runwayEvents)...)
	for _, tt := range []struct {
		name     string
		sentence []views.Sentence
		want     []string
	}{
		{name: "its lines", sentence: runwayLatelyOf(m), want: []string{"LATELY", "14:31      work reached its cap: 3 sessions moved to side                     11m ago"}},
		{name: "none", want: []string{"LATELY", "nothing lately"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := latelyFrame(173, 53, 4, 0)
			f.Runway.Lately = tt.sentence
			fit := f.runwayFitted(f.top())
			c := newCanvas(f.Width, f.Height)
			f.runwayLately(c, f.runwayLaidOut(m, 0, fit))
			for i, want := range tt.want {
				if got := strings.TrimRight(textAt(c, 0, fit.lately+i, 85), " "); got != want {
					t.Errorf("LATELY's row %d reads %q, want %q", i, got, want)
				}
			}
			if got := c.at(0, fit.lately).ink; got != dimInk {
				t.Errorf("LATELY's heading is in %+v, want text.subtle", got)
			}
		})
	}
}
