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
	// from the least to fit the longest title beside every badge.
	minContent = 40
	maxContent = 50
	// titleChrome is the cells of a top border that aren't its title or
	// badges: a rule and a space before the title, a space and a rule after.
	titleChrome = 4
	// A failed window's reason, and an account's error, wrap to at most
	// these many lines.
	reasonLines = 2
	errorLines  = 3
)

const (
	// primaryMark marks the primary account, and primaryBadge its card.
	primaryMark  = "◆"
	primaryBadge = primaryMark + " primary"
	// bestMark marks the account to use next, and bestBadge its card.
	bestMark  = "▲"
	bestBadge = bestMark + " best"
	// pinMark marks each account the router pins new sessions to, and
	// pinBadge its card.
	pinMark  = "●"
	pinBadge = pinMark + " pinned"
	// errorMark leads the reason an account couldn't be read.
	errorMark = "✗ "
)

// badges are what a card's top border carries beside its title: whether its
// account is the primary, whether the router pins new sessions to it, and
// whether it's the best.
type badges struct {
	primary, pinned, best bool
}

// badgesOf are the badges of the account's card in doc.
func badgesOf(doc status.Document, a status.Account) badges {
	return badges{primary: a.Primary, pinned: doc.Pin.Has(a.ID), best: a.ID == doc.Best}
}

// shown returns the badges that are on, in order, each as the text given for
// it, in its ink: the primary's, the pin's and the best's.
func (b badges) shown(primary, pinned, best string) []span {
	var shown []span
	for _, badge := range []struct {
		on   bool
		span span
	}{
		{b.primary, span{primary, primaryInk}},
		{b.pinned, span{pinned, pinBadgeInk}},
		{b.best, span{best, badgeInk}},
	} {
		if badge.on {
			shown = append(shown, badge.span)
		}
	}
	return shown
}

// tail ends a card's top border with its badges.
func (b badges) tail(border ink) line {
	var l line
	for _, badge := range b.shown(primaryBadge, pinBadge, bestBadge) {
		l = append(l, span{" ", border}, badge, span{" ─", border})
	}
	return l
}

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
		body = append(body, cardRow(doc, row, now, cw)...)
	}
	return body, max(perRow*(cardWidth+gutter)-gutter, 0), true
}

// contentWidth is how wide every card's content is: wide enough for the
// longest title beside every badge, within bounds, so a card doesn't change
// width as its badges come and go.
func contentWidth(accounts []status.Account) int {
	widest := 0
	for _, a := range accounts {
		widest = max(widest, ansi.StringWidth(a.Title()))
	}
	all := badges{primary: true, pinned: true, best: true}.tail(borderInk).width()
	return min(max(widest+titleChrome+all-2*padding, minContent), maxContent)
}

// cardRow draws the cards of doc's accounts in row side by side, each padded
// to the tallest so the row's borders line up.
func cardRow(doc status.Document, row []status.Account, now time.Time, cw int) []line {
	contents := make([][]line, len(row))
	height := 0
	for i, a := range row {
		contents[i] = content(doc, a, now, cw)
		height = max(height, len(contents[i]))
	}
	var lines []line
	for i, a := range row {
		c := card(a.Title(), badgesOf(doc, a), contents[i], height, cw)
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
// to height lines, with the title in the top border and its badges beside it.
func card(title string, marks badges, content []line, height, cw int) []line {
	border := borderInk
	if marks.best {
		border = bestBorderInk
	}
	lines := []line{top(title, marks, cw, border), side(nil, cw, border)}
	for i := range height {
		var l line
		if i < len(content) {
			l = content[i]
		}
		lines = append(lines, side(l, cw, border))
	}
	return append(lines, side(nil, cw, border), line{{"╰" + rule(cw+2*padding) + "╯", border}})
}

// top is a card's top border: its title at the left, and its badges at the
// right.
func top(title string, marks badges, cw int, border ink) line {
	inner := cw + 2*padding
	tail := marks.tail(border)
	room := inner - titleChrome - tail.width()
	title = truncate(title, room)
	l := line{{"╭─ ", border}, {title, titleInk}, {" " + rule(1+room-ansi.StringWidth(title)), border}}
	return append(append(l, tail...), span{"╮", border})
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

// content is what a card says of an account in doc: what holds it back, when
// anything does; its usage; and last how many sessions it has, when it has
// any. A blank line comes between each block.
func content(doc status.Document, a status.Account, now time.Time, cw int) []line {
	var blocks [][]line
	if held := heldBy(doc, a, now, cw); held != nil {
		blocks = append(blocks, held)
	}
	blocks = append(blocks, usage(doc, a, now, cw)...)
	if a.Sessions > 0 {
		blocks = append(blocks, []line{{{status.SessionCount(a.Sessions), dimInk}}})
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

// heldBy is what holds an account in doc back at now, a line each: the limit
// it reached, then the upstream's refusal, in red, while each holds; then its
// reserve, in the warning colour, once a window has reached it, whether it
// holds the account back or the global pin spends it; then, in the warning
// colour too, that it's under pressure, which passes it over for new
// sessions. It's nil when nothing does.
func heldBy(doc status.Document, a status.Account, now time.Time, cw int) []line {
	var lines []line
	for _, held := range a.HeldBy(now) {
		lines = append(lines, line{{truncate(held, cw), exhaustedInk}})
	}
	for _, note := range []string{doc.Reserved(a), doc.Pressed(a, now)} {
		if note != "" {
			lines = append(lines, line{{truncate(note, cw), warningInk}})
		}
	}
	return lines
}

// usage is what a card says of an account's usage in doc, a block each: each
// window, then each window that couldn't be read, then why the account
// couldn't be; or that nothing has been read of it yet.
func usage(doc status.Document, a status.Account, now time.Time, cw int) [][]line {
	var blocks [][]line
	for _, w := range a.Windows {
		blocks = append(blocks, windowBlock(doc, a, w, now, cw))
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
	return blocks
}

// windowBlock shows an account's window in doc cw cells wide: its label and
// how much of it is used, a bar marking where the account's reserve starts
// and where even use would be, and where it's heading, or, once it has
// lapsed, that it hasn't started, and when it's primed.
func windowBlock(doc status.Document, a status.Account, w quota.Window, now time.Time, cw int) []line {
	p := a.Project(w, now)
	return []line{
		spread(line{{status.Clean(w.Label), textInk}}, line{use(w, p)}, cw),
		bar(w.Utilization, cw).mark(reserveCell(a.Reserve, cw), reserveMarker).mark(pace(w, p, now, cw), paceMarker),
		outlook(doc, a, w, p, now, cw),
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
	return cellAt(elapsed, width)
}

// failureBlock shows a window that couldn't be read, and why.
func failureBlock(f quota.Failure, cw int) []line {
	lines := []line{{{truncate(status.Clean(f.Label)+" offline", cw), offlineInk}}}
	for _, text := range wrap(status.Clean(f.Error), cw, reasonLines) {
		lines = append(lines, line{{text, dimInk}})
	}
	return lines
}

// errorBlock shows why an account couldn't be read, the lines after the first
// indented to its text.
func errorBlock(err string, cw int) []line {
	var lines []line
	lead := errorMark
	for _, text := range wrap(status.Clean(err), cw-ansi.StringWidth(lead), errorLines) {
		lines = append(lines, line{{lead + text, errorInk}})
		lead = strings.Repeat(" ", ansi.StringWidth(errorMark))
	}
	return lines
}
