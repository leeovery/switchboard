package dashboard

import (
	"cmp"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// noSessionsRunning is what the dashboard says where it lists the sessions
// running and none runs, as while no router lists any: SESSIONS on the
// Overview, and Sessions' Switchboard, with cords or in lanes.
const noSessionsRunning = "no sessions running"

// name is what the dashboard calls an account: its label, which is its id
// unless the config gives another, cleaned.
func name(a status.AccountNow) string {
	return labelled(a.Label, a.ID)
}

// labelled is what the dashboard calls the account with the given label
// and id: its label, else its id, cleaned.
func labelled(label, id string) string {
	return prose.Clean(cmp.Or(label, id))
}

// modelNamed is what the dashboard calls the model with the given id,
// wherever it names one by its id: as ledger's ShortModelID has it, the id
// cleaned, less claude-.
func modelNamed(id string) string {
	return ledger.ShortModelID(id)
}

// named is what the dashboard calls the account of m with the given id: its
// name, or, where m lacks it, its id, cleaned.
func named(m status.Moment, id string) string {
	if a, ok := m.Account(id); ok {
		return name(a)
	}
	return prose.Clean(id)
}

// names names the accounts of m with the given ids as a list, as in "work and
// side".
func names(m status.Moment, ids []string) string {
	listed := make([]string, len(ids))
	for i, id := range ids {
		listed[i] = named(m, id)
	}
	return prose.List(listed)
}

// place is the account's place in m, counting from 1 in the order they're
// configured, as the digit that pins it numbers it: 0 where m lacks it.
func place(m status.Moment, id string) int {
	a, _ := m.Account(id)
	return a.Place
}

// either lists the names as prose lists what's chosen between, each in k
// and the words between them in between: "work", "work or side", "work,
// side or personal".
func either(names []string, k, between ink) line {
	var l line
	for i, n := range names {
		if joiner := prose.Joiner(i, len(names), prose.Or); joiner != "" {
			l = append(l, span{joiner, between})
		}
		l = append(l, span{n, k})
	}
	return l
}

// because follows what with why, cleaned, where there's a why.
func because(what, why string) string {
	if why = prose.Clean(why); why == "" {
		return what
	}
	return what + ": " + why
}

// moveArrow sets a move's two accounts apart, as "work → side".
const moveArrow = " → "

// moveNames names a move's accounts, the one it left and the one it went
// to, in the ink given, the arrow between them in text.muted, in no more
// than cells cells: "personal → side". Where they don't fit, each is cut
// with an ellipsis, as movedCells shares the room, the arrow kept,
// "pers… → side"; and with less room than an ellipsis each, the arrow and
// the account moved to alone, as a phone has a move, "→si…".
func moveNames(from, to string, cells int, k ink) line {
	room := cells - ansi.StringWidth(moveArrow)
	if room < 2 {
		return line{{strings.TrimSpace(moveArrow), mutedInk}, {prose.Cut(to, cells-1), k}}.fit(cells)
	}
	f, t := movedCells(ansi.StringWidth(from), ansi.StringWidth(to), room)
	return line{{prose.Cut(from, f), k}, {moveArrow, mutedInk}, {prose.Cut(to, t), k}}
}

// movedCells shares room cells between a move's two names, from cells and to
// cells wide: each its whole where they fit; else a cell at a time taken
// from the longer, from where they're alike, so the longer is cut to the
// other's width, then the two alike, the odd cell to's.
func movedCells(from, to, room int) (int, int) {
	switch {
	case from+to <= room:
		return from, to
	case from > to && room-to >= to:
		return room - to, to
	case to > from && room-from >= from:
		return from, room - from
	}
	return room / 2, room - room/2
}
