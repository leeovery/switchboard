package watch

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/router"
)

// BenchmarkTheTrafficOfABusyHour times what the traffic does with an hour
// of a busy run's requests, 3,600 of them, kept: taking in an answer
// streaming on, as the stream tells every 250 ms of each; a tick with
// nothing to let go; a tick with one to let go, as one ended each second an
// hour before, which copies every request it keeps; and comparing what the
// Log's Requests are made of, as each frame on the Log does.
func BenchmarkTheTrafficOfABusyHour(b *testing.B) {
	now := past(2 * time.Hour)
	tr := busyHour(3600, now)
	b.Run("an answer streaming on", func(b *testing.B) {
		streamed := []router.StreamEvent{alter(told(router.StreamProgress, "00000e0f", "work", now.Sub(start)), func(e *router.StreamEvent) { e.Chars = 10 })}
		b.ReportAllocs()
		for b.Loop() {
			tr.took(streamed, heldNowhere, now)
		}
	})
	b.Run("a tick", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			tr.tidied(now.Add(pulseFor / 2))
		}
	})
	b.Run("a tick with one due", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			tr.tidied(now.Add(time.Second))
		}
	})
	b.Run("the Log's Requests' basis", func(b *testing.B) {
		basis := requestsBasis{told: streamTold{answering: true, live: tr.live, changes: tr.changes, rowChanges: tr.rowChanges}}
		b.ReportAllocs()
		for b.Loop() {
			basis.same(basis)
		}
	})
}
