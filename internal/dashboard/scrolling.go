package dashboard

import (
	"slices"
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

var (
	// thumbInk is the part of a scrollbar its view shows, and railInk its
	// track.
	thumbInk = ink{token: theme.TextMuted}
	railInk  = ink{token: theme.TextFaint}
)

// Scrolling is how far a frame's view scrolls: Most, the furthest, in rows,
// or where it scrolls a thing at a time, as ScrollsRows says it doesn't, in
// those things, zero where it fits; and Page, how many of them show at once,
// as the frame has it scrolled.
type Scrolling struct {
	Most, Page int
}

// byRows is how far content rows tall scrolls in a view rows tall: as far
// as it runs past it, the view's rows at once.
func byRows(content, rows int) Scrolling {
	return Scrolling{Most: max(content-rows, 0), Page: rows}
}

// clamped is offset as far as what scrolls as s says goes: none before the
// first, and no further than Most.
func (s Scrolling) clamped(offset int) int {
	return min(max(offset, 0), s.Most)
}

// ScrollsRows reports whether the view shown scrolls a row at a time, as
// Sessions, Runway and History's Year do, rather than a thing at a time: an
// account, as the Overview's USAGE does, and History's Weeks and Days, or a
// period, as its Tokens does.
func (f Frame) ScrollsRows() bool {
	switch f.View {
	case Overview:
		return false
	case History:
		return f.History.Shown == HistoryYear
	}
	return true
}

// Revealing is how far the Overview's USAGE of doc at now is scrolled to show
// the account with the given id: as far as the frame has it scrolled, where
// that shows it, else no further than it must go to. Scrolled down, a page
// may show fewer than the one it left, as USAGE's does once a ▲ row comes
// over it, so it goes on an account at a time until the account shows, the
// page laid out once for every account it goes on.
func (f Frame) Revealing(doc status.Document, now time.Time, id string) int {
	i := slices.IndexFunc(doc.Accounts, func(a status.Account) bool { return a.ID == id })
	scrolling := f.scrollingAt(doc, now)
	at := scrolling(f.Scroll).revealing(f.Scroll, i)
	for s := scrolling(at); at < s.Most && i >= at+s.Page; s = scrolling(at) {
		at++
	}
	return at
}

// revealing is how far what's scrolled at, and scrolls as s says, is
// scrolled to show its i-th account: no further than it must go to, and no
// further than it goes. An i of none of them leaves it where it is.
func (s Scrolling) revealing(at, i int) int {
	at = s.clamped(at)
	switch {
	case i < 0:
		return at
	case i < at:
		return i
	case i >= at+s.Page:
		return min(i-s.Page+1, s.Most)
	}
	return at
}

// Scrolling is how far the frame's view of doc scrolls at now, scrolled as
// the frame has it, as scrollingAt says.
func (f Frame) Scrolling(doc status.Document, now time.Time) Scrolling {
	return f.scrollingAt(doc, now)(f.Scroll)
}

// scrollingAt is how far the frame's view of doc scrolls at now, scrolled to
// each offset given: History's, of the ledger's days alone, as
// historyScrolling says; the Overview's, as overviewScrolling says, laid out
// once for every offset; Sessions, where its calls and lines, or its plain
// list, don't fit; and Runway, where its lanes don't, as runwayRows counts
// them, each alike at every offset. A frame too short for the view, as
// cramped says, never scrolls, nor one of a document of no accounts, but
// History.
func (f Frame) scrollingAt(doc status.Document, now time.Time) func(offset int) Scrolling {
	var s Scrolling
	switch top := f.top(); {
	case f.cramped():
	case f.View == History:
		return f.historyScrolling(top)
	case len(doc.Accounts) == 0:
	case f.View == Overview:
		return f.overviewScrolling(doc, now)
	case f.View == Sessions:
		s = byRows(f.sessionsRows(doc, now, top))
	case f.View == Runway:
		content, view := f.runwayRows(len(doc.Accounts), top)
		s = Scrolling{Most: content - view, Page: view}
	}
	return func(int) Scrolling { return s }
}

// outOfView says what's out of view where a view scrolls: how many more
// accounts there are above, and below, and how to scroll to them, as in "▼ 2
// more accounts below · j/k or wheel to scroll".
func outOfView(above, below int) line {
	var l line
	for _, out := range []struct {
		n           int
		mark, where string
	}{{above, "▲ ", " above"}, {below, "▼ ", " below"}} {
		if out.n == 0 {
			continue
		}
		if len(l) > 0 {
			l = append(l, span{" · ", dimInk})
		}
		l = append(l, span{out.mark, keyInk}, span{moreAccounts(out.n) + out.where, mutedInk})
	}
	return append(l, span{" · j/k or wheel to scroll", dimInk})
}

// moreAccounts counts n more accounts, as in "2 more accounts".
func moreAccounts(n int) string {
	if n == 1 {
		return "1 more account"
	}
	return strconv.Itoa(n) + " more accounts"
}

// scrollbar draws a scrollbar down column x from row y, rows tall, for
// content rows that scroll, shown from the row offset: ┃ the part shown,
// as long as the share it shows, on a │ track, the whole of it.
func scrollbar(c *canvas, x, y, rows, content, offset int) {
	if rows < 1 || content <= rows {
		return
	}
	thumb := max(1, rows*rows/content)
	most := content - rows
	start := (offset*(rows-thumb) + most/2) / most
	for i := range rows {
		if i >= start && i < start+thumb {
			c.text(x, y+i, "┃", thumbInk)
		} else {
			c.text(x, y+i, "│", railInk)
		}
	}
}
