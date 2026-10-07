package dashboard

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// The rows Runway takes under the heading, from its top: now's label, the
// hours or the days, and their ticks; a blank; the strip, two rows; a blank;
// then the lanes, each a row and its words, laneGap blank rows between them.
const (
	axisRows  = 3
	stripRows = 2
	laneRows  = 2
	laneGap   = 1
	lanesFrom = axisRows + 1 + stripRows + 1
)

// The labels at the lanes' left: the strip's, over the day and over the
// week, under its heading; the most cells a name takes; the blank cells
// between a badge, at the labels' right, and the lane; and the next
// account's badge, briefly, where its name has too little room beside it.
const (
	stripHead      = "ROOM"
	dayStripLabel  = "accounts with room"
	weekStripLabel = "weeks with room"
	nameCells      = 24
	badgeGap       = 2
	briefNext      = "▲"
)

const (
	// pastFade dims what Runway draws of the past.
	pastFade = 0.55
	// saysGap is the fewest blank cells between the words under a lane.
	saysGap = 2
	// legendGap is the blank cells between the legend's glyphs.
	legendGap = 6
	// noRoomGlyph is a lane where its account has no room, and resetGlyph
	// where its week resets.
	noRoomGlyph = "─"
	resetGlyph  = "┃"
)

// The day's ticks fall on hours as many apart as the first of hourSteps
// that puts them tickCells apart, at least, and its labels the first that
// puts them labelCells apart: each divides a day, so they fall on the same
// hours every day.
var hourSteps = []int{1, 2, 3, 6, 12, 24}

const (
	tickCells  = 3
	labelCells = 10
	// quarterCells is the fewest cells a day spans for the week's axis to
	// tick its quarters.
	quarterCells = 12
)

// runwayLayout is Runway laid out under the heading: the column its lanes
// start at, and the timeline across them; its lanes, and the columns of
// each; from row top, the rows the lanes take, content, and the rows of them
// shown at once, view, fewer where they scroll; and whether the legend
// shows.
type runwayLayout struct {
	x             int
	timeline      timeline
	lanes         []lane
	cells         [][]room
	top           int
	content, view int
	legend        bool
}

// layOutRunway lays Runway of doc at now out under the heading, which ends
// at row top. The lanes start after the labels, as lanesAt says, and run to
// a cell short of the frame's edge, clear of a scrollbar. They get the rows
// first; then the legend, where a blank and its line are left over, and it
// fits the width whole; where not even the lanes fit, they scroll between
// the strip and the line over the footer.
func (f Frame) layOutRunway(doc status.Document, now time.Time, top int) runwayLayout {
	l := runwayLayout{top: top + lanesFrom}
	for _, a := range doc.Accounts {
		l.lanes = append(l.lanes, f.laneOf(doc, a, now))
	}
	l.x = lanesAt(l.lanes, f.Width)
	l.timeline = timelineOf(f.Span, now, f.edge()-1-l.x)
	for _, ln := range l.lanes {
		l.cells = append(l.cells, ln.cells(l.timeline))
	}
	l.content, l.view = f.runwayRows(len(l.lanes), top)
	l.legend = (f.printed() || l.content+2 <= f.Height-2-l.top) && f.legendFits(l.timeline)
	return l
}

// runwayRows are how many rows n accounts' lanes take, content, and how many
// of them show at once under the heading, which ends at row top, view: fewer
// where they don't fit over the line over the footer, and all of them,
// printed once, where there's no height to fit.
func (f Frame) runwayRows(n, top int) (content, view int) {
	content = max(n*(laneRows+laneGap)-laneGap, 0)
	if f.printed() {
		return content, content
	}
	return content, min(content, max(f.Height-2-top-lanesFrom, 0))
}

// scrolls reports whether the lanes scroll, more of them than the view
// shows.
func (l runwayLayout) scrolls() bool {
	return l.content > l.view
}

// laneAt is the row lane i takes, from the lanes' top.
func laneAt(i int) int {
	return i * (laneRows + laneGap)
}

// hidden counts the lanes out of view, wholly or partly, as the view shows
// them from the row offset: those above, and those below.
func (l runwayLayout) hidden(offset int) (above, below int) {
	for i := range l.lanes {
		if laneAt(i) < offset {
			above++
		}
		if laneAt(i)+laneRows > offset+l.view {
			below++
		}
	}
	return above, below
}

// lanesAt is the column lanes start at, as their labels leave it, on a
// frame width cells wide: labelGap cells after the widest of the strip's
// labels, and badgeGap after the widest of the lanes' labels with room
// beside it for the widest badge, so the lanes stay put as w switches
// between the day and the week; but never past halfway across, where names
// and badges give way, as runwayLane draws them.
func lanesAt(lanes []lane, width int) int {
	least := margin + max(ansi.StringWidth(dayStripLabel), ansi.StringWidth(weekStripLabel)) + labelGap
	x := least
	for _, l := range lanes {
		x = max(x, margin+laneLabel(l).width()+1+ansi.StringWidth(nextBadge)+badgeGap)
	}
	return min(x, max(least, width/2))
}

// runway draws Runway of doc at now from row top: over the timeline, the
// axis; the strip; and the lanes, each with its words under it, scrolled
// where they don't fit, a scrollbar beside them; now's column picked out
// from the axis to the lanes' foot. Then, over the footer, the legend where
// it shows, and under it, what the lanes don't show: those out of view,
// where they scroll, or that the key's behind ?, where the legend doesn't
// show.
func (f Frame) runway(c *canvas, doc status.Document, now time.Time, top int) {
	if len(doc.Accounts) == 0 {
		return
	}
	l := f.layOutRunway(doc, now, top)
	f.runwayAxis(c, l, top)
	f.roomStrip(c, l, top+axisRows+1)
	offset := min(max(f.Scroll, 0), l.content-l.view)
	area := newCanvas(f.Width, l.content)
	for i, ln := range l.lanes {
		f.runwayLane(area, doc, ln, l.cells[i], l, now, laneAt(i))
	}
	c.paste(area, offset, l.top, l.view)
	if col := l.timeline.nowColumn(); col >= 0 && col < l.timeline.columns {
		for y := top; y < l.top+l.view; y++ {
			c.surface(l.x+col, y, 1, hue{token: theme.BgSubtle})
		}
	}
	legend, over := f.foot(l.top+l.view, l.legend)
	if l.legend {
		c.line(margin, legend, f.legend(l.timeline))
	}
	if over < l.top {
		return
	}
	switch {
	case l.scrolls():
		says := outOfView(l.hidden(offset)).fit(f.edge() - margin)
		c.line((f.Width-says.width())/2, over, says)
		scrollbar(c, f.Width-1, l.top, l.view, l.content, offset)
	case !l.legend:
		c.right(f.edge(), over, forTheKey.fit(f.edge()-margin))
	}
}

// runwayAxis draws the axis over the timeline of l from row y: now, over its
// column; under it, the hours, over the day, or the days, over the week;
// and under them, their ticks along a rule.
func (f Frame) runwayAxis(c *canvas, l runwayLayout, y int) {
	tl := l.timeline
	if col := tl.nowColumn(); col >= 0 && col < tl.columns {
		c.text(l.x+col-1, y, "now", titleInk)
	}
	c.text(l.x, y+2, rule(tl.columns), borderInk)
	if f.Span == Week {
		weekdays(c, tl, l.x, y+1)
		return
	}
	hours(c, tl, l.x, y+1)
}

// apart is the first of hourSteps whose hours span cells columns of the
// timeline at least.
func (tl timeline) apart(cells int) int {
	for _, h := range hourSteps {
		if time.Duration(h)*time.Hour >= time.Duration(cells)*tl.step {
			return h
		}
	}
	return hourSteps[len(hourSteps)-1]
}

// hours draws the day's hours over its timeline from x along row y, and
// their ticks along the rule under them: a tick on the hours as far apart as
// tickCells asks, and on the hours as far apart as labelCells asks, a tick
// too, and over it, where it fits, the hour, as 16:00, muted, none on an
// hour the clocks went forward over; and on each day's start, as
// dayfile.DayStart gives it, its midnight or wherever the clocks put it, a
// tick, and over it, where it fits, the day it starts, as Fri, picked out.
func hours(c *canvas, tl timeline, x, y int) {
	ticks, labels := tl.apart(tickCells), tl.apart(labelCells)
	from := tl.start
	for i := 0; ; i++ {
		hour := (from.Hour() + i) % 24
		t := time.Date(from.Year(), from.Month(), from.Day(), from.Hour()+i, 0, 0, 0, from.Location())
		col := tl.column(t)
		if col >= tl.columns {
			break
		}
		if col < 0 || t.Hour() != hour || t.Equal(dayfile.DayStart(t, 0)) || hour%ticks != 0 && hour%labels != 0 {
			continue
		}
		c.text(x+col, y+1, "┬", borderInk)
		if label := (line{{t.Format("15:04"), mutedInk}}); hour%labels == 0 && col+label.width() <= tl.columns {
			c.line(x+col, y, label)
		}
	}
	dayStarts(c, tl, x, y)
}

// dayStarts ticks the start of each day over the timeline from x along the
// rule on row y+1, as hours says, and names it over the tick on row y, where
// that fits.
func dayStarts(c *canvas, tl timeline, x, y int) {
	for at := dayfile.DayStart(tl.start, 0); ; at = dayfile.DayStart(at, 1) {
		col := tl.column(at)
		switch {
		case col >= tl.columns:
			return
		case col < 0:
			continue
		}
		c.text(x+col, y+1, "┬", borderInk)
		if name := (line{{at.Format("Mon"), strongInk}}); col+name.width() <= tl.columns {
			c.line(x+col, y, name)
		}
	}
}

// weekdays draws the week's days over its timeline from x along row y, and
// their ticks along the rule under them: where a day spans quarterCells
// columns, its quarters, as quarters ticks them; and a tick at each
// midnight, and over it, the day's name, where it clears the one before and
// fits, as midnights has them, today's picked out.
func weekdays(c *canvas, tl timeline, x, y int) {
	if day >= quarterCells*tl.step {
		quarters(c, tl, x, y+1)
	}
	end := tl.start.Add(time.Duration(tl.columns) * tl.step)
	for _, m := range midnights(tl.start, end, tl.now, tl.column, tl.columns, len("Mon")) {
		c.text(x+m.col, y+1, "┬", borderInk)
		if !m.named {
			continue
		}
		name := line{{m.at.Format("Mon"), mutedInk}}
		if m.today {
			name[0].ink = strongInk
		}
		c.line(x+m.col, y, name)
	}
}

// quarters ticks the quarters of the days over the timeline from x along
// row y, faint, the first day's among them, which starts before the
// timeline, but each fourth, the midnight, which its day's own tick marks:
// time.Date puts one the clocks went forward over in the hour before.
func quarters(c *canvas, tl timeline, x, y int) {
	from := tl.start
	for i := 1; ; i++ {
		at := time.Date(from.Year(), from.Month(), from.Day(), 6*i, 0, 0, 0, from.Location())
		col := tl.column(at)
		switch {
		case col >= tl.columns:
			return
		case col >= 0 && i%4 != 0:
			c.text(x+col, y, "╵", faintInk)
		}
	}
}

// roomStrip draws the strip over the lanes of l from row y: its heading and
// label at the left; then in each column of the timeline, how many of the
// lanes' accounts known there have room, as a level two rows tall, filled to
// the share of them that do, coloured along the ramp as stripTone says, the
// past dimmed, and where none does, a line along its floor. Where nothing is
// known of any, as before anything is read, nothing is drawn.
func (f Frame) roomStrip(c *canvas, l runwayLayout, y int) {
	label := dayStripLabel
	if f.Span == Week {
		label = weekStripLabel
	}
	c.line(margin, y, line{{stripHead, labelInk}})
	c.line(margin, y+1, line{{label, mutedInk}})
	for col := range l.timeline.columns {
		known, with := 0, 0
		for _, cells := range l.cells {
			if r := cells[col]; r != roomUnknown {
				known++
				if r.has() {
					with++
				}
			}
		}
		if known == 0 {
			continue
		}
		k := ink{ramp: true, at: stripTone(with, known)}
		if col < l.timeline.nowColumn() {
			k.fade = pastFade
		}
		if with == 0 {
			c.text(l.x+col, y+1, floorLine, k)
			continue
		}
		filled := max(int(math.Round(float64(8*stripRows*with)/float64(known))), 1)
		c.text(l.x+col, y+1, levels[min(filled, 8)], k)
		c.text(l.x+col, y, levels[max(filled-8, 0)], k)
	}
}

// stripTone is where along the ramp the strip is coloured where with of n
// accounts have room: at its first stop where every one does, its second
// where at least half do, its third where fewer, and its last where none.
func stripTone(with, n int) float64 {
	switch {
	case with == n:
		return 0
	case 2*with >= n:
		return 1.0 / 3
	case with > 0:
		return 2.0 / 3
	default:
		return 1
	}
}

// laneLabel is what heads lane l in the labels' column: its place, dim, and
// its account's name, cut to nameCells.
func laneLabel(l lane) line {
	return line{{strconv.Itoa(l.place) + " ", dimInk}, {truncate(name(l.account), nameCells), titleInk}}
}

// badge is what lane l has at the labels' right: over the day, ▲ next on the
// account new sessions go to, of several; over the week, its week's use,
// bold, in attention where it runs out before its reset, in destructive at
// its limit, else secondary.
func (f Frame) badge(doc status.Document, l lane, now time.Time) line {
	if f.Span != Week {
		if l.known && len(doc.Accounts) > 1 && doc.Best == l.account.ID {
			return line{{nextBadge, nextInk}}
		}
		return nil
	}
	weeks := f.weeks(l)
	if len(weeks) == 0 {
		return nil
	}
	s := standingOf(doc, l.account, weeks[0], now, f.Policy)
	if s.runsOut {
		return line{s.use(warningInk)}
	}
	return line{s.use(secondaryInk)}
}

// runwayLane draws lane ln, its columns cells, on c from row y as layout l
// lays it out: its label, and its badge at the labels' right, the label cut
// to fit beside it, and the next account's badge brief where the label
// can't show whole beside it; its room in each column of the timeline, the
// past dimmed; over the week, ┃ where each week resets; and under it, its
// words.
func (f Frame) runwayLane(c *canvas, doc status.Document, ln lane, cells []room, l runwayLayout, now time.Time, y int) {
	label, badge, room := laneLabel(ln), f.badge(doc, ln, now), l.x-badgeGap-margin
	if label.width()+1+badge.width() > room && len(badge) == 1 && badge[0].text == nextBadge {
		badge[0].text = briefNext
	}
	c.line(margin, y, label.fit(room-1-badge.width()))
	c.right(l.x-badgeGap, y, badge)
	for col, r := range cells {
		glyph, k := f.cellOf(r)
		if col < l.timeline.nowColumn() {
			k.fade = pastFade
		}
		c.text(l.x+col, y, glyph, k)
	}
	for _, w := range f.weeks(ln) {
		if col := l.timeline.column(w.ResetsAt); col >= 0 && col < l.timeline.columns {
			c.text(l.x+col, y, resetGlyph, titleInk)
		}
	}
	f.laneWords(c, doc, ln, l, now, y+1)
}

// cellOf is how a lane draws a column where its account stands as r: ▆ in
// positive with room; draining, ▆ in attention, or without colour, in shade;
// ─ in destructive without room; and nothing where nothing is known.
func (f Frame) cellOf(r room) (string, ink) {
	switch r {
	case roomOpen:
		return roomGlyph, positiveInk
	case roomDraining:
		if !f.Look.coloured {
			return shadeCell, warningInk
		}
		return roomGlyph, warningInk
	case roomNone:
		return noRoomGlyph, errorInk
	default:
		return "", ink{}
	}
}

// weeks are the windows of lane l's account the week counts, in the
// account's order: none over the day, nor of an account nothing is known of.
func (f Frame) weeks(l lane) []quota.Window {
	if f.Span != Week || !l.known {
		return nil
	}
	return slices.DeleteFunc(slices.Clone(l.account.Windows), func(w quota.Window) bool { return !f.counts(w) })
}

// placed is words placed under a lane, from its column col of the timeline.
type placed struct {
	col   int
	words line
}

// laneWords draws the words under lane ln, of layout l, along row y, each at
// the column it's of, left to right, saysGap clear of those before, and
// those after them left off where they don't fit whole: what holds it back
// over each stretch without room, as placedTold places it; over the week, at
// each week's reset in view that no stretch is back at, ending under it,
// when it resets and how much it will have used by then; and where neither
// is, from just after now, that it has room all along, or that nothing has
// been read of it.
func (f Frame) laneWords(c *canvas, doc status.Document, ln lane, l runwayLayout, now time.Time, y int) {
	tl := l.timeline
	var words []placed
	for _, s := range ln.stretches {
		words = append(words, placedTold(s, tl, now)...)
	}
	for _, w := range f.weeks(ln) {
		col := tl.column(w.ResetsAt)
		back := slices.ContainsFunc(ln.stretches, func(s stretch) bool { return s.until.Equal(w.ResetsAt) })
		if col < 0 || col >= tl.columns || back {
			continue
		}
		says := resetSays(standingOf(doc, ln.account, w, now, f.Policy), now)
		words = append(words, placed{col: max(col-says.width()+1, 0), words: says})
	}
	if len(words) == 0 {
		words = []placed{{col: tl.nowColumn() + 1, words: f.allAlong(ln, now)}}
	}
	slices.SortStableFunc(words, func(a, b placed) int { return a.col - b.col })
	end := l.x - saysGap
	for _, p := range words {
		x := max(l.x+p.col, end+saysGap)
		fitted := p.words.fitWhole(f.edge() - x)
		if fitted.width() < min(leastShown, p.words.width()) {
			return
		}
		end = c.line(x, y, fitted)
	}
}

// placedTold places what's told of the stretch s, as stretch.told has it, on
// the timeline: its first cause's words from the column the stretch shows
// from, and each other's from the column it shows from, none before the
// first's; a cause that shows in none, as one starting after the timeline,
// left untold.
func placedTold(s stretch, tl timeline, now time.Time) []placed {
	first, ok := s.column(tl)
	if !ok {
		return nil
	}
	var told []placed
	for i, says := range s.told(now) {
		col := first
		if i > 0 {
			if col, ok = s.causes[i].column(tl); !ok {
				continue
			}
			col = max(col, first)
		}
		told = append(told, placed{col: col, words: says})
	}
	return told
}

// resetSays is what's said where a week standing as s resets: when, and
// where it's heading, how much it will have used by then, as in "resets Mon
// 21:00 · 87% used by then".
func resetSays(s standing, now time.Time) line {
	l := line{{"resets " + status.Dated(now, s.window.ResetsAt), secondaryInk}}
	if share, ok := s.projected(); ok {
		l = append(l, span{status.Separator + status.Percent(share) + " used by then", mutedInk})
	}
	return l
}

// allAlong is what's said under lane l without a stretch or a reset in
// view: that nothing has been read of its account; else that it has room
// all day, and when it runs out, where it's heading to beyond the day shown,
// as in "room all day · week runs out ~Sun 04:06"; or all week, and when its
// week resets, beyond the week shown.
func (f Frame) allAlong(l lane, now time.Time) line {
	switch weeks := f.weeks(l); {
	case !l.known:
		return line{{"not read yet", dimInk}}
	case f.Span != Week && l.drained != "":
		return line{{"room all day" + status.Separator + l.drained, dimInk}}
	case f.Span != Week:
		return line{{"room all day", dimInk}}
	case len(weeks) > 0 && !weeks[0].ResetsAt.IsZero():
		return line{{"room all week" + status.Separator + "resets " + status.Dated(now, weeks[0].ResetsAt), dimInk}}
	default:
		return line{{"room all week", dimInk}}
	}
}

// legendGlyphs are the glyphs of Runway's lanes, drawn bold, as the legend
// draws them, and what each means: room, room but running out, and no room;
// and over the week, where a week resets.
func (f Frame) legendGlyphs() []glyph {
	var glyphs []glyph
	for _, r := range []struct {
		room  room
		means string
	}{{roomOpen, "has room"}, {roomDraining, "has room, but running out"}, {roomNone, "no room"}} {
		drawn, k := f.cellOf(r.room)
		k.bold = true
		glyphs = append(glyphs, glyph{drawn: line{{drawn, k}}, means: r.means})
	}
	if f.Span == Week {
		glyphs = append(glyphs, glyph{drawn: line{{resetGlyph, titleInk}}, means: "week resets"})
	}
	return glyphs
}

// legendFits reports whether the legend, for the timeline tl, fits the
// frame's width whole: where it doesn't, the key's behind ?.
func (f Frame) legendFits(tl timeline) bool {
	return margin+f.legend(tl).width() <= f.edge()
}

// legend is Runway's key to its glyphs, the line over the footer, for the
// timeline tl: each glyph and what it means, legendGap apart, or on a phone,
// as close as the footer's keys; then, where it fits, over the day, that the
// past is dimmed, where the look dims it, and what the words say; or over
// the week, how long a column is, and that yesterday's dimmed, where it is.
func (f Frame) legend(tl timeline) line {
	gap := legendGap
	if f.phone() {
		gap = phoneKeysGap
	}
	var l line
	for i, g := range f.legendGlyphs() {
		if i > 0 {
			l = append(l, spaces(gap))
		}
		l = append(append(l, g.drawn...), span{" " + g.means, mutedInk})
	}
	if noted := slices.Concat(l, line{spaces(gap), {f.legendNote(tl), dimInk}}); margin+noted.width() <= f.edge() {
		return noted
	}
	return l
}

// legendNote is what the legend says besides its glyphs, for the timeline
// tl: over the day, that the past is dimmed, where the look dims it, and
// what the words say; over the week, how long a column is, and that
// yesterday's dimmed, where it is.
func (f Frame) legendNote(tl timeline) string {
	dims := f.Look.blends()
	if f.Span == Week {
		if dims {
			return "one column ≈ " + about(tl.step) + " · yesterday dimmed"
		}
		return "one column ≈ " + about(tl.step)
	}
	if dims {
		return "past hours dimmed · labels say when, and when it's back"
	}
	return "labels say when, and when it's back"
}

// about says about how long d is: to the five minutes, as "75 min", under
// two hours; else to the hour, as "6h".
func about(d time.Duration) string {
	if d < 2*time.Hour {
		return fmt.Sprintf("%d min", d.Round(5*time.Minute)/time.Minute)
	}
	return fmt.Sprintf("%dh", d.Round(time.Hour)/time.Hour)
}
