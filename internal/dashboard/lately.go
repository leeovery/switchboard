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
// room for that whole. Of a move a limit counts, limit is that limit's
// event's id; and counts is set on the line that counts the sessions the
// limit it tells of moved.
type lateLine struct {
	id     int
	at     time.Time
	mark   span
	words  string
	more   line
	limit  int
	counts bool
}

// lately draws the card fc's LATELY on its back, under what's drawn above
// it, which ends before row y, and over the keys on row foot, from x, width
// cells wide: a blank row, its label, and under it as many of what has
// befallen its account lately as fit with a blank row left over the keys,
// the newest first, as newest picks them, each picked out while it's fresh;
// where there's room for one at least.
func (f Frame) lately(c *canvas, fc face, now time.Time, x, y, width, foot int) {
	lines := newest(fc.lately, max(foot-y-3, 0), lateLine.counted, lateLine.counting)
	if len(lines) == 0 {
		return
	}
	c.line(x, y+1, line{{latelyLabel, labelInk}})
	column := 0
	for _, l := range lines {
		column = max(column, len(status.Dated(now, l.at)))
	}
	for i, l := range lines {
		row := y + 2 + i
		said := line{{padded(status.Dated(now, l.at), column) + "  ", dimInk}, l.mark, spaces(1), {l.words, mutedInk}}
		if whole := slices.Concat(said, l.more); whole.width() <= width {
			said = whole
		}
		c.line(x, row, said.fit(width))
		f.freshen(c, l.id, x-padding, row, x+width+padding)
	}
}

// counted is the id of the limit's event that counts the move the line tells
// of, zero for a line of anything else.
func (l lateLine) counted() int {
	return l.limit
}

// counting is the id of the limit's event whose moves the line counts, zero
// for a line that counts none.
func (l lateLine) counting() int {
	if l.counts {
		return l.id
	}
	return 0
}

// latelies are what each account's LATELY tells of, by its id, of doc's
// events, the newest first, times in now's time zone, as toldLately tells
// each: the events taken in once, for every account at once.
func latelies(doc status.Document, now time.Time) map[string][]lateLine {
	arrived := arrivals(doc)
	lines := make(map[string][]lateLine)
	for _, e := range doc.Events {
		e.At = e.At.In(now.Location())
		for _, l := range toldLately(e, doc, arrived) {
			lines[l.account] = append(lines[l.account], l.lateLine)
		}
	}
	return lines
}

// toldTo is a line of LATELY and the account whose card tells it.
type toldTo struct {
	account string
	lateLine
}

// toldLately is what LATELY tells of an event, a line each thing it tells,
// each on the card of the account it befell, from that account's side, its
// name left out: what befell it, as aside tells it, as RECENT tells it after
// its name; a session starting there; a limit it reached, then the sessions
// that limit moved; a session moving off it and onto another; and sessions
// another's limit moved to it, as arrived counts them. A move a limit counts
// is marked so. It's nothing of a kind LATELY doesn't tell of.
func toldLately(e status.Event, doc status.Document, arrived map[int]map[string][]string) []toldTo {
	tell := func(account string, mark span, words string, more line) toldTo {
		return toldTo{account: account, id: e.ID, at: e.At, mark: mark, words: words, more: more}
	}
	switch e.Kind {
	case status.EventMoved:
		off := tell(e.From, movedMark, sessionID(e.Session)+" moved to "+named(doc, e.To), why(doc, e.Reason))
		on := tell(e.To, movedMark, sessionID(e.Session)+" moved here from "+named(doc, e.From), why(doc, e.Reason))
		off.limit, on.limit = e.Limit, e.Limit
		return []toldTo{off, on}
	case status.EventStarted:
		return []toldTo{tell(e.Account, span{"▲", accentInk}, sessionID(e.Session)+" started here", why(doc, e.Reason))}
	case status.EventLimit:
		mark, gist, _, _ := aside(e, doc)
		told := []toldTo{tell(e.Account, mark, gist, nil)}
		if e.Count > 0 {
			moved := tell(e.Account, movedMark, status.SessionCount(e.Count)+" moved"+movedTo(doc, e), nil)
			moved.counts = true
			told = append(told, moved)
		}
		for _, a := range doc.Accounts {
			if n := arrivedAt(e, a.ID, arrived); n > 0 && a.ID != e.Account {
				here := tell(a.ID, movedMark, status.SessionCount(n)+" arrived from "+named(doc, e.Account), nil)
				here.counts = true
				told = append(told, here)
			}
		}
		return told
	}
	if mark, gist, tail, ok := aside(e, doc); ok {
		return []toldTo{tell(e.Account, mark, gist, tail)}
	}
	return nil
}

// arrivedAt counts the sessions the limit e moved to the account with the
// given id: every one it moved, where they all went there; else, where they
// went to several, those arrived says went there.
func arrivedAt(e status.Event, id string, arrived map[int]map[string][]string) int {
	switch {
	case e.To == id:
		return e.Count
	case e.To != "":
		return 0
	default:
		return len(arrived[e.ID][id])
	}
}

// arrivals are the sessions the moves of doc's events that a limit counts
// took to each account, by the limit's event's id, then by the account's id,
// each session once, however many of its models moved.
func arrivals(doc status.Document) map[int]map[string][]string {
	arrived := make(map[int]map[string][]string)
	for _, m := range doc.Events {
		if m.Kind != status.EventMoved || m.Limit == 0 {
			continue
		}
		if arrived[m.Limit] == nil {
			arrived[m.Limit] = make(map[string][]string)
		}
		if !slices.Contains(arrived[m.Limit][m.To], m.Session) {
			arrived[m.Limit][m.To] = append(arrived[m.Limit][m.To], m.Session)
		}
	}
	return arrived
}

// movedTo says where the sessions a limit moved went, where they all went to
// one account, as in " to side": nothing where they didn't.
func movedTo(doc status.Document, e status.Event) string {
	if e.To == "" {
		return ""
	}
	return " to " + named(doc, e.To)
}

// newest are as many of items as n allows, in their order, the newest
// first, but a move a limit counts, as counted says, while a line counting
// that limit's moves, as counting says, is among them: a move whose limit's
// line doesn't show, as it's too old to, shows itself.
func newest[T any](items []T, n int, counted, counting func(T) int) []T {
	shows := make(map[int]bool)
	for {
		var kept []T
		told := make(map[int]bool)
		for _, item := range items {
			if len(kept) == n {
				break
			}
			if limit := counted(item); limit != 0 && !shows[limit] {
				continue
			}
			kept = append(kept, item)
			told[counting(item)] = true
		}
		more := false
		for _, item := range items {
			if limit := counted(item); limit != 0 && !shows[limit] && !told[limit] {
				shows[limit], more = true, true
			}
		}
		if !more {
			return kept
		}
	}
}
