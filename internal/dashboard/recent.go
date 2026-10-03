package dashboard

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// passedOver ends the reason the router gives for a choice that passed over
// an account under pressure, as in "new, work under pressure".
const passedOver = " under pressure"

// recent are the events RECENT lists at now, newest first, n at most: those
// of the kinds it tells of, but a move a limit counts while that limit's
// line is among them, as newest has them.
func recent(doc status.Document, now time.Time, n int) []status.Event {
	var events []status.Event
	for _, e := range doc.Events {
		if _, _, ok := told(e, doc, now); ok {
			events = append(events, e)
		}
	}
	return newest(events, n, countedIn, countingOf)
}

// counted reports whether an event is a move a limit forced, which the
// limit's own line counts.
func counted(e status.Event) bool {
	return e.Kind == status.EventMoved && e.Limit != 0
}

// countedIn is the id of the limit's event that counts the move e tells of,
// as counted has it: zero for any other event, a limit's own, whose Limit is
// its identity, among them.
func countedIn(e status.Event) int {
	if counted(e) {
		return e.Limit
	}
	return 0
}

// countingOf is the id of the limit's event e, whose line counts the moves
// it forced: zero for any other event.
func countingOf(e status.Event) int {
	if e.Kind == status.EventLimit {
		return e.ID
	}
	return 0
}

// quiet is what RECENT says where it lists no events: that the router isn't
// running, or answering as it should, while probing for want of it; that
// probing as asked reads none; that the router, from before it told of
// events, wants restarting; or that it has told of none lately. It's nil
// while nothing has been read.
func (f Frame) quiet(doc status.Document) line {
	switch {
	case doc.Source == status.SourceRouter && f.Outdated:
		return line{{"restart the router for recent events", dimInk}}
	case doc.Source == status.SourceRouter:
		return line{{"nothing lately", dimInk}}
	case doc.Source != status.SourceProbe:
		return nil
	case doc.Fallback.Router == status.RouterNotRunning:
		return line{{"the router isn't running", dimInk}}
	case doc.Fallback.Router == status.RouterUnhealthy:
		return line{{"the router isn't answering", dimInk}}
	default:
		return line{{"none while probing, as asked", dimInk}}
	}
}

// told is what RECENT says of an event: its mark, a glyph in the ink of what
// it tells of, and its words, its times in now's time zone, accounts by their
// names; of what befell an account, its name, then what aside says of it;
// reporting false for a kind it doesn't tell of, as one from a later router.
func told(e status.Event, doc status.Document, now time.Time) (span, line, bool) {
	e.At = e.At.In(now.Location())
	switch e.Kind {
	case status.EventStarted:
		return span{"▲", accentInk}, slices.Concat(line{{sessionID(e.Session), titleInk}, {" started on ", mutedInk}, {named(doc, e.Account), strongInk}}, why(doc, e.Reason)), true
	case status.EventMoved:
		return movedMark, moveSaid(e, doc, why(doc, e.Reason)), true
	case status.EventRestart:
		return span{"!", warningInk}, line{{"restart due (" + status.Clean(e.Reason) + ")", mutedInk}}, true
	case status.EventHealth:
		if e.Reason != "" {
			return span{"●", errorInk}, line{{because("the router turned unhealthy", e.Reason), mutedInk}}, true
		}
		return span{"●", positiveInk}, line{{"the router is healthy again", mutedInk}}, true
	}
	if mark, gist, tail, ok := aside(e, doc); ok {
		return mark, slices.Concat(line{{named(doc, e.Account), strongInk}, {" " + gist, mutedInk}}, tail), true
	}
	return span{}, nil, false
}

// aside is what's told of an event that befell an account, from the
// account's side, which RECENT tells after its name and the account's card's
// LATELY as it is: its mark, a glyph in the ink of what it tells of; its
// gist, as in "reached its session limit"; and what follows it, as in "; 3
// sessions moved to side", in the ink of their own. It reports false for an
// event of any other kind.
func aside(e status.Event, doc status.Document) (mark span, gist string, tail line, ok bool) {
	switch e.Kind {
	case status.EventPressure:
		return span{"●", warningInk}, "came under pressure", runsOut(e, doc), true
	case status.EventLimit:
		return span{"■", errorInk}, "reached its " + limits(e, doc), moves(e, doc), true
	case status.EventRefused:
		mark, words, until := refusal(e)
		return mark, words, line{{until, mutedInk}}, true
	case status.EventPrimed:
		return span{"◇", primedInk}, "primed: its " + spanOf(first(e.Windows)) + " started", line{{resetting(e), mutedInk}}, true
	case status.EventRoom:
		return span{"●", positiveInk}, "has room again", nil, true
	}
	return span{}, "", nil, false
}

// movedMark marks a session's move, as RECENT and LOG tell of it.
var movedMark = span{"▸", dimInk}

// moveSaid says a session's model moved, from where to where, as in "d28c
// moved work → side", then why, as said gives it.
func moveSaid(e status.Event, doc status.Document, said line) line {
	return slices.Concat(line{{sessionID(e.Session), titleInk}, {" moved ", mutedInk}, {named(doc, e.From), strongInk}, {" → ", mutedInk}, {named(doc, e.To), strongInk}}, said)
}

// why says why a session went where it did, as the router's reason gives it,
// accounts by their names in doc: ", the best" where it was chosen afresh,
// " (pin)" where a pin sent it, else the reason itself, as namedIn has it;
// and the account under pressure the choice passed over, where it passed
// one.
func why(doc status.Document, reason string) line {
	said, passed := passedOverIn(reason)
	var l line
	switch said {
	case "":
	case status.ReasonNew:
		l = line{{", the best", mutedInk}}
	case status.ReasonPinned, status.ReasonGlobalPin, status.ReasonMovedByPin:
		l = line{{" (pin)", mutedInk}}
	default:
		l = line{{": " + namedIn(doc, said), mutedInk}}
	}
	return slices.Concat(l, passing(doc, passed))
}

// namedIn is a reason the router gives, the account it leads with named as
// doc names it: one moved off, as in "work has no room", without the reason's
// "moved: "; or one a session's own pin yielded at, as in "pin yields: work
// has no room". Any other reason is as it is.
func namedIn(doc status.Document, said string) string {
	if rest, ok := strings.CutPrefix(said, status.ReasonMovedOff); ok {
		return namedFirst(doc, rest)
	}
	if rest, ok := strings.CutPrefix(said, status.ReasonPinYields); ok {
		return status.ReasonPinYields + namedFirst(doc, rest)
	}
	return said
}

// namedFirst is words led by an account's id, the id named as doc names it.
func namedFirst(doc status.Document, words string) string {
	id, _, _ := strings.Cut(words, " ")
	return named(doc, id) + words[len(id):]
}

// passedOverIn parts the reason the router gives for where a session went,
// cleaned, into what it says of the choice, and the account under pressure
// the choice passed over, "" where it passed none.
func passedOverIn(reason string) (said, passed string) {
	reason = status.Clean(reason)
	if i := strings.LastIndex(reason, ", "); i >= 0 && strings.HasSuffix(reason, passedOver) {
		return reason[:i], reason[i+2:]
	}
	return reason, ""
}

// passing says the choice passed over the account under pressure with the
// given id, named as doc names it, as in ", passing over work under
// pressure": nothing where it passed none.
func passing(doc status.Document, passed string) line {
	if passed == "" {
		return nil
	}
	name, pressed, _ := strings.Cut(passed, " ")
	return line{{", passing over " + named(doc, name) + " " + pressed, mutedInk}}
}

// runsOut says when, at the rate an account came under pressure at, its
// window runs out, and the span that rate is measured over, as in ": its
// session runs out ~16:05 at its last-30-min rate"; or reaches its reserve,
// where its reserve held it back as it came under pressure, as reserveHeld
// says. It's nothing where the event doesn't say when.
func runsOut(e status.Event, doc status.Document) line {
	if e.Until.IsZero() {
		return nil
	}
	rate := "at its pace"
	if !e.Since.IsZero() {
		rate = "at its " + lately(e.Since, e.At) + " rate"
	}
	out := " runs out ~"
	if reserveHeld(e, doc) {
		out = " reaches its reserve ~"
	}
	window := windowName(doc, e.Account, first(e.Windows))
	return line{{": its " + window + out + status.When(e.At, e.Until) + " " + rate, mutedInk}}
}

// reserveHeld reports whether doc's account the event e befell had its
// reserve hold it back when e came, as the router judged its pressure: it
// keeps one, and no pin spent it then, as the global pin naming it now does
// only where it was set before e.
func reserveHeld(e status.Event, doc status.Document) bool {
	a, ok := doc.Account(e.Account)
	if !ok || a.Reserve <= 0 {
		return false
	}
	return !doc.Pin.Has(a.ID) || doc.Pin.Since.After(e.At)
}

// limits names the limits an event says were reached, as in "session limit"
// or "session and week limits", or "limit" where it names none.
func limits(e status.Event, doc status.Document) string {
	windows := make([]string, len(e.Windows))
	for i, key := range e.Windows {
		windows[i] = windowName(doc, e.Account, key)
	}
	switch len(windows) {
	case 0:
		return "limit"
	case 1:
		return windows[0] + " limit"
	default:
		return prose.List(windows) + " limits"
	}
}

// moves says how many sessions a limit moved, and where, where they all went
// to one account, as in "; 3 sessions moved to side"; nothing where it moved
// none.
func moves(e status.Event, doc status.Document) line {
	if e.Count == 0 {
		return nil
	}
	l := line{{"; ", mutedInk}, {status.SessionCount(e.Count), secondaryInk}, {" moved", mutedInk}}
	if e.To != "" {
		l = append(l, span{" to ", mutedInk}, span{named(doc, e.To), strongInk})
	}
	return l
}

// refusal is what's said of the upstream refusing requests on an account,
// after its name: its mark, a refusal of its token, which holds back every
// request, in state.destructive, or of a model family's requests alone,
// which hold back less, in accent.attention; what was refused, as in "was
// refused (403, opus)"; and until when, as in " until 21:40", where the event
// says.
func refusal(e status.Event) (mark span, words, until string) {
	mark, answer := span{"■", errorInk}, strconv.Itoa(e.Status)
	if e.Family != "" {
		mark, answer = span{"■", warningInk}, answer+", "+status.Clean(e.Family)
	}
	if !e.Until.IsZero() {
		until = " until " + status.When(e.At, e.Until)
	}
	words = "was refused (" + answer + ")"
	return mark, words, until
}

// resetting says when the window a prime started resets, as in ",
// resetting 18:50", where the event says.
func resetting(e status.Event) string {
	if e.Until.IsZero() {
		return ""
	}
	return ", resetting " + status.When(e.At, e.Until)
}

// windowName names an account's window in a sentence, as prosed has its
// label, or by its key, cleaned, where doc lacks it.
func windowName(doc status.Document, account, key string) string {
	if a, ok := doc.Account(account); ok {
		if w, ok := a.Window(key); ok {
			return status.InProse(w.Label)
		}
	}
	return status.Clean(key)
}

// first is the first of keys, or "" for none.
func first(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

// telling is an event as RECENT tells of it: its id, what leads its line,
// its time and its mark, and its words.
type telling struct {
	id    int
	lead  line
	words line
}

// tellings are the events RECENT tells of at now, newest first, n at most,
// as tellingsOf tells of them.
func (f Frame) tellings(doc status.Document, now time.Time, n int) []telling {
	return f.tellingsOf(recent(doc, now, n), doc, now, told)
}

// tellingsOf are the events given, as tell tells of each at now: each led by
// its time, as Dated shows it, the first's standing out, in a column as wide
// as the widest, timeGap after it, then its mark.
func (f Frame) tellingsOf(events []status.Event, doc status.Document, now time.Time, tell func(status.Event, status.Document, time.Time) (span, line, bool)) []telling {
	column := 0
	for _, e := range events {
		column = max(column, len(status.Dated(now, e.At)))
	}
	tellings := make([]telling, len(events))
	for i, e := range events {
		mark, words, _ := tell(e, doc, now)
		at := span{fmt.Sprintf("%-*s", column, status.Dated(now, e.At)) + f.timeGap(), dimInk}
		if i == 0 {
			at.ink = secondaryInk
		}
		tellings[i] = telling{id: e.ID, lead: line{at, mark, spaces(1)}, words: words}
	}
	return tellings
}

// timeGap is the blanks after a column of times, as RECENT and COMING UP
// have them: two, but on a phone, one.
func (f Frame) timeGap() string {
	if f.phone() {
		return " "
	}
	return "  "
}

// recentStrip draws RECENT's lines from row y, from x until the column end,
// n of them at most, and returns how many it drew: its events, newest first,
// each with its time, its mark and its words, picked out while it's fresh;
// or why there are none.
func (f Frame) recentStrip(c *canvas, doc status.Document, now time.Time, x, y, end, n int) int {
	tellings := f.tellings(doc, now, n)
	if len(tellings) == 0 {
		return f.quietly(c, doc, x, y, end)
	}
	for i, t := range tellings {
		c.line(x, y+i, slices.Concat(t.lead, t.words).fit(end-x))
		f.freshen(c, t.id, x, y+i, end)
	}
	return len(tellings)
}

// recentColumn draws RECENT's lines in a column, from row y until row last,
// from x to the frame's edge: as many of its events as fit, recentLines at
// most, newest first, each on a line with its time, its mark and its
// subject, the account or session it befell, and under it, from its
// subject's column, the rest of its words, which open with a blank; or why
// there are none.
func (f Frame) recentColumn(c *canvas, doc status.Document, now time.Time, x, y, last int) {
	end := f.edge()
	tellings := f.tellings(doc, now, recentLines)
	if len(tellings) == 0 && y < last {
		f.quietly(c, doc, x, y, end)
	}
	for _, t := range tellings {
		subject, rest := t.words[:min(1, len(t.words))], t.words[min(1, len(t.words)):]
		rows := 1
		if len(rest) > 0 {
			rows = 2
		}
		if y+rows > last {
			return
		}
		c.line(x, y, slices.Concat(t.lead, subject).fit(end-x))
		if rows == 2 {
			under := x + t.lead.width()
			c.line(under, y+1, rest.fit(end-under))
		}
		for i := range rows {
			f.freshen(c, t.id, x, y+i, end)
		}
		y += rows
	}
}

// quietly draws why RECENT lists no events, from x along row y until the
// column end, where it says, and returns the lines it drew.
func (f Frame) quietly(c *canvas, doc status.Document, x, y, end int) int {
	quiet := f.quiet(doc)
	if quiet == nil {
		return 0
	}
	c.line(x, y, quiet.fit(end-x))
	return 1
}

// freshen picks out the row of the event with the given id, from x along
// row y until the column end, while it's fresh: its highlight fading back.
func (f Frame) freshen(c *canvas, id, x, y, end int) {
	if fade, ok := f.Fresh[id]; ok {
		c.surface(x, y, end-x, hue{token: theme.BgAttention, fade: fade})
	}
}

// recentCount is how many lines RECENT takes at now, n at most.
func (f Frame) recentCount(doc status.Document, now time.Time, n int) int {
	if events := recent(doc, now, n); len(events) > 0 {
		return len(events)
	}
	if f.quiet(doc) != nil {
		return 1
	}
	return 0
}
