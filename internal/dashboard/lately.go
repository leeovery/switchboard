package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// latelyLabel is the label of what a card's back says has befallen its
// account lately.
const latelyLabel = "LATELY"

// lateLine is a line of a card's LATELY: the event it tells of, by id, when
// that happened, its mark, its words, and what follows them where there's
// room for that whole.
type lateLine struct {
	id    int
	at    time.Time
	mark  span
	words string
	more  line
}

// lately draws LATELY on a card's back, under what's drawn above it, which
// ends before row y, and over the keys on row foot, from x, width cells
// wide: a blank row, its label, and under it as many of what has befallen
// the account with the given id lately as fit with a blank row left over the
// keys, the newest first, each picked out while it's fresh; where there's
// room for one at least.
func (f Frame) lately(c *canvas, doc status.Document, id string, now time.Time, x, y, width, foot int) {
	lines := latelyOf(doc, id, now)
	lines = lines[:min(len(lines), max(foot-y-3, 0))]
	if len(lines) == 0 {
		return
	}
	c.line(x, y+1, line{{latelyLabel, labelInk}})
	column := 0
	for _, l := range lines {
		column = max(column, len(status.When(now, l.at)))
	}
	for i, l := range lines {
		row := y + 2 + i
		said := line{{padded(status.When(now, l.at), column) + "  ", dimInk}, l.mark, spaces(1), {l.words, mutedInk}}
		if whole := slices.Concat(said, l.more); whole.width() <= width {
			said = whole
		}
		c.line(x, row, said.fit(width))
		f.freshen(c, l.id, x-padding, row, x+width+padding)
	}
}

// latelyOf is what LATELY tells of the account with the given id, of doc's
// events, the newest first, times in now's time zone: what befell it, as
// RECENT tells of it, but from its side, its name left out; a limit it
// reached and the sessions that limit moved, a line each; and sessions
// another's limit moved to it. A move a limit forced is left out, as the
// limit's line counts it.
func latelyOf(doc status.Document, id string, now time.Time) []lateLine {
	var lines []lateLine
	for _, e := range doc.Events {
		if counted(e) {
			continue
		}
		e.At = e.At.In(now.Location())
		lines = append(lines, toldLately(e, doc, id)...)
	}
	return lines
}

// toldLately is what LATELY tells of an event, from the side of the account
// with the given id, a line each thing it tells: nothing where the event
// isn't the account's, or of a kind LATELY doesn't tell of.
func toldLately(e status.Event, doc status.Document, id string) []lateLine {
	tell := func(mark span, words string, more line) lateLine {
		return lateLine{id: e.ID, at: e.At, mark: mark, words: words, more: more}
	}
	moved := span{"▸", dimInk}
	switch {
	case e.Kind == status.EventMoved && e.From == id:
		return []lateLine{tell(moved, sessionID(e.Session)+" moved to "+named(doc, e.To), why(e.Reason))}
	case e.Kind == status.EventMoved && e.To == id:
		return []lateLine{tell(moved, sessionID(e.Session)+" moved here from "+named(doc, e.From), why(e.Reason))}
	case e.Kind == status.EventLimit && e.Account != id && e.To == id && e.Count > 0:
		return []lateLine{tell(moved, status.SessionCount(e.Count)+" arrived from "+named(doc, e.Account), nil)}
	case e.Account != id:
		return nil
	}
	switch e.Kind {
	case status.EventStarted:
		return []lateLine{tell(span{"▲", accentInk}, sessionID(e.Session)+" started here", why(e.Reason))}
	case status.EventPressure:
		return []lateLine{tell(span{"●", warningInk}, "came under pressure", runsOut(e, doc))}
	case status.EventLimit:
		lines := []lateLine{tell(span{"■", errorInk}, "reached its "+limits(e, doc), nil)}
		if e.Count > 0 {
			lines = append(lines, tell(moved, status.SessionCount(e.Count)+" moved"+movedTo(doc, e), nil))
		}
		return lines
	case status.EventRefused:
		mark, words, until := refusal(e)
		return []lateLine{tell(mark, words, line{{until, mutedInk}})}
	case status.EventPrimed:
		return []lateLine{tell(span{"◇", primedInk}, "primed: its "+spanOf(first(e.Windows))+" started", line{{resetting(e), mutedInk}})}
	case status.EventRoom:
		return []lateLine{tell(span{"●", positiveInk}, "has room again", nil)}
	}
	return nil
}

// movedTo says where the sessions a limit moved went, where they all went to
// one account, as in " to side": nothing where they didn't.
func movedTo(doc status.Document, e status.Event) string {
	if e.To == "" {
		return ""
	}
	return " to " + named(doc, e.To)
}
