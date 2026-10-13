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
