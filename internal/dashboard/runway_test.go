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

// The preferences keep whether Runway's account column is collapsed, and
// read back so, and collapsing it again expands it; never a step back, so a
// week stepped back to, kept and read again, is the live view.
func TestThePreferencesKeepRunwaysAccountColumnAndNeverAStepBack(t *testing.T) {
	if got := RunwayKept(theme.RunwayPrefs{}); got.Collapsed || !got.Back.IsZero() {
		t.Errorf("with nothing kept, Runway is %+v; want the live view, expanded", got)
	}
	collapsed := RunwayShown{Back: civilDay(9, 21)}.Collapsing()
	if got := collapsed.Kept(); got != (theme.RunwayPrefs{Collapsed: true}) {
		t.Errorf("collapsed and stepped back, Runway keeps %+v, want its column collapsed alone", got)
	}
	if got := RunwayKept(collapsed.Kept()); !got.Collapsed || !got.Back.IsZero() {
		t.Errorf("collapsed and stepped back, kept and read again, Runway is %+v, want the live view, collapsed", got)
	}
	if collapsed.Collapsing().Collapsed {
		t.Error("collapsing a collapsed column leaves it collapsed; want it expanded")
	}
}

// Runway's heading is its name, and stepped back, the days it shows, faint:
// nothing at its right, no window between < and > to click, its rule running
// to the frame's last column, as Sessions' does.
func TestRunwaysHeadingNamesTheDaysSteppedBackToAndNoWindow(t *testing.T) {
	tests := []struct {
		name  string
		shown RunwayShown
		state line
	}{
		{name: "live", shown: RunwayShown{}},
		{name: "this week stepped back to", shown: RunwayShown{Back: civilDay(9, 28)}, state: line{{"Mon 28 Sep – Sun 4 Oct", faintInk}}},
		{name: "a week gone", shown: RunwayShown{Back: civilDay(9, 21)}, state: line{{"Mon 21 Sep – Sun 27 Sep", faintInk}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.shown
			r.Timeline = r.TimelineAt(runwayAt(), time.Monday, 160)
			h := r.heading()
			if h.name != "RUNWAY" || h.period != "" || h.right != nil || !reflect.DeepEqual(h.state, tt.state) {
				t.Errorf("heading() = %q, %q, %v, %v, want RUNWAY, no window, nothing at its right, %v", h.name, h.period, h.right, h.state, tt.state)
			}
			c, pieces := laidOn(h, 160, false)
			if slices.ContainsFunc(pieces, func(p piece) bool { return p.click.On == OnStep }) {
				t.Errorf("the heading has a step to click, want none")
			}
			if got := c.at(159, 0).glyph; got != "─" {
				t.Errorf("the heading's last column holds %q, want its rule", got)
			}
		})
	}
}

func TestLeftStepsBackAWeekAtATimeAndRightOnToTheLiveView(t *testing.T) {
	want := []time.Time{civilDay(9, 28), civilDay(9, 21), civilDay(9, 14)}
	var r RunwayShown
	for _, w := range want {
		r = steppedOnce(t, r, -1)
		if !r.Back.Equal(w) {
			t.Fatalf("← steps back to %s, want %s", r.Back, w)
		}
	}
	for _, w := range []time.Time{want[1], want[0], {}} {
		r = steppedOnce(t, r, 1)
		if !r.Back.Equal(w) {
			t.Fatalf("→ steps on to %s, want %s", r.Back, w)
		}
	}
	if got, changed := r.Stepped(1, runwayAt(), time.Monday); changed || !got.Back.IsZero() {
		t.Errorf("→ from the live view gives %+v, %t, want it to stay", got, changed)
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

// A week stepped back to stays itself as the clock passes into the next:
// this week becomes last week, so → steps on to the new this week before the
// live view.
func TestAWeekSteppedBackToStaysItselfAsTheClockPassesIntoTheNext(t *testing.T) {
	before := time.Date(2026, 10, 4, 23, 50, 0, 0, time.Local)
	r, _ := RunwayShown{}.Stepped(-1, before, time.Monday)

	after := before.Add(20 * time.Minute)
	on, changed := r.Stepped(1, after, time.Monday)
	if !changed || !on.Back.Equal(civilDay(10, 5)) {
		t.Errorf("→ from the week of the 28th, stepped back to before Monday, gives %+v at 00:10, want the week of the 5th, the new this week", on)
	}
	if live, _ := on.Stepped(1, after, time.Monday); !live.Back.IsZero() {
		t.Errorf("→ from the new this week gives %+v, want the live view", live)
	}
}

// framesLanesFrom is the column the tests draw Runway's lanes from, as the
// screen lays it for the frames' accounts: after "personal ◆", ▲'s cells
// kept, and nameGap.
const framesLanesFrom = 14

// Runway's label column takes the widest of its labels whole, and its gap,
// at every width, and the lanes take the rest to the edge. Expanded, the
// widest of the names, ▲'s cells kept on every account, then nameGap, or of
// the uses under them, as wide as "week 100%", each label whole up to 24
// cells, a label past that cut, its marks and gap beside it; narrower only
// where the frame is, to a label's 8. Collapsed, there's no label column:
// the lanes start at column 0, whatever the width and the labels.
func TestRunwaysLabelColumnTakesItsWidestLabelAndTheLanesTheRest(t *testing.T) {
	work := accountName{label: "work"}
	personal := accountName{label: "personal", primary: true}
	side := accountName{label: "side", best: true}
	frames := []accountName{work, personal, side}
	long := []accountName{{label: "work-team-plan"}, {label: "personal-research", primary: true}}
	both := []accountName{work, {label: "personal", primary: true, best: true}, {label: "side"}}
	tests := []struct {
		name      string
		names     []accountName
		collapsed bool
		width     int
		from      int
	}{
		{name: "the frames' on a phone, personal whole", names: frames, width: 73, from: framesLanesFrom},
		{name: "the frames' on an iPad", names: frames, width: 108, from: framesLanesFrom},
		{name: "the frames' on a desktop", names: frames, width: 173, from: framesLanesFrom},
		{name: "a short name, the widest use", names: []accountName{work}, width: 108, from: 12},
		{name: "long names, whole", names: long, width: 108, from: 23},
		{name: "a long name, whole with ▲'s cells", names: long[:1], width: 108, from: 18},
		{name: "a name of 22, whole", names: append(long, accountName{label: "side-hustle-consulting", best: true}), width: 108, from: 26},
		{name: "a name past the most, its label at 24", names: append(long, accountName{label: "side-hustle-consulting-group", best: true}), width: 108, from: 28},
		{name: "none labelled yet, the widest use", width: 108, from: 12},
		{name: "narrower than its labels, the column first", names: long, width: 20, from: 20},
		{name: "narrower than the least, a label's 8 beside its marks", names: long, width: 10, from: 14},
		{name: "collapsed, the frames' on an iPad, from column 0", names: frames, collapsed: true, width: 108},
		{name: "collapsed, on a phone, alike", names: frames, collapsed: true, width: 52},
		{name: "collapsed, on a desktop, alike", names: frames, collapsed: true, width: 173},
		{name: "collapsed, both marks on one, alike", names: both, collapsed: true, width: 108},
		{name: "collapsed, long names, alike", names: long, collapsed: true, width: 108},
		{name: "collapsed, none labelled yet, alike", collapsed: true, width: 108},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RunwayShown{Collapsed: tt.collapsed, labels: runwayLabelsOf(tt.names)}
			if got := r.lanesFrom(tt.width); got != tt.from {
				t.Errorf("the lanes start at column %d, want %d", got, tt.from)
			}
			if got, want := r.Columns(tt.width), max(tt.width-tt.from, 0); got != want {
				t.Errorf("the lanes take %d columns, want %d, to the edge", got, want)
			}
		})
	}
}

// The lanes hold still as ▲ moves between the accounts, as routing picks
// another for new sessions, expanded and collapsed, at every width: the label
// column keeps ▲'s cells on every account.
func TestRunwaysLanesHoldStillAsTheMarkOfNewSessionsMoves(t *testing.T) {
	named := func(best int) []accountName {
		names := []accountName{{label: "work"}, {label: "personal", primary: true}, {label: "side"}}
		if best >= 0 {
			names[best].best = true
		}
		return names
	}
	for _, collapsed := range []bool{false, true} {
		for _, width := range []int{52, 73, 108, 173} {
			from := RunwayShown{Collapsed: collapsed, labels: runwayLabelsOf(named(-1))}.lanesFrom(width)
			for best := range 3 {
				if got := (RunwayShown{Collapsed: collapsed, labels: runwayLabelsOf(named(best))}).lanesFrom(width); got != from {
					t.Errorf("collapsed %t at %d, with ▲ on account %d the lanes start at column %d, want %d, as with ▲ on none", collapsed, width, best+1, got, from)
				}
			}
		}
	}
}

func TestRunwaysTimelineIsTheLiveViewsOrTheWeekSteppedBacksAcrossItsColumns(t *testing.T) {
	tests := []struct {
		name  string
		shown RunwayShown
		want  views.Timeline
	}{
		{name: "live", shown: RunwayShown{}, want: views.LiveTimeline(runwayAt(), 148)},
		{name: "a week stepped back to", shown: RunwayShown{Back: civilDay(9, 21)}, want: views.Stepped(civilDay(9, 21), runwayAt(), time.Monday, 148)},
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
// the ledger files none; its timeline, lanes, COMING UP and LATELY built at
// the look, as the watch builds them; in nord, 160 by 37. It gives the frame
// and the document it's drawn of, at the moment's now. Runway shows the
// week alone, so a moment judging another window has no frame.
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
	if rm.Key != "" {
		t.Fatalf("Runway shows the week alone; %q judges %q", name, rm.Key)
	}
	doc := rm.Doc
	doc.Best = "side"
	m := doc.At(rm.Now, claude.Policy())
	tl := views.Stepped(rm.Day, rm.Now, rm.WeekStarts, RunwayShown{}.Columns(width))
	lanes := views.Lanes{}.Look(views.LanesFrom{Moment: m, Readings: rm.Readings, Days: filed(rm.Days), Kept: rm.Kept, Timeline: tl})
	f := Frame{
		Width: width, Height: height, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claude.Policy(),
		Runway: RunwayShown{
			Back: rm.Day, Timeline: tl, Lanes: &lanes, Coming: views.RunwayComing(m), Lately: runwayLatelyOf(m),
		},
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

// Each account's use of the week under its name is a number a look reads,
// which brightens as it changes, as is the pool's room under Σ pool; so is
// the highest a week gone read, which never changes, and it has no room.
func TestTheUseUnderEachNameIsANumberALookReads(t *testing.T) {
	tests := []struct {
		moment string
		want   map[Number]string
	}{
		{moment: "the auto world's week view", want: map[Number]string{
			{Kind: NumberUsed, Account: "work", Of: "7d"}: "34%", {Kind: NumberUsed, Account: "personal", Of: "7d"}: "66%", {Kind: NumberUsed, Account: "side", Of: "7d"}: "78%",
			{Kind: NumberRoom, Of: "7d"}: "1.2",
		}},
		{moment: "stepped back to a week its account's week reset in", want: map[Number]string{
			{Kind: NumberUsed, Account: "work", Of: "7d"}: "98%",
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
