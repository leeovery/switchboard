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

// limitsPrefix begins the name of each of the usage headers, lowercased.
const limitsPrefix = "anthropic-ratelimit-unified-"

// usageHeader matches a lowercased usage header: its window key, then its
// field.
var usageHeader = regexp.MustCompile(`^` + limitsPrefix + `(.+)-(utilization|reset|status)$`)

// overageKey names extra usage, which the headers report like a window,
// though it isn't one.
const overageKey = "overage"

// windowLabels are the human names of the windows Claude reports.
var windowLabels = map[string]string{
	"5h":        "Session",
	"7d":        "Week",
	"7d_oi":     "Fable week",
	"7d_opus":   "Opus week",
	"7d_sonnet": "Sonnet week",
}

// ParseUsage reads an account's usage off a Claude API response's headers:
// its windows, in quota.Sort's order, and its extra usage. It takes every
// window the headers report rather than a fixed list, so a new cap shows up
// without a code change. A window without a readable utilization is dropped,
// and a field of extra usage missing or unreadable is left zero, the rest
// read all the same.
func ParseUsage(h http.Header) quota.Usage {
	windows, overage := usageFields(h)
	var usage quota.Usage
	for key, f := range windows {
		if w, ok := parseWindow(key, f); ok {
			usage.Windows = append(usage.Windows, w)
		}
	}
	quota.Sort(usage.Windows)
	usage.Extra = parseExtra(overage)
	return usage
}

// LimitWindows returns the windows a request ledger's line gives in its
// limits, the answer's usage headers as limitsOf kept them, read as
// ParseUsage reads them off the headers.
func LimitWindows(limits map[string]string) []quota.Window {
	h := make(http.Header, len(limits))
	for name, value := range limits {
		h[limitsPrefix+name] = []string{value}
	}
	return ParseUsage(h).Windows
}

// limitsOf returns each of the usage headers h holds, by its name lowercased,
// limitsPrefix taken off, and its value as given: of a window, as
// 5h-utilization, and of the answer as a whole, as status. It's nil where h
// holds none.
func limitsOf(h http.Header) map[string]string {
	var limits map[string]string
	for name, values := range h {
		limit := len(name) > len(limitsPrefix) && strings.EqualFold(name[:len(limitsPrefix)], limitsPrefix)
		if !limit || len(values) == 0 {
			continue
		}
		if limits == nil {
			limits = make(map[string]string)
		}
		limits[strings.ToLower(name[len(limitsPrefix):])] = values[0]
	}
	return limits
}

// fields are the usage headers of a window, or of extra usage, as given.
type fields struct {
	utilization, reset, status string
}

// set sets the field named to value.
func (f *fields) set(field, value string) {
	switch field {
	case "utilization":
		f.utilization = value
	case "reset":
		f.reset = value
	case "status":
		f.status = value
	}
}

// usageFields groups the usage headers: the windows' by key, and extra
// usage's.
func usageFields(h http.Header) (windows map[string]fields, overage fields) {
	windows = make(map[string]fields)
	for name, values := range h {
		m := usageHeader.FindStringSubmatch(strings.ToLower(name))
		if m == nil || len(values) == 0 {
			continue
		}
		key, field := m[1], m[2]
		if key == overageKey {
			overage.set(field, values[0])
			continue
		}
		f := windows[key]
		f.set(field, values[0])
		windows[key] = f
	}
	return windows, overage
}

// windowFields groups the windows' usage headers by key.
func windowFields(h http.Header) map[string]fields {
	windows, _ := usageFields(h)
	return windows
}

// parseWindow reports false when the window's utilization is missing, or
// can't be read, as parseFraction says.
func parseWindow(key string, f fields) (quota.Window, bool) {
	utilization, ok := parseFraction(f.utilization)
	if !ok {
		return quota.Window{}, false
	}
	return quota.Window{
		Key:         key,
		Label:       WindowLabel(key),
		Utilization: utilization,
		ResetsAt:    parseReset(f.reset),
		Status:      quota.Status(f.status),
	}, true
}

// parseExtra reads extra usage's fields, leaving out each that's missing or
// can't be read.
func parseExtra(f fields) quota.ExtraUsage {
	extra := quota.ExtraUsage{Status: quota.Status(f.status), ResetsAt: parseReset(f.reset)}
	if utilization, ok := parseFraction(f.utilization); ok {
		// Copied, so the heap allocation its address costs, which escape
		// analysis puts at the variable's declaration, comes only with a value.
		used := utilization
		extra.Utilization = &used
	}
	return extra
}

// parseFraction reads a fraction used, reporting false when it's missing, or
// isn't a finite number (which JSON couldn't carry).
func parseFraction(value string) (float64, bool) {
	if value == "" {
		return 0, false
	}
	fraction, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(fraction) || math.IsInf(fraction, 0) {
		return 0, false
	}
	return fraction, true
}

// parseReset reads a time given in Unix seconds, returning zero when it can't.
// A missing value, as every answer without extra usage gives its overage, is
// passed over before strconv, which allocates the error that fails it, on
// every answer's path.
func parseReset(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(seconds, 0).UTC()
}

// WindowLabel is the human name of the window Claude reports by the key
// given, as Session is 5h's: the key itself for one it doesn't name.
func WindowLabel(key string) string {
	if label, ok := windowLabels[key]; ok {
		return label
	}
	return key
}
