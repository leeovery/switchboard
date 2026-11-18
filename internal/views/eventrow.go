package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/status"
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
// tells it, a hold with its lift where Pairing paired them, its times in
// now's time zone.
func (t *Telling) Row(h events.Held, now time.Time) EventRow {
	line, _ := t.tell(filedEvent(h.Line), now)
	return EventRow{At: h.At.In(now.Location()), Session: h.Session, EventLine: line}
}

// Pairing has t tell each hold of the lines held, those of the router's
// files read before any is told, with its lift, as the Log's Events pair
// them. It returns t.
func (t *Telling) Pairing(held []events.Held) *Telling {
	read := make([]Event, len(held))
	for i, h := range held {
		read[i] = filedEvent(h.Line)
	}
	return t.pairing(read)
}

// Named is the row with each account its line names named as n names it.
func (r EventRow) Named(n status.Namer) EventRow {
	r.Account, r.What = r.Account.Named(n), r.What.Named(n)
	return r
}
