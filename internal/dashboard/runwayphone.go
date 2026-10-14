package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// phoneRunway is Runway on a phone, of doc at now, as its form draws it:
// nothing yet.
func (f Frame) phoneRunway(status.Document, time.Time) runwayForm {
	return runwayForm{}
}
