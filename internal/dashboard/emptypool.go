package dashboard

import "github.com/leeovery/switchboard/internal/prose"

// emptyIndent is the blank cells the page of a pool with no accounts sets its
// words in by.
const emptyIndent = 2

// emptyPool draws the page of a pool with no accounts to route, where the
// pool shown is one, as prose.EmptyPool says it, of the accounts the config
// gives it waiting on the router where it's waiting, muted, as setup says it
// has none:
// from the row after the blank one under the status line, emptyIndent
// cells in, each line wrapped at the frame's edge, a blank row between its
// paragraphs, as many rows as there are over the key line.
func (f Frame) emptyPool(c *canvas) {
	if !f.Pool.Empty() {
		return
	}
	waiting := 0
	if f.Pool.Waiting() {
		waiting = f.Pool.Accounts
	}
	y, last := f.statusRow()+2, f.Height-underView
	for i, paragraph := range prose.EmptyPool(f.Pool.Label, f.Pool.ID, f.Pool.Dirs, waiting) {
		if i > 0 {
			y++
		}
		for _, said := range paragraph {
			for _, row := range (line{{said, mutedInk}}).wrap(f.edge() - emptyIndent) {
				if y > last {
					return
				}
				c.line(emptyIndent, y, row)
				y++
			}
		}
	}
}
