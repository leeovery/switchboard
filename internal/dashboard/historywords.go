package dashboard

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// noFigure is a figure the dashboard doesn't know, or doesn't yet, as
// History's or a bar's: a dash, in text.faint.
var noFigure = span{"—", faintInk}

// The cells the widest of a bounded figure takes, as its format draws it: a
// day, as prose.Day writes one of two figures; a run of a year's days, a
// leap year's whole; and a share, all of it.
var (
	widestDay   = ansi.StringWidth(prose.Day(time.Date(2026, time.September, 22, 0, 0, 0, 0, time.UTC)))
	widestRun   = ansi.StringWidth(prose.Counted(366, "day"))
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

// countedParts count n of the thing a noun names, as prose.Counted does, in
// the ink given: the count a figure, as "1,240", and the words after it, as
// " requests".
func countedParts(n int, noun string, k ink) []part {
	count := prose.Grouped(n)
	return []part{
		figurePart(line{{count, k}}, Number{}),
		wordsPart(line{{strings.TrimPrefix(prose.Counted(n, noun), count), k}}),
	}
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
	name := prose.Clean(family)
	if first, size := utf8.DecodeRuneInString(name); size > 0 {
		name = string(unicode.ToUpper(first)) + name[size:]
	}
	return name
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
	var mark line
	if primary {
		mark = line{{" ◆", primaryInk}}
	}
	return append(line{{prose.Cut(name, room-mark.width()), titleInk}}, mark...).fit(room)
}
