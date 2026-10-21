package dashboard

import (
	"slices"
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

var (
	// accountInk names an account, white wherever one's named.
	accountInk = ink{token: theme.TextPrimary}
	// countingInk is a request in flight's took, counting.
	countingInk = ink{token: theme.AccentKey}
	// litInk is the surface of the line chosen, across the width.
	litInk = hue{token: theme.BgSelection}
)

// rowPartInks are the inks of a request's row's words, by the kind of each
// part: words said in text.muted, between them text.faint, a tool or an
// account named text.secondary, what a request in flight is doing bold
// accent.key, and a move's cost accent.attention.
var rowPartInks = map[views.RowPartKind]ink{
	views.RowSaid: mutedInk, views.RowBetween: faintInk, views.RowNamed: secondaryInk, views.RowDoing: keyInk, views.RowCost: warningInk,
}

// requestLines are the Requests' lines, the oldest first, as the table
// holds them, each made as it's read; then, where it shows none of today's,
// empty, the words saying so.
type requestLines struct {
	table views.LogTable
	empty bool
}

func (r requestLines) len() int {
	if r.empty {
		return r.table.Len() + 1
	}
	return r.table.Len()
}

func (r requestLines) at(i int) logLine {
	if i == r.table.Len() {
		return logLine{key: LogKey{other: emptyKey}, empty: noRequestsToday}
	}
	l := r.table.Line(i)
	drawn := logLine{key: LogKey{request: l.Key()}, rule: l.Rule, oldest: l.Oldest}
	if l.IsRow() {
		drawn.row, drawn.at = &l.Row, l.Row.At
	}
	return drawn
}

// requestsEmpty reports whether the Requests show none of today's lines,
// so they say so at their foot.
func (p LogPage) requestsEmpty() bool {
	return p.today().Shown == 0
}

// log draws the Log's table shown from row top, once its first read lands:
// its heads, then its lines in view, as it stands, the line chosen lit
// across the width.
func (f Frame) log(c *canvas, doc status.Document, now time.Time, top int) {
	p := f.Log
	if !p.Read {
		return
	}
	m := f.Moment(doc, now)
	var draw func(c *canvas, y int, l logLine)
	if p.Shown == LogRequests {
		layout := f.requestsLayout(m)
		logHeads(c, top, layout, dimInk)
		draw = f.logRequest(m, now, layout)
	} else {
		columns := eventsLaid(f.Width, eventsAccountCells(m))
		eventsHeads(c, top, columns)
		draw = logEvent(m, now, columns)
	}
	lines, s := p.lines(), p.stand()
	first, last := p.window(lines, s, f.LogRows())
	lit, isLit := p.lit(lines, s)
	for i := first; i <= last; i++ {
		y := top + 1 + i - first
		f.logLine(c, y, lines.at(i), draw)
		if isLit && i == lit {
			c.surface(0, y, f.Width, litInk)
		}
	}
}

// logLine draws the line l along row y: a row as draw draws it; a day's
// rule, the oldest day's line and a table's words from column logLeft to
// the last.
func (f Frame) logLine(c *canvas, y int, l logLine, draw func(c *canvas, y int, l logLine)) {
	room := f.Width - logLeft
	switch {
	case l.rule != nil:
		c.line(logLeft, y, logDayRule(*l.rule, room))
	case l.oldest != nil:
		c.line(logLeft, y, oldestDay(l.oldest.Keep, room))
	case l.empty != nil:
		c.line(logLeft, y, l.empty.fit(room))
	default:
		draw(c, y, l)
	}
}

// logHeads draws the heads of the columns laid out as l along row y, in the
// ink given, text.subtle over the Log's table and text.faint under a turn
// opened: each at its column's start, or ending at its end where the column
// is right-aligned.
func logHeads(c *canvas, y int, l logLayout, k ink) {
	for _, p := range l.places {
		head := line{{p.column.Name(), k}}
		if p.aligned == alignRight {
			c.right(p.x+p.width, y, head)
			continue
		}
		c.line(p.x, y, head)
	}
}

// requestsLayout are the Requests' columns laid out across the frame, as
// logColumns lays them out, on's natural the widest account the moment m
// knows, as widestLabel has it.
func (f Frame) requestsLayout(m status.Moment) logLayout {
	return logColumns(f.Width, f.Log.Columns, widestLabel(m))
}

// logRequest draws a request's row along a row, its columns laid out as
// layout, of the moment m at now, as requestRow draws it, as it stands at
// now, the stream telling what the page holds of it.
func (f Frame) logRequest(m status.Moment, now time.Time, layout logLayout) func(c *canvas, y int, l logLine) {
	return func(c *canvas, y int, l logLine) {
		requestRow(c, y, l.row.Live(f.Log.Stream, now), layout, func(id string) string { return named(m, id) }, now)
	}
}

// requestRow draws the request's row r along row y, its columns laid out as
// layout, its mark, bold, two cells before its first, each account named as
// name names it, its times in now's time zone: each column's words in its
// cells, a number ending at its end, cut with an ellipsis to its width, a
// move's cost kept whole. A session's page draws a turn's requests by it, in
// a layout of its own.
func requestRow(c *canvas, y int, r views.RequestRow, layout logLayout, name func(id string) string, now time.Time) {
	if len(layout.places) == 0 {
		return
	}
	if mark := rowMark(r); mark.text != "" {
		c.line(layout.places[0].x-logColumnGap, y, line{mark})
	}
	for _, p := range layout.places {
		said := requestCell(r, p.column, layout, name, now).fit(p.width)
		if p.column == LogStopped || p.column == LogRouting {
			said = rowWordsIn(requestWords(r, p.column, layout), p.width)
		}
		if p.aligned == alignRight {
			c.right(p.x+p.width, y, said)
			continue
		}
		c.line(p.x, y, said)
	}
}

// rowMarkInks are the inks of a request's row's marks, by the mark, each
// bold: in flight in accent.key, failed state.destructive, moved its session
// accent.key, a session's first accent.mode, and a check's or a count's
// text.faint.
var rowMarkInks = map[views.RowMark]ink{
	views.MarkAsking: keyInk, views.MarkAnswering: keyInk, views.MarkFailed: exhaustedInk, views.MarkMoved: keyInk,
	views.MarkNew: nameInk, views.MarkHidden: {token: theme.TextFaint, bold: true},
}

// rowMark is the request's row's mark, as its views say it: none of a
// request that stayed and succeeded.
func rowMark(r views.RequestRow) span {
	return span{r.Mark.String(), rowMarkInks[r.Mark]}
}

// requestCell is what the request's row r says in the column given, but
// stopped and routing, which requestWords say, laid out as layout, each
// account named as name names it, its times in now's time zone, and in
// flight, how long it has taken counting.
func requestCell(r views.RequestRow, column LogColumn, layout logLayout, name func(id string) string, now time.Time) line {
	switch column {
	case LogAt:
		return line{{prose.ToTheSecond(now, r.At), mutedInk}}
	case LogSession:
		return sessionCell(r.Session, r.Dir, layout.named)
	case LogModel:
		return line{{modelNamed(r.Model), mutedInk}}
	case LogOn:
		return line{{name(r.Account), accountInk}}
	case LogStatus:
		return statusCell(r)
	case LogFirst:
		return seconds(r.First, mutedInk)
	case LogTook:
		if r.Stage.InFlight() {
			return seconds(r.Took, countingInk)
		}
		return seconds(r.Took, mutedInk)
	case LogWorth:
		return lined(r, r.Worth.Cents())
	case LogAgent:
		return lined(r, prose.Clean(r.Agent))
	case LogRequestID:
		return line{{prose.Clean(r.Request), mutedInk}}
	}
	return countCell(r, column)
}

// countCell is what the request's row r says in a column that counts: its
// tokens, as the Log counts them, — for none where its answer gave usage,
// its context written again bold accent.attention; its attempts, size,
// messages, tools and thinking budget, where its line gives any.
func countCell(r views.RequestRow, column LogColumn) line {
	switch column {
	case LogRead:
		return tokens(r, r.Read, mutedInk)
	case LogWritten:
		if r.Rewrote {
			return tokens(r, r.Written, alertInk)
		}
		return tokens(r, r.Written, mutedInk)
	case LogOut:
		return tokens(r, r.Out, mutedInk)
	case LogInput:
		return tokens(r, r.Input, mutedInk)
	case LogAttempts:
		return given(r.Attempts, strconv.Itoa)
	case LogSize:
		return given(r.Size, prose.Bytes)
	case LogMessages:
		return given(r.Messages, strconv.Itoa)
	case LogTools:
		return given(r.Tools, strconv.Itoa)
	case LogThinking:
		return given(r.Thinking, prose.Tokens)
	}
	return nil
}

// sessionCell names a request's session: its id in text.secondary; with its
// directory where withDir says, a space and the directory, as
// views.SessionDirName names it, in text.muted; and — of no session.
func sessionCell(id, dir string, withDir bool) line {
	if prose.Clean(id) == "" {
		return line{{"—", faintInk}}
	}
	l := line{{status.ShownID(id), secondaryInk}}
	if name := views.SessionDirName(dir); name != "" && withDir {
		l = append(l, span{" " + name, mutedInk})
	}
	return l
}

// statusCell is what a request was answered, once its line came or the
// stream told its end, in text.muted; bold state.destructive where it
// failed, 400 and over, or 0, cut off or given up, as its mark has it.
func statusCell(r views.RequestRow) line {
	if r.Stage != views.RowLined && r.Stage != views.RowEnded {
		return nil
	}
	if r.Failed {
		return line{{strconv.Itoa(r.Status), exhaustedInk}}
	}
	return line{{strconv.Itoa(r.Status), mutedInk}}
}

// seconds is how long d is, as the Log writes it, in the ink given: nothing
// where it isn't known.
func seconds(d *time.Duration, k ink) line {
	if d == nil {
		return nil
	}
	return line{{prose.Seconds(*d), k}}
}

// tokens counts n of the request's tokens, as the Log counts them, in the
// ink given: — in text.faint for none; nothing where its answer gave no
// usage, as before its line comes.
func tokens(r views.RequestRow, n int, k ink) line {
	switch {
	case !r.Metered:
		return nil
	case n == 0:
		return line{{"—", faintInk}}
	}
	return line{{prose.Tokens(n), k}}
}

// given is n as said says it, in text.muted, where it's given: nothing for
// none.
func given(n int, said func(int) string) line {
	if n == 0 {
		return nil
	}
	return line{{said(n), mutedInk}}
}

// lined is what's said of a request once its line came, in text.muted:
// nothing before.
func lined(r views.RequestRow, said string) line {
	if r.Stage != views.RowLined || said == "" {
		return nil
	}
	return line{{said, mutedInk}}
}

// requestWords are what the request's row r says in stopped or routing, as
// layout lays them out: routing, how the router chose; stopped, briefly, as
// PhoneStopped says it, routing folded in; folded, as Folded says it; else
// as Stopped does.
func requestWords(r views.RequestRow, column LogColumn, layout logLayout) views.RowWords {
	switch {
	case column == LogRouting:
		return r.Routing.Said()
	case layout.brief:
		return r.PhoneStopped()
	case layout.folded:
		return r.Folded()
	}
	return r.Stopped()
}

// rowWordsIn are a row's words in cells cells, each part in its kind's ink:
// whole where they fit; else cut with an ellipsis in the ink of what it
// cuts, a move's cost, which ends them, kept whole after them, as "from
// work, at it…  $1.82", where the words before it keep a cell and the
// ellipsis.
func rowWordsIn(w views.RowWords, cells int) line {
	return costKept(rowLine(w), len(w) > 1 && w[len(w)-1].Kind == views.RowCost, cells)
}

// rowLine is the words w, each part in its kind's ink.
func rowLine(w views.RowWords) line {
	said := make(line, len(w))
	for i, p := range w {
		said[i] = span{p.Text, rowPartInks[p.Kind]}
	}
	return said
}

// costCut is the fewest cells the words before a cost kept whole are cut
// to: a cell of them and the ellipsis.
const costCut = 2

// costKept is said cut to cells cells: where it ends in a cost, with the
// span before it setting it apart, the words before them cut and the two
// kept whole, where the words keep costCut cells; else cut as fit cuts it.
func costKept(said line, costs bool, cells int) line {
	if !costs || said.width() <= cells {
		return said.fit(cells)
	}
	words, kept := said[:len(said)-2], said[len(said)-2:]
	spans, fits := flex.Lay(cells, flex.Part{Least: costCut, Natural: words.width()}, flex.Part{Least: kept.width(), Natural: kept.width()})
	if !fits {
		return said.fit(cells)
	}
	return slices.Concat(words.fit(spans[0].Size), kept)
}
