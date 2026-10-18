package ledger_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// BenchmarkTodaysSessionsAtHalfPastOne times a read of today's sessions at
// 01:30, as the List reads them, over nine days of sixty sessions running
// every day, every day digested, each state some 10 KB: read whole, as the
// views read them, and resting on yesterday's digest.
func BenchmarkTodaysSessionsAtHalfPastOne(b *testing.B) {
	w := newNearWorld(b)
	w.filler = 10 << 10
	for days := -8; days <= 1; days++ {
		at := func(s int) time.Time { return on(days, 9, s) }
		if days == 1 {
			at = func(s int) time.Time { return on(days, 0, s) }
		}
		lines := make([]ledger.Line, 60)
		for s := range lines {
			lines[s] = askedBy(fmt.Sprintf("%d-%d", days, s), fmt.Sprintf("s%d", s), at(s))
		}
		w.file(days, lines...)
	}
	w.by(on(1, 1, 30))
	follower := w.follower()
	b.Run("read whole", func(b *testing.B) {
		for b.Loop() {
			follower.Sessions(nil, eachWhole, nil)
		}
	})
	b.Run("on yesterday's digest", func(b *testing.B) {
		for b.Loop() {
			follower.Sessions(nil, nil, nil)
		}
	})
}
