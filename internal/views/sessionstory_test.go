package views_test

import (
	"math"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// october8 is the local time on 8 October, the day after the views' tests'
// own, at hour:minute:second.
func october8(hour, minute, second int) time.Time {
	return time.Date(2026, 10, 8, hour, minute, second, 0, time.Local)
}

// minutely are requests of paged's own conversation, of Claude Opus 5.5, on
// the account given, one a minute from from to to, both included, each with
// an id of its own.
func minutely(account string, from, to time.Time) []ledger.Line {
	var lines []ledger.Line
	for at := from; !at.After(to); at = at.Add(time.Minute) {
		lines = append(lines, asked(account, "sticky", at))
	}
	return lines
}

// asked is a request of paged's own conversation, of Claude Opus 5.5, on the
// account given, routed for the reason given, at at, with an id of its own.
func asked(account, reason string, at time.Time) ledger.Line {
	return filedAs("r"+strconv.FormatInt(at.UnixMilli(), 10), serving(p1, ledger.ClassMain, line(paged, opus, account, reason, at, hourUsage)))
}

// movedTo is a request of paged's own conversation that moved it from one
// account to another for the reason given, at at.
func movedTo(from, to, reason string, at time.Time) ledger.Line {
	return movedFrom(from, asked(to, reason, at))
}

// storied is paged's page and story of lines, oldest first, at the time
// given, no router listing the sessions, so it runs while its last request
// came within the hour.
func storied(t *testing.T, at time.Time, lines []ledger.Line) (views.SessionPage, views.Story) {
	t.Helper()
	l := summarisedLedger(lines)
	l.now = at
	held := views.HoldPage(paged, views.ReadSessions(l, nil, at, paged), ledger.Pricing, kept90, at)
	live := views.PageLive{Unlisted: true, Now: at}
	page, ok := held.Page(live)
	if !ok {
		t.Fatalf("no page of %d lines", len(lines))
	}
	return page, held.Story(page, live)
}

// laid is what a story's tests hold of it laid across a width.
type laid struct {
	columns   int
	axis      []views.AxisLabel
	folds     []int
	moves     []int
	stretches []views.StoryStretch
}

// laidOf is what a story's tests hold of c.
func laidOf(c views.StoryColumns) laid {
	l := laid{columns: len(c.Rates), axis: c.Axis, folds: c.Folds, stretches: c.Stretches}
	for _, m := range c.Moves {
		l.moves = append(l.moves, m.Column)
	}
	return l
}

// clock is an axis's label of a time at the column given.
func clock(column int, text string) views.AxisLabel {
	return views.AxisLabel{Column: column, Text: text}
}

// A story is laid across its width as the frames draw it: its first minute
// at column 0, its newest at the last, a column standing for an exact share
// of the minutes but never more than one; a time on the axis each half hour,
// or hour past four, at its own column, but none in its first 8 minutes or
// last 10, within 20 minutes of a fold, under 8 columns after the one before,
// or starting past 10 columns from the end; a fold of five columns, its hours
// at its first and the time it came back after it; a move's marker where the
// stretch it opens starts; a stretch running to the column before the next,
// or to its fold, and left out under three columns.
func TestAStoryIsLaidAcrossItsWidth(t *testing.T) {
	moved := slices.Concat(minutely("work", local(12, 2, 0), local(14, 30, 0)),
		[]ledger.Line{movedTo("work", "side", "moved: work is at its reserve", local(14, 31, 0))}, minutely("side", local(14, 32, 0), local(14, 41, 0)))
	accounts := []string{"work", "side", "personal", "spare", "acct-5", "acct-6", "acct-7", "acct-8", "acct-9", "acct-10"}
	var ten []ledger.Line
	for i, account := range accounts {
		from := local(12, 0, 0).Add(time.Duration(16*i) * time.Minute)
		if i > 0 {
			ten = append(ten, movedTo(accounts[i-1], account, "moved: "+accounts[i-1]+" is at its reserve", from))
			from = from.Add(time.Minute)
		}
		ten = append(ten, minutely(account, from, local(12, 15, 0).Add(time.Duration(16*i)*time.Minute))...)
	}
	halves := []views.AxisLabel{clock(0, "12:00"), clock(30, "12:30"), clock(60, "13:00"), clock(90, "13:30"), clock(120, "14:00"), clock(150, "14:30"), {Column: 157, Text: "now", End: true}}
	tests := []struct {
		name  string
		at    time.Time
		lines []ledger.Line
		width int
		want  laid
	}{
		{
			name: "moved at its cap, 160 minutes old: a minute a column", at: local(14, 42, 7), lines: moved, width: 160,
			want: laid{columns: 160,
				axis:  []views.AxisLabel{clock(0, "12:02"), clock(28, "12:30"), clock(58, "13:00"), clock(88, "13:30"), clock(118, "14:00"), clock(148, "14:30"), {Column: 157, Text: "now", End: true}},
				moves: []int{149}, stretches: []views.StoryStretch{{Account: "work", First: 0, Last: 148}, {Account: "side", First: 149, Last: 159}}},
		},
		{
			name: "moved at its cap, on a phone", at: local(14, 42, 7), lines: moved, width: 52,
			want: laid{columns: 52,
				axis:  []views.AxisLabel{clock(0, "12:02"), clock(9, "12:30"), clock(18, "13:00"), clock(28, "13:30"), clock(38, "14:00"), {Column: 49, Text: "now", End: true}},
				moves: []int{48}, stretches: []views.StoryStretch{{Account: "work", First: 0, Last: 47}, {Account: "side", First: 48, Last: 51}}},
		},
		{
			name: "idle 9 minutes, its columns each a little over a minute", at: local(14, 42, 7), lines: minutely("personal", local(11, 55, 0), local(14, 33, 0)), width: 160,
			want: laid{columns: 160,
				axis:      []views.AxisLabel{clock(0, "11:55"), clock(33, "12:30"), clock(62, "13:00"), clock(91, "13:30"), clock(119, "14:00"), clock(148, "14:30"), {Column: 157, Text: "now", End: true}},
				stretches: []views.StoryStretch{{Account: "personal", First: 0, Last: 159}}},
		},
		{
			name: "ended yesterday, a time each hour past four hours", at: october8(14, 42, 7), width: 160,
			lines: slices.Concat(minutely("work", local(13, 20, 0), local(16, 1, 0)),
				[]ledger.Line{movedTo("work", "side", "moved: work is at its reserve", local(16, 2, 0))}, minutely("side", local(16, 3, 0), local(18, 47, 0))),
			want: laid{columns: 160,
				axis:  []views.AxisLabel{clock(0, "13:20"), clock(19, "14:00"), clock(48, "15:00"), clock(78, "16:00"), clock(107, "17:00"), clock(137, "18:00"), {Column: 155, Text: "ended", End: true}},
				moves: []int{79}, stretches: []views.StoryStretch{{Account: "work", First: 0, Last: 78}, {Account: "side", First: 79, Last: 159}}},
		},
		{
			name: "back after a night, rescored", at: october8(14, 42, 7), width: 160,
			lines: slices.Concat(minutely("side", local(16, 40, 0), local(18, 5, 0)),
				[]ledger.Line{movedTo("side", "personal", "rescored after 15h idle", october8(9, 12, 0))}, minutely("personal", october8(9, 13, 0), october8(14, 41, 0))),
			want: laid{columns: 160,
				axis: []views.AxisLabel{clock(0, "16:40"), clock(7, "17:00"), clock(32, "15h"), clock(37, "09:12"), clock(54, "10:00"), clock(77, "11:00"),
					clock(99, "12:00"), clock(121, "13:00"), clock(144, "14:00"), {Column: 157, Text: "now", End: true}},
				folds: []int{32}, moves: []int{36}, stretches: []views.StoryStretch{{Account: "side", First: 0, Last: 32}, {Account: "personal", First: 36, Last: 159}}},
		},
		{
			name: "one request", at: local(12, 5, 40), lines: minutely("work", local(12, 5, 0), local(12, 5, 0)), width: 160,
			want: laid{columns: 1, axis: []views.AxisLabel{clock(0, "12:05"), {Column: 6, Text: "now", End: true}}},
		},
		{
			name: "young, 20 minutes, drawn from the left", at: local(12, 25, 30), lines: minutely("work", local(12, 5, 0), local(12, 25, 0)), width: 160,
			want: laid{columns: 21, axis: []views.AxisLabel{clock(0, "12:05"), {Column: 21, Text: "now", End: true}},
				stretches: []views.StoryStretch{{Account: "work", First: 0, Last: 20}}},
		},
		{
			name: "young, 40 minutes, a time at its own column", at: local(12, 45, 30), lines: minutely("work", local(12, 5, 0), local(12, 45, 0)), width: 160,
			want: laid{columns: 41, axis: []views.AxisLabel{clock(0, "12:05"), clock(25, "12:30"), {Column: 41, Text: "now", End: true}},
				stretches: []views.StoryStretch{{Account: "work", First: 0, Last: 40}}},
		},
		{
			name: "across midnight with no fold, its times alone", at: october8(0, 50, 30), lines: minutely("work", local(23, 10, 0), october8(0, 50, 0)), width: 160,
			want: laid{columns: 101,
				axis:      []views.AxisLabel{clock(0, "23:10"), clock(20, "23:30"), clock(50, "00:00"), clock(80, "00:30"), {Column: 101, Text: "now", End: true}},
				stretches: []views.StoryStretch{{Account: "work", First: 0, Last: 100}}},
		},
		{
			name: "four accounts, a stay under three columns not drawn", at: local(14, 40, 30), width: 160,
			lines: slices.Concat(minutely("work", local(12, 0, 0), local(12, 59, 0)),
				[]ledger.Line{movedTo("work", "side", "moved: work is at its reserve", local(13, 0, 0)), asked("side", "sticky", local(13, 1, 0)),
					movedTo("side", "personal", "moved: side is at its reserve", local(13, 2, 0))},
				minutely("personal", local(13, 3, 0), local(13, 59, 0)),
				[]ledger.Line{movedTo("personal", "spare", "moved: personal is at its reserve", local(14, 0, 0))}, minutely("spare", local(14, 1, 0), local(14, 40, 0))),
			want: laid{columns: 160, axis: halves, moves: []int{60, 62, 120},
				stretches: []views.StoryStretch{{Account: "work", First: 0, Last: 59}, {Account: "personal", First: 62, Last: 119}, {Account: "spare", First: 120, Last: 159}}},
		},
		{
			name: "ten accounts", at: local(14, 40, 30), lines: append(ten, asked("acct-10", "sticky", local(14, 40, 0))), width: 160,
			want: laid{columns: 160, axis: halves, moves: []int{16, 32, 48, 64, 80, 96, 112, 128, 144},
				stretches: []views.StoryStretch{{Account: "work", First: 0, Last: 15}, {Account: "side", First: 16, Last: 31}, {Account: "personal", First: 32, Last: 47},
					{Account: "spare", First: 48, Last: 63}, {Account: "acct-5", First: 64, Last: 79}, {Account: "acct-6", First: 80, Last: 95}, {Account: "acct-7", First: 96, Last: 111},
					{Account: "acct-8", First: 112, Last: 127}, {Account: "acct-9", First: 128, Last: 143}, {Account: "acct-10", First: 144, Last: 159}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, story := storied(t, tt.at, tt.lines)
			if got := laidOf(story.Columns(tt.width)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("laid across %d, the story is\n%+v\nwant\n%+v", tt.width, got, tt.want)
			}
		})
	}
}

// A story 160 minutes old is a minute a column, each column its minute's
// requests, its last its newest two minutes' mean; a time's column is the
// minute it falls in, now's the last.
func TestAStory160MinutesOldIsAMinuteAColumn(t *testing.T) {
	var lines []ledger.Line
	for m := range 161 {
		at := local(12, 0, 0).Add(time.Duration(m) * time.Minute)
		for i := range m%3 + 1 {
			lines = append(lines, asked("work", "sticky", at.Add(time.Duration(i)*time.Second)))
		}
	}
	_, story := storied(t, local(14, 40, 30), lines)
	c := story.Columns(160)
	want := make([]float64, 160)
	for i := range want {
		want[i] = float64(i%3 + 1)
	}
	want[159] = (float64(159%3+1) + float64(160%3+1)) / 2
	if !slices.Equal(c.Rates, want) {
		t.Errorf("its rates are %v, want %v", c.Rates, want)
	}
	for m, want := range map[int]int{0: 0, 28: 28, 159: 159, 160: 159} {
		if got := c.At(local(12, 0, 0).Add(time.Duration(m)*time.Minute + 30*time.Second)); got != want {
			t.Errorf("its minute %d falls at column %d, want %d", m, got, want)
		}
	}
}

// A column standing for a minute and a half shares the minutes its edges cut
// by their overlap, its rate the mean over its span; its last stands for its
// newest minute too. A request the router answered itself, which never went
// upstream, counts in none, as the traces count.
func TestAColumnsRateIsTheMeanOverTheMinutesItSpans(t *testing.T) {
	lines := []ledger.Line{asked("work", "sticky", local(12, 0, 0))}
	for i := range 3 {
		lines = append(lines, asked("work", "sticky", local(12, 1, 10*i)))
	}
	answered := asked("work", "sticky", local(12, 40, 0))
	answered.Attempts = 0
	lines = append(lines, answered, asked("work", "sticky", local(13, 18, 0)))
	_, story := storied(t, local(14, 30, 0), lines)
	rates := story.Columns(52).Rates
	want := map[int]float64{0: (1 + 3*0.5) / 1.5, 1: 3 * 0.5 / 1.5, 2: 0, 26: 0, 50: 0, 51: 1 / 2.5}
	if len(rates) != 52 {
		t.Fatalf("its columns are %d, want 52", len(rates))
	}
	for column, rate := range want {
		if math.Abs(rates[column]-rate) > 1e-9 {
			t.Errorf("column %d's rate is %v, want %v", column, rates[column], rate)
		}
	}
}

// Past half the width in folds, the oldest merge into one, from the start of
// the first to the end of the last merged, what came between hidden: 16
// folds at 160, five at 52, each stretch of requests between keeping its
// columns.
func TestAStorysOldestFoldsMergeOnceTheyWouldTakeHalfItsWidth(t *testing.T) {
	var lines []ledger.Line
	for b := range 33 {
		at := local(0, 0, 0).Add(time.Duration(63*b) * time.Minute)
		lines = append(lines, minutely("work", at, at.Add(2*time.Minute))...)
	}
	last := lines[len(lines)-1].At
	_, story := storied(t, last.Add(30*time.Second), lines)
	if len(story.Folds) != 32 {
		t.Fatalf("the story folds %d gaps, want 32", len(story.Folds))
	}
	tests := []struct {
		width, folds, columns int
		shown                 float64
	}{
		{width: 160, folds: 16, columns: 131, shown: 3 * 17},
		{width: 52, folds: 5, columns: 43, shown: 3 * 6},
	}
	for _, tt := range tests {
		c := story.Columns(tt.width)
		var want []int
		for j := range tt.folds {
			want = append(want, 8*j+3)
		}
		var shown float64
		for column, rate := range c.Rates {
			if !c.Folded(column) {
				shown += rate
			}
		}
		if !slices.Equal(c.Folds, want) || len(c.Rates) != tt.columns || shown != tt.shown {
			t.Errorf("at %d, folds at %v in %d columns showing %v requests; want folds at %v in %d showing %v",
				tt.width, c.Folds, len(c.Rates), shown, want, tt.columns, tt.shown)
		}
	}
}

// A story starts at the oldest line its ledger keeps, the day before gone,
// and is kept until a [ledger] keep after that day.
func TestAStoryStartsAtItsOldestLineKept(t *testing.T) {
	page, story := storied(t, october8(14, 0, 0), minutely("work", october8(9, 0, 0), october8(13, 30, 0)))
	if !story.Start.Equal(october8(9, 0, 0)) || page.KeptUntil != "2027-01-06" {
		t.Errorf("the story starts at %v, kept until %q; want 09:00, kept 90 days from its first kept day", story.Start, page.KeptUntil)
	}
}
