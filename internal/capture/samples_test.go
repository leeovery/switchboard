package capture

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

func TestTheSamplesReadAsTheFramesHaveThem(t *testing.T) {
	now := moment(time.UTC)
	tests := []struct {
		sample          sample
		window          string
		used, projected string
	}{
		{sample: work(now), window: fiveHourKey, used: "58%", projected: "runs out ~Thu 16:05"},
		{sample: work(now), window: weekKey, used: "34%", projected: "on pace for 87%"},
		{sample: work(now), window: fableKey, used: "12%", projected: "on pace for 31%"},
		{sample: personal(now), window: fiveHourKey, used: "100%", projected: "exhausted"},
		{sample: personal(now), window: weekKey, used: "89%", projected: "runs out ~Fri 04:06"},
		{sample: side(now), window: fiveHourKey, used: "12%", projected: "on pace for 54%"},
		{sample: side(now), window: weekKey, used: "33%", projected: "on pace for 72%"},
		{sample: side(now), window: fableKey, used: "4%", projected: "on pace for 9%"},
		{sample: client(now), window: fiveHourKey, used: "31%", projected: "on pace for 48%"},
		{sample: lab(now), window: fiveHourKey, used: "44%", projected: "on pace for 71%"},
		{sample: lab(now), window: weekKey, used: "52%", projected: "on pace for 75%"},
		{sample: team(now), window: fiveHourKey, used: "8%", projected: "on pace for 20%"},
		{sample: team(now), window: weekKey, used: "21%", projected: "on pace for 28%"},
	}
	for _, tt := range tests {
		t.Run(tt.sample.id+" "+tt.window, func(t *testing.T) {
			a := tt.sample.account(now)
			w, ok := a.Window(tt.window)
			if !ok {
				t.Fatalf("%s has no window %s", a.ID, tt.window)
			}
			if got := status.Percent(w.Utilization); got != tt.used {
				t.Errorf("%s's %s is %s used, want %s", a.ID, tt.window, got, tt.used)
			}
			if got := status.Projection(now, a.Project(w, now).Projection); got != tt.projected {
				t.Errorf("%s's %s projects %q, want %q", a.ID, tt.window, got, tt.projected)
			}
		})
	}
}

func TestTheSamplesStandAsTheFramesHaveThem(t *testing.T) {
	now := moment(time.UTC)
	byID := make(map[string]status.Account)
	for _, s := range []sample{work(now), personal(now), side(now), client(now), spare(now), lab(now), team(now), extra(now)} {
		byID[s.id] = s.account(now)
	}

	var pressed, lapsed []string
	for _, id := range []string{"work", "personal", "side", "client", "spare", "lab", "team", "extra"} {
		if byID[id].Pressure.Under {
			pressed = append(pressed, id)
		}
		if slices.Equal(byID[id].Lapsed, []string{fiveHourKey}) {
			lapsed = append(lapsed, id)
		}
	}
	if want := []string{"work"}; !slices.Equal(pressed, want) {
		t.Errorf("under pressure are %v, want %v", pressed, want)
	}
	if want := []string{"spare", "extra"}; !slices.Equal(lapsed, want) {
		t.Errorf("lapsed are %v, want %v", lapsed, want)
	}
	if limit := byID["personal"].Limit; !limit.Holds(now) || !limit.Until.Equal(on(now, 1, 15, 54)) {
		t.Errorf("personal's limit is %+v, want it holding until 15:54", limit)
	}
	if got := status.Over(byID["work"].Pressure.Since, now); got != "last 30 min" {
		t.Errorf("work's pressure is measured over the %s, want the last 30 min", got)
	}
}
