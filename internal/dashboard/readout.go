package dashboard

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// The words beside a readout's big digits start wordsGap cells after them,
// or wordsAt cells in, whichever is further.
const (
	wordsGap = 4
	wordsAt  = 16
)

// readout draws a card's featured window as its full readout, from x along
// rows y to y+2, ending before the column end: its use in big digits, in the
// colour of its account's state, dim once it has lapsed, or for the window a
// request starts at its limit, the time until it lifts, destructive; and
// beside them its name and length, where it's heading, and when it resets.
func (f Frame) readout(c *canvas, fc face, now time.Time, x, y, end int) {
	s := fc.featured
	digits, k := status.Percent(s.window.Utilization), ink{token: fc.tone()}
	switch {
	case s.held && !s.back.IsZero() && s.window.Key == f.Policy.Started:
		digits, k = timer(now, s.back), errorInk
	case s.lapsed:
		k = dimInk
	}
	beside := max(big(c, x, y, digits, k)+wordsGap, x+wordsAt)
	room := end - beside
	name := line{{strings.ToUpper(status.Clean(s.window.Label)), labelInk}, {"  " + spanOf(s.window.Key), faintInk}}
	c.line(beside, y, name.fit(room))
	c.line(beside, y+1, fc.whereHeading(now).fitHead(room))
	c.line(beside, y+2, fc.whenResets(now).fit(room))
}

// whereHeading says where a card's featured window is heading at now, as its
// readout does: at its limit, and when it was reached, as the router told of
// it; that it hasn't started; that it runs out, or reaches its account's
// reserve, where it does before it resets, as RunsOut has it, as in "→ runs
// out ~16:05", and at what rate, where that's its recent rate, as in "at its
// last-30-min rate"; else the share it's heading for by its reset, as in "→
// 72% by its reset". It's nil where nothing says.
func (fc face) whereHeading(now time.Time) line {
	s := fc.featured
	switch {
	case s.held && !fc.limitAt.IsZero():
		return line{{"limit reached at " + status.Dated(now, fc.limitAt), exhaustedInk}}
	case s.held:
		return line{{"limit reached", exhaustedInk}}
	case s.lapsed:
		return line{{"not started", ink{token: theme.TextTertiary, bold: true}}}
	case s.runsOut:
		verb := "→ runs out ~"
		if s.out.Reserve {
			verb = "→ reaches its reserve ~"
		}
		l := line{{verb + status.Dated(now, s.out.At), alertInk}}
		if s.out.Recent {
			l = append(l, span{" at its " + lately(s.out.Since, now) + " rate", mutedInk})
		}
		return l
	}
	if share, ok := s.projected(); ok {
		return line{{"→ " + status.Percent(share) + " by its reset", secondaryInk}}
	}
	return nil
}

// whenResets says when a card's featured window resets, as its readout
// does: when, and how long until it, as in "resets 17:10 · in 2h 27m"; at
// its limit, that its window resets then; and once it has lapsed, when it's
// next primed, where the router says, as in "next prime 16:20 · in 1h 37m",
// else that it starts with its next request. It's nil where its reset isn't
// known.
func (fc face) whenResets(now time.Time) line {
	reset := fc.featured.window.ResetsAt
	switch {
	case fc.featured.lapsed && !fc.primed.IsZero():
		return line{{"next prime " + status.Dated(now, fc.primed) + " · " + status.Until(now, fc.primed), mutedInk}}
	case fc.featured.lapsed:
		return line{{"starts with its next request", mutedInk}}
	case reset.IsZero():
		return nil
	case fc.featured.held:
		return line{{"its window resets " + status.Dated(now, reset), mutedInk}}
	default:
		return line{{"resets " + status.Dated(now, reset) + " · " + status.Until(now, reset), mutedInk}}
	}
}

// header draws a card's featured window as one line, as a card too short for
// its readout has it, width cells from x along row y, its label in a column
// labelled cells wide: its label; its use, bold in the colour of its
// account's state; in brief where it's heading, as in "→ out ~16:05"; and at
// the right when it resets, or once it has lapsed, when it's next primed.
func (f Frame) header(c *canvas, fc face, now time.Time, x, y, width, labelled int) {
	s := fc.featured
	var when line
	switch {
	case s.lapsed && !fc.primed.IsZero():
		when = line{{"next prime " + status.Dated(now, fc.primed), dimInk}}
	case !s.lapsed && !s.window.ResetsAt.IsZero():
		when = line{{"resets " + status.Dated(now, s.window.ResetsAt), dimInk}}
	}
	label := truncate(status.Short(s.window.Label), labelled-1)
	says := line{{label, textInk}, spaces(labelled - ansi.StringWidth(label)), s.use(ink{token: fc.tone()}), spaces(2), s.whither(now, "~")}
	start := c.right(x+width, y, when.fit(width))
	c.line(x, y, says.fit(start-x-1))
}
