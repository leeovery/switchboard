package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// compareForm is Compare as it's drawn wide, of doc at now: nothing yet.
func (f Frame) compareForm(status.Document, time.Time) accountsForm {
	return accountsForm{}
}

// compareState is what Compare's heading says after [ ] of the accounts it
// shows: nothing yet.
func (f Frame) compareState() line {
	return nil
}
