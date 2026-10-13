package capture

import (
	_ "embed"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// madeNotLive are the ledger's last lines as log-not-live-160 draws them,
// then the quota check the ledger wrote last: testdata/README.md says how.
//
//go:embed testdata/notlive.jsonl
var madeNotLive []byte

// notLiveGone is how long before the frames' moment the router of the world
// of log-not-live-160 stopped answering: at 14:40:55.
const notLiveGone = 72 * time.Second

// notLive is the auto world as log-not-live-160 draws it, its router gone
// at 14:40:55, its ledger's lines as notLiveLines has them.
func (a accountWorlds) notLive() accountWorld {
	return a.kept("not live", func() accountWorld {
		w := a.auto()
		w.lines = notLiveLines
		return w
	})
}

// notLiveLines are the ledger's lines as the router gone at 14:40:55 left
// them: the sessions' world's from before 14:36:18 today, as 1,171 of
// today's requests had arrived, and the days before; then the frame's, the
// newest 27 it draws, and Claude Code's quota check ending at 14:40:52, the
// last line the ledger wrote. So today's lines count 1,198 requests, and 35
// checks and counts, as its frame does.
func notLiveLines(now time.Time) []ledger.Line {
	cut := clockAt(now, 0, 14, 36, 18)
	route := newRouting()
	var lines []ledger.Line
	for _, l := range sessionsLines(now) {
		if l.At.Before(cut) {
			route.saw(l)
			lines = append(lines, l)
		}
	}
	return route.made(madeNotLive, now, lines)
}
