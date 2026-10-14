package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// listForm is Sessions' List as its form draws it, of doc at now: nothing
// yet.
func (f Frame) listForm(status.Document, time.Time) sessionsForm {
	return sessionsForm{}
}
