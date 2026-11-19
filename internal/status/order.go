package status

import (
	"cmp"
	"iter"
	"math"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// CompareWindows orders windows' keys as the owner reads windows listed, in
// the order of care, by the policy p, as in 7d, 5h, 7d_oi: those every model
// shares first, the longest first, a key whose length can't be read last of
// them; then each model's own, as any window the policy doesn't share is, in
// quota's order. Lists for agents, as the status document's, and what's
// recorded keep quota's order.
func CompareWindows(p score.Policy) func(a, b string) int {
	return func(a, b string) int {
		sharedA, sharedB := p.IsShared(a), p.IsShared(b)
		switch {
		case sharedA != sharedB && sharedA:
			return -1
		case sharedA != sharedB:
			return 1
		case sharedA:
			return cmp.Or(cmp.Compare(shortness(a), shortness(b)), cmp.Compare(a, b))
		}
		return quota.CompareKeys(a, b)
	}
}

// shortness is a key's length negated, for the longest to sort first, where
// an unreadable one counts as shortest.
func shortness(key string) time.Duration {
	if length, ok := quota.Length(key); ok {
		return -length
	}
	return math.MaxInt64
}

// CompareWindows orders windows' keys as the owner reads windows listed at
// the moment, by the policy it was judged by, as CompareWindows has it.
func (m Moment) CompareWindows(a, b string) int {
	return CompareWindows(m.policy)(a, b)
}

// SharedWindows are the keys of the windows every model shares, as the
// policy the moment was judged by has them, that some account has or may
// have, in the order the owner reads windows listed, as CompareWindows has
// it: not one every account lacks, as lacking has them, where there's any
// account.
func (m Moment) SharedWindows() []string {
	shared := slices.DeleteFunc(slices.Clone(m.policy.Shared), m.lackedByAll)
	return slices.SortedFunc(slices.Values(shared), m.CompareWindows)
}

// lackedByAll reports whether every account at the moment lacks the window
// every model shares with the given key, as lackedByEvery has it.
func (m Moment) lackedByAll(key string) bool {
	return lackedByEvery(func(yield func([]string) bool) {
		for _, a := range m.Accounts {
			if !yield(a.lacks) {
				return
			}
		}
	}, key)
}

// lackedByEvery reports whether every account, given by the keys of the
// windows every model shares that it lacks, as lacking has them, lacks the
// one with the given key: never where none is given.
func lackedByEvery(lacks iter.Seq[[]string], key string) bool {
	some := false
	for l := range lacks {
		if !slices.Contains(l, key) {
			return false
		}
		some = true
	}
	return some
}

// Listed are the keys of the windows the owner reads listed at the moment
// where none is asked for, as USAGE, the Switchboard's lines, a session's
// page, Accounts' Detail and usage's printout list them: those every model
// shares that some account has or may have, as SharedWindows has them, so
// every account keeps its rows before any is read; then each model's own,
// as any window the policy doesn't share is, that some account has to show
// at Now, as shows says, so it shows as the 5-hour does, only where some
// account has it to show. In the order CompareWindows has.
func (m Moment) Listed() []string {
	return m.listed(AccountNow.shows)
}

// shows reports whether the account's model window w shows where windows
// are listed as they stand: the account has used it, as used says, or what
// holds it names it, as a limit may still once it has reset to 0%, or its
// cap, reached in it. A limit naming no window holds every window alike, so
// says nothing of any one's use, and brings none up alone.
func (a AccountNow) shows(w WindowNow) bool {
	return w.used() || w.Capped || w.Held && slices.Contains(a.Hold.Windows, w.Key)
}

// Reported are the keys of every window Listed may list at the moment: as it
// has them, but each model's own that any account reports, used or not, for
// a layout to hold still as their use comes and goes.
func (m Moment) Reported() []string {
	return m.listed(func(AccountNow, WindowNow) bool { return true })
}

// listed are the keys of the windows every model shares that some account
// has or may have, as SharedWindows has them, then of each model's own where
// models picks some account's, in the order CompareWindows has.
func (m Moment) listed(models func(AccountNow, WindowNow) bool) []string {
	keys := m.SharedWindows()
	for _, a := range m.Accounts {
		for _, w := range a.Windows {
			if !w.Shared && models(a, w) {
				keys = append(keys, w.Key)
			}
		}
	}
	return slices.Compact(slices.SortedFunc(slices.Values(keys), m.CompareWindows))
}
