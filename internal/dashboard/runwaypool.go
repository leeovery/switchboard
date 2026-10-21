package dashboard

import (
	"slices"
	"strconv"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayPool draws the pool's lane, as laid out: its columns, and Σ pool
// beside them on its first row, as an account's name heads its lane, or with
// the account column collapsed, Σ alone; and on the row of its own under it,
// its room, as roomSaid has it. With one account, there's none, nor for a
// moment of none, whatever the lanes hold.
func (f Frame) runwayPool(c *canvas, laid runwayLaid) {
	fit := laid.fit
	if !fit.draws(fit.pool) || len(laid.m.Accounts) == 0 {
		return
	}
	f.poolColumns(c, laid, f.Runway.Lanes.Pool)
	names := namesOf(laid.m)
	label := poolNamed(names)
	if f.Runway.Collapsed {
		label = line{poolSigma(names)}
	}
	c.line(0, fit.pool, label)
	if x, room := f.roomSaid(laid); room != nil {
		c.line(x, fit.room, room)
	}
}

// roomSaid is what the row under the pool's lane says of its room, as laid
// out, and the column it starts at: from runwayUseFrom, as poolRoom says it,
// or with the account column collapsed, from column 0, as poolShort has it.
// It says nothing where the pool's lane or its room isn't drawn, or none of
// the pool's accounts has the view's window.
func (f Frame) roomSaid(laid runwayLaid) (int, line) {
	fit := laid.fit
	if !fit.draws(fit.pool) || !fit.draws(fit.room) || len(laid.m.Accounts) == 0 {
		return 0, nil
	}
	room, ok := runwayRoom(laid.m, f.Runway.Lanes.Pool.Key)
	switch {
	case !ok:
		return 0, nil
	case f.Runway.Collapsed:
		return 0, poolShort(room)
	}
	return runwayUseFrom, f.poolRoom(room)
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

// poolShort is the pool's room r as the collapsed account column says it,
// as in "1.2/3": the room of how many accounts the pool is, the room bold
// text.secondary, brightening as it changes, and "/3" in text.muted.
func poolShort(r runwayPoolRoom) line {
	return line{
		span{prose.Tenths(r.room), strongInk}.numbered(Number{Kind: NumberRoom, Of: r.key}),
		{"/" + strconv.Itoa(r.accounts), mutedInk},
	}
}

// poolColumns draws the pool's columns on its lane's rows, from the lanes'
// column, as fillUp fills a column: each as high as the share of the pool's
// accounts with room at the column's moment, of its rows, in eighths of a
// cell, in the colour usage gives the pool's use, the share without room,
// faded as the past is before now's column. A column where the pool is at
// its limit is poolLimitMark's on its lower row, faded as the past is
// likewise; one where none's state is known is left blank.
func (f Frame) poolColumns(c *canvas, laid runwayLaid, pool views.Pool) {
	now, floor := f.Runway.Timeline.NowColumn(), laid.fit.pool+runwayPoolRows-1
	for i, col := range pool.Columns {
		if col.Limited() {
			mark := f.Look.poolLimitMark()
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

// poolLimitCell is a column of the pool's lane where the pool is at its
// limit: a quarter-cell along the lane's floor, so it steps down from a
// half-cell column beside it, where a lane's ─ would sit level with its top.
const poolLimitCell = "▂"

// poolLimitMark is poolLimitCell in a limit's ink, as laneMark inks a lane
// at its limit.
func (l Look) poolLimitMark() span {
	mark := l.laneMark(views.CellLimited)
	mark.text = poolLimitCell
	return mark
}

// poolRoom is the pool's room r, in the view's window, as usage words the
// pool's, from runwayUseFrom: the window's name, as in "5-hour ", in
// text.subtle, then as poolWords has it, as in "1.5 of 3 left · pinned", its
// figure brightening as it changes, as many whole parts as fit before the
// frame's edge.
func (f Frame) poolRoom(r runwayPoolRoom) line {
	name := line{{runwayNames[f.Runway.Window] + " ", dimInk}}
	g := gauge{of: Ref{Window: r.key}, words: poolWords(r.room, r.accounts, r.pinned)}
	return slices.Concat(name, g.said(f.Width, f.Width-runwayUseFrom-name.width()))
}
