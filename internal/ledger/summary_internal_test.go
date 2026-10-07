package ledger

import (
	"iter"
	"slices"
	"testing"
	"time"

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
	d.stamp("2026-10-05", time.Date(2026, 10, 5, 23, 59, 0, 0, time.Local))
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

func (a *asking) readings(from, to time.Time) iter.Seq[readings.Reading] {
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

func TestReadingsReadAheadGiveThoseOfEachDayAndTheWeekBeforeAsTheHistoryWould(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	day := func(n int) time.Time { return start.AddDate(0, 0, n) }
	history := &asking{held: hourly(start, 60*24)}
	read, stop := readAhead(history.readings, day(40))
	defer stop()

	// Days 10 to 14, oldest first, each asking for its readings and the week
	// before's, then day 30, well after.
	for _, n := range []int{10, 11, 12, 13, 14, 30} {
		from, to := day(n-7), day(n+1)
		if got, want := slices.Collect(read(from, to)), between(history.held, from, to); !slices.Equal(got, want) {
			t.Errorf("day %d: gave %d readings, want the %d the history holds of its day and the week before", n, len(got), len(want))
		}
	}
	if want := [][2]time.Time{{day(3), day(40)}, {day(23), day(40)}}; !slices.Equal(history.asked, want) {
		t.Errorf("the history was asked for the readings of %v, want %v: once for the days together, and again for the day well after them", history.asked, want)
	}
	// Up to the first of day 15's, then from the week before day 30 to the
	// first of day 31's.
	if want := (15-3)*24 + 1 + (31-23)*24 + 1; history.given != want {
		t.Errorf("the history gave %d readings, want %d: none between the days asked for", history.given, want)
	}
}

func TestReadingsReadAheadHoldAWeekAndADayOfThemAtMost(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	day := func(n int) time.Time { return start.AddDate(0, 0, n) }
	history := &asking{held: hourly(start, 60*24)}
	a := &readingsAhead{history: history.readings, until: day(60)}
	defer a.stop()

	for n := 7; n < 59; n++ {
		a.readings(day(n-7), day(n+1))
		if most := 8*24 + 1; len(a.held) > most {
			t.Fatalf("asked for day %d, it holds %d readings, want %d at most: a week and a day's, and the first after", n, len(a.held), most)
		}
	}
}

func TestReadingsReadAheadReadAfreshThoseOfTimesBeforeOrAfterWhatTheyRead(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	day := func(n int) time.Time { return start.AddDate(0, 0, n) }
	history := &asking{held: hourly(start, 60*24)}
	read, stop := readAhead(history.readings, day(20))
	defer stop()

	for _, ask := range [][2]time.Time{{day(10), day(12)}, {day(5), day(8)}, {day(18), day(25)}} {
		if got, want := slices.Collect(read(ask[0], ask[1])), between(history.held, ask[0], ask[1]); !slices.Equal(got, want) {
			t.Errorf("asked for %v, gave %d readings, want the %d the history holds of them", ask, len(got), len(want))
		}
	}
	if want := [][2]time.Time{{day(10), day(20)}, {day(5), day(20)}, {day(18), day(25)}}; !slices.Equal(history.asked, want) {
		t.Errorf("the history was asked for the readings of %v, want %v", history.asked, want)
	}
}
