package capture

import (
	"fmt"
	"testing"
	"time"
)

// A time of day the clocks skip, as they go forward, comes no later than the
// first time after the skip, its seconds and all, so the times of a day keep
// the order the clocks give them, though two may tie: Sydney's clocks go from
// 02:00 to 03:00 on 4 October 2026, and Santiago's from midnight to 01:00 on
// 6 September.
func TestATimeTheClocksSkipComesNoLaterThanTheFirstAfterIt(t *testing.T) {
	type clock struct {
		hour, minute, second int
		skipped              bool
	}
	tests := []struct {
		zone string
		date time.Time
		// resumes is the first time the clocks read after the skip.
		resumes clock
		// times are in the order the clocks give them.
		times []clock
	}{
		{
			zone: "Australia/Sydney", date: time.Date(2026, time.October, 4, 0, 0, 0, 0, time.UTC), resumes: clock{hour: 3},
			times: []clock{{1, 59, 59, false}, {2, 0, 0, true}, {2, 30, 30, true}, {2, 59, 59, true}, {3, 0, 0, false}, {3, 0, 59, false}},
		},
		{
			zone: "America/Santiago", date: time.Date(2026, time.September, 6, 0, 0, 0, 0, time.UTC), resumes: clock{hour: 1},
			times: []clock{{0, 0, 0, true}, {0, 0, 59, true}, {0, 59, 59, true}, {1, 0, 0, false}, {1, 0, 1, false}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.zone, func(t *testing.T) {
			loc, err := time.LoadLocation(tt.zone)
			if err != nil {
				t.Fatal(err)
			}
			resumes := clockOn(tt.date, tt.resumes.hour, tt.resumes.minute, tt.resumes.second, loc)
			var before time.Time
			for _, c := range tt.times {
				at := clockOn(tt.date, c.hour, c.minute, c.second, loc)
				asked := fmt.Sprintf("%02d:%02d:%02d", c.hour, c.minute, c.second)
				if read := at.In(loc).Format(time.TimeOnly); !c.skipped && read != asked {
					t.Errorf("%s comes at %s, which the clocks read %s", asked, at, read)
				}
				if c.skipped && at.After(resumes) {
					t.Errorf("%s, which the clocks skip, comes at %s, after %s, the first time after the skip", asked, at.In(loc), resumes.In(loc))
				}
				if at.Before(before) {
					t.Errorf("%s comes at %s, before the time the clocks read before it, at %s", asked, at.In(loc), before.In(loc))
				}
				before = at
			}
		})
	}
}
