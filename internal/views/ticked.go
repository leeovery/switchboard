package views

import "slices"

// ticked reports whether a filter's choice, of those ticked, keeps the one
// given: one of them, or any where none is ticked, as the Log's Requests
// and its Events tick each choice.
func ticked[T comparable](choice []T, one T) bool {
	return len(choice) == 0 || slices.Contains(choice, one)
}

// tickedAny reports whether a filter's choice, of those ticked, keeps any of
// those given, as ticked keeps one: every one, even none, where none is
// ticked.
func tickedAny[T comparable](choice, ones []T) bool {
	return len(choice) == 0 || slices.ContainsFunc(ones, func(one T) bool { return slices.Contains(choice, one) })
}

// Ticks reports whether a filter's choice ticks the one given, as the Log's
// panel draws its tick: as the filter keeps it, so every one is ticked where
// none is.
func Ticks[T comparable](choice []T, one T) bool {
	return ticked(choice, one)
}

// Toggled is a filter's choice of every one there is to tick, as Ticks has
// it, with the one given ticked where it isn't, and not where it is, as the
// Log's panel ticks it: none, which is every one, where every one or none
// would be ticked.
func Toggled[T comparable](choice, every []T, one T) []T {
	var kept []T
	for _, each := range every {
		if (each == one) != ticked(choice, each) {
			kept = append(kept, each)
		}
	}
	if len(kept) == len(every) {
		return nil
	}
	return kept
}
