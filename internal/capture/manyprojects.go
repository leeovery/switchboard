package capture

import (
	"fmt"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// moreProjects are the projects the world of many projects adds to the
// scene's, each a session of its own this morning on work.
const moreProjects = 29

// moreProjectsLines are the lines of the sessions moreProjects adds, the
// nth in ~/Code/repo-n from 08:n, its turns sampled from seed n over its
// first 10+n minutes.
func moreProjectsLines(now time.Time) []ledger.Line {
	var lines []ledger.Line
	for n := 1; n <= moreProjects; n++ {
		p := sessionPlan{
			id: fmt.Sprintf("%08x-5e1f-4c2d-9a3b-%012x", 0xc0de0000+n, n), dir: fmt.Sprintf("~/Code/repo-%02d", n),
			start: clockAt(now, 0, 8, n, 0), turns: turnsFrom([][2]int{{0, 10 + n}}, n), stays: []stay{{"work", 0}},
		}
		lines = append(lines, p.lines()...)
	}
	return lines
}

// manyProjects is the scene's world with moreProjects more projects this
// week, so Projects' list runs well past the rows it takes, as a busy
// machine's does.
func (a accountWorlds) manyProjects() accountWorld {
	return a.kept("manyProjects", func() accountWorld {
		w := a.scene()
		w.lines = func(now time.Time) []ledger.Line {
			lines := slices.Concat(sessionsLines(now), moreProjectsLines(now))
			slices.SortStableFunc(lines, func(a, b ledger.Line) int { return a.At.Compare(b.At) })
			return lines
		}
		return w
	})
}
