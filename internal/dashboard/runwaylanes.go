package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// The glyphs a lane draws: a column where its account has room, whether it
// runs out at its pace or not, as its colour tells, or where the look has
// no colour, runsOutCell; one where it has none; and where the view's window
// resets. Room fills the cell whole, so a held column's ─ meets it at its
// middle and a reset's │ at its top and bottom.
const (
	roomCell    = "█"
	runsOutCell = "▒"
	heldCell    = "─"
	resetCell   = "│"
)

const (
	// pastFade is how far Runway fades what's before now's column into the
	// canvas, mix(canvas, colour, 0.45): 45% of its colour shows.
	pastFade = 0.55
	// runwayUseFrom is the column the use under each account's name starts
	// at, and the pool's room under Σ pool.
	runwayUseFrom = 2
)

// laidLane is an account's lane as Runway lays it out: its facts, as
// views.Lanes built them; its account, as the moment has it stand; its name,
// as its label heads it, faint where the pool leaves it out; and the row
// it's on, its use and its words on the row under it.
type laidLane struct {
	views.Lane
	account status.AccountNow
	name    accountName
	row     int
}

// laidLanes are the lanes of the moment m that fit shows, each as laidLane has
// it, on the row the fit gives it, in the lanes' order: none before the
// watch builds them, nor where nothing fits.
func (f Frame) laidLanes(m status.Moment, fit runwayFit) []laidLane {
	if f.Runway.Lanes == nil {
		return nil
	}
	names := namesOf(m)
	var lanes []laidLane
	for n, l := range f.Runway.Lanes.Accounts {
		row, shown := fit.laneRow(n)
		if ln, ok := laneOf(l, m, names); shown && ok {
			ln.row = row
			lanes = append(lanes, ln)
		}
	}
	return lanes
}

// laneOf is the lane l as laneLaid has it, of its account as the moment m has
// it stand, named as names name m's accounts: reporting false where m has
// no account of it.
func laneOf(l views.Lane, m status.Moment, names []accountName) (laidLane, bool) {
	i := slices.IndexFunc(m.Accounts, func(a status.AccountNow) bool { return a.ID == l.ID })
	if i < 0 {
		return laidLane{}, false
	}
	return laneLaid(l, m.Accounts[i], names[i]), true
}

// laneLaid is the lane l of the account a as laidLane has it, named as given,
// faint where the pool leaves it out.
func laneLaid(l views.Lane, a status.AccountNow, name accountName) laidLane {
	name.faint = name.out
	return laidLane{Lane: l, account: a, name: name}
}

// runwayLanes draws the lanes, as laid out, each on its row: its account's
// name in the label column before it, cut to fit as line has it, and its
// use of the view's window from runwayUseFrom on the row under, as useLine
// has it; or with the account column collapsed, its number and marks, and
// its use under them, as collapsedUse has it; then the lane itself, as
// drawLane draws it.
func (f Frame) runwayLanes(c *canvas, laid runwayLaid) {
	window := runwayNames[f.Runway.Window]
	for _, ln := range laid.lanes {
		if f.Runway.Collapsed {
			c.line(0, ln.row, ln.name.head())
			c.line(0, ln.row+1, line{ln.collapsedUse()})
		} else {
			c.line(0, ln.row, ln.name.line(laid.x))
			c.line(runwayUseFrom, ln.row+1, ln.useLine(window, laid.m.Now, laid.x-runwayUseFrom-1))
		}
		f.drawLane(c, ln, laid.x)
	}
}

// head is the name as Runway's collapsed account column heads its lane, as
// in "2 ◆": its number and the marks it carries.
func (n accountName) head() line {
	return append(line{n.numbered()}, n.marks()...)
}

// runwayLabelsOf are the labels of lanes named as names has them, ▲'s
// cells kept on every account, as headCells keeps them, so the lanes hold
// still as ▲ moves: expanded, their name column, nameGap after it, as
// accountsColumn has it, as "2 personal ◆"; collapsed, the cells of the
// widest number and marks, as head has them.
func runwayLabelsOf(names []accountName) runwayLabels {
	labels := runwayLabels{expanded: accountsColumn(nameGap, names)}
	for _, n := range names {
		labels.collapsed = max(labels.collapsed, n.headCells())
	}
	return labels
}

// widestFigure is how many cells the widest use a lane's label says takes,
// as 100%.
const widestFigure = len("100%")

// collapsedUse is the lane's use of the view's window as its collapsed label
// says it: its figure, as useFigure has it, in text.faint where the pool
// leaves its account out; or — in text.faint where its use is stale, it has
// no token or it's not read yet, which the expanded label words.
func (ln laidLane) collapsedUse() span {
	u := ln.Use
	switch {
	case u.Stale || u.Tokenless || u.Unread:
		return noFigure
	case ln.account.OutOfPool:
		return span{ln.useFigure().text, faintInk}
	}
	return ln.useFigure()
}

// drawLane draws the lane ln along its row from column x: each column known,
// as laneMark marks it, and │ in text.subtle at each reset of the view's
// window it marks; each faded as laneInk fades it, before now's column as the
// past.
func (f Frame) drawLane(c *canvas, ln laidLane, x int) {
	now, fade := f.Runway.Timeline.NowColumn(), ln.fade()
	for col, cell := range ln.Cells {
		if cell != views.CellUnknown {
			mark := f.Look.laneMark(cell)
			c.text(x+col, ln.row, mark.text, laneInk(mark.ink, col < now, fade))
		}
	}
	for _, r := range ln.Resets {
		c.text(x+r.Column, ln.row, resetCell, laneInk(dimInk, r.Column < now, fade))
	}
}

// laneMark is a column of a lane where its account stands as c, as the lane
// draws it in the look: room, █ in the ramp's first colour; room that runs
// out at its pace, █ in accent.attention, or without colour, runsOutCell;
// at its cap, ─ in accent.attention; and held back from every request, at
// its limit, its token refused or without a usable token, ─ in
// state.destructive, bold without colour, so it tells from a cap. Nothing
// known, it's blank.
func (l Look) laneMark(c views.Cell) span {
	switch c {
	case views.CellRoom:
		return span{roomCell, ink{token: theme.VizRamp1}}
	case views.CellRunsOut:
		if !l.coloured {
			return span{runsOutCell, warningInk}
		}
		return span{roomCell, warningInk}
	case views.CellCapped:
		return span{heldCell, warningInk}
	case views.CellLimited, views.CellRefused, views.CellTokenless:
		if !l.coloured {
			return span{heldCell, exhaustedInk}
		}
		return span{heldCell, errorInk}
	}
	return spaces(1)
}

// laneInk is k as a lane draws it: faded fade into the canvas, as a lane
// stale or out of the pool is, drawn faint where the look can't fade it; and
// in the past, dimmed besides, keeping 45% of what's left of its colour,
// where the look can blend, as it's left as it is where it can't.
func laneInk(k ink, past bool, fade float64) ink {
	keep := 1 - fade
	if past {
		keep *= 1 - pastFade
	}
	k.fade, k.dim = 1-keep, fade > 0
	return k
}

// fade is how far the lane fades into the canvas, as accountFade has it of
// its use.
func (ln laidLane) fade() float64 {
	return accountFade(ln.account, ln.Use.Stale)
}

// accountFade is how far what's drawn of account a's use fades into the
// canvas, as USAGE fades a bar, alike on Runway's lanes and Graphs' cards:
// staleFade, keeping 35% of its colour, where its use is stale, its last
// read failed, but not where its token's refused, which what's drawn of it
// says; as far where the pool leaves the account out; and the two
// compounded, where both hold.
func accountFade(a status.AccountNow, stale bool) float64 {
	keep := 1.0
	if stale && !a.TokenRefused {
		keep *= 1 - staleFade
	}
	if a.OutOfPool {
		keep *= 1 - staleFade
	}
	return 1 - keep
}

// useLine is what the row under the lane's name says of its account's use of
// the view's window, named as given, at now, in cells cells: always short,
// as the label column is never widened for the full words, so it holds
// still as an account goes stale; in text.faint, every part, where the pool
// leaves its account out.
func (ln laidLane) useLine(window string, now time.Time, cells int) line {
	words := ln.useWords(window, now)
	if ln.account.OutOfPool {
		words = words.faint()
	}
	return words.short.fitWhole(cells)
}

// useWords are what's said of the lane's use, as useLine has it: where it's
// stale, as its token's refused or its last read failed, when it was last
// read, not read yet or no token, as USAGE says each of a window; else after
// the window's name in text.subtle, its share used, bold in its level's
// colour, as usage colours it, stepped back the highest the days shown read,
// numbered as its window's use, which brightens as it changes, as today's
// highest can and a day gone's never does; or — in text.faint, where none's
// read of it.
func (ln laidLane) useWords(window string, now time.Time) usageWords {
	u := ln.Use
	switch {
	case u.Stale:
		return wordsOf(status.WindowNow{Kind: status.WindowStale, At: u.ReadAt}, now)
	case u.Tokenless:
		return wordsOf(status.WindowNow{Kind: status.WindowTokenless}, now)
	case u.Unread:
		return wordsOf(status.WindowNow{Kind: status.WindowUnread}, now)
	}
	return alikeLine(line{{window + " ", dimInk}, ln.useFigure()})
}

// useFigure is the lane's share used of the view's window, as its label says
// it after the window's name: bold in its level's colour, as usage colours
// it, numbered as its window's use; or — in text.faint, where none's read of
// it.
func (ln laidLane) useFigure() span {
	u := ln.Use
	if !u.Read {
		return noFigure
	}
	used := span{prose.Percent(u.Used), usedInk(u.Used, ink{token: theme.VizRamp1, bold: true})}
	return used.numbered(Number{Kind: NumberUsed, Account: ln.ID, Of: u.Key})
}

// nowTo is the last row now's column is picked out on, as fitted: the pool's
// lane's last, or without a pool's lane, the last lane shown's words'.
func (r runwayFit) nowTo() int {
	if r.draws(r.pool) {
		return r.pool + runwayPoolRows - 1
	}
	return r.lanes + (r.shown-1)*r.apart + 1
}

// alikeLine is words that read as l in either form.
func alikeLine(l line) usageWords {
	return usageWords{full: l, short: l}
}

// runwayNow picks out now's column, as laid out, where the timeline has a
// now: bg.subtle under every cell of it from the axis's first row down to
// the fit's nowTo that has no surface of its own. Where the look can't
// blend, as it can't dim the past either, it's left as it is.
func (f Frame) runwayNow(c *canvas, laid runwayLaid) {
	tl, fit := f.Runway.Timeline, laid.fit
	col := tl.NowColumn()
	if !fit.fits || col < 0 || col >= tl.Columns || !f.Look.blends() {
		return
	}
	for y := fit.axis; y <= fit.nowTo(); y++ {
		if here := c.at(laid.x+col, y); here != nil && here.ink.on == (hue{}) {
			here.ink.on = hue{token: theme.BgSubtle}
		}
	}
}
