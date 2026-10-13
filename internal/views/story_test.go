package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

func TestTodayIsTheLedgersLocalDayWhateverNowsTimeZone(t *testing.T) {
	now := time.Date(2026, 10, 5, 0, 30, 0, 0, time.Local)
	_, offset := now.Zone()
	away := now.In(time.FixedZone("away", offset-12*60*60))
	if away.Day() == now.Day() {
		t.Fatalf("now in the zone 12 hours behind is %v, on the local day %v: the test reads nothing", away, now)
	}
	today := views.Today(away)
	if today.Date != "2026-10-05" {
		t.Errorf("Today(%v).Date = %q, want the local day's, 2026-10-05", away, today.Date)
	}
	if start := time.Date(2026, 10, 5, 0, 0, 0, 0, time.Local); !today.Start.Equal(start) {
		t.Errorf("Today(%v).Start = %v, want the local day's start, %v", away, today.Start, start)
	}
	if !today.Holds(now) || today.Holds(today.Start.Add(-time.Nanosecond)) {
		t.Errorf("Today(%v) holds now: %v, and the instant before it starts: %v; want it to hold from its start on",
			away, today.Holds(now), today.Holds(today.Start.Add(-time.Nanosecond)))
	}
}
