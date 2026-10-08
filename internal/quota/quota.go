// Package quota is the provider-neutral usage model: the windows an account's
// quota is measured over, the ones a provider expected to read but couldn't,
// what a response says of the account it came from, and the tokens an answer
// counted.
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
	// RestartedAt is when the window started again before its reset, as a
	// reset made by hand starts it, dropping its use but keeping its reset:
	// it runs from then until its reset. Zero when it runs a whole length
	// before its reset, as a window does, and a provider never says.
	RestartedAt time.Time `json:"restarted_at,omitzero"`
}

// Span returns when the window began and how long it runs until it resets:
// from a whole length before its reset, or from when it started again, where
// it did since. It reports false unless both its length and its reset are
// known.
func (w Window) Span() (time.Time, time.Duration, bool) {
	length, ok := Length(w.Key)
	if !ok || w.ResetsAt.IsZero() {
		return time.Time{}, 0, false
	}
	start := w.ResetsAt.Add(-length)
	if w.RestartedAt.After(start) && w.RestartedAt.Before(w.ResetsAt) {
		start = w.RestartedAt
	}
	return start, w.ResetsAt.Sub(start), true
}

// ResetBy reports whether the window had reset by t, as its reading said it
// would: it gives a reset, at t or before. Its reading is stale from then:
// the window has started afresh.
func (w Window) ResetBy(t time.Time) bool {
	return !w.ResetsAt.IsZero() && !w.ResetsAt.After(t)
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
	Windows []Window `json:"windows,omitempty"`
	// Extra is the account's extra usage: zero where the reading didn't give
	// it.
	Extra    ExtraUsage `json:"extra_usage,omitzero"`
	Failures []Failure  `json:"failures,omitempty"`
}

// ExtraUsage is how an account stands for extra usage, which serves its
// requests past its limit, and bills for them, as a provider reports it. It
// isn't a window. Each field is zero where the provider didn't give it, or
// gave it unreadably.
type ExtraUsage struct {
	// Status is the provider's verdict, as given: extra usage is on while it
	// isn't StatusRejected.
	Status Status `json:"status,omitempty"`
	// Utilization is the fraction of it used: nil where it wasn't given, as
	// none used is 0.
	Utilization *float64 `json:"utilization,omitempty"`
	// ResetsAt is when its period resets.
	ResetsAt time.Time `json:"resets_at,omitzero"`
}

// Given reports whether any of e was given.
func (e ExtraUsage) Given() bool {
	return e != ExtraUsage{}
}

// Probe is what probing an account read: its usage, and which models'
// responses reported each window, which says whose requests a window counts.
type Probe struct {
	Usage
	// Models are the models whose responses reported each window, by the
	// window's key.
	Models map[string][]string `json:"models,omitempty"`
	// Admitted is set when a request of the probe was answered with success:
	// the account took it, whatever limit it had reached before.
	Admitted bool `json:"admitted,omitempty"`
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
	// Forbidden is the request refused on the account, whose token stands:
	// the account can't make requests of its kind, as of a model its plan
	// lacks.
	Forbidden
)

// Outcome is a response's verdict on the account a request went out on.
type Outcome struct {
	Verdict Verdict
	// RetryAfter is how long a throttled account is asked to wait before
	// sending again, or zero when the response doesn't say.
	RetryAfter time.Duration
	// Rejected are the keys of the windows an account's limit is reached in,
	// in Sort's order, however little else the response says of them: none
	// when only its overall verdict says so.
	Rejected []string
	// LimitedUntil is when an account whose limit is reached has room again,
	// as far as the response says, or zero when it doesn't say.
	LimitedUntil time.Time
}

// Tokens are the tokens an answer counted, as its closing usage gives them:
// the input it read afresh, the output it wrote, and the input it read from
// the prompt cache, and wrote to it.
type Tokens struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cache_read"`
	CacheWrite int `json:"cache_write"`
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
	return CompareKeys(a.Key, b.Key)
}

// CompareKeys orders the keys of two windows as Sort orders the windows.
func CompareKeys(a, b string) int {
	return cmp.Or(cmp.Compare(sortLength(a), sortLength(b)), cmp.Compare(a, b))
}

// sortLength is a key's length for sorting, where an unreadable one counts as endless.
func sortLength(key string) time.Duration {
	if length, ok := Length(key); ok {
		return length
	}
	return math.MaxInt64
}

// MergeMax returns the union of two readings of an account's windows taken
// together, as one probe's model families are, in Sort's order. Where both
// have a window it keeps the one with the higher utilization: probing itself
// uses a little quota, so the higher reading is the more current. Readings
// taken apart mustn't be merged so, as a window may have reset in between:
// the router merges those by their resets.
func MergeMax(a, b []Window) []Window {
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
