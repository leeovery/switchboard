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
// of the kinds it tells of, but for the moves a limit counts.
func recent(doc status.Document, now time.Time, n int) []status.Event {
	var events []status.Event
	for _, e := range doc.Events {
		if _, _, ok := told(e, doc, now); ok && !counted(e) && len(events) < n {
			events = append(events, e)
		}
	}
	return events
}

// counted reports whether an event is a move a limit forced, which the
// limit's own line counts, so RECENT, and a card's LATELY, leave it out.
func counted(e status.Event) bool {
	return e.Limit != 0
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
// it tells of, and its words, its times in now's time zone; reporting false
// for a kind it doesn't tell of, as one from a later router.
func told(e status.Event, doc status.Document, now time.Time) (span, line, bool) {
	account := span{named(doc, e.Account), strongInk}
	e.At = e.At.In(now.Location())
	switch e.Kind {
	case status.EventStarted:
		return span{"▲", accentInk}, slices.Concat(line{{sessionID(e.Session), titleInk}, {" started on ", mutedInk}, account}, why(e.Reason)), true
	case status.EventPressure:
		return span{"●", warningInk}, slices.Concat(line{account, {" came under pressure", mutedInk}}, runsOut(e, doc)), true
	case status.EventLimit:
		return span{"■", errorInk}, slices.Concat(line{account, {" reached its " + limits(e, doc), mutedInk}}, moves(e, doc)), true
	case status.EventMoved:
		moved := line{{sessionID(e.Session), titleInk}, {" moved ", mutedInk}, {named(doc, e.From), strongInk}, {" → ", mutedInk}, {named(doc, e.To), strongInk}}
		return span{"▸", dimInk}, slices.Concat(moved, why(e.Reason)), true
	case status.EventRefused:
		return refusal(e, account)
	case status.EventPrimed:
		return span{"◇", primedInk}, line{account, {" primed: its " + spanOf(first(e.Windows)) + " started" + resetting(e), mutedInk}}, true
	case status.EventRoom:
		return span{"●", positiveInk}, line{account, {" has room again", mutedInk}}, true
	case status.EventRestart:
		return span{"!", warningInk}, line{{"restart due (" + status.Clean(e.Reason) + ")", mutedInk}}, true
	case status.EventHealth:
		if e.Reason != "" {
			return span{"●", errorInk}, line{{because("the router turned unhealthy", e.Reason), mutedInk}}, true
		}
		return span{"●", positiveInk}, line{{"the router is healthy again", mutedInk}}, true
	}
	return span{}, nil, false
}

// why says why a session went where it did, as the router's reason gives it:
// ", the best" where it was chosen afresh, " (pin)" where a pin sent it, else
// the reason itself; and the account under pressure the choice passed over,
// where it passed one.
func why(reason string) line {
	reason = status.Clean(reason)
	var passed string
	if i := strings.LastIndex(reason, ", "); i >= 0 && strings.HasSuffix(reason, passedOver) {
		reason, passed = reason[:i], reason[i+2:]
	}
	var l line
	switch reason {
	case "":
	case "new":
		l = line{{", the best", mutedInk}}
	case "pinned", "pinned (global)", "moved by pin":
		l = line{{" (pin)", mutedInk}}
	default:
		l = line{{": " + strings.TrimPrefix(reason, "moved: "), mutedInk}}
	}
	if passed != "" {
		l = append(l, span{", passing over " + passed, mutedInk})
	}
	return l
}

// runsOut says when, at the rate an account came under pressure at, its
// window runs out, and the span that rate is measured over, as in ": its
// session runs out ~16:05 at its last-30-min rate"; or reaches its reserve,
// where doc has the account's reserve hold it back. It's nothing where the
// event doesn't say when.
func runsOut(e status.Event, doc status.Document) line {
	if e.Until.IsZero() {
		return nil
	}
	rate := "at its pace"
	if !e.Since.IsZero() {
		rate = "at its " + lately(e.Since, e.At) + " rate"
	}
	out := " runs out ~"
	if a, ok := doc.Account(e.Account); ok && doc.ReserveHolds(a) {
		out = " reaches its reserve ~"
	}
	window := windowName(doc, e.Account, first(e.Windows))
	return line{{": its " + window + out + status.When(e.At, e.Until) + " " + rate, mutedInk}}
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

// refusal is what RECENT says of the upstream refusing requests on an
// account, as told says: its token, which holds back every request, or a
// model family's requests alone, which hold back less, and until when.
func refusal(e status.Event, account span) (span, line, bool) {
	mark, answer := span{"■", errorInk}, strconv.Itoa(e.Status)
	if e.Family != "" {
		mark, answer = span{"■", warningInk}, answer+", "+status.Clean(e.Family)
	}
	words := " was refused (" + answer + ")"
	if !e.Until.IsZero() {
		words += " until " + status.When(e.At, e.Until)
	}
	return mark, line{account, {words, mutedInk}}, true
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

// recentStrip draws RECENT's lines from row y, from x until the column end,
// n of them at most, and returns how many it drew: its events, newest first,
// each with its time, its mark and its words, picked out while it's fresh;
// or why there are none. On a phone, a time is followed by a single blank
// rather than two.
func (f Frame) recentStrip(c *canvas, doc status.Document, now time.Time, x, y, end, n int) int {
	events := recent(doc, now, n)
	if len(events) == 0 {
		if quiet := f.quiet(doc); quiet != nil {
			c.line(x, y, quiet.fit(end-x))
			return 1
		}
		return 0
	}
	gap, column := "  ", 0
	if f.phone() {
		gap = " "
	}
	for _, e := range events {
		column = max(column, len(status.When(now, e.At)))
	}
	for i, e := range events {
		mark, words, _ := told(e, doc, now)
		at := span{fmt.Sprintf("%-*s", column, status.When(now, e.At)) + gap, dimInk}
		if i == 0 {
			at.ink = secondaryInk
		}
		c.line(x, y+i, slices.Concat(line{at, mark, spaces(1)}, words).fit(end-x))
		if fade, ok := f.Fresh[e.ID]; ok {
			c.surface(x, y+i, end-x, hue{token: theme.BgAttention, fade: fade})
		}
	}
	return len(events)
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
