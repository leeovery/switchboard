package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/events"
)

// EventRow is an event as events prints it, a line of the router's files
// each: when it happened, the session it's of, "" where it's of none, and
// its line, as Telling tells it.
type EventRow struct {
	At      time.Time
	Session string
	EventLine
}

// Row is the row of the event h, a line of the router's files, told as Line
// tells it, its times in now's time zone.
func (t *Telling) Row(h events.Held, now time.Time) EventRow {
	return EventRow{At: h.At.In(now.Location()), Session: h.Session, EventLine: t.Line(h.Event, now)}
}
