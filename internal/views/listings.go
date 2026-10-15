package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/status"
)

// KeptListings are the sessions the router listed, each kept its own, as
// keptListing keeps it, so what's built of them can be told from what a
// later listing builds, as SameListings has it, however the listing they
// were read from changes in place.
func KeptListings(sessions []status.Session) []status.Session {
	kept := make([]status.Session, len(sessions))
	for i, s := range sessions {
		kept[i] = keptListing(s)
	}
	return kept
}

// SameListings reports whether the router lists the sessions in a as in b,
// in the same order, each alike, as sameListing has it: what a view builds
// of one listing it builds of the other, so a look listing the sessions the
// same builds nothing again.
func SameListings(a, b []status.Session) bool {
	return slices.EqualFunc(a, b, func(x, y status.Session) bool { return sameListing(&x, &y) })
}

// keptListing is the router's listing of the session s, its models' places
// its own.
func keptListing(s status.Session) status.Session {
	s.Assignments = slices.Clone(s.Assignments)
	return s
}

// sameListing reports whether the router lists a session in a as in b: its
// id, its pin, and its models' places, where they go, why, and when they
// were last seen there, each alike; or neither lists one.
func sameListing(a, b *status.Session) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ID == b.ID && a.Pin == b.Pin && slices.Equal(a.Assignments, b.Assignments)
}
