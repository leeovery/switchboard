package dashboard

import (
	"cmp"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// runwayAt is Thursday 1 October 2026, 14:42, in the machine's own zone, as
// the watch tells the time, with weeks starting on Monday the 28th.
func runwayAt() time.Time { return time.Date(2026, 10, 1, 14, 42, 0, 0, time.Local) }

// civilDay is the day given as dayfile.Civil gives one.
func civilDay(month time.Month, day int) time.Time {
	return time.Date(2026, month, day, 0, 0, 0, 0, time.UTC)
}

func TestThePreferencesKeepRunwaysWindowAndNeverAStepBack(t *testing.T) {
	tests := []struct {
		name string
		kept theme.RunwayPrefs
		want views.RunwayWindow
	}{
		{name: "none kept: the week, which Runway opens on", want: views.RunwayWeek},
		{name: "the 5-hour", kept: theme.RunwayPrefs{Window: "5-hour"}, want: views.RunwayFiveHour},
		{name: "the week as earlier builds kept it", kept: theme.RunwayPrefs{Window: "week"}, want: views.RunwayWeek},
		{name: "a window a later build names", kept: theme.RunwayPrefs{Window: "month"}, want: views.RunwayWeek},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RunwayKept(tt.kept); got.Window != tt.want || !got.Back.IsZero() {
				t.Errorf("RunwayKept(%+v) = %+v, want the live view of %q", tt.kept, got, tt.want)
			}
		})
	}

	back := RunwayShown{Window: views.RunwayFiveHour, Back: civilDay(9, 30)}
	if got := back.Kept(); got != (theme.RunwayPrefs{Window: "5-hour"}) {
		t.Errorf("a day stepped back keeps %+v, want its window alone", got)
	}
	if got := RunwayKept(back.Kept()); got.Window != views.RunwayFiveHour || !got.Back.IsZero() {
		t.Errorf("a day stepped back, kept and read again, is %+v, want the 5-hour's live view", got)
	}
}

// Runway's account column is expanded at first, as with nothing kept;
// collapsing it, as a does, is kept beside the window, and read back so, and
// collapsing it again expands it.
func TestThePreferencesKeepRunwaysAccountColumnCollapsedOrNot(t *testing.T) {
	if got := RunwayKept(theme.RunwayPrefs{}); got.Collapsed {
		t.Error("with nothing kept, Runway's account column is collapsed; want it expanded")
	}
	collapsed := RunwayShown{Window: views.RunwayFiveHour}.Collapsing()
	if got := collapsed.Kept(); got != (theme.RunwayPrefs{Window: "5-hour", Collapsed: true}) {
		t.Errorf("collapsed, Runway keeps %+v, want its window and its column collapsed", got)
	}
	if got := RunwayKept(collapsed.Kept()); !got.Collapsed || got.Window != views.RunwayFiveHour {
		t.Errorf("collapsed, kept and read again, Runway is %+v, want the 5-hour's, collapsed", got)
	}
	if got := RunwayKept(theme.RunwayPrefs{Window: "month", Collapsed: true}); !got.Collapsed || got.Window != views.RunwayWeek {
		t.Errorf("a window a later build names, collapsed, reads as %+v, want the week's, collapsed", got)
	}
	if collapsed.Collapsing().Collapsed {
		t.Error("collapsing a collapsed column leaves it collapsed; want it expanded")
	}
}

// Runway opens on the week: its zero value, as a watch given nothing kept
// shows it.
func TestRunwayOpensOnTheWeek(t *testing.T) {
	if got := (RunwayShown{}).Window; got != views.RunwayWeek {
		t.Errorf("Runway opens on %q, want the week", got)
	}
	if got := RunwayKept(theme.RunwayPrefs{}); got.Window != views.RunwayWeek {
		t.Errorf("with nothing kept, Runway opens on %q, want the week", got.Window)
	}
}

func TestLessAndMoreShowTheOtherWindowsLiveViewRound(t *testing.T) {
	tests := []struct {
		name    string
		from    RunwayShown
		by      int
		want    views.RunwayWindow
		changed bool
	}{
		{name: "> from the week, which it opens on", from: RunwayShown{}, by: 1, want: views.RunwayFiveHour, changed: true},
		{name: "< from the week, round", from: RunwayShown{}, by: -1, want: views.RunwayFiveHour, changed: true},
		{name: "> from the 5-hour, round", from: RunwayShown{Window: views.RunwayFiveHour}, by: 1, want: views.RunwayWeek, changed: true},
		{name: "< from the 5-hour", from: RunwayShown{Window: views.RunwayFiveHour}, by: -1, want: views.RunwayWeek, changed: true},
		{name: "from a day stepped back", from: RunwayShown{Window: views.RunwayFiveHour, Back: civilDay(9, 30)}, by: 1, want: views.RunwayWeek, changed: true},
		{name: "round to a day stepped back's own window", from: RunwayShown{Window: views.RunwayFiveHour, Back: civilDay(9, 30)}, by: 2, want: views.RunwayFiveHour, changed: true},
		{name: "round to the live view shown", from: RunwayShown{}, by: 2, want: views.RunwayWeek},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := tt.from.Windowed(tt.by)
			if got.Window != tt.want || !got.Back.IsZero() || changed != tt.changed {
				t.Errorf("Windowed(%d) = %+v, %t, want the live view of %q, %t", tt.by, got, changed, tt.want, tt.changed)
			}
		})
	}
}

func TestLeftStepsBackADayOrAWeekAtATimeAndRightOnToTheLiveView(t *testing.T) {
	tests := []struct {
		name   string
		window views.RunwayWindow
		want   []time.Time
	}{
		{name: "the 5-hour, from today a day at a time", window: views.RunwayFiveHour, want: []time.Time{civilDay(10, 1), civilDay(9, 30), civilDay(9, 29)}},
		{name: "the week, from this week a week at a time", window: views.RunwayWeek, want: []time.Time{civilDay(9, 28), civilDay(9, 21), civilDay(9, 14)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RunwayShown{Window: tt.window}
			for _, want := range tt.want {
				r = steppedOnce(t, r, -1)
				if !r.Back.Equal(want) {
					t.Fatalf("← steps back to %s, want %s", r.Back, want)
				}
			}
			for _, want := range []time.Time{tt.want[1], tt.want[0], {}} {
				r = steppedOnce(t, r, 1)
				if !r.Back.Equal(want) {
					t.Fatalf("→ steps on to %s, want %s", r.Back, want)
				}
			}
			if got, changed := r.Stepped(1, runwayAt(), time.Monday); changed || !got.Back.IsZero() {
				t.Errorf("→ from the live view gives %+v, %t, want it to stay", got, changed)
			}
			if r.Window != tt.window {
				t.Errorf("stepping shows the %q window, want %q still", r.Window, tt.window)
			}
		})
	}
}

// steppedOnce is r stepped by by at runwayAt(), failing the test where it
// stays.
func steppedOnce(t *testing.T, r RunwayShown, by int) RunwayShown {
	t.Helper()
	next, changed := r.Stepped(by, runwayAt(), time.Monday)
	if !changed {
		t.Fatalf("Stepped(%d) from %+v stays", by, r)
	}
	return next
}

// A day stepped back to stays itself as the clock passes midnight: today
// becomes yesterday, so → steps on to the new today before the live view.
func TestADaySteppedBackToStaysItselfAsTheClockPassesMidnight(t *testing.T) {
	before := time.Date(2026, 10, 1, 23, 50, 0, 0, time.Local)
	r, _ := RunwayShown{Window: views.RunwayFiveHour}.Stepped(-1, before, time.Monday)

	after := before.Add(20 * time.Minute)
	on, changed := r.Stepped(1, after, time.Monday)
	if !changed || !on.Back.Equal(civilDay(10, 2)) {
		t.Errorf("→ from Thursday, stepped back to before midnight, gives %+v at 00:10, want Friday, the new today", on)
	}
	if live, _ := on.Stepped(1, after, time.Monday); !live.Back.IsZero() {
		t.Errorf("→ from the new today gives %+v, want the live view", live)
	}
}

func TestRunwaysHeadingNamesItsWindowAndTheDaysSteppedBackTo(t *testing.T) {
	tests := []struct {
		name       string
		shown      RunwayShown
		wantPeriod string
		wantState  line
	}{
		{name: "the 5-hour, live", shown: RunwayShown{Window: views.RunwayFiveHour}, wantPeriod: "5-hour"},
		{name: "the week, live", shown: RunwayShown{Window: views.RunwayWeek}, wantPeriod: "week"},
		{name: "a day stepped back to", shown: RunwayShown{Window: views.RunwayFiveHour, Back: civilDay(9, 30)}, wantPeriod: "5-hour", wantState: line{{"Wed 30 Sep", faintInk}}},
		{name: "a week stepped back to", shown: RunwayShown{Window: views.RunwayWeek, Back: civilDay(9, 28)}, wantPeriod: "week", wantState: line{{"Mon 28 Sep – Sun 4 Oct", faintInk}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.shown
			r.Timeline = r.TimelineAt(runwayAt(), time.Monday, 160)
			h := r.heading()
			if h.name != "RUNWAY" || h.period != tt.wantPeriod || !reflect.DeepEqual(h.state, tt.wantState) {
				t.Errorf("heading() = %q, %q, %v, want RUNWAY, %q, %v", h.name, h.period, h.state, tt.wantPeriod, tt.wantState)
			}
		})
	}
}

// runwayLanesFrom is the column the tests draw Runway's lanes from: after
// the label column at its most.
const runwayLanesFrom = mostRunwayLabels

// Runway's label column takes the widest of its labels whole, and its gap,
// at every width, and the lanes take the rest to the edge. Expanded, the
// widest of the names, the marks each carries and no more, then nameGap, or
// of the uses under them, as wide as "5-hour 100%" in either window, from
// 8 cells to 24, a name past that cut; collapsed, the widest number and
// marks or a use of 100%, then a cell, whatever the width.
func TestRunwaysLabelColumnTakesItsWidestLabelAndTheLanesTheRest(t *testing.T) {
	work := accountName{number: "1", label: "work"}
	personal := accountName{number: "2", label: "personal", primary: true}
	side := accountName{number: "3", label: "side", best: true}
	frames := []accountName{work, personal, side}
	long := []accountName{{number: "1", label: "work-team-plan"}, {number: "2", label: "personal-research", primary: true}}
	both := []accountName{work, {number: "2", label: "personal", primary: true, best: true}, {number: "3", label: "side"}}
	ten := []accountName{{number: " 1", label: "work"}, {number: "10", label: "spare", primary: true, best: true}}
	tests := []struct {
		name      string
		names     []accountName
		window    views.RunwayWindow
		collapsed bool
		width     int
		from      int
	}{
		{name: "the frames' on a phone, personal whole", names: frames, width: 73, from: 14},
		{name: "the frames' on an iPad", names: frames, width: 108, from: 14},
		{name: "the frames' on a desktop", names: frames, width: 173, from: 14},
		{name: "the 5-hour's, alike", names: frames, window: views.RunwayFiveHour, width: 108, from: 14},
		{name: "a short name, the widest use", names: []accountName{work}, width: 108, from: 14},
		{name: "a short name in the 5-hour, alike", names: []accountName{work}, window: views.RunwayFiveHour, width: 108, from: 14},
		{name: "long names, whole", names: long, width: 108, from: 23},
		{name: "a name past the most", names: append(long, accountName{number: "3", label: "side-hustle-consulting", best: true}), width: 108, from: 24},
		{name: "none labelled yet, the widest use", width: 108, from: 14},
		{name: "narrower than its labels, the column first", names: frames, width: 10, from: 10},
		{name: "narrower than the least", names: frames, width: 6, from: 8},
		{name: "collapsed, the frames' on an iPad", names: frames, collapsed: true, width: 108, from: 5},
		{name: "collapsed, on a phone, alike", names: frames, collapsed: true, width: 52, from: 5},
		{name: "collapsed, both marks on one", names: both, collapsed: true, width: 108, from: 6},
		{name: "collapsed, numbered from ten", names: ten, collapsed: true, width: 108, from: 7},
		{name: "collapsed, none labelled yet, a use of 100%", collapsed: true, width: 108, from: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RunwayShown{Window: cmp.Or(tt.window, views.RunwayWeek), Collapsed: tt.collapsed, labels: runwayLabelsOf(tt.names)}
			if got := r.lanesFrom(tt.width); got != tt.from {
				t.Errorf("the lanes start at column %d, want %d", got, tt.from)
			}
			if got, want := r.Columns(tt.width), max(tt.width-tt.from, 0); got != want {
				t.Errorf("the lanes take %d columns, want %d, to the edge", got, want)
			}
		})
	}
}

func TestRunwaysTimelineIsTheLiveViewsOrTheDaySteppedBacksAcrossItsColumns(t *testing.T) {
	tests := []struct {
		name  string
		shown RunwayShown
		want  views.Timeline
	}{
		{name: "the 5-hour, live", shown: RunwayShown{Window: views.RunwayFiveHour}, want: views.LiveTimeline(views.RunwayFiveHour, runwayAt(), 146)},
		{name: "the week, live", shown: RunwayShown{Window: views.RunwayWeek}, want: views.LiveTimeline(views.RunwayWeek, runwayAt(), 146)},
		{name: "a day stepped back to", shown: RunwayShown{Window: views.RunwayFiveHour, Back: civilDay(9, 30)}, want: views.Stepped(views.RunwayFiveHour, civilDay(9, 30), runwayAt(), time.Monday, 146)},
		{name: "a week stepped back to", shown: RunwayShown{Window: views.RunwayWeek, Back: civilDay(9, 21)}, want: views.Stepped(views.RunwayWeek, civilDay(9, 21), runwayAt(), time.Monday, 146)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.shown.TimelineAt(runwayAt(), time.Monday, 160); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TimelineAt(160) = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// runwayFramed is Runway of the frames' moment views' RunwayMoments name as
// given, as the frames draw its world: new sessions going to side, ▲ on it,
// and its days' summaries filing no reset of a window that read nothing, as
// the ledger files none; its timeline, lanes and COMING UP built at the
// look, as the watch builds them; in nord, 160 by 37. It gives the frame and
// the document it's drawn of, at the moment's now.
func runwayFramed(t *testing.T, name string) (Frame, status.Document, time.Time) {
	t.Helper()
	return runwayFramedAt(t, name, 160, 37)
}

// runwayFramedAt is Runway of the moment named as runwayFramed has it, on a
// terminal width columns wide and height rows tall.
func runwayFramedAt(t *testing.T, name string, width, height int) (Frame, status.Document, time.Time) {
	t.Helper()
	moments := viewstest.RunwayMoments()
	i := slices.IndexFunc(moments, func(rm viewstest.RunwayMoment) bool { return rm.Name == name })
	if i < 0 {
		t.Fatalf("no Runway moment is named %q", name)
	}
	rm := moments[i]
	doc := rm.Doc
	doc.Best = "side"
	m := doc.At(rm.Now, claude.Policy())
	tl := views.Stepped(rm.Window, rm.Day, rm.Now, rm.WeekStarts, RunwayShown{}.Columns(width))
	lanes := views.Lanes{}.Look(views.LanesFrom{Moment: m, Readings: rm.Readings, Days: filed(rm.Days), Kept: rm.Kept, Timeline: tl, Window: rm.Window})
	f := Frame{
		Width: width, Height: height, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claude.Policy(),
		Runway: RunwayShown{Window: rm.Window, Back: rm.Day, Timeline: tl, Lanes: &lanes, Coming: views.RunwayComing(m, rm.Window)},
	}
	return f, doc, rm.Now
}

// filed are the days given as the ledger files them: no reset of a window
// whose run read nothing, as nothing was open to reset.
func filed(days []ledger.Summary) []ledger.Summary {
	days = slices.Clone(days)
	for i := range days {
		days[i].Accounts = slices.Clone(days[i].Accounts)
		for j, a := range days[i].Accounts {
			days[i].Accounts[j].Resets = slices.DeleteFunc(slices.Clone(a.Resets), func(r ledger.Reset) bool { return r.Before == 0 })
		}
	}
	return days
}

// Each account's use of the view's window under its name is a number a look
// reads, which brightens as it changes, as is the pool's room under Σ pool;
// so is the highest a day gone read, which never changes, and it has no
// room.
func TestTheUseUnderEachNameIsANumberALookReads(t *testing.T) {
	tests := []struct {
		moment string
		want   map[Number]string
	}{
		{moment: "the auto world's 5-hour view", want: map[Number]string{
			{Kind: NumberUsed, Account: "work", Of: "5h"}: "95%", {Kind: NumberUsed, Account: "personal", Of: "5h"}: "39%", {Kind: NumberUsed, Account: "side", Of: "5h"}: "13%",
			{Kind: NumberRoom, Of: "5h"}: "1.5",
		}},
		{moment: "the auto world's week view", want: map[Number]string{
			{Kind: NumberUsed, Account: "work", Of: "7d"}: "34%", {Kind: NumberUsed, Account: "personal", Of: "7d"}: "66%", {Kind: NumberUsed, Account: "side", Of: "7d"}: "78%",
			{Kind: NumberRoom, Of: "7d"}: "1.2",
		}},
		{moment: "the auto world, Wednesday stepped back to", want: map[Number]string{
			{Kind: NumberUsed, Account: "work", Of: "5h"}: "100%", {Kind: NumberUsed, Account: "personal", Of: "5h"}: "100%", {Kind: NumberUsed, Account: "side", Of: "5h"}: "62%",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.moment, func(t *testing.T) {
			f, doc, now := runwayFramed(t, tt.moment)
			if got := f.Numbers(doc, now); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Runway's numbers are %v, want %v", got, tt.want)
			}
		})
	}
}
