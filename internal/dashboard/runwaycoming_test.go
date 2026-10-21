package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayComingDrawn are the rows of Runway's COMING UP of the frames' world, as a
// frame width cells wide and as tall as given draws them, listing the things
// coming up given: its heading's and those under it, as text.
func runwayComingDrawn(t *testing.T, width, height int, coming []status.Upcoming) []string {
	t.Helper()
	doc := framesDoc()
	r := poolShown(doc, views.Pool{Of: 3, Key: "5h"})
	r.Timeline, r.Coming = views.LiveTimeline(views.RunwayFiveHour, routedAt, RunwayShown{}.Columns(width)), coming
	f := Frame{Width: width, Height: height, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: r}
	f.WindowName = func(key string) string { return map[string]string{"5h": "5-hour", "7d": "week"}[key] }
	fit := f.runwayFitted(f.top())
	c := newCanvas(width, height)
	f.runwayComing(c, runwayLaid{x: framesLanesFrom, fit: fit, m: f.Moment(doc, routedAt)})
	return textOf(c.rows(Look{}))[fit.coming : fit.coming+1+fit.lines]
}

// The frames' world's things coming up in the 5-hour view: personal's
// 5-hour resetting, work back from its cap, and side's 5-hour resetting.
var fiveHourComing = []status.Upcoming{
	{At: routedClock(1, 16, 30), Account: "personal", Kind: status.UpcomingReset, Window: "5h"},
	{At: routedClock(1, 17, 10), Account: "work", Kind: status.UpcomingBack, Cap: true},
	{At: routedClock(1, 18, 50), Account: "side", Kind: status.UpcomingReset, Window: "5h"},
}

// Runway's COMING UP lists what Runway's own list holds, as the watch gives
// it, never what else the moment has coming up, a line each, the Overview's
// rows: in one column, its countdowns ending at column 71 at their natural,
// and at the frame's edge on a phone.
func TestRunwaysComingUpListsWhatRunwaysListHolds(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
		coming []status.Upcoming
		want   []string
	}{
		{
			name: "the 5-hour frame's three", width: 120, height: 37, coming: fiveHourComing,
			want: []string{
				"COMING UP",
				"16:30  personal's 5-hour resets                                in 1h 48m",
				"17:10  work back from its cap                                  in 2h 28m",
				"18:50  side's 5-hour resets                                    in 4h 08m",
			},
		},
		{
			name: "one of them, the moment's others left out", width: 120, height: 37, coming: fiveHourComing[1:2],
			want: []string{"COMING UP", "17:10  work back from its cap                                  in 2h 28m"},
		},
		{
			name: "on a phone, the frame's width", width: 52, height: 33, coming: fiveHourComing,
			want: []string{
				"COMING UP",
				"16:30  personal's 5-hour resets            in 1h 48m",
				"17:10  work back from its cap              in 2h 28m",
				"18:50  side's 5-hour resets                in 4h 08m",
			},
		},
		{name: "nothing coming up, its heading alone", width: 120, height: 37, want: []string{"COMING UP"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runwayComingDrawn(t, tt.width, tt.height, tt.coming); !slices.Equal(got, tt.want) {
				t.Errorf("COMING UP reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

// Runway's COMING UP names the 5-hour window running out, as Runway's own
// section words it, where the Overview's leaves the window a request starts
// unnamed.
func TestRunwaysComingUpNamesTheFiveHourWindowRunningOut(t *testing.T) {
	out := status.Upcoming{At: routedClock(1, 16, 5), Account: "personal", Kind: status.UpcomingRunsOut, Window: "5h"}
	capped := out
	capped.Cap = true
	tests := []struct {
		name string
		u    status.Upcoming
		want string
	}{
		{name: "running out at its pace", u: out, want: "~16:05  personal's 5-hour runs out at its pace"},
		{name: "reaching its cap", u: capped, want: "~16:05  personal's 5-hour reaches its cap"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runwayComingDrawn(t, 160, 37, []status.Upcoming{tt.u})[1]; !strings.HasPrefix(got, tt.want+" ") {
				t.Errorf("COMING UP's line reads %q, want %q", got, tt.want)
			}
		})
	}
	m := framesDoc().At(routedAt, claudeLike)
	overview := comingTeller{moment: m, names: overviewNames(m), window: func(string) string { return "5-hour" }}
	if got := overview.happenings([]status.Upcoming{out})[0].what; got != " runs out at its pace" {
		t.Errorf("the Overview's COMING UP says personal%q, want it unnamed", got)
	}
}

// Where its lines don't all fit the rows it's given, COMING UP shows the
// lines it can, each whole, and its last place says how many more, in
// text.subtle, as what's out of view of the lanes is: never folding whole.
func TestRunwaysComingUpShowsTheLinesItCanAndHowManyMore(t *testing.T) {
	five := append(slices.Clone(fiveHourComing),
		status.Upcoming{At: routedClock(1, 19, 30), Account: "work", Kind: status.UpcomingReset, Window: "5h"},
		status.Upcoming{At: routedClock(1, 21, 0), Account: "personal", Kind: status.UpcomingReset, Window: "5h"},
	)
	tests := []struct {
		name   string
		coming []status.Upcoming
		lines  int
		want   []string
	}{
		{
			name: "every line where they fit", coming: fiveHourComing, lines: 3,
			want: []string{
				"COMING UP",
				"16:30  personal's 5-hour resets                                in 1h 48m",
				"17:10  work back from its cap                                  in 2h 28m",
				"18:50  side's 5-hour resets                                    in 4h 08m",
			},
		},
		{
			name: "a line and two more", coming: fiveHourComing, lines: 2,
			want: []string{"COMING UP", "16:30  personal's 5-hour resets                                in 1h 48m", "▼ 2 more"},
		},
		{
			name: "three lines of five and two more", coming: five, lines: 4,
			want: []string{
				"COMING UP",
				"16:30  personal's 5-hour resets                                in 1h 48m",
				"17:10  work back from its cap                                  in 2h 28m",
				"18:50  side's 5-hour resets                                    in 4h 08m",
				"▼ 2 more",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := framesDoc()
			r := poolShown(doc, views.Pool{Of: 3, Key: "5h"})
			r.Coming = tt.coming
			f := Frame{Width: 120, Height: 37, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: r}
			f.WindowName = func(key string) string { return map[string]string{"5h": "5-hour", "7d": "week"}[key] }
			c := newCanvas(f.Width, f.Height)
			fit := runwayFit{fits: true, coming: 10, lines: tt.lines}
			f.runwayComing(c, runwayLaid{x: framesLanesFrom, fit: fit, m: f.Moment(doc, routedAt)})
			if got := textOf(c.rows(Look{}))[10 : 11+tt.lines]; !slices.Equal(got, tt.want) {
				t.Errorf("COMING UP reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
			if got := textOf(c.rows(Look{}))[11+tt.lines]; got != "" {
				t.Errorf("the row after COMING UP's last reads %q, want nothing", got)
			}
			if !strings.HasPrefix(tt.want[tt.lines], "▼") {
				return
			}
			for x := range len([]rune("▼ 2 more")) {
				if got := c.at(x, 10+tt.lines).ink; got != dimInk {
					t.Errorf("cell %d of how many more is in %+v, want text.subtle", x, got)
				}
			}
		})
	}
}

// COMING UP's lines take two columns where both fit at the lines' natural
// width, 72, comingApart apart, sharing the frame's width, the odd cell the
// second's; else one, at its natural, or the frame's width where narrower.
func TestRunwaysComingUpTakesTwoColumnsWhereBothFitAtTheirNatural(t *testing.T) {
	tests := []struct {
		width int
		want  []flex.Span
	}{
		{width: 52, want: []flex.Span{{At: 0, Size: 52}}},
		{width: 120, want: []flex.Span{{At: 0, Size: 72}}},
		{width: 147, want: []flex.Span{{At: 0, Size: 72}}},
		{width: 148, want: []flex.Span{{At: 0, Size: 72}, {At: 76, Size: 72}}},
		{width: 160, want: []flex.Span{{At: 0, Size: 78}, {At: 82, Size: 78}}},
		{width: 173, want: []flex.Span{{At: 0, Size: 84}, {At: 88, Size: 85}}},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.width), func(t *testing.T) {
			if got := (Frame{Width: tt.width}).comingColumns(); !slices.Equal(got, tt.want) {
				t.Errorf("at %d columns, COMING UP's columns are %+v, want %+v", tt.width, got, tt.want)
			}
		})
	}
}

// In two columns, COMING UP's lines run in time order down the first, then
// across into the second, each whole, its countdown right-aligned at its
// column's end; where they don't all fit, the second's last place says how
// many more.
func TestRunwaysComingUpRunsDownItsFirstColumnThenAcross(t *testing.T) {
	coming := append(slices.Clone(fiveHourComing),
		status.Upcoming{At: routedClock(1, 19, 30), Account: "work", Kind: status.UpcomingReset, Window: "5h"},
		status.Upcoming{At: routedClock(1, 21, 0), Account: "personal", Kind: status.UpcomingReset, Window: "5h"},
	)
	// placed is a line of COMING UP in a column of 78 cells, 160's, its
	// countdown at the column's end.
	placed := func(words, until string) string {
		return words + strings.Repeat(" ", 78-len([]rune(words))-len(until)) + until
	}
	resets, back, side := placed("16:30  personal's 5-hour resets", "in 1h 48m"), placed("17:10  work back from its cap", "in 2h 28m"), placed("18:50  side's 5-hour resets", "in 4h 08m")
	apart := strings.Repeat(" ", comingApart)
	tests := []struct {
		name  string
		lines int
		want  []string
	}{
		{
			name: "all five in three rows", lines: 3,
			want: []string{
				"COMING UP",
				resets + apart + placed("19:30  work's 5-hour resets", "in 4h 48m"),
				back + apart + placed("21:00  personal's 5-hour resets", "in 6h 18m"),
				side,
			},
		},
		{
			name: "three of five in two rows, and two more", lines: 2,
			want: []string{"COMING UP", resets + apart + side, back + apart + "▼ 2 more"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := framesDoc()
			r := poolShown(doc, views.Pool{Of: 3, Key: "5h"})
			r.Coming = coming
			f := Frame{Width: 160, Height: 37, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: r}
			f.WindowName = func(key string) string { return map[string]string{"5h": "5-hour", "7d": "week"}[key] }
			c := newCanvas(f.Width, f.Height)
			f.runwayComing(c, runwayLaid{x: framesLanesFrom, fit: runwayFit{fits: true, coming: 10, lines: tt.lines}, m: f.Moment(doc, routedAt)})
			if got := textOf(c.rows(Look{}))[10 : 11+tt.lines]; !slices.Equal(got, tt.want) {
				t.Errorf("COMING UP reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

// Collapsing the account column leaves COMING UP as it is, every line of it
// or cut, in its words and its inks: each line names its account alone,
// never by its lane's number.
func TestCollapsingRunwaysAccountColumnLeavesComingUpAsItIs(t *testing.T) {
	tests := []struct {
		name   string
		height int
		want   []string
	}{
		{
			name: "open", height: 37,
			want: []string{
				"16:30  personal's 5-hour resets                                in 1h 48m",
				"17:10  work back from its cap                                  in 2h 28m",
				"18:50  side's 5-hour resets                                    in 4h 08m",
			},
		},
		{name: "cut", height: 24, want: []string{"16:30  personal's 5-hour resets                                in 1h 48m", "▼ 2 more"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := framesDoc()
			coming := func(collapsed bool) []string {
				r := poolShown(doc, views.Pool{Of: 3, Key: "5h"})
				r.Collapsed, r.Coming = collapsed, fiveHourComing
				r.Timeline = r.TimelineAt(routedAt, time.Monday, 120)
				f := Frame{Width: 120, Height: tt.height, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: r}
				f.WindowName = func(key string) string { return map[string]string{"5h": "5-hour", "7d": "week"}[key] }
				first := f.runwayFitted(f.top()).coming + 1
				return runwayDrawn(f, doc).rows(f.Look)[first : first+len(tt.want)]
			}
			expanded, collapsed := coming(false), coming(true)
			if got := textOf(expanded); !slices.Equal(got, tt.want) {
				t.Errorf("COMING UP reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
			if !slices.Equal(collapsed, expanded) {
				t.Errorf("collapsed, COMING UP is drawn\n%q\nwant it as expanded\n%q", collapsed, expanded)
			}
		})
	}
}

// A run-out's line is all in accent.attention, its time and countdown
// included, but its account, in bold text.primary, as every line names it.
func TestARunOutsLineOnRunwayIsInAccentAttention(t *testing.T) {
	out := status.Upcoming{At: routedClock(1, 16, 5), Account: "personal", Kind: status.UpcomingRunsOut, Window: "5h"}
	doc := framesDoc()
	r := poolShown(doc, views.Pool{Of: 3, Key: "5h"})
	r.Coming = []status.Upcoming{out}
	f := Frame{Width: 120, Height: 37, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: r}
	c := runwayDrawn(f, doc)
	y := f.runwayFitted(f.top()).coming + 1
	for x, want := range map[int]ink{0: warningInk, 8: titleInk, 17: warningInk, 71: warningInk} {
		if got := c.at(x, y).ink; got != want {
			t.Errorf("column %d of the run-out's line is in %+v, want %+v", x, got, want)
		}
	}
}
