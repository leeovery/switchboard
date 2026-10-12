package capture

import (
	"testing"
	"time"
)

// Every fixture pins the traces' scale at 16 requests a minute, as the frames
// were drawn with it, where the live page scales as its rule says.
func TestAFixturePinsTheTracesScaleAsTheFramesDo(t *testing.T) {
	now := func() time.Time { return localMoment() }
	for _, f := range []Fixture{fixtures(time.UTC)[0], {records: &records{}}} {
		if got := f.config(wide(37), now).TraceScale; got != 16 {
			t.Errorf("%q has the watch scale its traces at %v requests a minute, want 16", f.Name, got)
		}
	}
}

// A fixture names the Overview's windows as the dashboard does, so its
// windows are named before any account's are read.
func TestAFixtureNamesTheWindowsAsTheDashboardDoes(t *testing.T) {
	now := func() time.Time { return localMoment() }
	name := fixtures(time.UTC)[0].config(wide(37), now).WindowName
	if name == nil {
		t.Fatal("the fixture's watch names no window, want Claude's names")
	}
	for key, want := range map[string]string{"5h": "5-hour", "7d": "Week"} {
		if got := name(key); got != want {
			t.Errorf("the fixture's watch names window %s %q, want %q", key, got, want)
		}
	}
}
