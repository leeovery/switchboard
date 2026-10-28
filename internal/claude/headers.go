package claude

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unique"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
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
// limits, the answer's usage headers as limitsOf kept them, each by its
// <key>-utilization, <key>-reset and <key>-status, read as ParseUsage reads
// them off the headers, in quota.Sort's order.
func LimitWindows(limits map[string]string) []quota.Window {
	var windows []quota.Window
	for name, utilization := range limits {
		key, ok := strings.CutSuffix(name, "-utilization")
		if !ok || key == overageKey {
			continue
		}
		f := fields{utilization: utilization, reset: limits[key+"-reset"], status: limits[key+"-status"]}
		if w, ok := parseWindow(key, f); ok {
			windows = append(windows, w)
		}
	}
	quota.Sort(windows)
	return windows
}

// WindowRead is a window as a past answer read it: its key; its share used;
// when it resets, zero where the answer didn't say; and how far it held its
// account back.
type WindowRead struct {
	Key    string
	Use    float64
	Resets time.Time
	Held   score.Hold
}

// WindowsRead returns the windows a request ledger's line gives in its
// limits, as LimitWindows reads them, in its order, each as WindowRead has
// it: how far it held its account back as score.HoldOf judges it, the
// account's reserve the one given.
func WindowsRead(limits map[string]string, reserve float64) []WindowRead {
	return windowsRead(LimitWindows(limits), reserve)
}

// WindowsReadOf returns the windows kept, as KeptWindows keeps them of a
// line's limits, each as LimitWindows reads it, in their order, as
// WindowsRead has them.
func WindowsReadOf(kept []KeptWindow, reserve float64) []WindowRead {
	windows := make([]quota.Window, len(kept))
	for i, k := range kept {
		windows[i] = k.Window()
	}
	return windowsRead(windows, reserve)
}

// windowsRead are the windows given, in their order, each as WindowRead has
// it, the account's reserve the one given.
func windowsRead(windows []quota.Window, reserve float64) []WindowRead {
	var read []WindowRead
	for _, w := range windows {
		read = append(read, WindowRead{Key: w.Key, Use: w.Utilization, Resets: w.ResetsAt, Held: score.HoldOf(w, reserve)})
	}
	return read
}

// KeptWindow is a window a request ledger's line gives in its limits, as a
// reader that holds the line keeps it, decoded once and small: its key and
// status, interned, so every line's share one copy; its share used; and
// when it resets, in seconds since the epoch, as the header gives it, the
// zero time's where it didn't say.
type KeptWindow struct {
	key    unique.Handle[string]
	status unique.Handle[quota.Status]
	use    float64
	resets int64
}

// KeptWindows returns the windows a request ledger's line gives in its
// limits, as LimitWindows reads them, in its order, each kept as KeptWindow
// keeps it.
func KeptWindows(limits map[string]string) []KeptWindow {
	windows := LimitWindows(limits)
	if len(windows) == 0 {
		return nil
	}
	kept := make([]KeptWindow, len(windows))
	for i, w := range windows {
		kept[i] = KeptWindow{key: unique.Make(w.Key), status: unique.Make(w.Status), use: w.Utilization, resets: w.ResetsAt.Unix()}
	}
	return kept
}

// Window is the window k keeps, as LimitWindows reads it.
func (k KeptWindow) Window() quota.Window {
	key := k.key.Value()
	w := quota.Window{Key: key, Label: WindowLabel(key), Utilization: k.use, Status: k.status.Value()}
	if k.resets != unsaidReset {
		w.ResetsAt = time.Unix(k.resets, 0).UTC()
	}
	return w
}

// unsaidReset is when a window resets, kept as KeptWindow keeps it, where
// the header didn't say: the zero time's.
var unsaidReset = time.Time{}.Unix()

// storedWindow is a window kept as a stored form holds it, as MarshalJSON
// writes it and UnmarshalJSON reads it back: its key, its status, its share
// used, and when it resets, in seconds since the epoch, none where the header
// didn't say. Its keys are short and its zeros left out.
type storedWindow struct {
	Key    string       `json:"k,omitempty"`
	Status quota.Status `json:"s,omitempty"`
	Use    float64      `json:"u,omitzero"`
	Resets *int64       `json:"r,omitempty"`
}

// MarshalJSON writes k in its stored form.
func (k KeptWindow) MarshalJSON() ([]byte, error) {
	stored := storedWindow{Key: k.key.Value(), Status: k.status.Value(), Use: k.use}
	if k.resets != unsaidReset {
		stored.Resets = &k.resets
	}
	b, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("encode a window kept: %w", err)
	}
	return b, nil
}

// UnmarshalJSON reads k back from its stored form, as MarshalJSON wrote it:
// the window kept it was, its key and status interned again.
func (k *KeptWindow) UnmarshalJSON(b []byte) error {
	var stored storedWindow
	if err := json.Unmarshal(b, &stored); err != nil {
		return fmt.Errorf("decode a window kept: %w", err)
	}
	*k = KeptWindow{key: unique.Make(stored.Key), status: unique.Make(stored.Status), use: stored.Use, resets: unsaidReset}
	if stored.Resets != nil {
		k.resets = *stored.Resets
	}
	return nil
}

// LimitStatus returns the status a request ledger's line gives in its limits,
// the answer's usage headers as limitsOf kept them: the API's verdict on the
// request across every window, as the overall status gave it, "" where the
// answer gave none.
func LimitStatus(limits map[string]string) quota.Status {
	return quota.Status(limits[strings.TrimPrefix(overallStatus, limitsPrefix)])
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

// organizationHeader names the organization an answer's account is in.
const organizationHeader = "anthropic-organization-id"

// organizationOf is the id of the organization a Claude API response's headers
// name its account in: "" where they name none.
func organizationOf(h http.Header) string {
	return h.Get(organizationHeader)
}

// WindowLabel is the human name of the window Claude reports by the key
// given, as Session is 5h's: the key itself for one it doesn't name.
func WindowLabel(key string) string {
	if label, ok := windowLabels[key]; ok {
		return label
	}
	return key
}
