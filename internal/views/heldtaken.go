package views

import (
	"maps"
	"slices"
)

// stacked is what a holder has taken, in layers, the last taken over those
// before, which no taking changes, so one that takes more shares them with
// the one it took them from. A layer at least half as big as the one under
// it is merged into it, as with has it, so what's taken is copied again only
// as the layers under it double, and there are few to look through.
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

// answersHeld are a day's answers a page's points hold, every account's, in
// layers, as stacked has them, each with how many of its answers each key
// has, as answered's key has it, so a day that gains a few answers shares
// those it held with the page it took them from.
type answersHeld struct {
	layers *stacked[answerLayer]
}

// answerLayer is a layer of answers held, and how many of them each key
// has.
type answerLayer struct {
	answers []answered
	keys    map[turnKey]int
}

// heldOf are the answers given, held.
func heldOf(answers []answered) answersHeld {
	return answersHeld{}.with(answers)
}

// with is h with the answers added over it, afresh: h itself where there are
// none.
func (h answersHeld) with(added []answered) answersHeld {
	keys := make(map[turnKey]int, len(added))
	for i := range added {
		keys[added[i].key()]++
	}
	return answersHeld{h.layers.with(answerLayer{answers: added, keys: keys}, len(added), mergedLayers)}
}

// mergedLayers are the answers under, then those over, afresh.
func mergedLayers(under, over answerLayer) answerLayer {
	keys := maps.Clone(under.keys)
	for k, n := range over.keys {
		keys[k] += n
	}
	return answerLayer{answers: slices.Concat(under.answers, over.answers), keys: keys}
}

// all are the answers h holds, in the order they were taken.
func (h answersHeld) all() []answered {
	var layers [][]answered
	for s := h.layers; s != nil; s = s.under {
		layers = append(layers, s.top.answers)
	}
	slices.Reverse(layers)
	return slices.Concat(layers...)
}

// count is how many of the answers h holds have the key k.
func (h answersHeld) count(k turnKey) int {
	n := 0
	for s := h.layers; s != nil; s = s.under {
		n += s.top.keys[k]
	}
	return n
}

// given are the answers among given, a read of h's day, that h doesn't hold,
// as unheld has them, but those sealed, both read and ended before sealedTo,
// as the points took them in, and how many of those were, where the read is
// of the day whole. Of a read of the day's gain, one sealed is passed over
// too: a line is filed as its request ends, so one the gain gives that ended
// before what was sealed, a while before the read, is one given again, as a
// read of today afresh gives every one of its lines. It reports false where
// one it doesn't hold was read before sealedTo but ended after, which would
// change what's sealed: the points can't place it.
func (h answersHeld) given(given []answered, sealedTo instant, whole bool) ([]answered, int, bool) {
	var added []answered
	sealed := 0
	unheld := h.unheld(given)
	for i := range unheld {
		a := &unheld[i]
		switch both, either := a.sealedBy(sealedTo); {
		case both && whole:
			sealed++
		case both:
		case either:
			return nil, 0, false
		default:
			added = append(added, *a)
		}
	}
	return added, sealed, true
}

// unheld are the answers among given, a read of h's day, that h doesn't
// hold, in their order: each given again against those h holds, as
// countedOnce tells it by their keys, passed over. It's given itself,
// shared, where it passes none over.
func (h answersHeld) unheld(given []answered) []answered {
	read := countedOnceOf(len(given))
	var unheld []answered
	copied := false
	for i := range given {
		a := &given[i]
		k := a.key()
		again := read.again(k, h.count(k))
		switch {
		case again && !copied:
			unheld, copied = slices.Clone(given[:i]), true
		case !again && copied:
			unheld = append(unheld, *a)
		}
	}
	if !copied {
		return given
	}
	return unheld
}
