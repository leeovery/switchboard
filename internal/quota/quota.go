// Package quota is the provider-neutral usage model: the windows an account's
// quota is measured over, the ones a provider expected to read but couldn't,
// and what a response says of the account it came from.
package quota

import (
	"cmp"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"time"
)

// Status is a provider's verdict on a window.
type Status string

// The statuses a window can carry. A provider may also give none.
const (
	StatusAllowed        Status = "allowed"
	StatusAllowedWarning Status = "allowed_warning"
	StatusRejected       Status = "rejected"
)

// Window is how much of one quota window an account has used.
type Window struct {
	// Key names the window, such as "5h", "7d" or "7d_oi".
	Key string `json:"key"`
	// Label is the window's human name, filled in by the provider.
	Label string `json:"label"`
	// Utilization is the fraction used: normally 0 to 1, but it can exceed 1.
	Utilization float64 `json:"utilization"`
	// ResetsAt is zero when the provider didn't say.
	ResetsAt time.Time `json:"resets_at,omitzero"`
	Status   Status    `json:"status,omitempty"`
}

// Failure is a window the provider expected to read but couldn't. Reporting it
// keeps a broken read distinct from an account that has no such window.
type Failure struct {
	// Label names what should have reported the window, such as a model family.
	Label string `json:"label"`
	// Window is the key of the window that went unread.
	Window string `json:"window"`
	Error  string `json:"error"`
}

// Usage is one reading of an account's quota.
type Usage struct {
	// Windows are in Sort's order.
	Windows  []Window  `json:"windows,omitempty"`
	Failures []Failure `json:"failures,omitempty"`
}

// Probe is what probing an account read: its usage, and which models'
// responses reported each window, which says whose requests a window counts.
type Probe struct {
	Usage
	// Models are the models whose responses reported each window, by the
	// window's key.
	Models map[string][]string `json:"models,omitempty"`
}

// Verdict is what a response to a request says of the account it went out on.
type Verdict int

const (
	// Served says nothing against the account, whatever the response's
	// status: the response is the client's, as it came.
	Served Verdict = iota
	// LimitReached is the account out of quota in a window the request counts
	// against.
	LimitReached
	// Throttled is the account asked to slow down, with quota to spare.
	Throttled
	// Refused is the account's token refused.
	Refused
)

// Outcome is a response's verdict on the account a request went out on.
type Outcome struct {
	Verdict Verdict
	// RetryAfter is how long a throttled account is asked to wait before
	// sending again, or zero when the response doesn't say.
	RetryAfter time.Duration
	// LimitedUntil is when an account whose limit is reached has room again,
	// as far as the response says, or zero when it doesn't say.
	LimitedUntil time.Time
}

var lengthPattern = regexp.MustCompile(`^([1-9][0-9]*)([hd])(?:_|$)`)

// Length returns how long the window named key lasts, read from the "<n>h" or
// "<n>d" that makes up the key or leads it before an underscore: "5h" is five
// hours and "7d_oi" seven days. It reports false for any other key.
func Length(key string) (time.Duration, bool) {
	m := lengthPattern.FindStringSubmatch(key)
	if m == nil {
		return 0, false
	}
	unit := time.Hour
	if m[2] == "d" {
		unit = 24 * time.Hour
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || n > int64(math.MaxInt64/unit) {
		return 0, false
	}
	return time.Duration(n) * unit, true
}

// Sort orders windows shortest first, then by key, with windows whose length
// can't be read from their key last: 5h, 7d, 7d_oi.
func Sort(windows []Window) {
	slices.SortFunc(windows, Compare)
}

// Compare orders two windows as Sort does: negative when a comes first,
// positive when b does, and zero when they share a key.
func Compare(a, b Window) int {
	return cmp.Or(cmp.Compare(sortLength(a.Key), sortLength(b.Key)), cmp.Compare(a.Key, b.Key))
}

// sortLength is a key's length for sorting, where an unreadable one counts as endless.
func sortLength(key string) time.Duration {
	if length, ok := Length(key); ok {
		return length
	}
	return math.MaxInt64
}

// Merge returns the union of two readings of an account's windows, in Sort's
// order. Where both have a window it keeps the one with the higher
// utilization: probing itself uses a little quota, so the higher reading is
// the more current.
func Merge(a, b []Window) []Window {
	byKey := make(map[string]Window, len(a)+len(b))
	for _, w := range slices.Concat(a, b) {
		if kept, ok := byKey[w.Key]; !ok || w.Utilization > kept.Utilization {
			byKey[w.Key] = w
		}
	}
	merged := slices.Collect(maps.Values(byKey))
	Sort(merged)
	return merged
}
