package status

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// Separator sets apart the parts of a line that says several things, such as
// where the usage came from: wider than the dot within an account's title, so
// the title reads as one part.
const Separator = "  ·  "

// Lately is the span of the readings a window's recent rate is measured over,
// as it's said: "last 30 min".
var Lately = fmt.Sprintf("last %.0f min", score.Recent.Minutes())

// notStarted is what's said of a window that has lapsed: it isn't running,
// and reads empty, until a request starts it.
const notStarted = "not started"

// Text renders the document for a terminal: each account, the primary marked,
// with its windows, when they reset and where they're heading, or that one
// that has lapsed hasn't started, and when it's primed, whatever couldn't be
// read, what holds it back, whether it's under pressure, and how many
// sessions the router has sent it;
// then the sessions given, as the router lists those it has routed in the
// last hour, a line each; then the priming schedule, and what comes next of
// it; then the account to use next; then where the usage came from, and last
// a restart the router has due.
// Countdowns run from now, and times show in now's time zone. Every text it
// shows that came from elsewhere, such as the config's labels or the
// upstream's errors, it shows cleaned.
func (d Document) Text(now time.Time, sessions ...Session) string {
	width := labelWidth(d.Accounts)
	var b strings.Builder
	for _, account := range d.Accounts {
		d.writeAccount(&b, account, width, now)
		d.writeNotes(&b, account, now)
		b.WriteString("\n")
	}
	if len(sessions) > 0 {
		b.WriteString("sessions\n")
		for _, s := range sessions {
			fmt.Fprintf(&b, "  %s\n", s.Line(now))
		}
		b.WriteString("\n")
	}
	d.writePriming(&b, now)
	if best, ok := d.Account(d.Best); ok {
		fmt.Fprintf(&b, "best next: %s\n", best.Title())
	}
	fmt.Fprintf(&b, "%s\n", d.origin())
	if d.Restart.Due() {
		fmt.Fprintf(&b, "%s\n", d.Restart.Text(now))
	}
	return b.String()
}

// writePriming writes the priming schedule, when priming is on, such as
// "priming 08:00-23:00: work at 04:10 and side at 06:40", and under it what
// comes next of it at now.
func (d Document) writePriming(b *strings.Builder, now time.Time) {
	if len(d.Prime.Slots) == 0 {
		return
	}
	slots := make([]string, len(d.Prime.Slots))
	for i, s := range d.Prime.Slots {
		slots[i] = Clean(s.Account) + " at " + Clean(s.At)
	}
	fmt.Fprintf(b, "priming %s: %s\n", Clean(d.Prime.Day), prose.List(slots))
	if coming := d.Coming(now); len(coming) > 0 {
		parts := make([]string, len(coming))
		for i, c := range coming {
			parts[i] = c.What + ": " + c.Account.Title() + ", " + Clock(now, c.At)
		}
		fmt.Fprintf(b, "%s\n", strings.Join(parts, Separator))
	}
}

// writeNotes writes what's noted of the account beside its usage: what the
// router saw hold it back at now, how its reserve stands, whether it's under
// pressure, and how many sessions the router has sent it.
func (d Document) writeNotes(b *strings.Builder, a Account, now time.Time) {
	notes := a.HeldBy(now)
	if reserved := d.Reserved(a); reserved != "" {
		notes = append(notes, reserved)
	}
	if pressed := d.pressureNote(a, now); pressed != "" {
		notes = append(notes, pressed)
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
// over from that share of a window on, or, with the global pin naming it,
// "spending its reserve (pinned)". It's "" while no window has reached it.
func (d Document) Reserved(a Account) string {
	switch {
	case len(a.AtReserve) == 0:
		return ""
	case d.Pin.Has(a.ID):
		return "spending its reserve (pinned)"
	default:
		return "at its reserve (" + Percent(1-a.Reserve) + ")"
	}
}

// Pressed says the account is under pressure, and when, at the rate the
// router saw its pressure window used, it runs out for the router, in now's
// time zone: "under pressure: runs out ~18:21", or, with its reserve holding
// it back there, "under pressure: at its reserve ~18:21". It's "" while it
// isn't under pressure.
func (d Document) Pressed(a Account, now time.Time) string {
	if !a.Pressure.Under {
		return ""
	}
	out := "runs out"
	if a.Reserve > 0 && !d.Pin.Has(a.ID) {
		out = "at its reserve"
	}
	return "under pressure: " + out + " ~" + TimeOfDay(now, a.Pressure.RunsOut)
}

// pressureNote says the account is under pressure, as Pressed does, and why:
// the rate it goes by, and the reset its window runs out before, as in "under
// pressure: runs out ~18:21 at Session's rate over the last 30 min, before its
// reset at 20:10". It's "" while it isn't under pressure.
func (d Document) pressureNote(a Account, now time.Time) string {
	pressed := d.Pressed(a, now)
	w, ok := a.window(a.Pressure.Window)
	if pressed == "" || !ok {
		return pressed
	}
	over := "over the " + Lately
	if !a.Pressure.Recent {
		over = "since it started"
	}
	return pressed + " at " + Clean(w.Label) + "'s rate " + over + ", before its reset at " + TimeOfDay(now, w.ResetsAt)
}

// window returns the account's window with the given key.
func (a Account) window(key string) (quota.Window, bool) {
	i := slices.IndexFunc(a.Windows, func(w quota.Window) bool { return w.Key == key })
	if i < 0 {
		return quota.Window{}, false
	}
	return a.Windows[i], true
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

// Routing says where the router sends new sessions: to the accounts pinned,
// as in "pinned to side · Side" or "pinned to work · Work and side · Side", or
// wherever suits, "routing automatically".
func (d Document) Routing() string {
	if d.Pin.IsZero() {
		return "routing automatically"
	}
	return "pinned to " + d.Names(d.Pin.Accounts)
}

// Destination says where a pin to the accounts with the given ids sends
// sessions: to the one, as in "side · Side", or to the best of several, as in
// "the best of work · Work and side · Side", named as Names names them.
func (d Document) Destination(ids []string) string {
	if len(ids) > 1 {
		return "the best of " + d.Names(ids)
	}
	return d.Names(ids)
}

// Names names the accounts with the given ids, as in "work · Work and side ·
// Side": each by its title, or by its id, cleaned, when the document lacks it.
func (d Document) Names(ids []string) string {
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = Clean(id)
		if account, ok := d.Account(id); ok {
			names[i] = account.Title()
		}
	}
	return prose.List(names)
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

// Text says why the restart is due, and since when, in now's time zone, and
// how it's to come: once no request is in flight, or now, with the service
// restarted, as in "restart due since Mon 14:02 (config changed), once no
// request is in flight (3 now): switchboard service restart restarts it now,
// cutting off requests still in flight after 30 seconds"; or, the router run
// by hand, once it's run again.
func (r Restart) Text(now time.Time) string {
	due := "restart due since " + Clock(now, r.Since) + " (" + Clean(r.Reason) + ")"
	if r.ByHand {
		return due + ": run switchboard serve again to take it up"
	}
	return fmt.Sprintf("%s, once no request is in flight (%d now): switchboard service restart restarts it now, "+
		"cutting off requests still in flight after 30 seconds", due, r.InFlight)
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

// writeAccount writes the account's title, marking the primary, its windows
// with their labels width wide, and whatever couldn't be read.
func (d Document) writeAccount(b *strings.Builder, a Account, width int, now time.Time) {
	title := a.Title()
	if a.Primary {
		title += " (primary)"
	}
	fmt.Fprintf(b, "%s\n", title)
	for _, w := range a.Windows {
		notes := a.windowNotes(w, now)
		if a.HasLapsed(w) {
			notes = []string{d.NotStarted(a.ID, now)}
		}
		fmt.Fprintf(b, "  %s\n", windowLine(w, width, notes))
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

// windowLine shows a window's label, labelWidth wide, and its utilization,
// then the notes given of it.
func windowLine(w quota.Window, labelWidth int, notes []string) string {
	line := fmt.Sprintf("%-*s %4s", labelWidth, Clean(w.Label), Percent(w.Utilization))
	if len(notes) == 0 {
		return line
	}
	return line + "  " + strings.Join(notes, " · ")
}

// windowNotes say when the account's window w resets and where it's heading at
// now, as Project says, as far as those are known, and when that's at its
// rate over the last half hour, that it is.
func (a Account) windowNotes(w quota.Window, now time.Time) []string {
	var notes []string
	if !w.ResetsAt.IsZero() {
		notes = append(notes, Resets(now, w.ResetsAt)+" · "+Clock(now, w.ResetsAt))
	}
	heading := a.Project(w, now)
	if projection := Projection(now, heading.Projection); projection != "" {
		if heading.Recent {
			projection += " at its rate over the " + Lately
		}
		notes = append(notes, projection)
	}
	return notes
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
