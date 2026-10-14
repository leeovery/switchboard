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

// Over says the span a recent rate is measured over, from since until now,
// as prose.Over says it: "last 30 min", "last 18 min" for a window without a
// level that far back, or "last 2h" across a gap in its readings. A rate that
// doesn't say when it's measured from, as a router from before gave it, is
// the half hour's.
func Over(since, now time.Time) string {
	if since.IsZero() {
		since = now.Add(-score.Recent)
	}
	return prose.Over(now.Sub(since))
}

// notStarted is what's said of a window that has lapsed: it isn't running,
// and reads empty, until a request starts it.
const notStarted = "not started"

// Text renders the moment's document for a terminal, as it stands at Now:
// each account, the primary marked, with its windows, when they reset and
// where they're heading, or that one standing not started hasn't started,
// and when it's primed, whatever couldn't be read, what holds it back,
// whether it's under pressure, and how many sessions the router has sent it;
// then the sessions given, as the router lists those it has routed in the
// last hour, a line each; then the priming schedule, and what comes next of
// it; then the account new sessions go to, as Next has it, which the
// dashboard marks ▲; then where the usage came from, and last a restart the
// router has due. Each is said as the moment has it, worked out once.
// Countdowns run from Now, and times show in Now's time zone. Every text it
// shows that came from elsewhere, such as the config's labels or the
// upstream's errors, it shows cleaned.
func (m Moment) Text(sessions ...Session) string {
	d, now := m.doc, m.Now
	width := m.nameWidth()
	var b strings.Builder
	for i, a := range m.Accounts {
		j := m.judged[i]
		m.writeAccount(&b, a, j, width)
		m.writeNotes(&b, a, j.states.account)
		b.WriteString("\n")
	}
	if len(sessions) > 0 {
		b.WriteString("sessions\n")
		for _, s := range sessions {
			fmt.Fprintf(&b, "  %s\n", s.Line(now))
		}
		b.WriteString("\n")
	}
	m.writePriming(&b)
	if next, ok := m.Account(m.Next); ok {
		fmt.Fprintf(&b, "best next: %s\n", next.Title())
	}
	fmt.Fprintf(&b, "%s\n", d.origin())
	if d.Restart.Due() {
		fmt.Fprintf(&b, "%s\n", d.Restart.Text(now))
	}
	return b.String()
}

// writePriming writes the priming schedule, when priming is on, such as
// "priming 08:00-23:00: work at 04:10 and side at 06:40", and under it what
// comes next of it at the moment, as its Coming has it.
func (m Moment) writePriming(b *strings.Builder) {
	prime := m.doc.Prime
	if len(prime.Slots) == 0 {
		return
	}
	slots := make([]string, len(prime.Slots))
	for i, s := range prime.Slots {
		slots[i] = prose.Clean(s.Account) + " at " + prose.Clean(s.At)
	}
	fmt.Fprintf(b, "priming %s: %s\n", prose.Clean(prime.Day), prose.List(slots))
	if len(m.Coming) > 0 {
		parts := make([]string, len(m.Coming))
		for i, c := range m.Coming {
			parts[i] = c.What + ": " + c.Account.Title() + ", " + prose.Clock(m.Now, c.At)
		}
		fmt.Fprintf(b, "%s\n", strings.Join(parts, prose.Separator))
	}
}

// writeNotes writes what's noted of the account a, as read, beside its usage,
// as it stands at the moment as an has it: what the router saw hold it back
// then, how its reserve stands, whether it's under pressure, and how many
// sessions the router has sent it.
func (m Moment) writeNotes(b *strings.Builder, an AccountNow, a Account) {
	notes := a.heldBy(m.Now)
	if said := reserved(an.ReachedReserve, an.Pinned, an.Reserve); said != "" {
		notes = append(notes, said)
	}
	if pressed := m.pressureNote(a, an.Pressure); pressed != "" {
		notes = append(notes, pressed)
	}
	if an.Sessions > 0 {
		notes = append(notes, prose.SessionCount(an.Sessions))
	}
	for _, note := range notes {
		fmt.Fprintf(b, "  %s\n", note)
	}
}

// reservedOf says how the account's reserve stands at now once a window has
// reached it, as reachedReserve judges, as reserved says it.
func (d Document) reservedOf(a Account, now time.Time) string {
	return reserved(a.reachedReserve(now), d.Pin.Has(a.ID), a.Reserve)
}

// reserved says how an account's reserve, reserve of each window, stands
// once the windows reached have reached it: "at its reserve (90%)", the
// router's own choices passing the account over from that share of a window
// on, or, with the global pin naming it, as pinned says, "spending its
// reserve (pinned)". It's "" while no window has reached it.
func reserved(reached []string, pinned bool, reserve float64) string {
	switch {
	case len(reached) == 0:
		return ""
	case pinned:
		return "spending its reserve (pinned)"
	default:
		return "at its reserve (" + prose.Percent(1-reserve) + ")"
	}
}

// pressureLine says an account standing under pressure as p has it is under
// pressure, and when its pressure window runs out, as p's Out has it, in
// now's time zone: "under pressure: runs out ~18:21", or, with its reserve
// holding it back there, "under pressure: at its reserve ~18:21"; "under
// pressure" alone where it doesn't before it resets. It reports whether it
// says when. It's "" while it isn't under pressure. The router says an
// account is under pressure only where it can take a request of some model:
// of one that can take none, pressure isn't what passes it over.
func pressureLine(p PressureNow, now time.Time) (string, bool) {
	switch out := p.Out; {
	case !p.Under:
		return "", false
	case out.At.IsZero():
		return "under pressure", false
	case out.Reserve:
		return "under pressure: at its reserve ~" + prose.TimeOfDay(now, out.At), true
	default:
		return "under pressure: runs out ~" + prose.TimeOfDay(now, out.At), true
	}
}

// pressureNote says the account a, as read, standing under pressure as p has
// it, is under pressure, as pressureLine does, and, where that says when its
// window runs out, why: the rate it goes by, the window as the moment names
// it, put as a sentence has it, and the reset its window runs out before, as
// in "under pressure: runs out ~18:21 at 5-hour's rate over the last 30 min,
// before its reset at 20:10". It's "" while it isn't under pressure.
func (m Moment) pressureNote(a Account, p PressureNow) string {
	now := m.Now
	pressed, timed := pressureLine(p, now)
	w, ok := a.Window(p.Window)
	if !timed || !ok {
		return pressed
	}
	over := "over the " + Over(a.Pressure.Since, now)
	if !a.Pressure.Recent {
		over = "since it started"
	}
	return pressed + " at " + InProse(m.NameOf(w.Key)) + "'s rate " + over + ", before its reset at " + prose.TimeOfDay(now, w.ResetsAt)
}

// Window returns the account's window with the given key, reporting false
// when it has none.
func (a Account) Window(key string) (quota.Window, bool) {
	i := slices.IndexFunc(a.Windows, func(w quota.Window) bool { return w.Key == key })
	if i < 0 {
		return quota.Window{}, false
	}
	return a.Windows[i], true
}

// heldBy says what holds the account back at now, as the router saw it, a
// line each: the limit it reached, then the upstream's refusal, each while it
// holds.
func (a Account) heldBy(now time.Time) []string {
	var held []string
	if a.Limit.holds(now) {
		held = append(held, a.Limit.Text(now))
	}
	if a.Refused.holds(now) {
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
		return "from the router: " + health + prose.Separator + prose.SessionCount(d.Sessions) + prose.Separator + d.Routing()
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
	if why = prose.Clean(why); why == "" {
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

// destination says where a pin to the accounts with the given ids sends
// sessions: to the one, as in "side · Side", or to the best of several, as in
// "the best of work · Work and side · Side", named as Names names them.
func (d Document) destination(ids []string) string {
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
		names[i] = prose.Clean(id)
		if account, ok := d.Account(id); ok {
			names[i] = account.Title()
		}
	}
	return prose.List(names)
}

// Text says until when the limit holds, such as "limit until Mon 21:00", in
// now's time zone.
func (l Limit) Text(now time.Time) string {
	return "limit until " + prose.Clock(now, l.Until)
}

// Text says why the restart is due, and since when, in now's time zone, and
// how it's to come: once no request is in flight, or now, with the service
// restarted, as in "restart due since Mon 14:02 (config changed), once no
// request is in flight (3 now): switchboard service restart restarts it now,
// cutting off requests still in flight after 30 seconds"; or, the router run
// by hand, once it's run again.
func (r Restart) Text(now time.Time) string {
	due := "restart due since " + prose.Clock(now, r.Since) + " (" + prose.Clean(r.Reason) + ")"
	if r.ByHand {
		return due + ": run switchboard serve again to take it up"
	}
	return fmt.Sprintf("%s, once no request is in flight (%d now): switchboard service restart restarts it now, "+
		"cutting off requests still in flight after 30 seconds", due, r.InFlight)
}

// Text says how the upstream refused, and until when the refusal holds, such
// as "refused (403, opus) until 21:40", in now's time zone.
func (r Refusal) Text(now time.Time) string {
	return r.Answer() + " until " + prose.TimeOfDay(now, r.Until)
}

// Answer says how the upstream refused, as RefusalAnswer says it, such as
// "refused (401)", or, refusing a family's requests alone, "refused (403,
// opus)".
func (r Refusal) Answer() string {
	return RefusalAnswer(r.Status, r.Family)
}

// RefusalAnswer says how the upstream refused, with the HTTP status code
// given, of the model family given, cleaned, where it refused that family's
// requests alone: "refused (401)", "refused (403, opus)"; either left out
// where it's none, and "refused" alone where both are.
func RefusalAnswer(code int, family string) string {
	var answer []string
	if code != 0 {
		answer = append(answer, strconv.Itoa(code))
	}
	if family = prose.Clean(family); family != "" {
		answer = append(answer, family)
	}
	if len(answer) == 0 {
		return "refused"
	}
	return "refused (" + strings.Join(answer, ", ") + ")"
}

// writeAccount writes the account a, judged as j, as it stands at the
// moment: its title, marking the primary, its windows with their names
// width wide, each as accountWindow has it, and whatever couldn't be read.
func (m Moment) writeAccount(b *strings.Builder, a AccountNow, j judgedAccount, width int) {
	title := a.Title()
	if a.Primary {
		title += " (primary)"
	}
	fmt.Fprintf(b, "%s\n", title)
	for i, w := range j.windows {
		fmt.Fprintf(b, "  %s\n", m.accountWindow(a, a.Windows[i], w, width))
	}
	for _, f := range a.Failures {
		fmt.Fprintf(b, "  %s offline: %s\n", prose.Clean(f.Label), prose.Clean(f.Error))
	}
	if a.Error != "" {
		fmt.Fprintf(b, "  %s\n", a.Error)
	}
}

// InProse is a window's label as a sentence has it, cleaned: one word, as
// Session or Week, is a common noun, so lower-cased; more, as Fable week,
// leads with a model's name, which keeps its capital.
func InProse(label string) string {
	label = prose.Clean(label)
	if label == "" || strings.Contains(label, " ") {
		return label
	}
	first, size := utf8.DecodeRuneInString(label)
	return string(unicode.ToLower(first)) + label[size:]
}

// Short is a window's label as a narrow column has it, cleaned: a model's own
// week, as Fable week, is its wk, and any other label as it is.
func Short(label string) string {
	label = prose.Clean(label)
	if before, ok := strings.CutSuffix(label, " week"); ok && before != "" {
		return before + " wk"
	}
	return label
}

// projection says where a window is heading, such as "on pace for 92%",
// "runs out ~Fri 19:40" or "exhausted", with times in now's time zone. It's
// empty when the projection says nothing.
func projection(now time.Time, p score.Projection) string {
	switch p.Kind {
	case score.OnPace:
		return "on pace for " + prose.Percent(p.AtReset)
	case score.RunsOut:
		return "runs out ~" + prose.Clock(now, p.At)
	case score.Exhausted:
		return "exhausted"
	default:
		return ""
	}
}

// Title names an account by its id and label, such as "work · Work", each
// cleaned.
func (a Account) Title() string {
	return title(a.ID, a.Label)
}

// title names the account with the given id and label, as Title does.
func title(id, label string) string {
	return prose.Clean(id) + " · " + prose.Clean(label)
}

// Account finds the account with the given id.
func (d Document) Account(id string) (Account, bool) {
	i := slices.IndexFunc(d.Accounts, func(a Account) bool { return a.ID == id })
	if i < 0 {
		return Account{}, false
	}
	return d.Accounts[i], true
}

// accountWindow shows account a's window, judged as w, standing at the moment
// as wn has it, by its name at the moment, width wide, and its use as it
// stands: not started, and when it's primed; else with when it resets and
// where it's heading, as windowNotes has them.
func (m Moment) accountWindow(a AccountNow, wn WindowNow, w judged, width int) string {
	name := m.NameOf(w.window.Key)
	if wn.Kind == WindowNotStarted {
		return windowLine(name, wn.Used, width, []string{notStartedNote(a.NextPrime, m.Now)})
	}
	return windowLine(name, wn.Used, width, windowNotes(w.stands, w.out.Heading, m.Now))
}

// windowLine shows a window's name, nameWidth wide, and how much of it is
// used, then the notes given of it.
func windowLine(name string, used float64, nameWidth int, notes []string) string {
	line := fmt.Sprintf("%-*s %4s", nameWidth, name, prose.Percent(used))
	if len(notes) == 0 {
		return line
	}
	return line + "  " + strings.Join(notes, " · ")
}

// windowNotes say when a window resets, standing at now as stands, as
// WindowAt has it, so its next reset once it has reset since it was read,
// and none where it has lapsed; and where it's heading at now, as heading,
// the document's project of it, says, as far as those are known, and when
// that's at its rate over the last half hour, that it is.
func windowNotes(stands quota.Window, heading Heading, now time.Time) []string {
	var notes []string
	if reset := stands.ResetsAt; !reset.IsZero() {
		notes = append(notes, prose.Resets(now, reset)+" · "+prose.Clock(now, reset))
	}
	if heads := projection(now, heading.Projection); heads != "" {
		if heading.Recent {
			heads += " at its rate over the " + Over(heading.Since, now)
		}
		notes = append(notes, heads)
	}
	return notes
}

// nameWidth is the length of the longest name of a window at the moment,
// which lines the windows of every account up.
func (m Moment) nameWidth() int {
	width := 0
	for _, account := range m.doc.Accounts {
		for _, w := range account.Windows {
			width = max(width, utf8.RuneCountInString(m.NameOf(w.Key)))
		}
	}
	return width
}
