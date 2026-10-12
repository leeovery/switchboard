package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// sessionsSection is the Overview's SESSIONS of doc at now: the sessions
// running, a row on each account a session's models go to, or a row saying
// none is, at its least and its most alike; under routingRows columns, and
// on a phone, two rows a session.
func (f Frame) sessionsSection(doc status.Document, now time.Time) section {
	rows := f.runningRows(doc, now)
	if f.Width < routingRows {
		rows *= 2
	}
	rows = sectionHead + max(rows, 1)
	return section{
		Section: SectionSessions, least: rows, most: rows,
		draw:   func(*canvas, bounds) {},
		folded: func() line { return nil },
	}
}

// runningRows counts the sessions running on doc's accounts at now, as the
// router lists them, one on each account its models go to: those active
// there, as active says.
func (f Frame) runningRows(doc status.Document, now time.Time) int {
	n, listing := 0, f.listing()
	for _, a := range doc.Accounts {
		n += len(f.busy(listing, a.ID, now))
	}
	return n
}
