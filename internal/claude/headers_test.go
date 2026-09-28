package claude_test

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
)

func TestParseWindows(t *testing.T) {
	sessionReset := time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC)
	weekReset := time.Date(2026, 10, 2, 21, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		header http.Header
		want   []quota.Window
	}{
		{
			name: "every field of every window, shortest window first",
			header: header(
				"anthropic-ratelimit-unified-7d-utilization", "0.93",
				"anthropic-ratelimit-unified-7d-reset", "1790974800",
				"anthropic-ratelimit-unified-7d-status", "allowed_warning",
				"anthropic-ratelimit-unified-5h-utilization", "0.23",
				"anthropic-ratelimit-unified-5h-reset", "1790619000",
				"anthropic-ratelimit-unified-5h-status", "allowed",
			),
			want: []quota.Window{
				{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: sessionReset, Status: quota.StatusAllowed},
				{Key: "7d", Label: "Week", Utilization: 0.93, ResetsAt: weekReset, Status: quota.StatusAllowedWarning},
			},
		},
		{
			name: "every known label",
			header: header(
				"anthropic-ratelimit-unified-5h-utilization", "0.1",
				"anthropic-ratelimit-unified-7d-utilization", "0.2",
				"anthropic-ratelimit-unified-7d_oi-utilization", "0.3",
				"anthropic-ratelimit-unified-7d_opus-utilization", "0.4",
				"anthropic-ratelimit-unified-7d_sonnet-utilization", "0.5",
			),
			want: []quota.Window{
				{Key: "5h", Label: "Session", Utilization: 0.1},
				{Key: "7d", Label: "Week", Utilization: 0.2},
				{Key: "7d_oi", Label: "Fable week", Utilization: 0.3},
				{Key: "7d_opus", Label: "Opus week", Utilization: 0.4},
				{Key: "7d_sonnet", Label: "Sonnet week", Utilization: 0.5},
			},
		},
		{
			name: "unknown windows discovered and labelled by key, unreadable lengths last",
			header: header(
				"anthropic-ratelimit-unified-burst-utilization", "0.7",
				"anthropic-ratelimit-unified-30d-utilization", "0.6",
				"anthropic-ratelimit-unified-7d-utilization", "0.2",
				"anthropic-ratelimit-unified-12h-utilization", "1.25",
				"anthropic-ratelimit-unified-12h-status", "rejected",
			),
			want: []quota.Window{
				{Key: "12h", Label: "12h", Utilization: 1.25, Status: quota.StatusRejected},
				{Key: "7d", Label: "Week", Utilization: 0.2},
				{Key: "30d", Label: "30d", Utilization: 0.6},
				{Key: "burst", Label: "burst", Utilization: 0.7},
			},
		},
		{
			name: "overage ignored, and the headers that aren't windows",
			header: header(
				"anthropic-ratelimit-unified-overage-utilization", "0.5",
				"anthropic-ratelimit-unified-overage-reset", "1790619000",
				"anthropic-ratelimit-unified-overage-status", "rejected",
				"anthropic-ratelimit-unified-overage-disabled-reason", "out_of_credits",
				"anthropic-ratelimit-unified-status", "allowed",
				"anthropic-ratelimit-unified-reset", "1790619000",
				"anthropic-ratelimit-unified-representative-claim", "five_hour",
				"anthropic-ratelimit-unified-fallback-percentage", "0.5",
				"anthropic-ratelimit-requests-remaining", "10",
				"anthropic-ratelimit-unified-5h-utilization", "0.23",
			),
			want: []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.23}},
		},
		{
			name: "windows without a readable utilization dropped",
			header: header(
				"anthropic-ratelimit-unified-5h-reset", "1790619000",
				"anthropic-ratelimit-unified-5h-status", "allowed",
				"anthropic-ratelimit-unified-7d-utilization", "",
				"anthropic-ratelimit-unified-7d_oi-utilization", "high",
				"anthropic-ratelimit-unified-7d_opus-utilization", "NaN",
				"anthropic-ratelimit-unified-7d_sonnet-utilization", "+Inf",
				"anthropic-ratelimit-unified-30d-utilization", "0",
			),
			want: []quota.Window{{Key: "30d", Label: "30d", Utilization: 0}},
		},
		{
			name: "unreadable reset left unknown",
			header: header(
				"anthropic-ratelimit-unified-5h-utilization", "0.23",
				"anthropic-ratelimit-unified-5h-reset", "tomorrow",
			),
			want: []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.23}},
		},
		{
			name: "mixed-case header names",
			header: http.Header{
				"ANTHROPIC-RATELIMIT-UNIFIED-5H-UTILIZATION":      {"0.23"},
				"Anthropic-RateLimit-Unified-5h-Reset":            {"1790619000"},
				"anthropic-ratelimit-unified-5h-STATUS":           {"allowed"},
				"Anthropic-Ratelimit-Unified-7D_OI-Utilization":   {"0.05"},
				"anthropic-ratelimit-unified-OVERAGE-utilization": {"0.5"},
			},
			want: []quota.Window{
				{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: sessionReset, Status: quota.StatusAllowed},
				{Key: "7d_oi", Label: "Fable week", Utilization: 0.05},
			},
		},
		{
			name:   "no usage headers",
			header: header("content-type", "application/json"),
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claude.ParseWindows(tt.header); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseWindows() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

// header builds a response header from name, value pairs.
func header(pairs ...string) http.Header {
	h := http.Header{}
	for i := 0; i < len(pairs); i += 2 {
		h.Set(pairs[i], pairs[i+1])
	}
	return h
}
