package status

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/quota"
)

// Text renders the document for a terminal: each account's windows with when
// they reset, then whatever couldn't be read. Countdowns run from now, and
// times show in now's time zone.
func (d Document) Text(now time.Time) string {
	width := labelWidth(d.Accounts)
	var b strings.Builder
	for i, account := range d.Accounts {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s · %s\n", account.ID, account.Label)
		for _, w := range account.Windows {
			fmt.Fprintf(&b, "  %s\n", windowLine(w, width, now))
		}
		for _, f := range account.Failures {
			fmt.Fprintf(&b, "  %s offline: %s\n", f.Label, f.Error)
		}
		if account.Error != "" {
			fmt.Fprintf(&b, "  %s\n", account.Error)
		}
	}
	return b.String()
}

// Countdown says how long it is from now until t, in whole units: "5d 12h"
// from a day away, "4h 57m" from an hour, else "7m"; "now" once t has come.
func Countdown(now, t time.Time) string {
	d := t.Sub(now)
	if d <= 0 {
		return "now"
	}
	hours, minutes := int(d/time.Hour), int(d%time.Hour/time.Minute)
	switch {
	case hours >= 24:
		return fmt.Sprintf("%dd %dh", hours/24, hours%24)
	case hours >= 1:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// Clock shows t as its weekday and 24-hour time, such as "Mon 18:10".
func Clock(t time.Time) string {
	return t.Format("Mon 15:04")
}

// windowLine shows a window's label and utilization, then when it resets if
// that's known.
func windowLine(w quota.Window, labelWidth int, now time.Time) string {
	line := fmt.Sprintf("%-*s %4s", labelWidth, w.Label, percent(w.Utilization))
	if w.ResetsAt.IsZero() {
		return line
	}
	return line + "  " + resets(now, w.ResetsAt)
}

// percent shows a utilization as a whole percentage, such as "23%".
func percent(utilization float64) string {
	return fmt.Sprintf("%.0f%%", utilization*100)
}

// resets says when a window resets, such as "resets in 4h 57m · Mon 18:10".
func resets(now, at time.Time) string {
	clock := Clock(at.In(now.Location()))
	if !at.After(now) {
		return "resets now · " + clock
	}
	return "resets in " + Countdown(now, at) + " · " + clock
}

// labelWidth is the length of the longest window label, which lines the
// windows of every account up.
func labelWidth(accounts []Account) int {
	width := 0
	for _, account := range accounts {
		for _, w := range account.Windows {
			width = max(width, utf8.RuneCountInString(w.Label))
		}
	}
	return width
}
