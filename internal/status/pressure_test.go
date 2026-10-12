package status_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

func TestPressureFrom(t *testing.T) {
	// The session rose from 15%, read an hour ago, to 40% now, 25% an hour,
	// resetting in 3 hours: work keeping 5% of it back reaches that in 2h 12m,
	// and, spending it, its limit in 2h 24m.
	session := windowIn("5h", 0.4, 3*time.Hour)
	trails := map[string][]score.Reading{"5h": {{At: built.Add(-time.Hour), Utilization: 0.15}, {At: built, Utilization: 0.4}}}
	since := built.Add(-time.Hour)
	pressured := func(outIn time.Duration, under bool) status.Pressure {
		return status.Pressure{Window: "5h", Rate: 0.25, Recent: true, Since: since, RunsOut: built.Add(outIn), Under: under}
	}
	tests := []struct {
		name   string
		latest []quota.Window
		spent  bool
		shut   bool
		want   status.Pressure
	}{
		{name: "reaching its reserve before its reset", latest: []quota.Window{session}, want: pressured(132*time.Minute, true)},
		{name: "a pin spending its reserve, reaching its limit", latest: []quota.Window{session}, spent: true, want: pressured(144*time.Minute, true)},
		{name: "taking no request, shut", latest: []quota.Window{session}, shut: true, want: pressured(132*time.Minute, false)},
		{name: "taking no request, its week at its reserve", latest: []quota.Window{session, windowIn("7d", 0.96, 2*day)}, want: pressured(132*time.Minute, false)},
		{name: "its week at its reserve, a pin spending it", latest: []quota.Window{session, windowIn("7d", 0.96, 2*day)}, spent: true, want: pressured(144*time.Minute, true)},
		{name: "its session not read", latest: []quota.Window{windowIn("7d", 0.5, 2*day)}},
		{name: "its session reset since it was read", latest: []quota.Window{windowIn("5h", 0.4, -time.Minute)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := status.PressureFrom(policy, status.Pressing{ID: "work", Reserve: 0.05, Latest: tt.latest, Trails: trails, Spent: tt.spent, Shut: tt.shut}, built)

			got.RunsOut = got.RunsOut.Round(time.Second)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PressureFrom() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
