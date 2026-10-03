package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// LOG, under the Sessions view's calls: its label, the most lines it takes,
// and the blank rows over it, under the calls.
const (
	logLabel = "LOG"
	logLines = 5
	logGap   = 2
)

// retried are the router's words for what befell an account a request went
// out on, so the router sent the request again on another, as they follow
// the account's id in the reason it gives for the move, as in "moved: work
// hit its limit", and LOG's words for them.
var retried = map[string]string{
	"hit its limit":     "reached its limit",
	"was refused":       "was refused",
	"was throttled":     "was throttled",
	"was reset by hand": "was reset by hand",
}

// logBlock is where LOG goes: its label's row, how many of its lines show
// under it, and the column they end before.
type logBlock struct {
	row, lines, end int
}

// logBlockOf is where LOG goes in the bay b of doc at now, view rows of which
// show at once: logGap blank rows under the last call, or under the row that
// says there are none, with as many lines as it has, logLines at most. Where
// the bay fits the view, it takes as many of those as fit under it, and none
// where not one does; where the bay doesn't, LOG scrolls with it, whole. Its
// lines end bendsApart cells before the leftmost cord that turns down
// through them, or before the rightmost bend where none does.
func (f Frame) logBlockOf(doc status.Document, now time.Time, b bay, view int) logBlock {
	row := 1 + logGap + 1
	if n := len(b.calls); n > 0 {
		row = b.calls[n-1].row + logGap + 1
	}
	lines := f.logCount(doc, now)
	if b.content <= view {
		lines = min(lines, view-1-row)
	}
	if lines < 1 {
		return logBlock{}
	}
	end := b.x - looseCells - bendClear
	for _, k := range b.cords {
		if k.from != k.to && k.from <= row+lines && k.to >= row {
			end = min(end, k.bend)
		}
	}
	return logBlock{row: row, lines: lines, end: end - bendsApart}
}

// logCount is how many lines LOG takes at now: a line for each of its
// events, logLines at most, or, where it has none, a line to say why.
func (f Frame) logCount(doc status.Document, now time.Time) int {
	if n := len(f.logged(doc, now, logLines)); n > 0 {
		return n
	}
	if f.quiet(doc) != nil {
		return 1
	}
	return 0
}

// drawLog draws LOG of doc at now on c, as l places it: its label, and under
// it its lines, from the newest, each with its time, its mark and its words,
// picked out while it's fresh; or why it has none.
func (f Frame) drawLog(c *canvas, doc status.Document, l logBlock, now time.Time) {
	if l.lines < 1 {
		return
	}
	c.line(callsAt, l.row, line{{logLabel, labelInk}})
	tellings := f.tellingsOf(f.logged(doc, now, l.lines), doc, now, logTold)
	if len(tellings) == 0 {
		f.quietly(c, doc, callsAt, l.row+1, l.end)
		return
	}
	for i, t := range tellings {
		c.line(callsAt, l.row+1+i, slices.Concat(t.lead, t.words).fit(l.end-callsAt))
		f.freshen(c, t.id, callsAt, l.row+1+i, l.end)
	}
}

// logged are the events LOG tells of at now, the newest first, n at most:
// the moves the request stream told of since the router's document was
// built, which it doesn't tell of yet; then the document's events of the
// kinds RECENT tells of, each move among them, those a limit forced too,
// which RECENT counts in the limit's line.
func (f Frame) logged(doc status.Document, now time.Time, n int) []status.Event {
	var events []status.Event
	for _, m := range slices.Backward(f.Traffic.repatching()) {
		if m.At.After(doc.GeneratedAt) {
			events = append(events, m.event())
		}
	}
	for _, e := range doc.Events {
		if _, _, ok := told(e, doc, now); ok {
			events = append(events, e)
		}
	}
	return events[:min(n, len(events))]
}

// logTold is what LOG says of an event: as RECENT says it, but of a move, why
// at length, as retriedWhy says it.
func logTold(e status.Event, doc status.Document, now time.Time) (span, line, bool) {
	if e.Kind == status.EventMoved {
		return movedMark, moveSaid(e, doc, retriedWhy(e, doc)), true
	}
	return told(e, doc, now)
}

// retriedWhy says why a session moved, as LOG says it: where the router sent
// its request again on another account, as the one it went out on couldn't
// serve it, what befell that one, and that the request was retried, as in
// ": work reached its limit, so its request was retried on side"; else as
// RECENT says it.
func retriedWhy(e status.Event, doc status.Document) line {
	said, passed := passedOverIn(e.Reason)
	if rest, ok := strings.CutPrefix(said, reasonMovedOff); ok {
		account, befell, _ := strings.Cut(rest, " ")
		if words, ok := retried[befell]; ok {
			retry := line{{": " + named(doc, account) + " " + words + ", so its request was retried on " + named(doc, e.To), mutedInk}}
			return slices.Concat(retry, passing(passed))
		}
	}
	return why(e.Reason)
}
