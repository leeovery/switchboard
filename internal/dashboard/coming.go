package dashboard

import (
	"fmt"
	"slices"
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

// linesRows is how many rows COMING UP or LATELY takes with lines to show:
// its heading and the blank row under it, and its lines, leastLines at the
// least, and mostLines at the most.
func linesRows(lines int) int {
	return sectionHead + min(max(lines, leastLines), mostLines)
}

// comingSection is the Overview's COMING UP of doc at now: what happens
// next, soonest first, as many of its things as it has, leastLines of them at
// its least.
func (f Frame) comingSection(doc status.Document, now time.Time) section {
	things := upcoming(doc, now, f.Policy)
	return section{
		Section: SectionComing, least: linesRows(0), most: linesRows(len(things)),
		draw: func(c *canvas, body bounds) {
			f.comingLines(c, things, now, body.x, body.y, body.rows)
		},
		folded: func() line { return nil },
	}
}

// happening is something coming up: when, the account it befalls, by its
// name, what befalls it, in words that follow the name, and their ink.
type happening struct {
	at   time.Time
	name string
	what string
	ink  ink
}

// upcoming are the things coming up after now, as doc's COMING UP lists
// them, in its order, soonest first, each as happeningOf says it: but a
// window running out, or resetting, that hides, as shownWindows says, as the
// document lists a model's own week's reset where it isn't its week's. A
// limit or cap lifting is listed whatever its window.
func upcoming(doc status.Document, now time.Time, policy score.Policy) []happening {
	shown := shownWindows(doc, now, policy)
	var all []happening
	for _, u := range doc.ComingUp {
		if hidden(u, shown) {
			continue
		}
		if h, ok := happeningOf(doc, u, policy); ok && h.at.After(now) {
			all = append(all, h)
		}
	}
	return all
}

// hidden reports whether u is a window running out, or resetting, that
// isn't among the windows shown.
func hidden(u status.Upcoming, shown []string) bool {
	ofWindow := u.Kind == status.UpcomingRunsOut || u.Kind == status.UpcomingReset
	return ofWindow && !slices.Contains(shown, u.Window)
}

// happeningOf is what doc's COMING UP says comes of u: its account back from
// its limit, as in "back from its limit", or from one of some models alone,
// named by its window, as in "back from its Fable week limit", or from its
// cap; a window running out, as runningOut says; a window resetting, as in
// "'s session resets"; or its being primed. It reports false for a kind it
// doesn't tell of, as one from a later router.
func happeningOf(doc status.Document, u status.Upcoming, policy score.Policy) (happening, bool) {
	h := happening{at: u.At, name: named(doc, u.Account), ink: mutedInk}
	switch u.Kind {
	case status.UpcomingBack:
		h.what = " back from its " + holder(doc, u)
	case status.UpcomingRunsOut:
		h.what, h.ink = runningOut(doc, u, policy), warningInk
	case status.UpcomingReset:
		h.what = "'s " + windowName(doc, u.Account, u.Window) + " resets"
	case status.UpcomingPrime:
		h.what, h.ink = " is primed", primedInk
	default:
		return happening{}, false
	}
	return h, true
}

// holder names what held back the account u is back from: its cap, or its
// limit, each named by the window that held back some models' requests
// alone, as in "Fable week cap".
func holder(doc status.Document, u status.Upcoming) string {
	what := "limit"
	if u.Cap {
		what = "cap"
	}
	if u.Window == "" {
		return what
	}
	return windowName(doc, u.Account, u.Window) + " " + what
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
	return "'s " + windowName(doc, u.Account, u.Window) + what
}

// limited reports whether a limit holds the account back from every request
// at now, as Held says.
func limited(a status.Account, now time.Time, policy score.Policy) bool {
	held, ok := a.Held(now, policy)
	return ok && held.Every
}

// comingLines draws up to n of the things coming up from row y, their words
// from x, each after its time, in a column, timeGap after it, and their
// countdowns ending at the frame's right.
func (f Frame) comingLines(c *canvas, items []happening, now time.Time, x, y, n int) {
	items = items[:min(n, len(items))]
	column := 0
	for _, h := range items {
		column = max(column, len(status.When(now, h.at)))
	}
	for i, h := range items {
		c.right(f.edge(), y+i, line{{status.Until(now, h.at), h.ink}})
		at := fmt.Sprintf("%-*s", column, status.When(now, h.at)) + f.timeGap()
		words := line{{at, secondaryInk}, {h.name, strongInk}, {h.what, h.ink}}
		c.line(x, y+i, words.fit(f.edge()-comingTail-x))
	}
}
