package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

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
	shape := runwayShape{legendGap: true, legend: 1}
	if f.phone() {
		shape = runwayShape{roomRow: true, legend: 2}
	}
	fit := f.runwayFittedAs(f.top(), shape)
	c := newCanvas(width, height)
	f.runwayComing(c, runwayLaid{x: runwayLanesFrom, fit: fit, m: f.Moment(doc, routedAt)})
	return textOf(c.rows(Look{}))[fit.coming:fit.end]
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
// rows: its countdowns ending at column 71 from 100 columns, and at the
// frame's edge on a phone.
func TestRunwaysComingUpListsWhatRunwaysListHolds(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
		coming []status.Upcoming
		want   []string
	}{
		{
			name: "the 5-hour frame's three", width: 160, height: 37, coming: fiveHourComing,
			want: []string{
				"COMING UP",
				"16:30  personal's 5-hour resets                                in 1h 48m",
				"17:10  work back from its cap                                  in 2h 28m",
				"18:50  side's 5-hour resets                                    in 4h 08m",
			},
		},
		{
			name: "one of them, the moment's others left out", width: 160, height: 37, coming: fiveHourComing[1:2],
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
		{name: "nothing coming up, its heading alone", width: 160, height: 37, want: []string{"COMING UP"}},
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

// Folded, at (c) and after, COMING UP keeps its heading alone, saying what's
// soonest three cells on, its time unpadded, and two cells on an ellipsis
// where more follow, as the Overview folds a section; nothing after it where
// nothing's coming up.
func TestRunwaysComingUpFoldsToItsHeading(t *testing.T) {
	tests := []struct {
		name   string
		coming []status.Upcoming
		want   string
	}{
		{name: "three coming up", coming: fiveHourComing, want: "COMING UP   16:30  personal's 5-hour resets  …"},
		{name: "one", coming: fiveHourComing[1:2], want: "COMING UP   17:10  work back from its cap"},
		{name: "none", want: "COMING UP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runwayComingDrawn(t, 160, 22, tt.coming)
			if !slices.Equal(got, []string{tt.want}) {
				t.Errorf("folded, COMING UP reads %q, want %q alone", got, tt.want)
			}
		})
	}
}

// With the account column collapsed, COMING UP puts each account's number
// before its name, in the number's ink, as the lanes' labels draw it, so a
// lane's number finds its name; folded too. Expanded, it names them alone.
func TestCollapsedRunwaysComingUpNumbersItsAccounts(t *testing.T) {
	tests := []struct {
		name      string
		collapsed bool
		height    int
		want      []string
	}{
		{
			name: "collapsed", collapsed: true, height: 37,
			want: []string{
				"16:30  2 personal's 5-hour resets                              in 1h 48m",
				"17:10  1 work back from its cap                                in 2h 28m",
				"18:50  3 side's 5-hour resets                                  in 4h 08m",
			},
		},
		{name: "collapsed, folded", collapsed: true, height: 21, want: []string{"COMING UP   16:30  2 personal's 5-hour resets  …"}},
		{
			name: "expanded", height: 37,
			want: []string{
				"16:30  personal's 5-hour resets                                in 1h 48m",
				"17:10  work back from its cap                                  in 2h 28m",
				"18:50  side's 5-hour resets                                    in 4h 08m",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := framesDoc()
			r := poolShown(doc, views.Pool{Of: 3, Key: "5h"})
			r.Collapsed, r.Coming = tt.collapsed, fiveHourComing
			r.Timeline = r.TimelineAt(routedAt, time.Monday, 160)
			f := Frame{Width: 160, Height: tt.height, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: r}
			f.WindowName = func(key string) string { return map[string]string{"5h": "5-hour", "7d": "week"}[key] }
			c := runwayDrawn(f, doc)
			fit := f.runwayFitted(f.top())
			first := fit.coming + 1
			if fit.folded {
				first = fit.coming
			}
			got := textOf(c.rows(Look{}))[first : first+len(tt.want)]
			if !slices.Equal(got, tt.want) {
				t.Errorf("COMING UP reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
			if !tt.collapsed {
				return
			}
			number := strings.Index(tt.want[0], "2 personal")
			if k := c.at(number, first).ink; k != dimInk {
				t.Errorf("personal's number is in %+v, want text.subtle, as its lane's label has it", k)
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
	f := Frame{Width: 160, Height: 37, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: r}
	c := runwayDrawn(f, doc)
	y := f.runwayFitted(f.top()).coming + 1
	for x, want := range map[int]ink{0: warningInk, 8: titleInk, 17: warningInk, 71: warningInk} {
		if got := c.at(x, y).ink; got != want {
			t.Errorf("column %d of the run-out's line is in %+v, want %+v", x, got, want)
		}
	}
}
