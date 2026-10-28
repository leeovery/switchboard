package status

import (
	"errors"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/tokens"
)

// The spans before a token's year is up that TokenEndsWithin is asked of.
const (
	// TokenWarning is 30 days.
	TokenWarning = 30 * 24 * time.Hour
	// TokenLastWarning is 7 days.
	TokenLastWarning = 7 * 24 * time.Hour
)

// withTokenTaken is the account a, its token token, taken when the record
// beside its file says, where that's known, as c's Taken reads it, logging
// why at debug where there's a record it can't go by.
func (c Collector) withTokenTaken(a Account, token tokens.Token) Account {
	if c.Taken == nil {
		return a
	}
	taken, err := c.Taken(a.ID, token)
	switch {
	case errors.Is(err, tokens.ErrUnrecorded):
	case err != nil:
		logger.Debug("not going by the record of when the token was taken", "account", a.ID, "error", err)
	default:
		a = a.WithTokenTaken(taken)
	}
	return a
}

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
