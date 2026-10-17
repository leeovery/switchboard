package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayPool draws the pool's lane, as laid out: its columns, Σ pool beside
// them on its first row, as an account's name heads its lane; and its room,
// where it has one. With one account, there's none, nor for a moment of
// none, whatever the lanes hold.
func (f Frame) runwayPool(c *canvas, laid runwayLaid) {
	fit := laid.fit
	if !fit.draws(fit.pool) || len(laid.m.Accounts) == 0 {
		return
	}
	pool := f.Runway.Lanes.Pool
	f.poolColumns(c, laid, pool)
	c.line(0, fit.pool, poolNamed(namesOf(laid.m)))
	if fit.draws(fit.room) {
		f.poolRoom(c, laid, pool.Key)
	}
}

// poolColumns draws the pool's columns on its lane's rows, from the lanes'
// column, as fillUp fills a column: each as high as the share of the pool's
// accounts with room at the column's moment, of its rows, in eighths of a
// cell, in the colour usage gives the pool's use, the share without room,
// faded as the past is before now's column. A column where none has room,
// of those whose state is known, is ─ on its lower row, as a lane draws one
// held back from every request, faded as the past is likewise; one where
// none's state is known is left blank.
func (f Frame) poolColumns(c *canvas, laid runwayLaid, pool views.Pool) {
	now, floor := f.Runway.Timeline.NowColumn(), laid.fit.pool+runwayPoolRows-1
	for i, col := range pool.Columns {
		if col.Known && col.With == 0 {
			mark := f.Look.laneMark(views.CellLimited)
			c.text(laid.x+i, floor, mark.text, laneInk(mark.ink, i < now, 0))
			continue
		}
		share := float64(col.With) / float64(pool.Of)
		tone := usedInk(1-share, ink{token: theme.VizRamp1})
		if i < now {
			tone.fade = pastFade
		}
		fillUp(c, f.Look, laid.x+i, floor, halfEven(share*8*runwayPoolRows), func(int) ink { return tone })
	}
}

// poolRoom draws the pool's room in the window with the key given, the
// view's, along its row from runwayUseFrom, as usage words the pool's: the
// window's name, as in "5-hour ", in text.subtle, then as poolWords has it,
// as in "1.5 of 3 left · pinned", its figure brightening as it changes, as
// many whole parts as fit before the column roomEnds gives. Where none of the
// pool's accounts has the window, it draws nothing.
func (f Frame) poolRoom(c *canvas, laid runwayLaid, key string) {
	pool := laid.m.Pool()
	i := slices.IndexFunc(pool.Windows, func(w status.PoolWindowNow) bool { return w.Key == key })
	if i < 0 {
		return
	}
	g := gauge{of: Ref{Window: key}, words: poolWords(pool.Windows[i].Room, len(pool.Accounts), pool.Pinned)}
	x := c.text(runwayUseFrom, laid.fit.room, runwayNames[f.Runway.Window]+" ", dimInk)
	c.line(x, laid.fit.room, g.said(f.Width, laid.fit.roomEnds(laid.x, f.Width)-x))
}
