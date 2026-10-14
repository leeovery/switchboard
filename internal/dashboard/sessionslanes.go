package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// lanesForm is the Switchboard of doc at now, laid out as given, as its form
// draws it in lanes, in place of cords: nothing yet.
func (f Frame) lanesForm(status.Document, time.Time, boardLayout) sessionsForm {
	return sessionsForm{}
}
