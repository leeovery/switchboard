package ledger

import (
	"iter"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/readings"
)

func TestASummaryThatCantBeStampedIsWarnedOf(t *testing.T) {
	log := logstest.Capture(t)
	logger := logs.For("router")
	d := days{files: filesIn(t.TempDir(), logger), logger: logger}

	// There's no summary of the day to stamp, as when it was removed once
	// written.
	d.stamp("2026-10-05", dayfile.DayStat{CompressedSize: 120, CompressedModified: time.Date(2026, 10, 5, 23, 59, 0, 0, time.Local)})
	want := []string{"level=WARN", `msg="can't stamp the request ledger's summary of a day; it's read again at the next round"`, "day=2026-10-05"}
	if !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant the summary that couldn't be stamped warned of: a line with %q", log, want)
	}
}

// hourly are readings of work's session, one at the start of each hour from
// start for as many hours as given.
func hourly(start time.Time, hours int) []readings.Reading {
	held := make([]readings.Reading, hours)
	for hour := range hours {
		held[hour] = readings.Reading{At: start.Add(time.Duration(hour) * time.Hour), Account: "work", Key: "5h", Utilization: float64(hour%100) / 100}
	}
	return held
}

// between returns those of held of times from from up to to.
func between(held []readings.Reading, from, to time.Time) []readings.Reading {
	return slices.DeleteFunc(slices.Clone(held), func(r readings.Reading) bool { return r.At.Before(from) || !r.At.Before(to) })
}

// asking is a readings history of held that notes each time it's asked, and
// counts the readings it gives.
type asking struct {
	held  []readings.Reading
	asked [][2]time.Time
	given int
}

func (a *asking) Between(from, to time.Time, _ *dayfile.Faults) iter.Seq[readings.Reading] {
	a.asked = append(a.asked, [2]time.Time{from, to})
	return func(yield func(readings.Reading) bool) {
		for _, r := range between(a.held, from, to) {
			a.given++
			if !yield(r) {
				return
			}
		}
	}
}

func (*asking) Dates() ([]string, error) {
	return nil, nil
}

func TestReadingsReadAheadGiveThoseOfEachDayAndTheWeekBeforeOnceForEachRunOfDays(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	day := func(n int) time.Time { return dayfile.DayStart(start, n) }
	history := &asking{held: hourly(start.Add(-time.Hour), 61*24)}
	var due []dueDay
	// Days 10 to 14, then day 30, well after them; and day 20, which the
	// history doesn't reach.
	for _, n := range []int{10, 11, 12, 13, 14, 20, 30} {
		due = append(due, dueDay{date: day(n).Format(time.DateOnly), reach: reach{reached: n != 20}})
	}

	got := readingAhead(history, due, func(d dueDay, read *ahead) []readings.Reading {
		from, to, _ := dayfile.Day(d.date)
		return slices.Collect(read.readings(from.Add(-readingsBefore), to))
	}, nil)
	for i, d := range due {
		from, to, _ := dayfile.Day(d.date)
		want := between(history.held, from.Add(-readingsBefore), to)
		if !d.reach.reached {
			want = nil
		}
		if !slices.Equal(got[i], want) {
			t.Errorf("%s: gave %d readings, want the %d the history holds of the day and the week before, none where it doesn't reach the day",
				d.date, len(got[i]), len(want))
		}
	}
	runs := [][2]time.Time{{day(10).Add(-readingsBefore), day(15)}, {day(30).Add(-readingsBefore), day(31)}}
	if !slices.Equal(history.asked, runs) {
		t.Errorf("the history was asked for the readings of %v, want %v: once for the days together, and again for the day well after them", history.asked, runs)
	}
	if want := len(between(history.held, runs[0][0], runs[0][1])) + len(between(history.held, runs[1][0], runs[1][1])); history.given != want {
		t.Errorf("the history gave %d readings, want %d: none between the runs of days", history.given, want)
	}
}

func TestReadingsReadAheadHoldAWeekAndADayOfThemAtMost(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	day := func(n int) time.Time { return start.AddDate(0, 0, n) }
	history := &asking{held: hourly(start, 60*24)}
	next, stop := iter.Pull(history.Between(day(0), day(60), nil))
	defer stop()
	a := &ahead{next: next}

	for n := 7; n < 59; n++ {
		a.readings(day(n-7), day(n+1))
		if most := 8*24 + 1; len(a.held) > most {
			t.Fatalf("asked for day %d, it holds %d readings, want %d at most: a week and a day's, and the first after", n, len(a.held), most)
		}
	}
}

func TestADaysReachTellsWhetherItsSummaryReadsTheReadingsHistory(t *testing.T) {
	const date = "2026-10-05"
	tests := []struct {
		name string
		// held are the days the history holds readings of, and want how far
		// they reach the 5th: a day of lines reads them where they're
		// reached, and one of none, where they're covered.
		held readingsHeld
		want reach
	}{
		{name: "none held"},
		{name: "none held from the day before its week to the day after it", held: readingsHeld{"2026-09-26", "2026-10-07"}},
		{
			name: "the day after's alone, as a change of time zone files a reading under the date beside its own",
			held: readingsHeld{"2026-10-06"}, want: reach{reached: true},
		},
		{name: "from within its week on, as the history began then", held: readingsHeld{"2026-09-28", date}, want: reach{reached: true}},
		{name: "from the day before its week on", held: readingsHeld{"2026-09-27", date}, want: reach{reached: true, covered: true}},
		{name: "from before that on", held: readingsHeld{"2026-09-20", "2026-10-01"}, want: reach{reached: true, covered: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.held.reach(date)
			if got != tt.want || got.reads(false) != tt.want.reached || got.reads(true) != tt.want.covered {
				t.Errorf("reach(%s) = %+v, reading them with lines %v, and with none %v; want %+v", date, got, got.reads(false), got.reads(true), tt.want)
			}
		})
	}
}
