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
	want := []string{"level=WARN", `msg="can't stamp the request ledger's summary of a day; its lines are counted again at the next round"`, "day=2026-10-05"}
	if !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant the summary that couldn't be stamped warned of: a line with %q", log, want)
	}
}

func TestReadingsReadOnceGiveThoseOfAnyTimesBetweenAsTheHistoryWould(t *testing.T) {
	start := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	at := func(hours int) time.Time { return start.Add(time.Duration(hours) * time.Hour) }
	var held []readings.Reading
	for hour := range 48 {
		held = append(held, readings.Reading{At: at(hour), Account: "work", Key: "5h", Utilization: float64(hour) / 100})
	}
	// between returns those held of times from from up to to.
	between := func(from, to time.Time) iter.Seq[readings.Reading] {
		return func(yield func(readings.Reading) bool) {
			for _, r := range held {
				if !r.At.Before(from) && r.At.Before(to) && !yield(r) {
					return
				}
			}
		}
	}
	var asked [][2]time.Time
	read := readOnce(func(from, to time.Time) iter.Seq[readings.Reading] {
		asked = append(asked, [2]time.Time{from, to})
		return between(from, to)
	}, at(48))
	tests := []struct {
		name     string
		from, to time.Time
	}{
		{name: "the first asked for, from which all are read", from: at(10), to: at(20)},
		{name: "some of those read", from: at(15), to: at(30)},
		{name: "to the end of those read", from: at(40), to: at(48)},
		{name: "some before those read, read afresh", from: at(5), to: at(12)},
	}
	for _, tt := range tests {
		if got, want := slices.Collect(read(tt.from, tt.to)), slices.Collect(between(tt.from, tt.to)); !slices.Equal(got, want) {
			t.Errorf("%s: gave %d readings, want the %d the history holds of the times from %v to %v", tt.name, len(got), len(want), tt.from, tt.to)
		}
	}
	if want := [][2]time.Time{{at(10), at(48)}, {at(5), at(12)}}; !slices.Equal(asked, want) {
		t.Errorf("the history was asked for the readings of %v, want %v: from the first time asked for to the end, once, and those before it afresh", asked, want)
	}
}
