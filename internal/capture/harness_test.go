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
