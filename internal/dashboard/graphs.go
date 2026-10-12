package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// The rows a GRAPHS chart takes: leastChart at the least, and mostChart at
// the most, as taller distorts.
const (
	leastChart = 5
	mostChart  = 19
)

// graphsSection is the Overview's GRAPHS of doc at now: the focused
// account's use so far, a chart a window, leastChart rows tall at its least
// and mostChart at its most.
func (f Frame) graphsSection(status.Document, time.Time) section {
	return section{
		Section: SectionGraphs, least: graphsRows(leastChart), most: graphsRows(mostChart),
		description: func() line { return nil },
		draw:        func(*canvas, bounds) {},
		folded:      func() line { return nil },
	}
}

// graphsRows is how many rows GRAPHS takes with charts h rows tall, h + 4:
// its heading and the blank row under it; the charts' top row, each its
// window's name and its limit's line; their h rows; and their axis row.
func graphsRows(h int) int {
	return sectionHead + 1 + h + 1
}
