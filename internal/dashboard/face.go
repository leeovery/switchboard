package dashboard

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// busyWithin is how lately a session was seen for its dot to light: it's
// busy when it was seen in the last minute, or, as the request stream tells,
// while a request of its is in flight.
const busyWithin = time.Minute

// activeWithin is how lately a session's model was seen on an account for it
// to count among the account's sessions, as the router counts them: within
// the hour its cache lasts.
const activeWithin = time.Hour

// face is an account's card, as the document has it at a moment: the
// account and its place, its state, the window its front features and its
// other windows, its badges, and its sessions; and as the watch has it,
// whether it has the focus, and whether it's flipped to show its back.
type face struct {
	account status.Account
	place   int
	state   status.State
	// featured is the window it features, where it has one to, and chart its
	// chart.
	featured    standing
	hasFeatured bool
	chart       plot
	// windows are the keys of the windows every card shows, in order, and
	// bars how those it doesn't feature stand, of those it has; unread are
	// those of them a probe expected but couldn't read, by key, each named
	// by its window's label, and why.
	windows []string
	bars    map[string]standing
	unread  map[string]quota.Failure
	// next is set on the card of the account new sessions go to, of several,
	// and pinned on one the global pin names.
	next, pinned bool
	// busy are its sessions, the one seen last first, each lit while busy,
	// as the router listed them; sessions is how many it has.
	busy     []bool
	sessions int
	// lately are what its back's LATELY tells of, the newest first, where
	// it's flipped.
	lately []lateLine
	// primed is when the router next primes it: zero where unknown.
	primed time.Time
	// read is when the document was read, which a recent rate is measured
	// to.
	read time.Time
	// focused is set on the card with the focus, and flipped on a card
	// turned over to show its sessions.
	focused, flipped bool
}

// faces are doc's accounts' cards at now, and the keys of the windows every
// card's front shows; and what each flipped card's LATELY tells of, as
// latelies has it, of the events taken in once for every card.
func (f Frame) faces(doc status.Document, now time.Time) ([]face, []string) {
	shown := shownWindows(doc, now, f.Policy)
	var lately map[string][]lateLine
	if len(f.Flipped) > 0 {
		lately = latelies(doc, now)
	}
	faces := make([]face, len(doc.Accounts))
	for i, a := range doc.Accounts {
		faces[i] = f.face(doc, a, now, shown)
		if faces[i].flipped {
			faces[i].lately = lately[a.ID]
		}
	}
	return faces, shown
}

// face is doc's account a's card at now, of the windows shown: the window
// its front features, as f.Featured says, its other windows, and its
// sessions, as the router listed them, else as many as doc says it has; and
// whether it has the focus, and is flipped, as f says.
func (f Frame) face(doc status.Document, a status.Account, now time.Time, shown []string) face {
	fc := face{
		account: a, place: place(doc, a.ID), state: doc.StateOf(a, now, f.Policy),
		windows: shown, bars: make(map[string]standing),
		next: len(doc.Accounts) > 1 && doc.Best == a.ID, pinned: doc.Pin.Has(a.ID),
		sessions: a.Sessions, primed: primedNext(doc, a.ID),
		read: cmp.Or(doc.GeneratedAt, now), focused: f.Focus == a.ID, flipped: f.Flipped[a.ID],
	}
	if f.Sessions != nil {
		fc.busy = f.busy(a.ID, now)
		fc.sessions = len(fc.busy)
	}
	w, ok := featured(doc, a, now, f.Policy, f.Featured, shown)
	if ok {
		fc.featured, fc.hasFeatured = standingOf(doc, a, w, now, f.Policy), true
		fc.chart = f.plotOf(doc, fc, now)
	}
	for _, key := range shown {
		if bar, has := a.Window(key); has && (!ok || key != w.Key) {
			fc.bars[key] = standingOf(doc, a, bar, now, f.Policy)
		}
	}
	for _, failed := range a.Failures {
		if slices.Contains(shown, failed.Window) {
			if fc.unread == nil {
				fc.unread = make(map[string]quota.Failure)
			}
			if label := labelOf(doc, failed.Window); label != failed.Window {
				failed.Label = label
			}
			fc.unread[failed.Window] = failed
		}
	}
	return fc
}

// plotOf is the chart of the window the card of doc's account fc features,
// as it stands at now, in the tone of its state, from its history, and
// while it's held at its limit, on the floor from when that was reached, as
// its standing says; how fast it's been used lately, as recentRate says; and
// whether a session on the account is busy, as its dots say.
func (f Frame) plotOf(doc status.Document, fc face, now time.Time) plot {
	p := plot{standing: fc.featured, tone: fc.tone(), now: now, starts: doc.StartsAt(fc.account.ID, now), busy: slices.Contains(fc.busy, true)}
	p.start, p.length, p.spanned = p.window.Span()
	p.trail = f.History[Ref{Account: fc.account.ID, Window: p.window.Key}]
	p.traced = len(p.trail.Readings) > 0
	p.rate = recentRate(fc.account, p.window, p.trail, now)
	return p
}

// recentRate is how fast account a's window w has been used lately, at now,
// a share of it an hour: as the router saw it over the last half hour, where
// it gives the rate; else as the window's readings, trail, rise over it, as
// score.RecentRate measures it; none where neither says.
func recentRate(a status.Account, w quota.Window, trail Trail, now time.Time) float64 {
	if i := slices.IndexFunc(a.Rates, func(r status.Rate) bool { return r.Window == w.Key }); i >= 0 {
		return a.Rates[i].Rate
	}
	rate, _, _ := score.RecentRate(w, trail.Readings, now)
	return rate
}

// primedNext is when the router next primes the account with the given id,
// as doc's priming schedule says: zero where it doesn't.
func primedNext(doc status.Document, id string) time.Time {
	i := slices.IndexFunc(doc.Prime.Slots, func(s status.Slot) bool { return s.Account == id })
	if i < 0 {
		return time.Time{}
	}
	return doc.Prime.Slots[i].Next
}

// busy are the sessions active on the account with the given id at now, as
// active says, of those the router listed, the request stream's moves among
// them, in their order, the one seen last first: each lit while any of its
// models is busy there, as lit says.
func (f Frame) busy(id string, now time.Time) []bool {
	var busy []bool
	for _, s := range f.listing() {
		on, lit := false, false
		for _, a := range s.Assignments {
			seat := seated{session: s, assignment: a}
			if a.Account == id && f.active(seat, now) {
				on, lit = true, lit || f.lit(seat, now)
			}
		}
		if on {
			busy = append(busy, lit)
		}
	}
	return busy
}

// active reports whether the seat s counts among its account's sessions at
// now, as the router counts them: seen there within activeWithin, as
// lastSeen has it, or put there since, as a move the stream told of puts it.
func (f Frame) active(s seated, now time.Time) bool {
	return now.Sub(latest(f.lastSeen(s), s.assignment.AssignedAt)) <= activeWithin
}

// lit reports whether the seat s is busy at now: a request of its in flight,
// as the request stream tells, or it was seen at work in the last minute.
func (f Frame) lit(s seated, now time.Time) bool {
	if c, ok := f.Traffic.call(s.plug()); ok && c.Doing.InFlight() {
		return true
	}
	return now.Sub(f.lastSeen(s)) < busyWithin
}

// lastSeen is when the seat s was last seen at work: as the router listed
// it, or later, as the request stream told of it.
func (f Frame) lastSeen(s seated) time.Time {
	return latest(s.assignment.LastSeen, f.Traffic.Seen[s.plug()])
}

// latest is the later of two times.
func latest(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// tone is the colour of the account's state, which its featured window's
// digits and chart take.
func (fc face) tone() theme.Token {
	_, tone := marked(fc.state.Condition)
	return tone
}

// marked is how a card marks an account's state: the glyph its words lead
// with, and its colour.
func marked(c status.Condition) (string, theme.Token) {
	switch c {
	case status.Tokenless:
		return "✕", theme.StateDestructive
	case status.Unreadable:
		return "!", theme.StateDestructive
	case status.Unread:
		return "…", theme.TextSubtle
	case status.Limited:
		return "■", theme.StateDestructive
	case status.PartlyLimited:
		return "■", theme.AccentAttention
	case status.Reserved, status.Pressed:
		return "●", theme.AccentAttention
	case status.Idle:
		return "○", theme.TextSubtle
	default:
		return "●", theme.StatePositive
	}
}

// stateLine is what a card says of its account's state: its mark, then what
// holds it back or what it's doing, bold in its colour, but idle's in
// text.tertiary, and nothing read yet's dim; then what that means, muted.
func (fc face) stateLine() line {
	l := fc.stateHead()
	if fc.state.Then != "" {
		l = append(l, span{" · " + fc.state.Then, mutedInk})
	}
	return l
}

// stateHead is what a card says of its account's state before what that
// means, as stateLine says it.
func (fc face) stateHead() line {
	mark, tone := marked(fc.state.Condition)
	says := ink{token: tone, bold: true}
	switch fc.state.Condition {
	case status.Idle:
		says.token = theme.TextTertiary
	case status.Unread:
		says = dimInk
	}
	return line{{mark + " ", ink{token: tone}}, {fc.state.Says, says}}
}

// stateLines are what a card says of its account's state at now, width
// cells wide, on rows lines at most: as stateLine says it, but where it
// doesn't fit, idle, what that means said more briefly, as startsAt says it;
// and what that means wrapping onto the lines under it, between its words,
// the last cut short.
func (fc face) stateLines(now time.Time, width, rows int) []line {
	then := fc.state.Then
	if fc.state.Condition == status.Idle {
		then = fc.startsAt(now, width-fc.stateHead().width()-ansi.StringWidth(stated))
	}
	return wrapped(fc.stateHead(), stated, span{then, mutedInk}, width, rows)
}

// stated sets what a state means apart from what holds the account back.
const stated = " · "

// startsAt says when the lapsed window of the card's account starts again,
// in words width cells wide where they can be: at its prime, where the
// router says when that is, as in "window starts at its prime, Tue 08:00",
// or more briefly, "starts at its prime, Tue 08:00", then with its time as
// When shows it, "starts at its prime, 08:00"; else with its next request.
func (fc face) startsAt(now time.Time, width int) string {
	if fc.primed.IsZero() {
		return fc.state.Then
	}
	forms := []string{fc.state.Then, "starts at its prime, " + status.Dated(now, fc.primed), "starts at its prime, " + status.When(now, fc.primed)}
	for _, form := range forms {
		if ansi.StringWidth(form) <= width {
			return form
		}
	}
	return forms[len(forms)-1]
}

// wrapped lays head, then tail's words after sep, out on lines width cells
// wide, rows lines at most: on one, as far as it fits; else with as many of
// tail's words as fit beside head on its line, and the rest broken onto the
// lines under it, as broken breaks them, the last cut short. Where not even
// tail's first word fits there, tail starts on the line under head, and sep
// is left off.
func wrapped(head line, sep string, tail span, width, rows int) []line {
	whole := head
	if tail.text != "" {
		whole = slices.Concat(head, line{{sep, mutedInk}, tail})
	}
	if whole.width() <= width || rows < 2 || tail.text == "" {
		return []line{whole.fit(width)}
	}
	words := strings.Fields(tail.text)
	beside := func(n int) line {
		return slices.Concat(head, line{{sep, mutedInk}, {strings.Join(words[:n], " "), tail.ink}})
	}
	n := 0
	for n < len(words) && beside(n+1).width() <= width {
		n++
	}
	first := head.fit(width)
	if n > 0 {
		first = beside(n)
	}
	return append([]line{first}, broken(line{{strings.Join(words[n:], " "), tail.ink}}, slices.Repeat([]int{width}, rows-1))...)
}
