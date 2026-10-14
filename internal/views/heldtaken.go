package views

import (
	"maps"
	"slices"

	"github.com/leeovery/switchboard/internal/ledger"
)

// stacked is what a held page has taken, in layers, the last taken over
// those before, which no taking changes, so a page that takes more shares
// them with the page it took them from. A layer at least half as big as the
// one under it is merged into it, as with has it, so what's taken is copied
// again only as the layers under it double, and there are few to look
// through.
type stacked[T any] struct {
	top   T
	size  int
	under *stacked[T]
}

// with is s with more taken over it, of the given size, merged with the
// layers under it as merge merges two, the one under first, into one afresh:
// s itself where there's nothing more.
func (s *stacked[T]) with(more T, size int, merge func(under, over T) T) *stacked[T] {
	if size == 0 {
		return s
	}
	top := &stacked[T]{top: more, size: size, under: s}
	for top.under != nil && 2*top.size >= top.under.size {
		u := top.under
		top = &stacked[T]{top: merge(u.top, top.top), size: u.size + top.size, under: u.under}
	}
	return top
}

// heldKeys are the keys of the lines a held page has taken, of whichever
// session, as keyOf has them: none where they have no id.
type heldKeys = *stacked[map[lineKey]bool]

// keysOf are the keys of lines, those with ids, as keyOf has them.
func keysOf(lines []ledger.Line) map[lineKey]bool {
	keys := make(map[lineKey]bool)
	for i := range lines {
		if k, ok := keyOf(&lines[i]); ok {
			keys[k] = true
		}
	}
	return keys
}

// holding is held with the keys of lines taken over it.
func holding(held heldKeys, lines []ledger.Line) heldKeys {
	keys := keysOf(lines)
	return held.with(keys, len(keys), mergedKeys)
}

// mergedKeys are the keys under and over hold, afresh.
func mergedKeys(under, over map[lineKey]bool) map[lineKey]bool {
	merged := maps.Clone(under)
	maps.Copy(merged, over)
	return merged
}

// holds reports whether held has the key k.
func holds(held heldKeys, k lineKey) bool {
	for ; held != nil; held = held.under {
		if held.top[k] {
			return true
		}
	}
	return false
}

// answersTaken are a day's answers, every account's requests, as a held page
// takes them in, in the order it took them.
type answersTaken = *stacked[[]answered]

// answering is answers, by date, with more taken in on the day with the
// given date, afresh: answers itself where there are none. Only the dates
// are copied, a few, each day's answers shared with answers.
func answering(answers map[string]answersTaken, date string, more []answered) map[string]answersTaken {
	if len(more) == 0 {
		return answers
	}
	grown := make(map[string]answersTaken, len(answers)+1)
	maps.Copy(grown, answers)
	grown[date] = answers[date].with(more, len(more), mergedAnswers)
	return grown
}

// mergedAnswers are the answers under, then those over, afresh.
func mergedAnswers(under, over []answered) []answered {
	return slices.Concat(under, over)
}

// takenAll are the answers taken, in the order they were taken.
func takenAll(taken answersTaken) []answered {
	var layers [][]answered
	for ; taken != nil; taken = taken.under {
		layers = append(layers, taken.top)
	}
	slices.Reverse(layers)
	return slices.Concat(layers...)
}
