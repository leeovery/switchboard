package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/score"
)

// History is how the accounts' windows have been used, which the charts draw
// from: the trail of each window as it runs now, by its account's id and its
// key.
type History map[Ref]Trail

// Ref names an account's window: the account's id, and the window's key.
type Ref struct {
	Account, Window string
}

// Trail is a window's use as it runs now: when it started, and the levels its
// use was read at since, the oldest first, each read again, the same, until
// its Last.
type Trail struct {
	Start    time.Time
	Readings []score.Reading
}
