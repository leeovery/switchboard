package watch

import (
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	tests := []struct {
		name         string
		failures     int
		failed       bool
		interval     time.Duration
		wantFailures int
		wantWait     time.Duration
	}{
		{name: "a read that didn't fail", failures: 0, failed: false, interval: 30 * time.Minute, wantFailures: 0, wantWait: 30 * time.Minute},
		{name: "a first failure", failures: 0, failed: true, interval: 30 * time.Minute, wantFailures: 1, wantWait: 2 * time.Minute},
		{name: "a second failure in a row", failures: 1, failed: true, interval: 30 * time.Minute, wantFailures: 2, wantWait: 4 * time.Minute},
		{name: "a third", failures: 2, failed: true, interval: 30 * time.Minute, wantFailures: 3, wantWait: 8 * time.Minute},
		{name: "a fourth", failures: 3, failed: true, interval: 30 * time.Minute, wantFailures: 4, wantWait: 16 * time.Minute},
		{name: "a fifth, capped at the interval", failures: 4, failed: true, interval: 30 * time.Minute, wantFailures: 5, wantWait: 30 * time.Minute},
		{name: "long past the cap", failures: 400, failed: true, interval: 30 * time.Minute, wantFailures: 401, wantWait: 30 * time.Minute},
		{name: "a fifth under a longer interval", failures: 4, failed: true, interval: time.Hour, wantFailures: 5, wantWait: 32 * time.Minute},
		{name: "a sixth, capped at the longer interval", failures: 5, failed: true, interval: time.Hour, wantFailures: 6, wantWait: time.Hour},
		{name: "a third, capped at the shortest interval", failures: 2, failed: true, interval: 5 * time.Minute, wantFailures: 3, wantWait: 5 * time.Minute},
		{name: "a read that didn't fail after failures starts again", failures: 7, failed: false, interval: 30 * time.Minute, wantFailures: 0, wantWait: 30 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failures, wait := backoff(tt.failures, tt.failed, tt.interval)
			if failures != tt.wantFailures || wait != tt.wantWait {
				t.Errorf("backoff(%d, %v, %v) = %d, %v; want %d, %v", tt.failures, tt.failed, tt.interval, failures, wait, tt.wantFailures, tt.wantWait)
			}
		})
	}
}

func TestTickDelay(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want time.Duration
	}{
		{name: "on the second", now: at(13, 12, 0), want: time.Second + tickSlack},
		{name: "part way through a second", now: at(13, 12, 30).Add(300 * time.Millisecond), want: 700*time.Millisecond + tickSlack},
		{name: "just past the second", now: at(13, 12, 0).Add(tickSlack), want: time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tickDelay(tt.now); got != tt.want {
				t.Errorf("tickDelay() = %v, want %v", got, tt.want)
			}
		})
	}
}
