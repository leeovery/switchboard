package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// routingRows is the fewest columns ROUTING gives each account a row at:
// narrower, and on a phone, it gives it two.
const routingRows = 110

// routingSection is the Overview's ROUTING of doc at now: how each account
// stands with the router, and its trace. At its least, size 1, it's a row an
// account and its caption under them, a + 3 rows for a accounts; at its
// most, size 2, two rows of trace an account and a blank row, 3a + 2. Under
// routingRows columns, and on a phone, it's two rows an account and a blank
// row, 3a + 1 at its least and its most. By account, from 150 columns, it's
// beside USAGE's blocks, and takes no rows of its own.
func (f Frame) routingSection(doc status.Document, now time.Time) section {
	a := len(doc.Accounts)
	least, most := a+3, 3*a+2
	switch {
	case f.By == ByAccount && f.Width >= matrixWidth:
		least, most = 0, 0
	case f.Width < routingRows:
		least, most = 3*a+1, 3*a+1
	}
	return section{
		Section: SectionRouting, least: least, most: most,
		description: func() line { return nil },
		folded:      func() line { return nil },
		draw: func(c *canvas, body bounds) {
			cells := f.Width - traceTail - traceFrom
			scale := f.traceScale(now, cells)
			for i, account := range doc.Accounts {
				t, y := f.Traces.Accounts[account.ID], body.y+i
				f.drawTrace(c, t, now, bounds{x: traceFrom, y: y, width: cells, rows: 1}, scale, false)
				c.right(f.Width, y, line{traceRate(t, now, false)})
			}
		},
	}
}
