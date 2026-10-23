package dashboard

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// noFigure is a figure the dashboard doesn't know, or doesn't yet, as
// History's or a bar's: a dash, in text.faint.
var noFigure = span{"—", faintInk}

// The cells the widest of a bounded figure takes, as its format draws it: a
// day, as views.DateNamed names one of two figures; a run of a year's days,
// as views.DaysInARow says it, a leap year's whole; and a share, all of it.
var (
	widestDay   = ansi.StringWidth(views.DateNamed("2026-09-22"))
	widestRun   = ansi.StringWidth(views.DaysInARow(366))
	widestShare = ansi.StringWidth(prose.Percent(1))
)

// worthSaid is a worth as History writes it, as views.Worth.Dollars says it,
// in the ink given: to the dollar, as "$27,480"; the mark of part of it
// unpriced faint after it, as "$184+"; unpriced where all of it is; and a
// dash for none.
func worthSaid(w views.Worth, k ink) line {
	said := w.Dollars()
	if said == "" {
		return line{noFigure}
	}
	if priced, part := strings.CutSuffix(said, views.PartUnpriced); part {
		return line{{priced, k}, {views.PartUnpriced, faintInk}}
	}
	return line{{said, k}}
}

// worthPart is a worth as worthSaid writes it, as a part of a line: a
// figure, but unpriced, which is words.
func worthPart(w views.Worth, k ink) part {
	if w.Amount == nil && len(w.Unpriced) > 0 {
		return wordsPart(worthSaid(w, k))
	}
	return figurePart(worthSaid(w, k), Number{})
}

// countedParts count n of the thing a noun names, as views.CountedWords
// does, in the ink given: the count a figure, as "1,240", and the words after
// it, as " requests".
func countedParts(n int, noun string, k ink) []part {
	return saidParts(views.CountedWords(n, noun), k)
}

// saidParts are words History says, as views words them, in the ink given:
// each figure among them a figure, the rest words.
func saidParts(said status.Words, k ink) []part {
	parts := make([]part, len(said))
	for i, p := range said {
		parts[i] = wordsIn(p.Text, k)
		if p.Kind == status.PartFigure {
			parts[i] = figureIn(p.Text, k)
		}
	}
	return parts
}

// againstSaid is a worth against its price, as prose.Ratio writes it, as
// "3.8×": bold, in state.positive at 1× or more as it's written, and
// accent.attention under; a dash where there's none.
func againstSaid(ratio *float64) line {
	if ratio == nil {
		return line{noFigure}
	}
	k := boldPositiveInk
	if prose.NearestTenth(*ratio) < 1 {
		k = alertInk
	}
	return line{{prose.Ratio(*ratio), k}}
}

// shareSaid is a share as a whole percentage, as "21%", in the ink given: a
// dash where it isn't known.
func shareSaid(share *float64, k ink) line {
	if share == nil {
		return line{noFigure}
	}
	return line{{prose.Percent(*share), k}}
}

// familyName is a family's name as History shows it, its first letter
// capital: Opus.
func familyName(family string) string {
	return capitalised(prose.Clean(family))
}

// capitalised is words with their first letter capital, as a sentence
// starts: Could go.
func capitalised(words string) string {
	if first, size := utf8.DecodeRuneInString(words); size > 0 {
		return string(unicode.ToUpper(first)) + words[size:]
	}
	return words
}

// familyDot is a family as History's keys and strips name it: its dot in
// its hue, then its name in the ink given, as ● Opus.
func familyDot(family string, name ink) line {
	return line{{"●", familyHue(family).ink()}, {" " + familyName(family), name}}
}

// historyAccount is an account as History's rows name it, in room cells: its
// name, as labelled gives it, bold text.primary, cut to keep the ◆ after
// the primary's, in text.tertiary, where there's room for it. History
// shows no routing, so no ▲.
func historyAccount(name string, primary bool, room int) line {
	mark := historyMark(primary)
	return append(line{{labelledIn(name, room-mark.width()), titleInk}}, mark...).fit(room)
}

// historyMark is the mark History's rows put after an account's name: ◆
// after the primary's, none after any other.
func historyMark(primary bool) line {
	if primary {
		return line{{" ◆", primaryInk}}
	}
	return nil
}

// historyNamed is an account's name as History's label column holds it,
// for namesColumn: its name, as labelled gives it, and its mark beside it.
func historyNamed(name string, primary bool) columnName {
	return columnName{label: ansi.StringWidth(name), beside: historyMark(primary).width()}
}
