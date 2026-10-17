package dashboard

import (
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

func TestRunwaysLanesRunFromTheirLabelsToTheEdge(t *testing.T) {
	tests := []struct {
		width, want int
	}{
		{width: 160, want: 136},
		{width: 52, want: 38},
		{width: 10, want: 0},
	}
	for _, tt := range tests {
		if got := RunwayColumns(tt.width); got != tt.want {
			t.Errorf("RunwayColumns(%d) = %d, want %d", tt.width, got, tt.want)
		}
	}
}

func TestRunwaysTimelineIsTheLiveViewsOrTheDaySteppedBacksAcrossItsColumns(t *testing.T) {
	tests := []struct {
		name  string
		shown RunwayShown
		want  views.Timeline
	}{
		{name: "the 5-hour, live", shown: RunwayShown{Window: views.RunwayFiveHour}, want: views.LiveTimeline(views.RunwayFiveHour, runwayAt(), 136)},
		{name: "the week, live", shown: RunwayShown{Window: views.RunwayWeek}, want: views.LiveTimeline(views.RunwayWeek, runwayAt(), 136)},
		{name: "a day stepped back to", shown: RunwayShown{Window: views.RunwayFiveHour, Back: civilDay(9, 30)}, want: views.Stepped(views.RunwayFiveHour, civilDay(9, 30), runwayAt(), time.Monday, 136)},
		{name: "a week stepped back to", shown: RunwayShown{Window: views.RunwayWeek, Back: civilDay(9, 21)}, want: views.Stepped(views.RunwayWeek, civilDay(9, 21), runwayAt(), time.Monday, 136)},
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
	tl := views.Stepped(rm.Window, rm.Day, rm.Now, rm.WeekStarts, RunwayColumns(width))
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
