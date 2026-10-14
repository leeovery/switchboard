package capture

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/score"
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
			if got := projection(now, projected(a, tt.window, now)); got != tt.projected {
				t.Errorf("%s's %s projects %q, want %q", a.ID, tt.window, got, tt.projected)
			}
		})
	}
}

func TestTheSamplesStandAsTheFramesHaveThem(t *testing.T) {
	now := moment(time.UTC)
	byID := make(map[string]status.Account)
	for _, s := range []sample{work(now), personal(now), side(now), client(now), spare(now)} {
		byID[s.id] = s.account(now)
	}

	var pressed, lapsed []string
	for _, id := range []string{"work", "personal", "side", "client", "spare"} {
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
	if want := []string{"spare"}; !slices.Equal(lapsed, want) {
		t.Errorf("lapsed are %v, want %v", lapsed, want)
	}
	if limit := byID["personal"].Limit; !limit.Until.Equal(on(now, 1, 15, 54)) {
		t.Errorf("personal's limit is %+v, want it holding until 15:54", limit)
	}
	if got := status.Over(byID["work"].Pressure.Since, now); got != "last 30 min" {
		t.Errorf("work's pressure is measured over the %s, want the last 30 min", got)
	}
}

// projection words where a window is heading, p, as the frames do, times in
// now's time zone.
func projection(now time.Time, p score.Projection) string {
	switch p.Kind {
	case score.OnPace:
		return "on pace for " + status.Percent(p.AtReset)
	case score.RunsOut:
		return "runs out ~" + status.Clock(now, p.At)
	case score.Exhausted:
		return "exhausted"
	}
	return ""
}

// projected is where account a's window with the given key is heading at now,
// read then, as its moment has it.
func projected(a status.Account, key string, now time.Time) score.Projection {
	at, _ := status.Document{GeneratedAt: now, Accounts: []status.Account{a}}.At(now, claude.Policy).Account(a.ID)
	w, _ := at.Window(key)
	return w.Out.Projection
}
