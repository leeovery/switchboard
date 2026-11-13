package capture

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// idE9A7 is the session the world of sizable values adds to the scene's.
const idE9A7 = "e9a7c3d1-6b2f-4e85-a0d9-3f7c1b8e5a26"

// sizablePlan is e9a7 as the world of sizable values has it: in
// ~/Code/reports, today from 07:30 on personal, then on side from 14:20,
// when its compaction moved it, its turns sampled from seed 5.
func sizablePlan(now time.Time) sessionPlan {
	return sessionPlan{
		id: idE9A7, dir: "~/Code/reports", start: clockAt(now, 0, 7, 30, 0),
		turns: turnsFrom([][2]int{{0, 425}}, 5),
		stays: []stay{{"personal", 0}, {"side", 410}},
		moves: []movePlan{{minute: 410, written: 77_000, reason: status.Compacted("side", 0.354)}},
	}
}

// sizableLines are e9a7's lines, as sizablePlan has them, but of sonnet,
// and its first answer giving no usage, so part of its worth is unpriced.
func sizableLines(now time.Time) []ledger.Line {
	lines := sizablePlan(now).lines()
	for i := range lines {
		lines[i].Model = sonnet
	}
	lines[0].Usage = nil
	return lines
}

// sizableValues is the scene's world with e9a7 among its sessions, so the
// List shows values the size real ones come to: a worth past $100, part of
// it unpriced; sonnet-5-5; and a move by a compaction.
func (a accountWorlds) sizableValues() accountWorld {
	return a.kept("sizableValues", func() accountWorld {
		w := a.scene()
		w.lines = func(now time.Time) []ledger.Line {
			lines := slices.Concat(sessionsLines(now), sizableLines(now))
			slices.SortStableFunc(lines, func(a, b ledger.Line) int { return a.At.Compare(b.At) })
			return lines
		}
		return w
	})
}
