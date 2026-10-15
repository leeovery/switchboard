package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
	"github.com/leeovery/switchboard/internal/views"
)

// Runway's COMING UP is the moment's, filtered: an account back from every
// request, never from a model's own window; the run-outs and resets of the
// windows every model shares, the 5-hour view's those of its 5-hour windows,
// the week view's the 5-hour's and the weeks'; and no prime. What it keeps
// stays in the moment's order.
func TestRunwaysComingUpIsTheMomentsBacksAndItsViewsWindows(t *testing.T) {
	at := func(minutes int) time.Time { return statustest.Built.Add(time.Duration(minutes) * time.Minute) }
	back := status.Upcoming{At: at(10), Account: "work", Kind: status.UpcomingBack}
	modelsBack := status.Upcoming{At: at(15), Account: "side", Kind: status.UpcomingBack, Window: "7d_oi"}
	runsOut := status.Upcoming{At: at(20), Account: "work", Kind: status.UpcomingRunsOut, Window: "5h"}
	resets := status.Upcoming{At: at(30), Account: "side", Kind: status.UpcomingReset, Window: "5h"}
	prime := status.Upcoming{At: at(40), Account: "work", Kind: status.UpcomingPrime}
	weekRunsOut := status.Upcoming{At: at(50), Account: "side", Kind: status.UpcomingRunsOut, Window: "7d", Cap: true}
	weekResets := status.Upcoming{At: at(60), Account: "work", Kind: status.UpcomingReset, Window: "7d"}
	modelsResets := status.Upcoming{At: at(70), Account: "side", Kind: status.UpcomingReset, Window: "7d_oi"}
	capBack := status.Upcoming{At: at(80), Account: "side", Kind: status.UpcomingBack, Cap: true}

	m := status.Document{Source: status.SourceRouter, GeneratedAt: statustest.Built}.At(statustest.Built, claude.Policy())
	m.ComingUp = []status.Upcoming{back, modelsBack, runsOut, resets, prime, weekRunsOut, weekResets, modelsResets, capBack}
	tests := []struct {
		name   string
		window views.RunwayWindow
		want   []status.Upcoming
	}{
		{name: "the 5-hour view", window: views.RunwayFiveHour, want: []status.Upcoming{back, runsOut, resets, capBack}},
		{name: "the week view", window: views.RunwayWeek, want: []status.Upcoming{back, runsOut, resets, weekRunsOut, weekResets, capBack}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := views.RunwayComing(m, tt.window); !slices.Equal(got, tt.want) {
				t.Errorf("RunwayComing = %v, want %v", got, tt.want)
			}
		})
	}
}
