package router

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/status"
)

func TestAssess(t *testing.T) {
	// answered are n requests answered ago before start, failed or not.
	answered := func(n int, ago time.Duration, failed bool) []result {
		return slices.Repeat([]result{{at: start.Add(-ago), failed: failed}}, n)
	}
	tests := []struct {
		name    string
		results []result
		want    status.Health
	}{
		{
			name: "healthy before any request",
			want: status.Health{Healthy: true},
		},
		{
			name:    "healthy when every request was served",
			results: answered(20, time.Minute, false),
			want:    status.Health{Healthy: true, Requests: 20},
		},
		{
			name:    "healthy with four failures, whatever their share",
			results: answered(4, time.Minute, true),
			want:    status.Health{Healthy: true, Requests: 4, Failures: 4},
		},
		{
			name:    "unhealthy with five failures of five",
			results: answered(5, time.Minute, true),
			want:    status.Health{Requests: 5, Failures: 5, Reason: "5 of the 5 requests in the last 5 minutes failed"},
		},
		{
			name:    "unhealthy with five failures of ten, half",
			results: slices.Concat(answered(5, time.Minute, true), answered(5, time.Minute, false)),
			want:    status.Health{Requests: 10, Failures: 5, Reason: "5 of the 10 requests in the last 5 minutes failed"},
		},
		{
			name:    "healthy with five failures of eleven, under half",
			results: slices.Concat(answered(5, time.Minute, true), answered(6, time.Minute, false)),
			want:    status.Health{Healthy: true, Requests: 11, Failures: 5},
		},
		{
			name:    "counting failures just inside the window",
			results: answered(5, 5*time.Minute-time.Nanosecond, true),
			want:    status.Health{Requests: 5, Failures: 5, Reason: "5 of the 5 requests in the last 5 minutes failed"},
		},
		{
			name:    "forgetting those that have left it",
			results: slices.Concat(answered(9, 5*time.Minute, true), answered(1, 0, false)),
			want:    status.Health{Healthy: true, Requests: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := assess(tt.results, start); got != tt.want {
				t.Errorf("assess() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestHealthLogsAndAnnouncesEachChangeOnce(t *testing.T) {
	log := logstest.Capture(t)
	clock := &testClock{now: start}
	var heard []Event
	h := newHealth(clock.read, func(e Event) { heard = append(heard, e) })

	for range 5 {
		h.record(clock.now, true)
	}
	h.record(clock.now, false)
	if got := h.report(); got.Healthy {
		t.Fatalf("after five failures of six, report() = %+v, want unhealthy", got)
	}
	clock.now = start.Add(5 * time.Minute)
	if got := h.report(); !got.Healthy || got.Requests != 0 {
		t.Errorf("five minutes on, report() = %+v, want healthy, with nothing in the window", got)
	}
	h.report()

	want := []Event{
		HealthChanged{Reason: "5 of the 5 requests in the last 5 minutes failed"},
		HealthChanged{Healthy: true},
	}
	if !reflect.DeepEqual(heard, want) {
		t.Errorf("events = %+v, want %+v", heard, want)
	}
	for _, want := range [][]string{
		{"level=WARN", "msg=unhealthy", `reason="5 of the 5 requests in the last 5 minutes failed"`},
		{"level=INFO", `msg="healthy again"`, "requests=0", "failures=0"},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
	if n := len(log.Lines()); n != 3 {
		t.Errorf("log reads\n%s\nwant the start and each change alone", log)
	}
}

func TestHealthForgetsWhatHasLeftTheWindow(t *testing.T) {
	clock := &testClock{now: start}
	h := newHealth(clock.read, func(Event) {})
	for range 100 {
		h.record(clock.now, false)
	}

	clock.now = start.Add(5 * time.Minute)
	h.record(clock.now, true)
	if n := len(h.results); n != 1 {
		t.Errorf("health holds %d requests, want the one still in the window", n)
	}
}
