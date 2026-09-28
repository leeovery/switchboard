package watch

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/logs/logstest"
)

func TestLogsEachRead(t *testing.T) {
	log := logstest.Capture(t)
	h := newHarness(t, document(
		account("work", "Work", session(0.25, 3*time.Hour), week(0.5)),
		partlyRead("personal", "Personal"),
		unreadable("side", "Side"),
	))
	h.start()

	want := []string{"level=INFO", `msg="usage read" component=watch`, "read=work,personal failed=side", "next=" + logged(at(13, 14, 0))}
	if !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}

	h.source.err = errors.New("connection refused")
	h.clock.now = at(13, 14, 0)
	h.read(calm())
	want = []string{"level=WARN", `msg="usage read failed" component=watch`, `error="connection refused"`, "next=" + logged(at(13, 18, 0))}
	if !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
}

func TestLogsTheRefreshKey(t *testing.T) {
	log := logstest.Capture(t)
	h := newHarness(t, calm())
	h.start()

	pending := h.press("r")
	h.press("r")
	h.deliver(pending...)
	for _, want := range [][]string{
		{"level=DEBUG", `msg="refresh key pressed" component=watch`, "already_reading=false"},
		{"level=DEBUG", `msg="refresh key pressed" component=watch`, "already_reading=true"},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
}

func TestLogsWhenAWatchStartsAndStops(t *testing.T) {
	log := logstest.Capture(t)
	clock := &fakeClock{now: start}
	cfg := Config{Source: &fakeSource{doc: calm()}, Notifier: &fakeNotifier{}, Now: clock.Now, Interval: interval, Policy: policy, Size: Size{Width: 80, Height: 24}}

	err := run(t.Context(), cfg, tea.WithInput(strings.NewReader("q")), tea.WithOutput(io.Discard), tea.WithoutSignals())
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	for _, want := range [][]string{
		{"level=INFO", `msg="watch started" component=watch`, "interval=30m0s"},
		{"level=INFO", `msg="watch stopped" component=watch`, "ran="},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
}

// logged is how the log shows t: in the local zone.
func logged(t time.Time) string {
	return t.Local().Format("2006-01-02T15:04:05.000-07:00")
}
