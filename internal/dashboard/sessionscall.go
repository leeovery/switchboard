package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// callColumns are where a call's row puts its parts, as the Switchboard and
// a lane each place them: its session's id at id, a cell, and its
// directory, cut to dir cells; what it's doing at doing; and at from, ▸,
// " from ", or where fromShort says, a cell alone, and the account it came
// from, cut to account cells.
type callColumns struct {
	id, dir, doing, from, account int
	fromShort                     bool
}

// drawCall draws call along row y, its parts where k places them, in m, the
// moment it's drawn in, at now: its session's id, cut to 4, bold
// text.primary while it works, else text.muted, and its directory's last
// part, in text.muted; what it's doing, in a session's page's words, as
// callSaidInk inks them, as last known where asKnown says; and where a move
// brought it, as callFrom says it.
func drawCall(c *canvas, y int, call views.Call, k callColumns, m status.Moment, now time.Time) {
	id := mutedInk
	if call.Activity.Works() {
		id = titleInk
	}
	x := c.text(k.id, y, status.ShownID(call.Session), id)
	if dir := status.DirName(call.Dir, k.dir); dir != "" {
		c.text(x, y, " "+dir, mutedInk)
	}
	c.text(k.doing, y, call.Activity.Said(now), asKnown(call.Activity, callSaidInk(call.Activity.Doing)))
	if call.From != "" {
		c.line(k.from, y, callFrom(m, call.From, k))
	}
}

// asKnown is k, the ink what a session's doing is said in, or where it's in
// flight as last known, as Stale has it, text.subtle, not bold, as a stale
// reading's words are: said as it was, never as happening now.
func asKnown(a views.Activity, k ink) ink {
	if a.Stale {
		return dimInk
	}
	return k
}

// callSaidInk is the ink what a session's doing is said in, by what it's
// doing: asking or answering bold accent.key; refused bold
// state.destructive; throttled text.subtle; else text.faint.
func callSaidInk(d views.Doing) ink {
	switch d {
	case views.Asking, views.Answering:
		return keyInk
	case views.Refused:
		return exhaustedInk
	case views.Throttled:
		return dimInk
	}
	return faintInk
}

// callFrom is where a call came from, the account of m with the given id
// it left: ▸ in accent.key; from in text.muted, but where k drops it; and
// the account's name, unfaded though it's out of the pool, cut to the cells
// k gives it, in text.secondary.
func callFrom(m status.Moment, id string, k callColumns) line {
	words := " from "
	if k.fromShort {
		words = " "
	}
	return line{{"▸", keyMarkInk}, {words, mutedInk}, {prose.Cut(named(m, id), k.account), secondaryInk}}
}
