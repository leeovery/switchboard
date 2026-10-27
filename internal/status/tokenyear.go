package status

import (
	"time"

	"github.com/leeovery/switchboard/internal/claude"
)

// How long before a token's year is up switchboard warns of it.
const (
	// TokenWarning is how long before: the Accounts tab says so from then on,
	// and a notification goes out then.
	TokenWarning = 30 * 24 * time.Hour
	// TokenLastWarning is how long before the last notification goes out.
	TokenLastWarning = 7 * 24 * time.Hour
)

// WithTokenTaken is the account, its token taken at taken: good until a
// setup token's lifetime later.
func (a Account) WithTokenTaken(taken time.Time) Account {
	a.TokenTaken = taken.UTC()
	a.TokenGoodUntil = a.TokenTaken.Add(claude.SetupTokenLifetime)
	return a
}

// TokenEndsWithin reports whether the account's token's year is up within
// span of the moment it stands at: less than span is left of it, or none, as
// of a token past its year. It's false where when it's up isn't known.
func (a AccountNow) TokenEndsWithin(span time.Duration) bool {
	return !a.TokenGoodUntil.IsZero() && a.TokenGoodUntil.Sub(a.now) < span
}
