package dashboard

import (
	"github.com/leeovery/switchboard/internal/score"
)

// Trails are how the accounts' windows have been used, which GRAPHS draws
// from: the trail of each window as it runs now, by its account's id and its
// key.
type Trails map[Ref]Trail

// Ref names an account's window: the account's id, and the window's key.
type Ref struct {
	Account, Window string
}

// Trail is a window's use as it runs now: the levels its use was read at
// since it started, the oldest first, each read again, the same, until its
// Last.
type Trail struct {
	Readings []score.Reading
}
