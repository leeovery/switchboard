package views

import (
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// TokenNote is what Accounts tells of an account's token's year ending within
// status.TokenWarning days, or up: Told, the line Detail tells it in, and
// Short, the words Compare does, as prose's TokenEnding has them; and
// Warning, the days of the nearest warning it ends within,
// status.TokenLastWarning or status.TokenWarning, as status.TokenYear.Warned
// has it, which its warning part is coloured by.
type TokenNote struct {
	Told, Short []prose.TokenPart
	Warning     int
}

// TokenNoteOf is what Accounts tells of the account's token's year at the
// moment it stands at, as TokenNote has it, reporting false where its year
// isn't known, or ends further off than status.TokenWarning days.
func TokenNoteOf(a status.AccountNow) (TokenNote, bool) {
	warning, warned := a.TokenWarned()
	if !warned {
		return TokenNote{}, false
	}
	ending := a.TokenEnding()
	return TokenNote{Told: ending.Told(), Short: ending.Short(), Warning: warning}, true
}
