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
	c.line(beside, y+1, briefest(room, line.fitHead, fc.whereHeading(now)...))
	c.line(beside, y+2, briefest(room, line.fitWhole, fc.whenResets(now)...))
}

// briefest is the first of forms, the fullest first, whose first span, which
// holds its time, fits width cells, fitted to them by fit, which cuts what
// follows it; or where none's does, the last, cut short.
func briefest(width int, fit func(line, int) line, forms ...line) line {
	for _, l := range forms {
		if len(l) > 0 && l[:1].width() <= width {
			return fit(l, width)
		}
	}
	if len(forms) == 0 {
		return nil
	}
	return forms[len(forms)-1].fit(width)
}

// whereHeading says where a card's featured window is heading at now, as its
// readout does, in forms from the fullest to the briefest, so its time shows
// whole where the fullest doesn't fit: at its limit, and when it was
// reached, as the router told of it, as in "limit reached at Sun 07:00", then
// "limit reached Sun 07:00"; that it hasn't started; that it runs out, or
// reaches its account's reserve, where it does before it resets, as RunsOut
// has it, as in "→ reaches its reserve ~Wed 04:06", and at what rate, where
// that's its recent rate, measured over the span to when the document was
// read, as the router measures it, as in "at its last-30-min rate", then
// "→ out ~Wed 04:06"; else the share it's heading for by its reset, as in
// "→ 72% by its reset". The briefest of each form gives its time as When
// does. It's nil where nothing says.
func (fc face) whereHeading(now time.Time) []line {
	s := fc.featured
	switch {
	case s.held && !s.since.IsZero():
		return dated(now, s.since, exhaustedInk, "limit reached at ", "limit reached ")
	case s.held:
		return []line{{{"limit reached", exhaustedInk}}}
	case s.lapsed:
		return []line{{{"not started", ink{token: theme.TextTertiary, bold: true}}}}
	case s.runsOut:
		verb := "→ runs out ~"
		if s.out.Reserve {
			verb = "→ reaches its reserve ~"
		}
		forms := dated(now, s.out.At, alertInk, verb, "→ out ~")
		if s.out.Recent {
			forms[0] = append(forms[0], span{" at its " + lately(s.out.Since, fc.read) + " rate", mutedInk})
		}
		return forms
	}
	if share, ok := s.projected(); ok {
		return []line{{{"→ " + status.Percent(share) + " by its reset", secondaryInk}}}
	}
	return nil
}

// dated are the forms of words that end with the time t at now, in the ink
// given: after the fullest of the words given, its time as Dated shows it;
// then after the briefest, as Dated, then as When shows it.
func dated(now, t time.Time, k ink, full, brief string) []line {
	return []line{
		{{full + status.Dated(now, t), k}},
		{{brief + status.Dated(now, t), k}},
		{{brief + status.When(now, t), k}},
	}
}

// whenResets says when a card's featured window resets, as its readout
// does, in forms from the fullest to the briefest, so its time shows whole
// where the fullest doesn't fit: when, and how long until it, as in "resets
// 17:10 · in 2h 27m", the countdown left off before the time is cut; at its
// limit, that its window resets then, and nothing where that isn't known, as
// of a window that has lapsed; once it has lapsed, when it's next primed,
// where the router says, as in "next prime 16:20 · in 1h 37m", else that it
// starts with its next request; and where its reset isn't known, so.
func (fc face) whenResets(now time.Time) []line {
	reset := fc.featured.window.ResetsAt
	switch {
	case fc.featured.lapsed && !fc.primed.IsZero():
		return countingDown(now, fc.primed, "next prime ")
	case fc.featured.lapsed:
		return []line{{{"starts with its next request", mutedInk}}}
	case fc.featured.held && reset.IsZero():
		return nil
	case reset.IsZero():
		return []line{{{"reset time unknown", mutedInk}}}
	case fc.featured.held:
		return dated(now, reset, mutedInk, "its window resets ", "resets ")
	default:
		return countingDown(now, reset, "resets ")
	}
}

// countingDown are the forms of the words given, then the time t at now,
// then a countdown from now to it, as in "resets Fri 04:06 · in 13h 24m":
// with its time as Dated shows it, then as When does.
func countingDown(now, t time.Time, words string) []line {
	countdown := span{" · " + status.Until(now, t), mutedInk}
	return []line{
		{{words + status.Dated(now, t), mutedInk}, countdown},
		{{words + status.When(now, t), mutedInk}, countdown},
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
