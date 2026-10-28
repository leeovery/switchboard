package status

import (
	"errors"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/tokens"
)

// The spans before a token's year is up that EndsWithin is asked of.
const (
	// TokenWarning is 30 days.
	TokenWarning = 30 * day
	// TokenLastWarning is 7 days.
	TokenLastWarning = 7 * day
)

// day is a day's length, as the noons calendarDays counts between are apart.
const day = 24 * time.Hour

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

// Known reports whether when the year is up is known.
func (y TokenYear) Known() bool {
	return !y.GoodUntil.IsZero()
}

// Up reports whether the year is known and up at now, to the instant.
func (y TokenYear) Up(now time.Time) bool {
	return y.Known() && !now.Before(y.GoodUntil)
}

// EndsWithin reports whether the year is up within span of now: less than
// span is left of it, or none, as of a token past its year. It's false where
// when it's up isn't known.
func (y TokenYear) EndsWithin(now time.Time, span time.Duration) bool {
	return y.Known() && y.GoodUntil.Sub(now) < span
}

// DaysLeft counts the calendar days from now's date to the date the year is
// up, both in now's time zone, however long the days between ran: 0 on its
// last day. ok is false where the year is up at now, or isn't known.
func (y TokenYear) DaysLeft(now time.Time) (days int, ok bool) {
	if !y.Known() || y.Up(now) {
		return 0, false
	}
	return calendarDays(now, y.GoodUntil.In(now.Location())), true
}

// Gone is the share of the year gone at now, from 0 as it's taken to 1 as
// it's up, held there either side: 0 where the year isn't known.
func (y TokenYear) Gone(now time.Time) float64 {
	if !y.Known() || y.Taken.IsZero() {
		return 0
	}
	gone := float64(now.Sub(y.Taken)) / float64(y.GoodUntil.Sub(y.Taken))
	return min(max(gone, 0), 1)
}

// calendarDays counts the days from the date from has to the date to has,
// each in its own time zone, by their noons in UTC, so a day the clocks
// change on counts as one.
func calendarDays(from, to time.Time) int {
	noon := func(t time.Time) time.Time {
		y, m, d := t.Date()
		return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
	}
	return int(noon(to).Sub(noon(from)) / day)
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

// TokenEndsWithin reports whether the account's token's year is up within
// span of the moment it stands at, as TokenYear.EndsWithin has it.
func (a AccountNow) TokenEndsWithin(span time.Duration) bool {
	return a.TokenYear.EndsWithin(a.now, span)
}
