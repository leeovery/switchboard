package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// gutter is the blank cells between cards side by side.
	gutter = 2
	// padding is the blank cells either side of a card's content.
	padding = 2
	// minContent and maxContent bound how wide a card's content is. It grows
	// from the least to fit the longest title beside the best badge.
	minContent = 40
	maxContent = 50
	// titleChrome is the cells of a top border that aren't its title or
	// badge: a rule and a space before the title, a space and a rule after.
	titleChrome = 4
	// A failed window's reason, and an account's error, wrap to at most
	// these many lines.
	reasonLines = 2
	errorLines  = 3
)

const (
	// bestMark marks the account to use next, and badge its card.
	bestMark = "▲"
	badge    = bestMark + " best"
	// errorMark leads the reason an account couldn't be read.
	errorMark = "✗ "
)

// grid lays the accounts' cards out in rows, as many to a row as fit in room
// cells, and reports how wide a full row is. It reports false when not even
// one card fits.
func grid(doc status.Document, now time.Time, room int) ([]line, int, bool) {
	cw := contentWidth(doc.Accounts)
	cardWidth := cw + 2*padding + 2
	if cardWidth > room {
		return nil, 0, false
	}
	perRow := min(len(doc.Accounts), (room+gutter)/(cardWidth+gutter))
	var body []line
	for row := range slices.Chunk(doc.Accounts, max(perRow, 1)) {
		if body != nil {
			body = append(body, nil)
		}
		body = append(body, cardRow(row, doc.Best, now, cw)...)
	}
	return body, max(perRow*(cardWidth+gutter)-gutter, 0), true
}

// contentWidth is how wide every card's content is: wide enough for the
// longest title beside the best badge, within bounds.
func contentWidth(accounts []status.Account) int {
	widest := 0
	for _, a := range accounts {
		widest = max(widest, ansi.StringWidth(clean(a.Title())))
	}
	return min(max(widest+titleChrome+badgeTail(borderInk).width()-2*padding, minContent), maxContent)
}

// cardRow draws the accounts' cards side by side, each padded to the tallest
// so the row's borders line up.
func cardRow(accounts []status.Account, best string, now time.Time, cw int) []line {
	contents := make([][]line, len(accounts))
	height := 0
	for i, a := range accounts {
		contents[i] = content(a, now, cw)
		height = max(height, len(contents[i]))
	}
	var lines []line
	for i, a := range accounts {
		c := card(clean(a.Title()), a.ID == best, contents[i], height, cw)
		lines = beside(lines, c)
	}
	return lines
}

// beside joins a card's lines onto the right of lines, a gutter between.
func beside(lines, card []line) []line {
	if lines == nil {
		return card
	}
	for i := range lines {
		lines[i] = slices.Concat(lines[i], line{spaces(gutter)}, card[i])
	}
	return lines
}

// card draws content cw cells wide in a rounded border, the content padded
// to height lines, with the title in the top border and, when it's the best,
// the badge beside it.
func card(title string, best bool, content []line, height, cw int) []line {
	border := borderInk
	if best {
		border = bestBorderInk
	}
	lines := []line{top(title, best, cw, border), side(nil, cw, border)}
	for i := range height {
		var l line
		if i < len(content) {
			l = content[i]
		}
		lines = append(lines, side(l, cw, border))
	}
	return append(lines, side(nil, cw, border), line{{"╰" + rule(cw+2*padding) + "╯", border}})
}

// top is a card's top border: its title at the left, and the badge at the
// right when best.
func top(title string, best bool, cw int, border ink) line {
	inner := cw + 2*padding
	var tail line
	if best {
		tail = badgeTail(border)
	}
	room := inner - titleChrome - tail.width()
	title = truncate(title, room)
	l := line{{"╭─ ", border}, {title, titleInk}, {" " + rule(1+room-ansi.StringWidth(title)), border}}
	return append(append(l, tail...), span{"╮", border})
}

// badgeTail ends the best card's top border with the badge.
func badgeTail(border ink) line {
	return line{{" ", border}, {badge, badgeInk}, {" ─", border}}
}

// side is a line of a card's content between its side borders.
func side(content line, cw int, border ink) line {
	content = content.fit(cw)
	return slices.Concat(
		line{{"│", border}, spaces(padding)},
		content,
		line{spaces(cw - content.width() + padding), {"│", border}},
	)
}

// content is what a card says of an account: each window, then each window
// that couldn't be read, then why the account couldn't be, with a blank line
// between each.
func content(a status.Account, now time.Time, cw int) []line {
	var blocks [][]line
	for _, w := range a.Windows {
		blocks = append(blocks, windowBlock(w, now, cw))
	}
	for _, f := range a.Failures {
		blocks = append(blocks, failureBlock(f, cw))
	}
	if a.Error != "" {
		blocks = append(blocks, errorBlock(a.Error, cw))
	}
	if len(blocks) == 0 {
		blocks = append(blocks, []line{{{"no usage yet", dimInk}}})
	}
	var lines []line
	for i, block := range blocks {
		if i > 0 {
			lines = append(lines, nil)
		}
		lines = append(lines, block...)
	}
	return lines
}

// windowBlock shows a window cw cells wide: its label and how much of it is
// used, a bar marking where even use would be, and where it's heading.
func windowBlock(w quota.Window, now time.Time, cw int) []line {
	p := score.Project(w, now)
	return []line{
		spread(line{{clean(w.Label), textInk}}, line{use(w, p)}, cw),
		bar(w.Utilization, pace(w, p, now, cw), cw),
		detail(w, p, now, cw),
	}
}

// use is how much of a window is used, as a percentage in its tone.
func use(w quota.Window, p score.Projection) span {
	return span{status.Percent(w.Utilization), ink{color: tone(w, p), bold: true}}
}

// pace is the cell of the window's bar that takes the pace marker. There's
// none when the window's length or reset is unknown, or once it's exhausted
// and pace no longer matters.
func pace(w quota.Window, p score.Projection, now time.Time, width int) int {
	elapsed, ok := score.Elapsed(w, now)
	if !ok || p.Kind == score.Exhausted {
		return noMarker
	}
	return paceCell(elapsed, width)
}

// failureBlock shows a window that couldn't be read, and why.
func failureBlock(f quota.Failure, cw int) []line {
	lines := []line{{{truncate(clean(f.Label)+" offline", cw), offlineInk}}}
	for _, text := range wrap(clean(f.Error), cw, reasonLines) {
		lines = append(lines, line{{text, dimInk}})
	}
	return lines
}

// errorBlock shows why an account couldn't be read, the lines after the first
// indented to its text.
func errorBlock(err string, cw int) []line {
	var lines []line
	lead := errorMark
	for _, text := range wrap(clean(err), cw-ansi.StringWidth(lead), errorLines) {
		lines = append(lines, line{{lead + text, errorInk}})
		lead = strings.Repeat(" ", ansi.StringWidth(errorMark))
	}
	return lines
}
