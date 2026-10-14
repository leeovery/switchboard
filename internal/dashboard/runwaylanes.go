package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// The glyphs a lane is drawn in: a column with room, running out or not, as
// its colour tells; one running out where the look has no colour; one
// without room; and a reset of the view's window.
const (
	roomGlyph = "▆"
	// shadeCell is a lane draining where the look has no colour.
	shadeCell   = "▒"
	noRoomGlyph = "─"
	resetGlyph  = "│"
)

const (
	// pastFade is how far a lane's past fades into the canvas: it keeps 45%
	// of its colour.
	pastFade = 0.55
	// runwayUseFrom is the column the use under an account's name starts at.
	runwayUseFrom = 2
)

// laidLane is an account's lane as Runway lays it out: its facts, as
// views.Lanes built them; its account, as the moment has it stand; its name,
// as its label heads it, faint where the pool leaves it out; and the row
// it's on, its use and its words on the row under it, -1 where it's out of
// view.
type laidLane struct {
	views.Lane
	account status.AccountNow
	name    accountName
	row     int
}

// lanesOf are the lanes Runway shows of the moment m, each as laidLane has
// it, in the lanes' order, out of view till they're laid out: none before
// the watch builds them.
func (f Frame) lanesOf(m status.Moment) []laidLane {
	if f.Runway.Lanes == nil {
		return nil
	}
	names := namesOf(m)
	var lanes []laidLane
	for _, l := range f.Runway.Lanes.Accounts {
		i := slices.IndexFunc(m.Accounts, func(a status.AccountNow) bool { return a.ID == l.ID })
		if i < 0 {
			continue
		}
		name := names[i]
		name.faint = name.out
		lanes = append(lanes, laidLane{Lane: l, account: m.Accounts[i], name: name, row: -1})
	}
	return lanes
}

// runwayLanes draws the lanes, as laid out, each on its row: its account's
// name in the label column before it, and its use of the view's window from
// runwayUseFrom on the row under, as useLine has it; then the lane itself, as
// drawLane draws it.
func (f Frame) runwayLanes(c *canvas, laid runwayLaid) {
	window := runwayNames[f.Runway.Window]
	for _, ln := range laid.lanes {
		if ln.row < 0 {
			continue
		}
		c.line(0, ln.row, ln.name.line(laid.x))
		c.line(runwayUseFrom, ln.row+1, ln.useLine(window, laid.now, f.Width, laid.x-runwayUseFrom-1))
		f.drawLane(c, ln, laid.x)
	}
}

// drawLane draws the lane ln along its row from column x: a cell a column, as
// cellDrawn draws each, and │ in text.subtle at each reset of the view's
// window it marks; each faded as laneInk fades it, before now's column as the
// past.
func (f Frame) drawLane(c *canvas, ln laidLane, x int) {
	now, fade := f.Runway.Timeline.NowColumn(), ln.fade()
	for col, cell := range ln.Cells {
		if glyph, k := cellDrawn(f.Look, cell); glyph != "" {
			c.text(x+col, ln.row, glyph, laneInk(k, col < now, fade))
		}
	}
	for _, r := range ln.Resets {
		c.text(x+r.Column, ln.row, resetGlyph, laneInk(dimInk, r.Column < now, fade))
	}
}

// cellDrawn is the glyph and ink a lane draws a column in where its account
// stands as cell says: none where nothing is known of it; with room, ▆ in the
// ramp's first stop; running out at its pace, ▆ in accent.attention, or where
// the look has no colour, ▒; at its cap, ─ in accent.attention; and at its
// limit, its token refused or without one, ─ in state.destructive, bold where
// the look has no colour to tell it from a cap.
func cellDrawn(look Look, cell views.Cell) (string, ink) {
	switch cell {
	case views.CellRoom:
		return roomGlyph, ink{token: theme.VizRamp1}
	case views.CellRunsOut:
		if !look.coloured {
			return shadeCell, warningInk
		}
		return roomGlyph, warningInk
	case views.CellCapped:
		return noRoomGlyph, warningInk
	case views.CellLimited, views.CellRefused, views.CellTokenless:
		return noRoomGlyph, ink{token: theme.StateDestructive, bold: !look.coloured}
	}
	return "", ink{}
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

// fade is how far the lane fades into the canvas, as USAGE fades a bar:
// staleFade, keeping 35% of its colour, where its use is stale, its last
// read failed, but not where its token's refused, which its lane draws; as
// far where the pool leaves its account out; and the two compounded, where
// both hold.
func (ln laidLane) fade() float64 {
	keep := 1.0
	if ln.Use.Stale && !ln.account.TokenRefused {
		keep *= 1 - staleFade
	}
	if ln.account.OutOfPool {
		keep *= 1 - staleFade
	}
	return 1 - keep
}

// useLine is what the row under the lane's name says of its account's use of
// the view's window, named as given, at now, on a frame width columns wide,
// in cells cells: in full where it fits, else short, as pick picks them; in
// text.faint, every part, where the pool leaves its account out.
func (ln laidLane) useLine(window string, now time.Time, width, cells int) line {
	words := ln.useWords(window, now)
	if ln.account.OutOfPool {
		words = words.faint()
	}
	return words.pick(width, cells)
}

// useWords are what's said of the lane's use, as useLine has it: where it's
// stale, as its token's refused or its last read failed, when it was last
// read, not read yet or no token, as USAGE says each of a window; else after
// the window's name in text.subtle, its share used, bold in its level's
// colour, as usage colours it, stepped back the highest the days shown read,
// live numbered as its window's use, which brightens as it changes; or — in
// text.faint, where none's read of it.
func (ln laidLane) useWords(window string, now time.Time) usageWords {
	u := ln.Use
	switch {
	case u.Stale:
		return wordsOf(status.WindowNow{Kind: status.WindowStale, At: u.ReadAt}, now)
	case u.Tokenless:
		return wordsOf(status.WindowNow{Kind: status.WindowTokenless}, now)
	case u.Unread:
		return wordsOf(status.WindowNow{Kind: status.WindowUnread}, now)
	case !u.Read:
		return alikeLine(line{{window + " ", dimInk}, noFigure})
	}
	used := span{prose.Percent(u.Used), usedInk(u.Used, ink{token: theme.VizRamp1, bold: true})}
	if !u.Highest {
		used = used.numbered(Number{Kind: NumberUsed, Account: ln.ID, Of: u.Key})
	}
	return alikeLine(line{{window + " ", dimInk}, used})
}

// alikeLine is words that read as l in either form.
func alikeLine(l line) usageWords {
	return usageWords{full: l, short: l}
}

// runwayNow picks out now's column, as laid out, where the timeline has a
// now: bg.subtle under every cell of it from the axis's first row down to
// row nowTo that has no surface of its own. Where the look can't blend, as
// it can't dim the past either, it's left as it is.
func (f Frame) runwayNow(c *canvas, laid runwayLaid) {
	tl := f.Runway.Timeline
	col := tl.NowColumn()
	if col < 0 || col >= tl.Columns || !f.Look.blends() {
		return
	}
	for y := laid.top; y <= laid.nowTo; y++ {
		if here := c.at(laid.x+col, y); here != nil && here.ink.on == (hue{}) {
			here.ink.on = hue{token: theme.BgSubtle}
		}
	}
}
