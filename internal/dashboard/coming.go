package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// comingTail is the cells at the end of a line of COMING UP that its
// countdown takes, with the gap before it.
const comingTail = 12

// The lines COMING UP and LATELY take, side by side: leastLines at the least,
// and mostLines at the most.
const (
	leastLines = 3
	mostLines  = 12
)

// afterTime are the blanks between a line's time and its words, in COMING UP
// and LATELY, at every width.
const afterTime = "  "

// projected marks the time of a window running out in COMING UP, as in
// "~Sat 06:00"; and primed is what it says befalls an account primed, the
// shortest of what it says befalls one.
const (
	projected = "~"
	primed    = " is primed"
)

// comingLeast is the fewest cells COMING UP is read in, beside LATELY: a
// line's time at its widest, projected; the blanks after it; an account's
// name at the cells given, its least; what befalls it at its shortest,
// primed; and its countdown, comingTail.
func comingLeast(name int) int {
	return len(projected) + prose.DatedWidth() + len(afterTime) + name + len(primed) + comingTail
}

// linesRows is how many rows COMING UP or LATELY takes with lines to show:
// its heading and the blank row under it, and its lines, leastLines at the
// least, and mostLines at the most.
func linesRows(lines int) int {
	return sectionHead + min(max(lines, leastLines), mostLines)
}

// comingSection is the Overview's COMING UP of doc at now: what happens
// next, soonest first, as many of its things as it has, leastLines of them at
// its least; folded, its first, as it's drawn, but for its countdown.
func (f Frame) comingSection(doc status.Document, now time.Time) section {
	things := f.upcoming(doc, now)
	return section{
		Section: SectionComing, least: linesRows(0), lines: func() int { return len(things) },
		readIn: comingLeast(labelsLeast(widestLabel(f.Moment(doc, now)))),
		draw: func(c *canvas, body bounds) {
			comingLines(c, things, now, body)
		},
		folded: oneForm(func() line {
			if len(things) == 0 {
				return nil
			}
			return things[0].said(now, 0)
		}),
	}
}

// happening is something coming up: when, the account it befalls, named in
// bold text.primary, what befalls it, in words that follow the name, and
// whether it's a window running out, which its whole line warns of.
type happening struct {
	at      time.Time
	name    span
	what    string
	runsOut bool
}

// upcoming are the things coming up after now, as the moment of doc at now
// has them, in their order, soonest first, each as happening says it, its
// account named as the Overview names it, and its windows as windowNamer
// names them.
func (f Frame) upcoming(doc status.Document, now time.Time) []happening {
	m := f.Moment(doc, now)
	return comingTeller{moment: m, names: overviewNames(m), window: f.windowNamer(m)}.happenings(m.ComingUp)
}

// comingTeller tells of the things coming up of a document at a moment, as
// COMING UP says them: its accounts named as names, overviewNames' of the
// moment, have them; its windows as window names them; and the window a
// request starts as the policy the moment was judged by says, unnamed but
// where namesStarted is set, as Runway names it, as in "lab's 5-hour runs
// out at its pace", where the Overview has "lab runs out at its pace".
type comingTeller struct {
	moment       status.Moment
	names        []accountName
	window       func(key string) string
	namesStarted bool
}

// happenings are what COMING UP says comes of each of the things coming up
// given, as happening says it, in their order, but those it doesn't tell of.
func (t comingTeller) happenings(coming []status.Upcoming) []happening {
	var all []happening
	for _, u := range coming {
		if h, ok := t.happening(u); ok {
			all = append(all, h)
		}
	}
	return all
}

// happening is what doc's COMING UP says comes of u: its account back from
// its limit, as in "back from its limit", or from one of some models alone,
// named by its window, as in "back from its Fable wk limit", from its cap,
// or from the refusal of its token, as in "back from its refused token"; a
// window running out, as runningOut says; a window resetting, as in "'s
// 5-hour resets"; or its being primed. It reports false for a kind it
// doesn't tell of, as one from a later router.
func (t comingTeller) happening(u status.Upcoming) (happening, bool) {
	h := happening{at: u.At, name: overviewNamed(t.moment, t.names, u.Account, titleInk)}
	switch u.Kind {
	case status.UpcomingBack:
		h.what = " back from its " + t.holder(u)
	case status.UpcomingRunsOut:
		h.what, h.runsOut = t.runningOut(u), true
	case status.UpcomingReset:
		h.what = "'s " + t.windowOf(u.Window) + " resets"
	case status.UpcomingPrime:
		h.what = primed
	default:
		return happening{}, false
	}
	return h, true
}

// holder names what held back the account u is back from: the refusal of
// its token, as "refused token"; or its cap, or its limit, each named by the
// window that held back some models' requests alone, as in "Fable wk cap".
func (t comingTeller) holder(u status.Upcoming) string {
	what := "limit"
	switch {
	case u.Refused:
		return "refused token"
	case u.Cap:
		what = "cap"
	}
	if u.Window == "" {
		return what
	}
	return t.windowOf(u.Window) + " " + what
}

// runningOut says what befalls an account as its window runs out, as u
// says: it runs out at its pace, or reaches its cap where that holds it
// back; as the window a request starts does, unnamed, as in "work runs out
// at its pace", but where the teller names it, or as any other, named, as in
// "client's week reaches its cap".
func (t comingTeller) runningOut(u status.Upcoming) string {
	what := " runs out at its pace"
	if u.Cap {
		what = " reaches its cap"
	}
	if u.Window == t.moment.Policy().Started && !t.namesStarted {
		return what
	}
	return "'s " + t.windowOf(u.Window) + what
}

// windowOf names the window with the given key as COMING UP has it in a
// sentence: as USAGE names it, a model's own week short, as "5-hour", "week"
// or "Fable wk".
func (t comingTeller) windowOf(key string) string {
	return status.InProse(status.Short(t.window(key)))
}

// limited reports whether a limit holds the account back from every request
// as it stands, as its Hold says.
func limited(a status.AccountNow) bool {
	return a.Held && a.Hold.Every
}

// comingLines draws as many of the things coming up as body has rows for,
// from its top: each its time, in a column as wide as the widest, then its
// account and what befalls it, and at body's right, how long until it.
func comingLines(c *canvas, things []happening, now time.Time, body bounds) {
	things = things[:min(body.rows, len(things))]
	column := 0
	for _, h := range things {
		column = max(column, len(h.when(now)))
	}
	for i, h := range things {
		_, _, until := h.inks()
		c.right(body.x+body.width, body.y+i, line{{prose.Until(now, h.at), until}})
		c.line(body.x, body.y+i, h.said(now, column).fit(body.width-comingTail))
	}
}

// said is what's coming as its line says it, from its time, padded to
// column cells, to the end of its words.
func (h happening) said(now time.Time, column int) line {
	at, words, _ := h.inks()
	return line{{padded(h.when(now), column) + afterTime, at}, h.name, {h.what, words}}
}

// when is when it comes, as its line says it, in now's time zone: as in
// "16:30" today, or "Sun 02:00" on another day; a window running out,
// projected, after a ~, as in "~Sat 06:00".
func (h happening) when(now time.Time) string {
	if h.runsOut {
		return projected + prose.Dated(now, h.at)
	}
	return prose.Dated(now, h.at)
}

// inks are the inks of what's coming's time, its words and its countdown:
// each accent.attention where a window runs out, else text.secondary,
// text.muted and text.subtle.
func (h happening) inks() (at, words, until ink) {
	if h.runsOut {
		return warningInk, warningInk, warningInk
	}
	return secondaryInk, mutedInk, dimInk
}
