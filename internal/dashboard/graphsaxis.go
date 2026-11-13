package dashboard

import (
	"slices"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/views"
)

// tickMark is a burn-down card's tick on its axis, ahead of its label.
const tickMark = "╵"

// axisTick is a tick on a card's axis: when it falls, and its label, in its ink.
type axisTick struct {
	at    time.Time
	label span
}

// axis draws v0.1's axis under the card's chart, width cells from x along
// row y: a ╵ in text.faint at each tick, as dayTicks has them, its label
// after it; but of each tick whose label wouldn't end a cell before the next
// tick drawn, or within the card, nothing, as the ticks from the last back
// give way.
func (cd burnCard) axis(c *canvas, x, y, width int) {
	tl := cd.timeline(width)
	next := width + 1
	ticks := cd.dayTicks(tl)
	for _, t := range slices.Backward(ticks) {
		col := tl.Column(t.at)
		if end := col + 1 + ansi.StringWidth(t.label.text); end >= next || col < 0 {
			continue
		}
		c.line(x+col, y, line{{tickMark, faintInk}, t.label})
		next = col
	}
}

// dayTicks are the ticks on the card's axis over tl, in order: its
// midnights, as its Midnights has them, each named its weekday, Mon, or
// where a day spans too few of its columns for that, as labelFor says, its
// first two letters, Mo, in text.subtle, but today's in bold
// text.secondary.
func (cd burnCard) dayTicks(tl views.Timeline) []axisTick {
	now := cd.window.now
	var ticks []axisTick
	for _, at := range tl.Midnights() {
		k := dimInk
		if prose.SameDay(now, at, now) {
			k = strongInk
		}
		name := labelFor(tl, quota.Day, prose.Weekday(at.Weekday()), prose.WeekdayPair(at.Weekday()))
		ticks = append(ticks, axisTick{at: at, label: span{name, k}})
	}
	return ticks
}

// labelFor is the label of a tick every so long on tl: long, where so long
// spans the columns its tick, long and a blank after them take; else short.
func labelFor(tl views.Timeline, every time.Duration, long, short string) string {
	if every >= time.Duration(ansi.StringWidth(long)+2)*tl.Step() {
		return long
	}
	return short
}
