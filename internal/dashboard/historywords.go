package dashboard

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// noFigure is a figure History doesn't know, or doesn't yet: a dash, in
// text.faint.
var noFigure = span{"—", faintInk}

// The cells the widest of a bounded figure takes, as its format draws it: a
// day, as dayLayout writes one of two figures; a run of a year's days, a leap
// year's whole; and a share, all of it.
var (
	widestDay   = ansi.StringWidth(time.Date(2026, time.September, 22, 0, 0, 0, 0, time.UTC).Format(dayLayout))
	widestRun   = ansi.StringWidth(prose.Counted(366, "day"))
	widestShare = ansi.StringWidth(status.Percent(1))
)

// worthSaid is a worth as History writes it, in the ink given: to the
// dollar, its thousands set apart, as "$27,480"; a faint + after it where
// part of it is unpriced, as "$184+"; unpriced where all of it is; and a
// dash for none.
func worthSaid(w views.Worth, k ink) line {
	switch {
	case w.Amount == nil && len(w.Unpriced) > 0:
		return line{{"unpriced", k}}
	case w.Amount == nil:
		return line{noFigure}
	case len(w.Unpriced) > 0:
		return line{{w.Amount.GroupedDollars(), k}, {"+", faintInk}}
	}
	return line{{w.Amount.GroupedDollars(), k}}
}

// WorthCents is a worth to the cent, as a request's, a turn's or a session's
// is said, the Log's rule: "$0.15"; "<$0.01" where it's more than nothing but
// under half a cent, which would read as nothing; "unpriced" where none of it
// is priced; and "+" after it where part of it is unpriced, as "$17.86+".
// Nothing at all is "$0.00".
func WorthCents(w views.Worth) string {
	if w.Amount == nil && len(w.Unpriced) > 0 {
		return "unpriced"
	}
	var amount ledger.Picodollars
	if w.Amount != nil {
		amount = *w.Amount
	}
	said := amount.Cents()
	if amount > 0 && said == noCents {
		said = "<$0.01"
	}
	if len(w.Unpriced) > 0 {
		said += "+"
	}
	return said
}

// noCents is a worth of none, to the cent.
var noCents = ledger.Picodollars(0).Cents()

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

// againstSaid is a worth against its price, as "3.8×", to a tenth, a half
// rounded to the even one, as every figure is: bold, in state.positive at 1×
// or more as it's written, and accent.attention under; a dash where there's
// none.
func againstSaid(ratio *float64) line {
	if ratio == nil {
		return line{noFigure}
	}
	shown := math.RoundToEven(*ratio*10) / 10
	k := boldPositiveInk
	if shown < 1 {
		k = alertInk
	}
	return line{{fmt.Sprintf("%.1f×", shown), k}}
}

// shareSaid is a share as a whole percentage, as "21%", in the ink given: a
// dash where it isn't known.
func shareSaid(share *float64, k ink) line {
	if share == nil {
		return line{noFigure}
	}
	return line{{status.Percent(*share), k}}
}

// familyName is a family's name as History shows it, its first letter
// capital: Opus.
func familyName(family string) string {
	name := status.Clean(family)
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
	return append(line{{truncate(name, room-mark.width()), titleInk}}, mark...).fit(room)
}
