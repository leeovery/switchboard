package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/score"
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
	things := upcoming(doc, now, f.Policy)
	return section{
		Section: SectionComing, least: linesRows(0), most: linesRows(len(things)), more: len(things) > 1,
		draw: func(c *canvas, body bounds) {
			comingLines(c, things, now, body)
		},
		folded: func() line {
			if len(things) == 0 {
				return nil
			}
			return things[0].said(now, 0)
		},
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

// upcoming are the things coming up after now, as doc's COMING UP lists
// them, in its order, soonest first, each as happeningOf says it, its
// account named as the Overview names it.
func upcoming(doc status.Document, now time.Time, policy score.Policy) []happening {
	names := overviewNames(doc)
	var all []happening
	for _, u := range doc.ComingUp {
		if h, ok := happeningOf(doc, names, u, policy); ok && h.at.After(now) {
			all = append(all, h)
		}
	}
	return all
}

// happeningOf is what doc's COMING UP says comes of u: its account back from
// its limit, as in "back from its limit", or from one of some models alone,
// named by its window, as in "back from its Fable wk limit", or from its
// cap; a window running out, as runningOut says; a window resetting, as in
// "'s 5-hour resets"; or its being primed. It reports false for a kind it
// doesn't tell of, as one from a later router. Its account is named as
// names, overviewNames' of doc, have it.
func happeningOf(doc status.Document, names []accountName, u status.Upcoming, policy score.Policy) (happening, bool) {
	h := happening{at: u.At, name: overviewNamed(doc, names, u.Account, titleInk)}
	switch u.Kind {
	case status.UpcomingBack:
		h.what = " back from its " + holder(doc, u)
	case status.UpcomingRunsOut:
		h.what, h.runsOut = runningOut(doc, u, policy), true
	case status.UpcomingReset:
		h.what = "'s " + comingWindow(doc, u.Account, u.Window) + " resets"
	case status.UpcomingPrime:
		h.what = " is primed"
	default:
		return happening{}, false
	}
	return h, true
}

// holder names what held back the account u is back from: its cap, or its
// limit, each named by the window that held back some models' requests
// alone, as in "Fable wk cap".
func holder(doc status.Document, u status.Upcoming) string {
	what := "limit"
	if u.Cap {
		what = "cap"
	}
	if u.Window == "" {
		return what
	}
	return comingWindow(doc, u.Account, u.Window) + " " + what
}

// runningOut says what befalls an account as its window runs out, as u
// says: it runs out at its pace, or reaches its cap where that holds it
// back; as the window a request starts does, unnamed, as in "work runs out
// at its pace", or as any other, named, as in "client's week reaches its
// cap".
func runningOut(doc status.Document, u status.Upcoming, policy score.Policy) string {
	what := " runs out at its pace"
	if u.Cap {
		what = " reaches its cap"
	}
	if u.Window == policy.Started {
		return what
	}
	return "'s " + comingWindow(doc, u.Account, u.Window) + what
}

// comingWindow names an account's window as COMING UP has it in a sentence:
// as usage's block names it, a model's own week short, as "5-hour", "week"
// or "Fable wk".
func comingWindow(doc status.Document, account, key string) string {
	var label string
	if a, ok := doc.Account(account); ok {
		if w, ok := a.Window(key); ok {
			label = w.Label
		}
	}
	return status.InProse(status.Short(nameOfWindow(key, label)))
}

// limited reports whether a limit holds the account back from every request
// at now, as Held says.
func limited(a status.Account, now time.Time, policy score.Policy) bool {
	held, ok := a.Held(now, policy)
	return ok && held.Every
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
		c.right(body.x+body.width, body.y+i, line{{status.Until(now, h.at), until}})
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
		return "~" + status.Dated(now, h.at)
	}
	return status.Dated(now, h.at)
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
