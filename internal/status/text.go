package status

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// Separator sets apart the parts of a line that says several things, such as
// where the usage came from: wider than the dot within an account's title, so
// the title reads as one part.
const Separator = "  ·  "

// Text renders the document for a terminal: each account, the primary marked,
// with its windows, when they reset and where they're heading, whatever
// couldn't be read, what holds it back, and how many sessions the router has
// sent it; then the account to use next, and last where the usage came from.
// Countdowns run from now, and times show in now's time zone. Every text it
// shows that came from elsewhere, such as the config's labels or the
// upstream's errors, it shows cleaned.
func (d Document) Text(now time.Time) string {
	width := labelWidth(d.Accounts)
	var b strings.Builder
	for i, account := range d.Accounts {
		if i > 0 {
			b.WriteString("\n")
		}
		account.write(&b, width, now)
		d.writeNotes(&b, account, now)
	}
	if len(d.Accounts) > 0 {
		b.WriteString("\n")
	}
	if best, ok := d.Account(d.Best); ok {
		fmt.Fprintf(&b, "best next: %s\n", best.Title())
	}
	fmt.Fprintf(&b, "%s\n", d.origin())
	return b.String()
}

// Text renders the account for a terminal as the document's Text does, but
// for what's noted of it beside its usage.
func (a Account) Text(now time.Time) string {
	var b strings.Builder
	a.write(&b, labelWidth([]Account{a}), now)
	return b.String()
}

// writeNotes writes what's noted of the account beside its usage: what the
// router saw hold it back at now, how its reserve stands, and how many
// sessions the router has sent it.
func (d Document) writeNotes(b *strings.Builder, a Account, now time.Time) {
	notes := a.HeldBy(now)
	if reserved := d.Reserved(a); reserved != "" {
		notes = append(notes, reserved)
	}
	if a.Sessions > 0 {
		notes = append(notes, SessionCount(a.Sessions))
	}
	for _, note := range notes {
		fmt.Fprintf(b, "  %s\n", note)
	}
}

// Reserved says how the account's reserve stands once a window has reached
// it: "at its reserve (90%)", the router's own choices passing the account
// over from that share of a window on, or, with the global pin on it,
// "spending its reserve (pinned)". It's "" while no window has reached it.
func (d Document) Reserved(a Account) string {
	switch {
	case len(a.AtReserve) == 0:
		return ""
	case a.ID == d.Pin.Account:
		return "spending its reserve (pinned)"
	default:
		return "at its reserve (" + Percent(1-a.Reserve) + ")"
	}
}

// HeldBy says what holds the account back at now, as the router saw it, a
// line each: the limit it reached, then the upstream's refusal, each while it
// holds.
func (a Account) HeldBy(now time.Time) []string {
	var held []string
	if a.Limit.Holds(now) {
		held = append(held, a.Limit.Text(now))
	}
	if a.Refused.Holds(now) {
		held = append(held, a.Refused.Text(now))
	}
	return held
}

// origin says where the document's usage came from: the router, with how it
// fares, how many sessions it has and where it sends new ones; or probing,
// and why the router's document wasn't read, when it was asked for.
func (d Document) origin() string {
	if d.Source == SourceRouter {
		health := "healthy"
		if !d.Router.Healthy {
			health = because("unhealthy", d.Router.Reason)
		}
		return "from the router: " + health + Separator + SessionCount(d.Sessions) + Separator + d.Routing()
	}
	switch d.Fallback.Router {
	case RouterNotRunning:
		return "probed directly: the router isn't running"
	case RouterUnhealthy:
		return "probed directly: " + because("the router is unhealthy", d.Fallback.Reason)
	default:
		return "probed directly"
	}
}

// because follows what with why, cleaned, when there's a why.
func because(what, why string) string {
	if why = Clean(why); why == "" {
		return what
	}
	return what + ", " + why
}

// Routing says where the router sends new sessions: to the account pinned, as
// in "pinned to side · Side", or wherever suits, "routing automatically".
func (d Document) Routing() string {
	if d.Pin.Account == "" {
		return "routing automatically"
	}
	name := Clean(d.Pin.Account)
	if account, ok := d.Account(d.Pin.Account); ok {
		name = account.Title()
	}
	return "pinned to " + name
}

// SessionCount counts sessions in words, such as "3 sessions", "1 session" or
// "no sessions".
func SessionCount(n int) string {
	switch n {
	case 0:
		return "no sessions"
	case 1:
		return "1 session"
	default:
		return fmt.Sprintf("%d sessions", n)
	}
}

// Text says until when the limit holds, such as "limit until Mon 21:00", in
// now's time zone.
func (l Limit) Text(now time.Time) string {
	return "limit until " + Clock(now, l.Until)
}

// Text says how the upstream refused, and until when the refusal holds, such
// as "refused (403, opus) until 21:40", in now's time zone.
func (r Refusal) Text(now time.Time) string {
	answer := strconv.Itoa(r.Status)
	if r.Family != "" {
		answer += ", " + Clean(r.Family)
	}
	return "refused (" + answer + ") until " + TimeOfDay(now, r.Until)
}

// write writes the account's title, marking the primary, its windows with
// their labels width wide, and whatever couldn't be read.
func (a Account) write(b *strings.Builder, width int, now time.Time) {
	title := a.Title()
	if a.Primary {
		title += " (primary)"
	}
	fmt.Fprintf(b, "%s\n", title)
	for _, w := range a.Windows {
		fmt.Fprintf(b, "  %s\n", windowLine(w, width, now))
	}
	for _, f := range a.Failures {
		fmt.Fprintf(b, "  %s offline: %s\n", Clean(f.Label), Clean(f.Error))
	}
	if err := Clean(a.Error); err != "" {
		fmt.Fprintf(b, "  %s\n", err)
	}
}

// Countdown says how long it is from now until t, in whole units: "5d 12h"
// from a day away, "4h 57m" from an hour, else "7m"; "now" once t has come.
// A second unit of zero is left off, as in "6d" and "4h".
func Countdown(now, t time.Time) string {
	d := t.Sub(now)
	if d <= 0 {
		return "now"
	}
	hours, minutes := int(d/time.Hour), int(d%time.Hour/time.Minute)
	switch {
	case hours >= 24:
		return withRest(fmt.Sprintf("%dd", hours/24), hours%24, "h")
	case hours >= 1:
		return withRest(fmt.Sprintf("%dh", hours), minutes, "m")
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// withRest follows a count with what's left over in the next unit down,
// unless nothing is: "5d 12h", but "6d".
func withRest(count string, rest int, unit string) string {
	if rest == 0 {
		return count
	}
	return fmt.Sprintf("%s %d%s", count, rest, unit)
}

// Clock shows t as its weekday and 24-hour time in now's time zone, such as
// "Mon 18:10".
func Clock(now, t time.Time) string {
	return t.In(now.Location()).Format("Mon 15:04")
}

// TimeOfDay shows t as its 24-hour time in now's time zone, such as "13:51".
func TimeOfDay(now, t time.Time) string {
	return t.In(now.Location()).Format("15:04")
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
		return "runs out ~" + Clock(now, p.At)
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

// Title names an account by its id and label, such as "work · Work", each
// cleaned.
func (a Account) Title() string {
	return Clean(a.ID) + " · " + Clean(a.Label)
}

// Clean makes text safe to show on one line of a terminal: control
// characters, which would move the cursor or restyle what follows, become
// spaces, and each run of spaces becomes one.
func Clean(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	return strings.Join(strings.Fields(text), " ")
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
	line := fmt.Sprintf("%-*s %4s", labelWidth, Clean(w.Label), Percent(w.Utilization))
	var notes []string
	if !w.ResetsAt.IsZero() {
		notes = append(notes, Resets(now, w.ResetsAt)+" · "+Clock(now, w.ResetsAt))
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
			width = max(width, utf8.RuneCountInString(Clean(w.Label)))
		}
	}
	return width
}
