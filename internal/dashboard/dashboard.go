// Package dashboard draws the status document as a terminal dashboard: a card
// per account showing each window's use, pace and reset, in as many columns as
// the terminal takes. Drawing is pure, so a one-off print and a live view draw
// the same frame from the same document and clock.
package dashboard

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/status"
)

// Options says how to draw a frame.
type Options struct {
	// Width is the terminal's width in cells. No line of the frame is wider.
	Width int
	// Height is the terminal's height in lines, or zero when it's unknown.
	// Cards that would make the frame taller give way to a line per account.
	Height int
	// Color styles the frame. Without it the frame has no escape codes.
	Color bool
	// Footer is a last line to show under the accounts, if any.
	Footer string
}

const (
	// margin is the blank cells before every line.
	margin = 1
	// stampGap is the fewest cells between the dashboard's name and the time.
	stampGap = 2
)

// Render draws doc as it stands at now: countdowns run from now, and times
// show in now's time zone. Each account is a card, as many to a row as fit
// the width; when not even one fits, or the cards run past the height, each
// account is a line instead.
func Render(doc status.Document, now time.Time, opts Options) string {
	room := opts.Width - margin
	if body, width, ok := grid(doc, now, room); ok {
		lines := frame(doc, now, body, width, room, opts.Footer)
		if opts.Height <= 0 || len(lines) <= opts.Height {
			return draw(lines, room, opts.Color)
		}
	}
	body, width := compact(doc, now, room)
	return draw(frame(doc, now, body, width, room, opts.Footer), room, opts.Color)
}

// frame puts the header over body, which is width cells wide, and the footer
// under it.
func frame(doc status.Document, now time.Time, body []line, width, room int, footer string) []line {
	lines := []line{heading(now, width, room)}
	if best, ok := bestNext(doc); ok {
		lines = append(lines, best)
	}
	if len(body) > 0 {
		lines = append(lines, nil)
		lines = append(lines, body...)
	}
	if footer = clean(footer); footer != "" {
		lines = append(lines, nil, line{{footer, dimInk}})
	}
	return lines
}

// heading names the dashboard and gives the date and time at the right, over
// the width of the body when it's wider than the heading needs. Short of room,
// the time stands alone, and then goes.
func heading(now time.Time, width, room int) line {
	name := span{"Switchboard", nameInk}
	for _, stamp := range []string{now.Format("Mon 2 Jan · 15:04"), now.Format("15:04")} {
		needs := ansi.StringWidth(name.text) + stampGap + ansi.StringWidth(stamp)
		if across := min(max(width, needs), room); needs <= across {
			return line{name, spaces(across - needs + stampGap), {stamp, dimInk}}
		}
	}
	return line{name}
}

// bestNext names the account to use next, or says that none has room. It
// reports false when there are no accounts to speak of.
func bestNext(doc status.Document) (line, bool) {
	if best, ok := doc.Account(doc.Best); ok {
		return line{{"best next: ", dimInk}, {clean(best.Title()), accentInk}}, true
	}
	if len(doc.Accounts) == 0 {
		return nil, false
	}
	return line{{"no account has room right now", errorInk}}, true
}

// draw writes the lines out, each after the margin and cut to room cells, in
// their inks when color is set.
func draw(lines []line, room int, color bool) string {
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if l = l.fit(room); l.width() > 0 {
			b.WriteString(strings.Repeat(" ", margin))
			l.draw(&b, color)
		}
	}
	return b.String()
}
