package claude

import (
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

// windowHeader matches a lowercased usage header: its window key, then its field.
var windowHeader = regexp.MustCompile(`^anthropic-ratelimit-unified-(.+)-(utilization|reset|status)$`)

// overageKey names the extra-usage state, which the headers report like a window.
const overageKey = "overage"

// windowLabels are the human names of the windows Claude reports.
var windowLabels = map[string]string{
	"5h":        "Session",
	"7d":        "Week",
	"7d_oi":     "Fable week",
	"7d_opus":   "Opus week",
	"7d_sonnet": "Sonnet week",
}

// ParseWindows reads the usage windows off a Claude API response's headers,
// in quota.Sort's order. It takes every window the headers report rather than
// a fixed list, so a new cap shows up without a code change. A window without
// a readable utilization is dropped.
func ParseWindows(h http.Header) []quota.Window {
	var windows []quota.Window
	for key, fields := range windowFields(h) {
		if w, ok := parseWindow(key, fields); ok {
			windows = append(windows, w)
		}
	}
	quota.Sort(windows)
	return windows
}

// windowFields groups the usage headers by window key, then by field.
func windowFields(h http.Header) map[string]map[string]string {
	windows := make(map[string]map[string]string)
	for name, values := range h {
		m := windowHeader.FindStringSubmatch(strings.ToLower(name))
		if m == nil || m[1] == overageKey || len(values) == 0 {
			continue
		}
		key, field := m[1], m[2]
		if windows[key] == nil {
			windows[key] = make(map[string]string)
		}
		windows[key][field] = values[0]
	}
	return windows
}

// parseWindow reports false when the window's utilization is missing, or isn't
// a finite number (which JSON couldn't carry).
func parseWindow(key string, fields map[string]string) (quota.Window, bool) {
	utilization, err := strconv.ParseFloat(fields["utilization"], 64)
	if err != nil || math.IsNaN(utilization) || math.IsInf(utilization, 0) {
		return quota.Window{}, false
	}
	return quota.Window{
		Key:         key,
		Label:       windowLabel(key),
		Utilization: utilization,
		ResetsAt:    parseReset(fields["reset"]),
		Status:      quota.Status(fields["status"]),
	}, true
}

// parseReset reads a time given in Unix seconds, returning zero when it can't.
func parseReset(value string) time.Time {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(seconds, 0).UTC()
}

func windowLabel(key string) string {
	if label, ok := windowLabels[key]; ok {
		return label
	}
	return key
}
