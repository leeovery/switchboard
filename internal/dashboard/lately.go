package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// eventMarks are each kind of event's mark, a glyph in the colour of what it
// tells of, as LATELY and the Log's Events share them: one a kind, but for ■,
// which a limit and a refusal share, their words telling them apart.
var eventMarks = map[string]span{
	status.EventStarted:  {"▲", accentInk},
	status.EventMoved:    {"▸", keyMarkInk},
	status.EventPressure: {"◔", warningInk},
	status.EventCap:      {"╎", warningInk},
	status.EventLimit:    {"■", errorInk},
	status.EventRefused:  {"■", errorInk},
	status.EventPrimed:   {"◇", dimInk},
	status.EventRoom:     {"✓", positiveInk},
	status.EventPin:      {"●", keyMarkInk},
	status.EventAuto:     {"↩", keyMarkInk},
	status.EventRestart:  {"!", warningInk},
	status.EventHealth:   {"⊕", errorInk},
}

// keyMarkInk draws a mark in accent.key, as a move's, a pin's and auto's are.
var keyMarkInk = ink{token: theme.AccentKey}

// eventMark is a sentence's mark, as eventMarks gives its kind's: but the
// router's health well again, in state.positive.
func eventMark(s views.Sentence) span {
	if s.Well {
		return span{eventMarks[s.Kind].text, positiveInk}
	}
	return eventMarks[s.Kind]
}

// latelySection is the Overview's LATELY of doc at now: what's happened,
// newest first, as many of its sentences as it has, leastLines of them at its
// least, or a line saying why it has none; folded, its first, as it's drawn.
func (f Frame) latelySection(doc status.Document, now time.Time) section {
	return section{
		Section: SectionLately, least: linesRows(0), most: linesRows(len(views.Lately(doc, now, mostLines))),
		draw: func(c *canvas, body bounds) {
			f.drawLately(c, doc, now, body)
		},
		folded: func() line {
			first := f.latelyLines(doc, views.Lately(doc, now, 1), now)
			if len(first) == 0 {
				return f.quiet(doc)
			}
			return append(first[0], moreFolded)
		},
	}
}

// drawLately draws as many of LATELY's lines as body has rows for, from its
// top, each picked out while it's fresh; or, where it has none, why.
func (f Frame) drawLately(c *canvas, doc status.Document, now time.Time, body bounds) {
	sentences := views.Lately(doc, now, body.rows)
	end := body.x + body.width
	if len(sentences) == 0 {
		f.quietly(c, doc, body.x, body.y, end)
		return
	}
	for i, l := range f.latelyLines(doc, sentences, now) {
		c.line(body.x, body.y+i, l.fit(body.width))
		f.freshen(c, sentences[i].ID, body.x, body.y+i, end)
	}
}

// latelyLines are the sentences given as LATELY's lines at now: each its
// time, in a column as wide as the widest, the first's in text.secondary and
// the rest's in text.subtle; its mark; and its words, as saidLine draws them,
// short on a phone.
func (f Frame) latelyLines(doc status.Document, sentences []views.Sentence, now time.Time) []line {
	column := 0
	for _, s := range sentences {
		column = max(column, len(status.Dated(now, s.At)))
	}
	lines := make([]line, len(sentences))
	for i, s := range sentences {
		at := span{padded(status.Dated(now, s.At), column) + afterTime, dimInk}
		if i == 0 {
			at.ink = secondaryInk
		}
		said := s.Full
		if f.phone() {
			said = s.Short
		}
		lines[i] = append(line{at, eventMark(s), spaces(1)}, saidLine(doc, said)...)
	}
	return lines
}

// saidLine is a sentence's words as LATELY draws them: its subject in bold
// text.primary, its object in bold text.secondary, and its other words in
// text.muted, each account it names named as doc names it.
func saidLine(doc status.Document, s views.Said) line {
	var l line
	for _, part := range []struct {
		words status.Words
		ink   ink
	}{{s.Subject, titleInk}, {s.Words, mutedInk}, {s.Object, strongInk}, {s.Rest, mutedInk}} {
		for _, p := range part.words {
			text := p.Text
			if p.Kind == status.PartAccount {
				text = named(doc, p.Text)
			}
			l = append(l, span{text, part.ink})
		}
	}
	return l
}
