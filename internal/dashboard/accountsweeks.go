package dashboard

import (
	"strconv"
	"strings"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// Detail's last six weeks, under its rule: BY DAY, WEEKS and the verdict, of
// the account shown, or all accounts.

// byDay draws BY DAY along row y: its heading; from cx, the days of the last
// of its weeks, as many as given, a day dw cells wide and the weeks gap cells
// apart, each in Days' shades at its level on the account's own scale, a red
// ▪ in its middle where it hit a 5-hour limit, and · in text.faint a cell of
// each day still to come; and on the row under them, each week's first day,
// this week's bold.
func (d detailDrawn) byDay(c *canvas, y, weeks int) {
	c.line(d.l.panel, y, line{{"BY DAY", dimInk}})
	days, coming := d.row.Days, len(d.row.Days)-d.page.ToCome
	first := max(len(days)-7*weeks, 0)
	for i := first; i < len(days); i++ {
		x := d.l.cx() + (i-first)/7*d.l.step() + (i-first)%7*d.l.dw
		if i >= coming {
			c.line(x, y, dayToCome(d.l.dw))
			continue
		}
		c.line(x, y, d.f.daySwatch(dayInk(days[i].Level), days[i].Level, days[i].Limit, d.l.dw))
	}
	heads := d.page.Weeks[max(len(d.page.Weeks)-weeks, 0):]
	for i, week := range heads {
		c.line(d.l.cx()+i*d.l.step(), y+1, weekHead(week, i == len(heads)-1))
	}
}

// dayToCome is a day of BY DAY still to come, cells wide: a · a cell, in
// text.faint.
func dayToCome(cells int) line {
	return line{{strings.Repeat("·", cells), faintInk}}
}

// weeks draws WEEKS along row y: its heading; and from cx, a step apart,
// each of the six weeks' peaks as usage's bar draws a share, weekBar cells
// long, and a cell on, as peakSaid says it.
func (d detailDrawn) weeks(c *canvas, y int) {
	c.line(d.l.panel, y, line{{"WEEKS", dimInk}})
	cells := d.l.weekBar()
	for i, p := range d.row.Peaks {
		x := d.l.cx() + i*d.l.step()
		peakBar(p).draw(c, d.f.Look, x, y, cells)
		c.line(x+cells+1, y, peakSaid(p))
	}
}

// peakBar is a week's peak as WEEKS' bar fills to it: empty where it isn't
// known.
func peakBar(p views.WeekPeak) usageBar {
	if p.Share == nil {
		return usageBar{empty: true}
	}
	return usageBar{used: *p.Share, capAt: 1}
}

// peakSaid says a week's peak as WEEKS does, a whole percentage without its
// sign, this week's so far too: in text.muted, or at 100 bold
// state.destructive; a dash where it isn't known.
func peakSaid(p views.WeekPeak) line {
	if p.Share == nil {
		return line{noFigure}
	}
	peak := prose.WholePercent(*p.Share)
	if peak >= 100 {
		return line{{strconv.Itoa(peak), exhaustedInk}}
	}
	return line{{strconv.Itoa(peak), mutedInk}}
}

// verdict draws the verdict on the account's weeks along row y, from the
// panel's column, as verdictParts says it, cut short of the frame's edge.
func (d detailDrawn) verdict(c *canvas, y int) {
	c.line(d.l.panel, y, fitParts(d.f.edge()-d.l.panel, verdictParts(*d.page, d.row)...))
}
