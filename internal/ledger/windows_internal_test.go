package ledger

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

// A climb read back from its stored form is the climb written, every field
// of it, but the use a run whose start isn't known begins at, which a page's
// points give none of; and it takes the next reading, and ends, as the climb
// written does. Of generated readings of a window, stored after each: across
// resets, as they come and by hand; readings read late, past their own
// reset, or saying no reset; every field of a reading given; and times to
// the nanosecond, in years nanoseconds since the epoch don't hold.
func TestAClimbReadBackFromItsStoredFormTakesReadingsOnAsTheOneWritten(t *testing.T) {
	for seed := range uint64(200) {
		c := NewClimb("5h", nil)
		for i, r := range generatedReadings(seed) {
			back := storedAndBack(t, c)
			if !sameClimb(back, c) {
				t.Fatalf("seed %d, reading %d: read back as\n%+v\nwant\n%+v", seed, i, *back, *c)
			}
			ended, endedBack := c.Clone(), back.Clone()
			ended.End(r.at)
			endedBack.End(r.at)
			if !sameClimb(endedBack, ended) {
				t.Fatalf("seed %d, reading %d: read back, ended at %v as\n%+v\nwant\n%+v", seed, i, r.at, *endedBack, *ended)
			}
			climbed, took := c.Take(r.window, r.at)
			climbedBack, tookBack := back.Take(r.window, r.at)
			if climbedBack != climbed || tookBack != took || !sameClimb(back, c) {
				t.Fatalf("seed %d, reading %d: read back, it took %+v at %v climbing %v, %t, as\n%+v\nwant %v, %t, as\n%+v",
					seed, i, r.window, r.at, climbedBack, tookBack, *back, climbed, took, *c)
			}
		}
	}
}

// storedAndBack is c written in its stored form and read back.
func storedAndBack(t *testing.T, c *Climb) *Climb {
	t.Helper()
	stored, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("storing %+v: %v", *c, err)
	}
	var back Climb
	if err := json.Unmarshal(stored, &back); err != nil {
		t.Fatalf("reading back %s: %v", stored, err)
	}
	return &back
}

// sameClimb reports whether two climbs hold the same, every field but the
// use a run whose start isn't known begins at, a function.
func sameClimb(a, b *Climb) bool {
	x, y := *a, *b
	x.began, y.began = nil, nil
	return reflect.DeepEqual(x, y)
}

// windowRead is a reading of a window, read at a moment.
type windowRead struct {
	window quota.Window
	at     time.Time
}

// generatedReadings are readings of a five-hour window as the seed given
// makes them, read minutes apart: its use climbing through each run, now
// and then reading lower, and starting again from little as the window
// resets; now and then one reset by hand, its use falling, its reset the
// same; one read late, of the run before; one read past its own reset; one
// saying no reset; and every field of a reading given, now and then. Of one
// seed in four, in a year nanoseconds since the epoch don't hold.
func generatedReadings(seed uint64) []windowRead {
	r := rand.New(rand.NewPCG(seed, 21))
	at := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	switch seed % 4 {
	case 1:
		at = time.Date(1500, 3, 1, 9, 0, 0, 0, time.UTC)
	case 2:
		at = time.Date(2400, 3, 1, 9, 0, 0, 0, time.UTC)
	}
	resets, use := at.Add(5*time.Hour), 0.0
	var read []windowRead
	for range 10 + r.IntN(40) {
		at = at.Add(time.Duration(r.Int64N(int64(90 * time.Minute))))
		for !at.Before(resets) {
			resets, use = resets.Add(5*time.Hour), 0
		}
		use += r.Float64() / 10
		w := quota.Window{Key: "5h", Utilization: use, ResetsAt: resets}
		switch p := r.IntN(20); {
		case p == 0 && use > 0.2:
			use -= 0.15
			w.Utilization = use
		case p == 1:
			w.ResetsAt, w.Utilization = resets.Add(-5*time.Hour), use+0.05
		case p == 2:
			w.ResetsAt = at.Add(-time.Minute)
		case p == 3:
			w.ResetsAt = time.Time{}
		case p == 4:
			w.Utilization = math.Copysign(0, -1)
		case p == 5:
			pace := r.Float64()
			w.Label, w.Status, w.RestartedAt, w.Pace = "Session", quota.StatusAllowedWarning, resets.Add(-4*time.Hour), &pace
			w.Allowance = quota.Allowance{Share: r.Float64(), Per: quota.PerHour}
		case p < 9:
			w.Utilization = max(use-0.05, 0)
		}
		read = append(read, windowRead{window: w, at: at})
	}
	return read
}
