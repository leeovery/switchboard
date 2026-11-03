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

// lackedByEvery reports whether every account, of those whose lacks are
// given, the keys of the windows every model shares it lacks, as lacking has
// them, lacks the window with the given key: never where there are none.
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
// where none is asked for, as USAGE, the Switchboard's lines and usage's
// printout list them: those every model shares that some account has or may
// have, as SharedWindows has them, so every account keeps its rows before
// any is read; then each model's own any account reports, as any window the
// policy doesn't share is: with models, every one, as usage's printout lists
// them, and the live views while w shows them; without, only one nearing or
// at its limit on some account, as nearsLimit has it, for as long as that
// holds. In the order CompareWindows has.
func (m Moment) Listed(models bool) []string {
	keys := m.SharedWindows()
	for _, a := range m.Accounts {
		for _, w := range a.Windows {
			if models || w.nearsLimit() {
				keys = append(keys, w.Key)
			}
		}
	}
	return slices.Compact(slices.SortedFunc(slices.Values(keys), m.CompareWindows))
}

// nearsLimit reports whether the window nears or reaches its limit at Now:
// held at it, at its account's cap, running out, or nearing where it runs
// out.
func (w WindowNow) nearsLimit() bool {
	return w.Held || w.Capped || w.RunsOut || w.Nearing
}
