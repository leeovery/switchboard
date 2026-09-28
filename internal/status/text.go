package status

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// Text renders the document for a terminal: each account's windows with when
// they reset and where they're heading, then whatever couldn't be read, and
// last the account to use next. Countdowns run from now, and times show in
// now's time zone.
func (d Document) Text(now time.Time) string {
	width := labelWidth(d.Accounts)
	var b strings.Builder
	for i, account := range d.Accounts {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s\n", account.Title())
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
	if best, ok := d.Account(d.Best); ok {
		fmt.Fprintf(&b, "\nbest next: %s\n", best.Title())
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

// Resets counts down from now to a window's reset at t: "resets in 4h 57m",
// or "resets now" once t has come.
func Resets(now, t time.Time) string {
	if !t.After(now) {
		return "resets now"
	}
	return "resets in " + Countdown(now, t)
}

// Projection says where a window is heading, such as "on pace for 92%",
// "runs out ~Fri 19:40" or "exhausted", with times in now's time zone. It's
// empty when the projection says nothing.
func Projection(now time.Time, p score.Projection) string {
	switch p.Kind {
	case score.OnPace:
		return "on pace for " + Percent(p.AtReset)
	case score.RunsOut:
		return "runs out ~" + Clock(p.At.In(now.Location()))
	case score.Exhausted:
		return "exhausted"
	default:
		return ""
	}
}

// Percent shows a utilization as a whole percentage, such as "23%".
func Percent(utilization float64) string {
	return fmt.Sprintf("%.0f%%", utilization*100)
}

// Title names an account by its id and label, such as "work · Work".
func (a Account) Title() string {
	return a.ID + " · " + a.Label
}

// Account finds the account with the given id.
func (d Document) Account(id string) (Account, bool) {
	i := slices.IndexFunc(d.Accounts, func(a Account) bool { return a.ID == id })
	if i < 0 {
		return Account{}, false
	}
	return d.Accounts[i], true
}

// windowLine shows a window's label and utilization, then when it resets and
// where it's heading, as far as those are known.
func windowLine(w quota.Window, labelWidth int, now time.Time) string {
	line := fmt.Sprintf("%-*s %4s", labelWidth, w.Label, Percent(w.Utilization))
	var notes []string
	if !w.ResetsAt.IsZero() {
		notes = append(notes, Resets(now, w.ResetsAt)+" · "+Clock(w.ResetsAt.In(now.Location())))
	}
	if projection := Projection(now, score.Project(w, now)); projection != "" {
		notes = append(notes, projection)
	}
	if len(notes) == 0 {
		return line
	}
	return line + "  " + strings.Join(notes, " · ")
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
