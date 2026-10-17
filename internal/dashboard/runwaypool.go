package dashboard

import (
	"slices"
	"strconv"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayPool draws the pool's lane, as laid out: its columns, Σ pool beside
// them on its first row, as an account's name heads its lane; and its room,
// where it has one, as poolRoom draws it, or with the account column
// collapsed, in Σ pool's place, as poolShort has it. With one account,
// there's none, nor for a moment of none, whatever the lanes hold.
func (f Frame) runwayPool(c *canvas, laid runwayLaid) {
	fit := laid.fit
	if !fit.draws(fit.pool) || len(laid.m.Accounts) == 0 {
		return
	}
	pool := f.Runway.Lanes.Pool
	f.poolColumns(c, laid, pool)
	names := namesOf(laid.m)
	label := poolNamed(names)
	if room, ok := runwayRoom(laid.m, pool.Key); ok && fit.draws(fit.room) {
		if f.Runway.Collapsed {
			label = poolShort(names, room, laid.x-nameGap)
		} else {
			f.poolRoom(c, laid, room)
		}
	}
	c.line(0, fit.pool, label)
}

// runwayPoolRoom is the pool's room in a window, as the moment has it: the
// room, how many accounts the pool is, and whether a pin holds it; and the
// window's key, its figure numbered by it.
type runwayPoolRoom struct {
	key      string
	room     float64
	accounts int
	pinned   bool
}

// runwayRoom is the pool's room of the moment m in the window with the key
// given, the view's, reporting false where none of the pool's accounts has
// the window.
func runwayRoom(m status.Moment, key string) (runwayPoolRoom, bool) {
	pool := m.Pool()
	i := slices.IndexFunc(pool.Windows, func(w status.PoolWindowNow) bool { return w.Key == key })
	if i < 0 {
		return runwayPoolRoom{}, false
	}
	return runwayPoolRoom{key: key, room: pool.Windows[i].Room, accounts: len(pool.Accounts), pinned: pool.Pinned}, true
}

// poolShort is the pool's lane's label with the account column collapsed,
// under the lanes' labels named as names has them, in the cells given, as in
// "Σ  1.2/3": Σ, as poolSigma has it, and the pool's room of how many
// accounts it is, the room bold text.secondary, brightening as it changes,
// and "/3" in text.muted, as collapsedLabelled lays them out.
func poolShort(names []accountName, r runwayPoolRoom, cells int) line {
	figure := line{
		span{prose.Tenths(r.room), strongInk}.numbered(Number{Kind: NumberRoom, Of: r.key}),
		{"/" + strconv.Itoa(r.accounts), mutedInk},
	}
	return collapsedLabelled(line{poolSigma(names)}, figure, cells)
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

// poolRoom draws the pool's room r, in the view's window, along its row from
// runwayUseFrom, as usage words the pool's: the window's name, as in
// "5-hour ", in text.subtle, then as poolWords has it, as in "1.5 of 3 left
// · pinned", its figure brightening as it changes, as many whole parts as
// fit before the column roomEnds gives.
func (f Frame) poolRoom(c *canvas, laid runwayLaid, r runwayPoolRoom) {
	g := gauge{of: Ref{Window: r.key}, words: poolWords(r.room, r.accounts, r.pinned)}
	x := c.text(runwayUseFrom, laid.fit.room, runwayNames[f.Runway.Window]+" ", dimInk)
	c.line(x, laid.fit.room, g.said(f.Width, laid.fit.roomEnds(laid.x, f.Width)-x))
}
