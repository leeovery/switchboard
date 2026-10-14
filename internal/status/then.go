package status

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
)

// WasKind is how an account stood at a moment gone, as a Was says.
type WasKind int

// The kinds, in the order Then tries them, which is ROUTING's, as an
// account's Routing says how it stands at a moment: the first that holds is
// the one said.
const (
	// WasTokenless is an account without a usable token now, which had no
	// room at any moment, as Runway draws it.
	WasTokenless WasKind = iota + 1
	// WasLimited is an account a limit held back from every request.
	WasLimited
	// WasRefused is an account whose token the upstream refused.
	WasRefused
	// WasUnread is an account nothing was read of yet.
	WasUnread
	// WasCapped is an account at its cap, which held it back.
	WasCapped
	// WasRoom is an account with room.
	WasRoom
)

// Was is how an account stood at a moment gone, as Then has it: its kind,
// and, of a hold, when it began and when it lifts, each zero where what Then
// is given doesn't tell.
type Was struct {
	Kind         WasKind
	Since, Until time.Time
}

// Span is a hold of an account the router told of, which the readings
// history can't show: of Kind WasLimited, a limit naming no window, which
// holds back every request; of Kind WasRefused, the upstream refusing its
// token. It holds from From, zero where nothing tells when it began, as from
// before any moment asked of, up to Until.
type Span struct {
	Kind        WasKind
	From, Until time.Time
}

// holds reports whether the span holds at t.
func (s Span) holds(t time.Time) bool {
	return !t.Before(s.From) && s.Until.After(t)
}

// Told are the holds of the account with the given id that the router told
// of and the readings history can't show, as Then takes them: those its
// events tell, as toldBy has them, newest first; then its limit naming no
// window, and the refusal of its token, where either holds at Now and none
// told of its kind holds then, as where the router no longer keeps its event,
// from before anything.
func (m Moment) Told(id string) []Span {
	told := m.doc.toldBy(id)
	a, _ := m.doc.Account(id)
	if a.Limit.holds(m.Now) && len(a.Limit.Windows) == 0 {
		told = untold(told, Span{Kind: WasLimited, Until: a.Limit.Until}, m.Now)
	}
	if a.Refused.holds(m.Now) && a.Refused.Family == "" {
		told = untold(told, Span{Kind: WasRefused, Until: a.Refused.Until}, m.Now)
	}
	return told
}

// toldBy are the holds of the account with the given id that the router's
// events tell, as the document gives them, newest first: each limit naming
// no window, till its until, or the account's room after it, as roomAfter
// has it, where that comes first, as the router keeps a limit's until as it
// lifts early and tells of the room instead; and each refusal of its token,
// till its until, which the router brings forward as a refusal lifts early.
func (d Document) toldBy(id string) []Span {
	var told []Span
	for _, e := range d.Events {
		switch {
		case e.Account != id:
		case e.Kind == EventLimit && len(e.Windows) == 0:
			told = append(told, Span{Kind: WasLimited, From: e.At, Until: d.roomAfter(id, e.At, e.Until)})
		case e.Kind == EventRefused && e.Family == "":
			told = append(told, Span{Kind: WasRefused, From: e.At, Until: e.Until})
		}
	}
	return told
}

// roomAfter is when the router told of the account with the given id having
// room again first after from and before until, else until.
func (d Document) roomAfter(id string, from, until time.Time) time.Time {
	for _, e := range d.Events {
		if e.Kind == EventRoom && e.Account == id && e.At.After(from) && e.At.Before(until) {
			until = e.At
		}
	}
	return until
}

// untold is the spans told, with s, a hold holding at now, added where none
// of its kind told holds then.
func untold(told []Span, s Span, now time.Time) []Span {
	if slices.ContainsFunc(told, func(held Span) bool { return held.Kind == s.Kind && held.holds(now) }) {
		return told
	}
	return append(told, s)
}

// Then is how the moment's account a stood at t: one rule for every moment
// gone, live or stepped back to, which at Now agrees with how the moment has
// the account stand. It goes by the readings history's readings given,
// oldest first, as the history reads them, of the windows the moment's
// policy says every model shares, each by its last reading by t, a window
// whose reset has passed by then counting as reset, unused; and by the holds
// the router told of that the readings can't show, as Told has them. Its
// kind is the first that holds of: without a usable token now, none at any
// moment; a limit told of, or a window read spent, holding at t; its token
// refused, as told of; nothing read by t; its cap, where its reserve holds
// now, a window read at where its reserve, as configured now, starts, as the
// readings keep no reserve; and room. Of a hold, Since is when the first part
// of what holds it at t began: a told hold's From, or the first reading, of
// those given, of the unbroken run of a window's readings that shows it; and
// Until is when the last part lifts, zero where one lifts at no known time.
// A run begun at either edge of what the readings reach tells no start, as
// Reach says.
func (m Moment) Then(a AccountNow, read []readings.Reading, reach Reach, told []Span, t time.Time) Was {
	if !a.TokenSet {
		return Was{Kind: WasTokenless}
	}
	windows := m.readBy(a, read, reach, t)
	if was, ok := heldAs(WasLimited, slices.Concat(toldAt(told, WasLimited, t), shownAt(windows, WasLimited, t))); ok {
		return was
	}
	if was, ok := heldAs(WasRefused, toldAt(told, WasRefused, t)); ok {
		return was
	}
	if len(windows) == 0 {
		return Was{Kind: WasUnread}
	}
	if was, ok := heldAs(WasCapped, shownAt(windows, WasCapped, t)); ok && a.ReserveHolds {
		return was
	}
	return Was{Kind: WasRoom}
}

// Reach is how far back what the readings Then is given read reaches, each
// edge zero where it reaches back past anything asked of: Kept, when the
// first day the readings history holds starts, those before it standing for
// the days its readings are gone from; and Seen, when the readings the
// dashboard read itself begin, as it probes, or the router doesn't answer,
// which the history holds none of, nor keeps once it's closed. A run whose
// first reading is its window's first from either edge on, its window begun
// before that edge, may have begun before anything read tells, so has no
// start.
type Reach struct {
	Kept, Seen time.Time
}

// edges are the reach's edges, Kept's and Seen's.
func (r Reach) edges() [2]time.Time {
	return [2]time.Time{r.Kept, r.Seen}
}

// windowRead is a window of an account as its readings by a moment have it:
// the last of them; the unbroken run of them ending with it that reads it
// spent, and that reads it at its account's cap; and whether one was read
// from each of the reach's edges on.
type windowRead struct {
	last          quota.Window
	spent, capped run
	reached       [2]bool
}

// run is an unbroken run of a window's readings that shows a hold: whether
// there's one, and when it began, zero where the readings don't tell.
type run struct {
	shown bool
	from  time.Time
}

// readBy are the windows of the account a that the moment's policy says
// every model shares, by key, each as its readings of those given, by t,
// have it, its cap where its reserve, as configured now, starts, the
// readings reaching back as reach says.
func (m Moment) readBy(a AccountNow, read []readings.Reading, reach Reach, t time.Time) map[string]windowRead {
	windows := make(map[string]windowRead)
	for _, r := range read {
		if r.Account != a.ID || !m.policy.IsShared(r.Key) || r.At.After(t) {
			continue
		}
		windows[r.Key] = windows[r.Key].readAgain(r.Window(), r.At, reach, a.Reserve)
	}
	return windows
}

// readAgain is the window w, read again at at as window, the readings
// reaching back as reach says, its account's reserve given: a run of its
// readings runs on to it where the last one's reset hadn't passed by then;
// and one it begins began at at, but where it's the window's first read from
// an edge of the reach on, of a window begun before it, as untold says.
func (w windowRead) readAgain(window quota.Window, at time.Time, reach Reach, reserve float64) windowRead {
	ranOn, begun := !w.last.ResetBy(at), at
	if w.untold(window, at, reach) {
		begun = time.Time{}
	}
	read := windowRead{
		last:   window,
		spent:  w.spent.joined(ranOn, score.Spent(window), begun),
		capped: w.capped.joined(ranOn, score.HeldByReserve(window, reserve), begun),
	}
	for i, edge := range reach.edges() {
		read.reached[i] = w.reached[i] || !edge.IsZero() && !at.Before(edge)
	}
	return read
}

// untold reports whether a run of the window's readings begun by its reading
// at at, as window, began at no time the readings tell: the reading is the
// window's first from an edge of the reach on, and the window began before
// that edge, or when isn't known, so the run may have begun before anything
// read tells.
func (w windowRead) untold(window quota.Window, at time.Time, reach Reach) bool {
	began, _, spanned := window.Span()
	for i, edge := range reach.edges() {
		if !edge.IsZero() && !at.Before(edge) && !w.reached[i] && (!spanned || began.Before(edge)) {
			return true
		}
	}
	return false
}

// joined is the run r, once a reading, which shows its hold, or not, joins
// it, a run begun by the reading beginning at begun, zero where the readings
// don't tell when: where it shows it, r, where it ran on to the reading,
// else a run begun then; and none where it doesn't.
func (r run) joined(ranOn, shows bool, begun time.Time) run {
	switch {
	case !shows:
		return run{}
	case ranOn && r.shown:
		return r
	}
	return run{shown: true, from: begun}
}

// heldAt is the part of a hold of the given kind, WasLimited or WasCapped,
// the window's readings show at t, reporting false where they show none then,
// as where its reset has passed: from when the run of them showing it began,
// till its reset.
func (w windowRead) heldAt(kind WasKind, t time.Time) (part, bool) {
	r := w.spent
	if kind == WasCapped {
		r = w.capped
	}
	if !r.shown || w.last.ResetBy(t) {
		return part{}, false
	}
	return part{from: r.from, until: w.last.ResetsAt}, true
}

// part is a part of what holds an account back at a moment: from when it
// began, zero where that's before anything Then is given tells, until when
// it lifts, zero where that isn't known.
type part struct {
	from, until time.Time
}

// shownAt are the parts of a hold of the given kind, WasLimited or
// WasCapped, the windows' readings show at t, as heldAt has them.
func shownAt(windows map[string]windowRead, kind WasKind, t time.Time) []part {
	var parts []part
	for _, w := range windows {
		if p, ok := w.heldAt(kind, t); ok {
			parts = append(parts, p)
		}
	}
	return parts
}

// toldAt are the parts of a hold of the given kind the router told of that
// hold at t, of the spans given.
func toldAt(told []Span, kind WasKind, t time.Time) []part {
	var parts []part
	for _, s := range told {
		if s.Kind == kind && s.holds(t) {
			parts = append(parts, part{from: s.From, until: s.Until})
		}
	}
	return parts
}

// heldAs is how an account stood, held back as kind says by the parts
// given, reporting false where there are none: since the first of them
// began, as score.Earlier has it, so a part from before anything Then is
// given begins first; until the last of them lifts, zero where one lifts at
// no known time.
func heldAs(kind WasKind, parts []part) (Was, bool) {
	if len(parts) == 0 {
		return Was{}, false
	}
	was := Was{Kind: kind, Since: parts[0].from, Until: parts[0].until}
	for _, p := range parts[1:] {
		was.Since = score.Earlier(was.Since, p.from)
		if was.Until.IsZero() || p.until.IsZero() {
			was.Until = time.Time{}
		} else {
			was.Until = score.Later(was.Until, p.until)
		}
	}
	return was, true
}
