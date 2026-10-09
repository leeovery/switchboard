package claude_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
)

func TestAWindowIsNamedByItsLabelOrElseItsKey(t *testing.T) {
	for key, want := range map[string]string{"5h": "Session", "7d": "Week", "7d_oi": "Fable week", "7d_new": "7d_new"} {
		if got := claude.WindowLabel(key); got != want {
			t.Errorf("WindowLabel(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestAWindowIsNamedByItsHoursOrElseItsLabel(t *testing.T) {
	tests := []struct{ key, want string }{
		{key: "5h", want: "5-hour"},
		{key: "1h", want: "1-hour"},
		{key: "7d", want: "Week"},
		{key: "7d_oi", want: "Fable week"},
		{key: "7d_opus", want: "Opus week"},
		{key: "90m", want: "90m"},
		{key: "7d_new", want: "7d_new"},
	}
	for _, tt := range tests {
		if got := claude.WindowName(tt.key); got != tt.want {
			t.Errorf("WindowName(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestParseUsageReadsTheWindows(t *testing.T) {
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
			name: "extra usage never a window, nor the headers that aren't windows",
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
			if got := claude.ParseUsage(tt.header).Windows; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseUsage().Windows =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestParseUsageReadsExtraUsage(t *testing.T) {
	reset := time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC)
	tests := []struct {
		name   string
		header http.Header
		want   quota.ExtraUsage
	}{
		{
			name: "every field given",
			header: header(
				"anthropic-ratelimit-unified-overage-status", "allowed_warning",
				"anthropic-ratelimit-unified-overage-utilization", "0.42",
				"anthropic-ratelimit-unified-overage-reset", "1790619000",
			),
			want: quota.ExtraUsage{Status: quota.StatusAllowedWarning, Utilization: new(0.42), ResetsAt: reset},
		},
		{
			name: "none used, which is given",
			header: header(
				"anthropic-ratelimit-unified-overage-status", "allowed",
				"anthropic-ratelimit-unified-overage-utilization", "0",
			),
			want: quota.ExtraUsage{Status: quota.StatusAllowed, Utilization: new(0.0)},
		},
		{
			name:   "the status alone, as an account without extra usage gives it",
			header: header("anthropic-ratelimit-unified-overage-status", "rejected"),
			want:   quota.ExtraUsage{Status: quota.StatusRejected},
		},
		{
			name:   "the utilization alone",
			header: header("anthropic-ratelimit-unified-overage-utilization", "1.25"),
			want:   quota.ExtraUsage{Utilization: new(1.25)},
		},
		{
			name:   "the reset alone",
			header: header("anthropic-ratelimit-unified-overage-reset", "1790619000"),
			want:   quota.ExtraUsage{ResetsAt: reset},
		},
		{
			name:   "a status as given, though no window's is so",
			header: header("anthropic-ratelimit-unified-overage-status", "paused"),
			want:   quota.ExtraUsage{Status: "paused"},
		},
		{
			name: "an unreadable utilization left out, the rest read",
			header: header(
				"anthropic-ratelimit-unified-overage-status", "allowed",
				"anthropic-ratelimit-unified-overage-utilization", "high",
				"anthropic-ratelimit-unified-overage-reset", "1790619000",
			),
			want: quota.ExtraUsage{Status: quota.StatusAllowed, ResetsAt: reset},
		},
		{
			name:   "a utilization that isn't finite left out",
			header: header("anthropic-ratelimit-unified-overage-utilization", "NaN"),
			want:   quota.ExtraUsage{},
		},
		{
			name:   "an infinite utilization left out",
			header: header("anthropic-ratelimit-unified-overage-utilization", "+Inf"),
			want:   quota.ExtraUsage{},
		},
		{
			name: "an unreadable reset left out, the rest read",
			header: header(
				"anthropic-ratelimit-unified-overage-status", "allowed",
				"anthropic-ratelimit-unified-overage-utilization", "0.42",
				"anthropic-ratelimit-unified-overage-reset", "tomorrow",
			),
			want: quota.ExtraUsage{Status: quota.StatusAllowed, Utilization: new(0.42)},
		},
		{
			name:   "a reset in a form the windows' never come in left out",
			header: header("anthropic-ratelimit-unified-overage-reset", "2026-09-28T18:10:00Z"),
			want:   quota.ExtraUsage{},
		},
		{
			name: "empty fields left out",
			header: header(
				"anthropic-ratelimit-unified-overage-status", "",
				"anthropic-ratelimit-unified-overage-utilization", "",
				"anthropic-ratelimit-unified-overage-reset", "",
			),
			want: quota.ExtraUsage{},
		},
		{
			name: "mixed-case header names",
			header: http.Header{
				"ANTHROPIC-RATELIMIT-UNIFIED-OVERAGE-STATUS":      {"allowed"},
				"Anthropic-Ratelimit-Unified-Overage-Utilization": {"0.42"},
				"anthropic-ratelimit-unified-OVERAGE-reset":       {"1790619000"},
			},
			want: quota.ExtraUsage{Status: quota.StatusAllowed, Utilization: new(0.42), ResetsAt: reset},
		},
		{
			name: "the headers that aren't its fields passed over",
			header: header(
				"anthropic-ratelimit-unified-overage-disabled-reason", "out_of_credits",
				"anthropic-ratelimit-unified-status", "rejected",
				"anthropic-ratelimit-unified-reset", "1790619000",
				"anthropic-ratelimit-unified-5h-utilization", "0.23",
				"anthropic-ratelimit-unified-5h-status", "allowed",
			),
			want: quota.ExtraUsage{},
		},
		{
			name:   "no usage headers",
			header: header("content-type", "application/json"),
			want:   quota.ExtraUsage{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claude.ParseUsage(tt.header).Extra; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseUsage().Extra = %s, want %s", extraText(got), extraText(tt.want))
			}
		})
	}
}

func TestExtraUsageUnreadableNeverFailsTheWindows(t *testing.T) {
	got := claude.ParseUsage(header(
		"anthropic-ratelimit-unified-overage-status", "",
		"anthropic-ratelimit-unified-overage-utilization", "high",
		"anthropic-ratelimit-unified-overage-reset", "tomorrow",
		"anthropic-ratelimit-unified-5h-utilization", "0.23",
		"anthropic-ratelimit-unified-5h-reset", "1790619000",
		"anthropic-ratelimit-unified-5h-status", "allowed",
	))
	want := quota.Usage{Windows: []quota.Window{{
		Key: "5h", Label: "Session", Utilization: 0.23,
		ResetsAt: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC), Status: quota.StatusAllowed,
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseUsage() = %+v, want %+v", got, want)
	}
}

// extraText gives extra usage as JSON, its utilization's value rather than
// its address.
func extraText(e quota.ExtraUsage) string {
	data, err := json.Marshal(e)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

// header builds a response header from name, value pairs.
func header(pairs ...string) http.Header {
	h := http.Header{}
	for i := 0; i < len(pairs); i += 2 {
		h.Set(pairs[i], pairs[i+1])
	}
	return h
}
