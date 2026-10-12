package dashboard

import (
	"fmt"
	"math"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// noFigure is a figure History doesn't know, or doesn't yet: a dash, in
// text.faint.
var noFigure = span{"—", faintInk}

// History's dates: a month as views names it, as 2026-10; a week by its
// first day, as 28 Sep; and a day, as Thu 1 Oct.
const (
	monthLayout = "2006-01"
	weekLayout  = "2 Jan"
	dayLayout   = "Mon 2 Jan"
)

// civilDay is the day with the given date, reporting false for one that
// isn't.
func civilDay(date string) (time.Time, bool) {
	day, err := time.Parse(time.DateOnly, date)
	return day, err == nil
}

// monthName is the month, as views names it, as History names it: Oct, or
// with its year, Oct 2025; as it is where it isn't a month.
func monthName(month string, withYear bool) string {
	m, err := time.Parse(monthLayout, month)
	if err != nil {
		return month
	}
	if withYear {
		return m.Format("Jan 2006")
	}
	return m.Format("Jan")
}

// weekHead heads a week, as Weeks and Days do, by its first day's date, as
// 10 Aug: in text.faint, this week's bold text.secondary; as it is, cleaned,
// where it isn't a date.
func weekHead(date string, this bool) line {
	k := faintInk
	if this {
		k = strongInk
	}
	if day, ok := civilDay(date); ok {
		return line{{day.Format(weekLayout), k}}
	}
	return line{{status.Clean(date), k}}
}

// dayAbbreviation and dayInitial name a weekday as History does, wide and
// on a phone or over Days' week: Mon, M.
func dayAbbreviation(d time.Weekday) string { return d.String()[:3] }
func dayInitial(d time.Weekday) string      { return d.String()[:1] }

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

// againstSaid is a worth against its price, as "3.8×", bold, in
// state.positive at 1× or more as it's written, and accent.attention under: a
// dash where there's none.
func againstSaid(ratio *float64) line {
	if ratio == nil {
		return line{noFigure}
	}
	shown := math.Round(*ratio*10) / 10
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
// label, bold text.primary, cut to keep the ◆ after the primary's, in
// text.tertiary. History shows no routing, so no ▲.
func historyAccount(label string, primary bool, room int) line {
	var mark line
	if primary {
		mark = line{{" ◆", primaryInk}}
	}
	return append(line{{truncate(status.Clean(label), room-mark.width()), titleInk}}, mark...)
}
