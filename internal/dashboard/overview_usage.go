package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// usageSection is the Overview's USAGE of doc at now: usage's block, of
// every window, grouped as the frame's By has it. At its least it's the
// block as usage lays it out at the frame's width, which from 150 columns,
// by window, is the compact matrix, 2a + 7 rows for a accounts and the pool;
// at its most, from 150 columns, by window, the big bars, its windows stacked
// as sections at the full width; otherwise its least. USAGE never folds.
func (f Frame) usageSection(doc status.Document, now time.Time) section {
	block := newUsageBlock(doc, now, f.Width, f.Look, f.By, nil)
	least := sectionHead + usageRows(block, block.draw)
	most := least
	if f.By == ByWindow && f.Width >= matrixWidth {
		most = sectionHead + usageRows(block, block.stacked)
	}
	return section{
		Section: SectionUsage, least: least, most: most,
		description: func() line { return nil },
		draw: func(c *canvas, body bounds) {
			block.key(c, block.draw(c, body.y)+1)
		},
	}
}

// usageRows is how many rows the usage block b takes, laid out as layout
// lays it out, with its key line after a blank row.
func usageRows(b usageBlock, layout func(c *canvas, y int) int) int {
	c := growing(b.width)
	b.key(c, layout(c, 0)+1)
	return c.height()
}
