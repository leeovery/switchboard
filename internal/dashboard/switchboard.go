package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// switchboardForm is the Switchboard of doc at now, laid out as given, as
// its form draws it with cords: its lines, as switchboardLines draws them.
func (f Frame) switchboardForm(doc status.Document, now time.Time, laid boardLayout) sessionsForm {
	m := f.Moment(doc, now)
	return sessionsForm{draw: func(c *canvas, top int) { f.switchboardLines(c, m, laid, top) }}
}
