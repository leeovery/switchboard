// Package dashboard draws the status document as a terminal dashboard: a card
// per account showing each window's use, pace and reset, in as many columns as
// the terminal takes. Drawing is pure, so a one-off print and a live view draw
// the same frame from the same document and clock.
package dashboard

import (
	"slices"
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
	if overview := dotted(origin(doc), bestNext(doc)); overview != nil {
		lines = append(lines, overview)
	}
	if next := priming(doc, now); next != nil {
		lines = append(lines, next)
	}
	if len(body) > 0 {
		lines = append(lines, nil)
		lines = append(lines, body...)
	}
	if footer = status.Clean(footer); footer != "" {
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

// origin says where the document came from, when that's news: the router,
// with how many sessions it has and where it sends new ones, unless it's
// unhealthy, which it says loudly; or that the accounts were probed, as the
// router wasn't running or wasn't answering as it should. A document probed as
// asked says nothing.
func origin(doc status.Document) line {
	switch {
	case doc.Source == status.SourceRouter && !doc.Router.Healthy:
		return unhealthy(doc.Router.Reason)
	case doc.Source == status.SourceRouter:
		return routing(doc)
	case doc.Fallback.Router == status.RouterNotRunning:
		return line{{"probing directly (router not running)", dimInk}}
	case doc.Fallback.Router == status.RouterUnhealthy:
		return unhealthy(doc.Fallback.Reason)
	default:
		return nil
	}
}

// routing says how many sessions the router has, and where it sends new ones.
func routing(doc status.Document) line {
	l := line{{"router" + status.Separator + status.SessionCount(doc.Sessions) + status.Separator, dimInk}}
	if doc.Pin.Account == "" {
		return append(l, span{doc.Routing(), dimInk})
	}
	return append(l, span{doc.Routing(), pinInk})
}

// unhealthy says the router is unhealthy, and why, in red.
func unhealthy(reason string) line {
	text := "router unhealthy"
	if reason = status.Clean(reason); reason != "" {
		text += " — " + reason
	}
	return line{{text, errorInk}}
}

// bestNext names the account to use next, or says that none has room, or
// that nothing has been read of any to tell. It's nil when there are no
// accounts to speak of.
func bestNext(doc status.Document) line {
	if best, ok := doc.Account(doc.Best); ok {
		return line{{"best next: ", dimInk}, {best.Title(), accentInk}}
	}
	switch {
	case len(doc.Accounts) == 0:
		return nil
	case !slices.ContainsFunc(doc.Accounts, read):
		return line{{"nothing read yet", dimInk}}
	}
	return line{{"no account has room right now", errorInk}}
}

// read reports whether the account's usage has been read.
func read(a status.Account) bool {
	return !a.FetchedAt.IsZero()
}

// priming says what comes next of priming at now, when it's on: the next
// reset among the accounts' windows a prime starts, and the router's next
// prime, each with its account and when, such as "next reset: work · Work,
// Mon 18:10". It's nil when there's nothing to say.
func priming(doc status.Document, now time.Time) line {
	var parts []line
	for _, c := range doc.Coming(now) {
		parts = append(parts, line{{c.What + ": ", dimInk}, {c.Account.Title(), textInk}, {", " + status.Clock(now, c.At), dimInk}})
	}
	return dotted(parts...)
}

// dotted runs the parts that aren't nil together, status.Separator between
// each. It's nil when they all are.
func dotted(parts ...line) line {
	var l line
	for _, part := range parts {
		if part == nil {
			continue
		}
		if l != nil {
			l = append(l, span{status.Separator, dimInk})
		}
		l = append(l, part...)
	}
	return l
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
