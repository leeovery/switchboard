package status

import (
	"errors"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/tokens"
)

// The days left of a token's year, as DaysLeft counts them, at which
// EndsWithin is asked whether it's warned of.
const (
	// TokenWarning is 30 days.
	TokenWarning = 30
	// TokenLastWarning is 7 days.
	TokenLastWarning = 7
)

// TokenYear is a token's year: Taken when it was taken, and GoodUntil when
// its year is up, a setup token's lifetime later. Each is zero where it isn't
// known.
type TokenYear struct {
	Taken, GoodUntil time.Time
}

// TokenYearFrom is the year of a token taken at taken, in UTC.
func TokenYearFrom(taken time.Time) TokenYear {
	taken = taken.UTC()
	return TokenYear{Taken: taken, GoodUntil: taken.Add(claude.SetupTokenLifetime)}
}

// Known reports whether when the year is up is known, whether or not when it
// began is.
func (y TokenYear) Known() bool {
	return !y.GoodUntil.IsZero()
}

// Up reports whether the year is known and up at now, to the instant.
func (y TokenYear) Up(now time.Time) bool {
	return y.Known() && !now.Before(y.GoodUntil)
}

// EndsWithin reports whether the year ends within days of now: days or fewer
// are left of it, as DaysLeft counts them, or none, as of a token past its
// year. It's false where when it's up isn't known.
func (y TokenYear) EndsWithin(now time.Time, days int) bool {
	left, counted := y.DaysLeft(now)
	return y.Up(now) || counted && left <= days
}

// DaysLeft counts the calendar days from now's date to the date the year is
// up, both in now's time zone, however long the days between ran: 0 on its
// last day. counted is false where the year is up at now, or isn't known.
func (y TokenYear) DaysLeft(now time.Time) (days int, counted bool) {
	if !y.Known() || y.Up(now) {
		return 0, false
	}
	return dayfile.DaysBetween(now, y.GoodUntil, now.Location()), true
}

// Gone is the share of the year gone at now, from 0 as it's taken to 1 as
// it's up, held there either side: 1 where it's up, else 0 where either end
// isn't known, or it has no length to share.
func (y TokenYear) Gone(now time.Time) float64 {
	switch {
	case y.Up(now):
		return 1
	case y.Taken.IsZero() || !y.Taken.Before(y.GoodUntil):
		return 0
	}
	return max(float64(now.Sub(y.Taken))/float64(y.GoodUntil.Sub(y.Taken)), 0)
}

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
// setup token's lifetime later, as TokenYearFrom has it.
func (a Account) WithTokenTaken(taken time.Time) Account {
	year := TokenYearFrom(taken)
	a.TokenTaken, a.TokenGoodUntil = year.Taken, year.GoodUntil
	return a
}

// TokenYear is the year of the account's token, as read.
func (a Account) TokenYear() TokenYear {
	return TokenYear{Taken: a.TokenTaken, GoodUntil: a.TokenGoodUntil}
}

// TokenEndsWithin reports whether the account's token's year ends within
// days of the moment it stands at, as TokenYear.EndsWithin has it.
func (a AccountNow) TokenEndsWithin(days int) bool {
	return a.TokenYear.EndsWithin(a.now, days)
}

// TokenDaysLeft counts the days left of the account's token's year at the
// moment it stands at, as TokenYear.DaysLeft counts them.
func (a AccountNow) TokenDaysLeft() (days int, counted bool) {
	return a.TokenYear.DaysLeft(a.now)
}

// TokenGone is the share of the account's token's year gone at the moment
// it stands at, as TokenYear.Gone has it.
func (a AccountNow) TokenGone() float64 {
	return a.TokenYear.Gone(a.now)
}
