package dashboard

import (
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
// zero where it fits; and Page, how many rows of it show at once.
type Scrolling struct {
	Most, Page int
}

// Scrolling is how far the frame's view of doc scrolls at now: the
// Overview's, as overviewScrolling says; Sessions, where its calls and lines,
// or its plain list, don't fit; and Runway, where its lanes don't, as
// runwayRows counts them. A frame too short for the view, as cramped says,
// never scrolls.
func (f Frame) Scrolling(doc status.Document, now time.Time) Scrolling {
	if len(doc.Accounts) == 0 || f.cramped() {
		return Scrolling{}
	}
	top := f.top()
	switch f.View {
	case Overview:
		return f.overviewScrolling(doc, now)
	case Sessions:
		content, view := f.sessionsRows(doc, now, top)
		return Scrolling{Most: max(content-view, 0), Page: view}
	case Runway:
		content, view := f.runwayRows(len(doc.Accounts), top)
		return Scrolling{Most: content - view, Page: view}
	default:
		return Scrolling{}
	}
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
