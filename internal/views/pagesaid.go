package views

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// MovedOnto is the move that brought the session onto the account its page
// names it on: its named model's last move, where that was onto it; false
// where it started there, or that model's last move took it elsewhere. A
// session's models are routed apart, so another's move onto the same account
// tells nothing of how it came there.
func (p SessionPage) MovedOnto() (SessionMove, bool) {
	moves := p.modelMoves()
	if len(moves) == 0 || moves[len(moves)-1].To != p.Account {
		return SessionMove{}, false
	}
	return moves[len(moves)-1], true
}

// RanOn is where the session ran, its named model's account history: the
// account it ran on first, and each of that model's moves after, in turn. Its
// first is its first move's From, where it moved, else the account it's
// named on: "" where it's named on none.
func (p SessionPage) RanOn() (first string, moves []SessionMove) {
	moves = p.modelMoves()
	if len(moves) > 0 {
		return moves[0].From, moves
	}
	return p.Account, nil
}

// modelMoves are the moves of the model the page names the session by, in
// the order they came.
func (p SessionPage) modelMoves() []SessionMove {
	return slices.DeleteFunc(slices.Clone(p.Moves), func(m SessionMove) bool { return m.Model != p.Model })
}

// Where says where the session runs and why, at now, as its page says it:
// on the account it's named on, since the move that brought it there, as
// MovedOnto has it, as "on side since 14:31, moved from work: work reached
// its cap"; else since it started, where it started there or the router
// gives no reason it's there, as "on work since it started"; else why the
// router has it there, as "on work: sticky". Once it has ended, as its
// Activity says, where it ran, as RanOn has it: "ran on work, then side from
// 16:02: work reached its cap". None where it's named on no account.
func (p SessionPage) Where(now time.Time) status.Words {
	if p.Activity.Doing == Ended {
		return p.ranOnWords(now)
	}
	if p.Account == "" {
		return nil
	}
	on := status.Words{status.Plain("on "), status.Named(p.Account)}
	if m, ok := p.MovedOnto(); ok {
		return slices.Concat(on, status.Words{status.Plain(" since " + prose.Past(now, m.At) + ", ")}, movedWords(m, now))
	}
	reason := p.reasonOn()
	words := status.SessionWords(reason, status.Facts{To: p.Account, Now: now})
	if reason == status.ReasonNew || words.String() == "" {
		return append(on, status.Plain(" since it started"))
	}
	return slices.Concat(on, status.Words{status.Plain(": ")}, words)
}

// reasonOn is why the router has the session's named model on the account
// it's named on, as it lists it: "" where it doesn't list that.
func (p SessionPage) reasonOn() string {
	i := slices.IndexFunc(p.Models, func(m SessionModel) bool { return m.Model == p.Model && m.Account == p.Account })
	if i < 0 {
		return ""
	}
	return p.Models[i].Reason
}

// movedWords say why m moved its session, as a session's page says it:
// "moved from work: work reached its cap" for a move off an account that
// couldn't take it, else as status.SessionWords has it, as "rescored after
// 15h idle: personal had the most room".
func movedWords(m SessionMove, now time.Time) status.Words {
	words := status.SessionWords(m.Reason, status.Facts{From: m.From, To: m.To, Now: now})
	if !strings.HasPrefix(m.Reason, status.ReasonMovedOff) {
		return words
	}
	return slices.Concat(status.Words{status.Plain("moved from "), status.Named(m.From), status.Plain(": ")}, words)
}

// ranOnWords say where the session ran, as RanOn has it, at now: on each
// account, in turn, with when its named model's moves came and why, as "ran
// on work, then side from 16:02: work reached its cap". None where it ran on
// no account.
func (p SessionPage) ranOnWords(now time.Time) status.Words {
	first, moves := p.RanOn()
	if first == "" {
		return nil
	}
	w := status.Words{status.Plain("ran on "), status.Named(first)}
	for _, m := range moves {
		w = append(w, status.Plain(", then "), status.Named(m.To), status.Plain(" from "+prose.Past(now, m.At)+": "))
		w = append(w, status.EventWords(m.Reason, status.Facts{From: m.From, To: m.To, Now: now})...)
	}
	return w
}

// toolsSaid is how many of the tools a turn's answers called most its row
// names.
const toolsSaid = 3

// ToolsSaid says the tools the turn's answers called most, three at the
// most, most first, with how many times each, as "Bash ×14, Edit ×9, Read
// ×8".
func (t Turn) ToolsSaid() string {
	said := make([]string, 0, toolsSaid)
	for _, c := range t.Tools[:min(len(t.Tools), toolsSaid)] {
		said = append(said, prose.Clean(c.Tool)+" ×"+prose.Grouped(c.Times))
	}
	return strings.Join(said, ", ")
}

// PointsSaid says the points of the account's windows the session took, the
// shortest window first, each named as status names it, as "≈ 41 points of
// its 5-hour · 6 of its week", leaving out those that round to none: "" where
// all do.
func (a SessionAccount) PointsSaid() string {
	keys := slices.SortedFunc(maps.Keys(a.Points), quota.CompareKeys)
	keys = slices.DeleteFunc(keys, func(key string) bool { return a.Points[key] == 0 })
	said := make([]string, len(keys))
	for i, key := range keys {
		of := " of its " + status.InProse(status.NameOfKey(key))
		if i > 0 {
			said[i] = prose.Grouped(a.Points[key]) + of
			continue
		}
		said[i] = "≈ " + prose.Counted(a.Points[key], "point") + of
	}
	return strings.Join(said, " · ")
}
