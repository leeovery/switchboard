package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// A call's parts' cells: its id, cut to callID; the fewest of its
// directory's, leastDir; what it's doing, callDoing with the gap after it;
// "▸ from ", fromLong, or "▸ " alone, fromShort; and the fewest of the
// account after it, leastAccount.
const (
	callID       = 4
	leastDir     = 6
	callDoing    = 13
	fromLong     = 7
	fromShort    = 2
	leastAccount = 6
)

// A call's parts across a row, in callParts' order, and how many they are.
const (
	callIDPart = iota
	callDirPart
	callDoingPart
	callFromPart
	callAccountPart
	callPartsCount
)

// callParts are a call's parts across a row, as flex lays them: its id; its
// directory, a cell on, from leastDir to views.SessionDirMost; what it's
// doing, doingGap cells on; and, where from gives the account's natural,
// ▸ and its from, and the account it came from, from cells at its natural,
// the lesser of from and leastAccount at its least. With no from, there's
// no from column. The directory and the account narrow in the last turns, so
// a call's names are the last of the Switchboard's parts to give, and the
// first raised.
//
// No natural comes from the calls shown, so the columns hold still as calls
// come and go and as they move. Only the from column's presence comes from
// the calls: it's laid where any of them has moved, as fromCells has it.
func callParts(doingGap, from int) []flex.Part {
	parts := make([]flex.Part, callPartsCount)
	parts[callIDPart] = flex.Part{Least: callID, Natural: callID}
	parts[callDirPart] = flex.Part{Least: leastDir, Natural: views.SessionDirMost, Narrows: dirToLeast, Gap: 1}
	parts[callDoingPart] = flex.Part{Least: callDoing, Natural: callDoing, Gap: doingGap}
	if from > 0 {
		parts[callFromPart] = flex.Part{Least: fromLong, Natural: fromLong, Leaves: fromDropped, Kept: fromShort}
		parts[callAccountPart] = flex.Part{Least: min(from, leastAccount), Natural: from, Narrows: accountToLeast}
	}
	return parts
}

// callColumns are where a call's row puts its parts, as the Switchboard and
// a lane each place them: its session's id at id, a cell, and its
// directory, cut to dir cells; what it's doing at doing; and at from, ▸,
// " from ", or where fromShort says, a cell alone, and the account it came
// from, cut to account cells.
type callColumns struct {
	id, dir, doing, from, account int
	fromShort                     bool
}

// callColumnsOf are a call's columns as its parts' spans place them, laid
// as callParts declares them, from column at.
func callColumnsOf(s []flex.Span, at int) callColumns {
	return callColumns{
		id:        at + s[callIDPart].At,
		dir:       s[callDirPart].Size,
		doing:     at + s[callDoingPart].At,
		from:      at + s[callFromPart].At,
		account:   s[callAccountPart].Size,
		fromShort: s[callFromPart].Left,
	}
}

// fromCells are the natural of the account a call's from column names, in
// the moment m: the widest of its accounts' names, as widestLabel has it,
// where any call the frame holds has moved; none where none has, so no from
// column is laid.
func (f Frame) fromCells(m status.Moment) int {
	if !slices.ContainsFunc(f.SessionsTab.Calls, func(c views.Call) bool { return c.From != "" }) {
		return 0
	}
	return widestLabel(m)
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
