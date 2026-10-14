package dashboard

import (
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// eventKind is a kind of the router's event as the dashboard draws it: the
// kind, as an event's Kind gives it; its mark, a glyph in the colour of what
// it tells of, as LATELY and the Log's Events share them; and what it is, as
// the Log's panel words it, and help after its mark. Its word, as the Log's
// event column and its panel name it, is views.KindWord's, as events prints
// it too.
type eventKind struct {
	kind  string
	mark  span
	means string
}

// eventKinds are the kinds of the router's events in the design's order, as
// help and the Log's panel list them: a mark a kind, but for ■, which a limit
// and a refusal share, their words telling them apart.
var eventKinds = []eventKind{
	{kind: status.EventStarted, mark: span{"▲", accentInk}, means: "a session first seen"},
	{kind: status.EventMoved, mark: span{"▸", keyMarkInk}, means: "a session moved to another account"},
	{kind: status.EventPressure, mark: span{"◔", warningInk}, means: "an account under pressure"},
	{kind: status.EventCap, mark: span{"╎", warningInk}, means: "an account at its cap"},
	{kind: status.EventLimit, mark: span{"■", errorInk}, means: "an account at its limit"},
	{kind: status.EventRefused, mark: span{"■", errorInk}, means: "an account's token refused"},
	{kind: status.EventPrimed, mark: span{"◇", dimInk}, means: "a window started early"},
	{kind: status.EventRoom, mark: span{"✓", positiveInk}, means: "an account with room again"},
	{kind: status.EventPin, mark: span{"●", keyMarkInk}, means: "routing set by hand"},
	{kind: status.EventAuto, mark: span{"↩", keyMarkInk}, means: "routing given back to the router"},
	{kind: status.EventRestart, mark: span{"!", warningInk}, means: "a restart of the router falling due"},
	{kind: status.EventHealth, mark: span{"⊕", errorInk}, means: "the router unhealthy, or well again"},
}

// keyMarkInk draws a mark in accent.key, as a move's, a pin's and auto's are.
var keyMarkInk = ink{token: theme.AccentKey}

// kindMark is the mark of the kind of event given, as eventKinds has it:
// none of a kind it doesn't know.
func kindMark(kind string) span {
	if i := slices.IndexFunc(eventKinds, func(k eventKind) bool { return k.kind == kind }); i >= 0 {
		return eventKinds[i].mark
	}
	return span{}
}

// eventMark is a sentence's mark, as kindMark gives its kind's: but the
// router's health well again, in state.positive.
func eventMark(s views.Sentence) span {
	mark := kindMark(s.Kind)
	if s.Well {
		mark.ink = positiveInk
	}
	return mark
}

// latelySection is the Overview's LATELY of doc at now: what's happened,
// newest first, as many of its sentences as it has, leastLines of them at its
// least, or a line saying why it has none; folded, its first, as it's drawn.
// Its sentences are told only as its draw, its fold, or a layout made
// afresh asks for them, as latelyTelling tells them.
func (f Frame) latelySection(doc status.Document, now time.Time) section {
	m := f.Moment(doc, now)
	told := latelyTelling(m)
	return section{
		Section: SectionLately, least: linesRows(0),
		lines: func() int { return len(told(mostLines)) },
		draw: func(c *canvas, body bounds) {
			f.drawLately(c, m, told(body.rows), body)
		},
		folded: func() line {
			first := f.latelyLines(m, told(1))
			if len(first) == 0 {
				return f.quiet(m)
			}
			return first[0]
		},
	}
}

// latelyTelling tells LATELY's sentences of the moment m in n lines,
// mostLines at most, each n told once: those it has in mostLines, where
// they're told and all fit; else told for n, as a move folded into a line
// that doesn't show then shows alone, so the first n of them aren't always
// them.
func latelyTelling(m status.Moment) func(n int) []views.Sentence {
	var mu sync.Mutex
	told := make(map[int][]views.Sentence)
	return func(n int) []views.Sentence {
		mu.Lock()
		defer mu.Unlock()
		n = min(n, mostLines)
		if all, ok := told[mostLines]; ok && len(all) <= n {
			return all
		}
		if _, ok := told[n]; !ok {
			told[n] = views.Lately(m, n)
		}
		return told[n]
	}
}

// drawLately draws the sentences given, those body has rows for, a line each
// from its top, each picked out while it's fresh; or, where there are none,
// why LATELY has none, of the document as m has it.
func (f Frame) drawLately(c *canvas, m status.Moment, sentences []views.Sentence, body bounds) {
	end := body.x + body.width
	if len(sentences) == 0 {
		f.quietly(c, m, body.x, body.y, end)
		return
	}
	for i, l := range f.latelyLines(m, sentences) {
		c.line(body.x, body.y+i, l.fit(body.width))
		f.freshen(c, sentences[i].ID, body.x, body.y+i, end)
	}
}

// latelyLines are the sentences given as LATELY's lines at m's moment: each
// led as latelyLeads has it, the first's time in text.secondary; then its
// words, as the frame tells them, as saidLine draws them.
func (f Frame) latelyLines(m status.Moment, sentences []views.Sentence) []line {
	names := overviewNames(m)
	lines := latelyLeads(m, sentences)
	for i, s := range sentences {
		if i == 0 {
			lines[i][0].ink = secondaryInk
		}
		lines[i] = append(lines[i], saidLine(m, names, f.latelySaid(s))...)
	}
	return lines
}

// latelyLeads are the leads of LATELY's lines, the Overview's and Detail's
// alike, of the sentences given at m's moment: each its time, in a column as
// wide as the widest, in text.subtle; then its mark.
func latelyLeads(m status.Moment, sentences []views.Sentence) []line {
	column := 0
	for _, s := range sentences {
		column = max(column, len(prose.Dated(m.Now, s.At)))
	}
	leads := make([]line, len(sentences))
	for i, s := range sentences {
		leads[i] = line{{padded(prose.Dated(m.Now, s.At), column) + afterTime, dimInk}, eventMark(s), spaces(1)}
	}
	return leads
}

// latelySaid are a sentence's words as the Overview's LATELY tells them on
// the frame: short on a phone. Detail's tells them in full, as
// accountLatelyLines has them.
func (f Frame) latelySaid(s views.Sentence) views.Said {
	if f.phone() {
		return s.Short
	}
	return s.Full
}

// saidLine is a sentence's words as LATELY draws them: its subject in bold
// text.primary, its object in bold text.secondary, and its other words in
// text.muted, each account it names named as names, overviewNames' of m, have
// it.
func saidLine(m status.Moment, names []accountName, s views.Said) line {
	var l line
	for _, part := range []struct {
		words status.Words
		ink   ink
	}{{s.Subject, titleInk}, {s.Words, mutedInk}, {s.Object, strongInk}, {s.Rest, mutedInk}} {
		for _, p := range part.words {
			said := span{p.Text, part.ink}
			if p.Kind == status.PartAccount {
				said = overviewNamed(m, names, p.Text, part.ink)
			}
			l = append(l, said)
		}
	}
	return l
}
