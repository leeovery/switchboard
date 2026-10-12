package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// latelySection is the Overview's LATELY of doc at now: what's happened,
// newest first, as many of the events it tells of as it has, leastLines of
// them at its least.
func (f Frame) latelySection(doc status.Document, now time.Time) section {
	return section{
		Section: SectionLately, least: linesRows(0), most: linesRows(len(recent(doc, now, mostLines))),
		draw:   func(*canvas, bounds) {},
		folded: func() line { return nil },
	}
}
