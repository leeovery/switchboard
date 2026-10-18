package ledger_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/readings"
)

// benchDirs returns a ledger's directory and a readings history's of the
// benchmark's own, both made.
func benchDirs(b *testing.B) (dir, history string) {
	b.Helper()
	state := b.TempDir()
	dir, history = ledger.Dir(state), readings.Dir(state)
	for _, d := range []string{dir, history} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			b.Fatal(err)
		}
	}
	return dir, history
}

// digestingAt returns the ledger in dir, by a clock stopped at now, opened to
// digest its days with fold.
func digestingAt(dir, history string, now time.Time, fold *folding) *ledger.Ledger {
	return ledger.Open(dir, 400*24*time.Hour, func() time.Time { return now }, historyIn(history), caps, logs.For("router"), ledger.Digesting(fold))
}

// BenchmarkAPassOverAYearOfDigests times the digesting's passes over 400
// finished days, each of two requests, one of a session of its own and one of
// a session that runs every day, summarised, as the router leaves them: a
// pass that finds nothing changed since the last, as every kick finds them
// but the first after a day ends, which reads no digest; the first pass of a
// ledger opened afresh, as after the router restarts, which reads every
// digest to judge it; and a backfill, every day digested, as after the
// release that brings digests in, its Digester's folds next to nothing.
func BenchmarkAPassOverAYearOfDigests(b *testing.B) {
	const days = 400
	dir, history := benchDirs(b)
	now := time.Date(2026, 10, 7, 13, 12, 0, 0, time.Local)
	for back := range days {
		day := now.AddDate(0, 0, -1-back)
		holdLines(b, dir, day.Format(time.DateOnly), askedBy(fmt.Sprintf("long-%d", back), "long", day),
			askedBy(fmt.Sprintf("own-%d", back), fmt.Sprintf("own-%d", back), day))
	}
	summariseAt(dir, history, now)
	l := digestingAt(dir, history, now, &folding{})
	l.DigestEnded(b.Context(), now)

	b.Run("a pass finding nothing changed", func(b *testing.B) {
		for b.Loop() {
			l.DigestEnded(b.Context(), now)
		}
	})
	b.Run("the first pass of a ledger opened afresh", func(b *testing.B) {
		for b.Loop() {
			digestingAt(dir, history, now, &folding{}).DigestEnded(b.Context(), now)
		}
	})
	b.Run("a backfill", func(b *testing.B) {
		for b.Loop() {
			b.StopTimer()
			removeDigests(b, dir)
			fresh := digestingAt(dir, history, now, &folding{})
			b.StartTimer()
			fresh.DigestEnded(b.Context(), now)
		}
	})
}

// BenchmarkDigestingAHeavyDay times the ledger's part of digesting a heavy
// day, as heavyLines gives it, of 25,000 lines, some 47 MB of them, in its
// plain file, as yesterday's are: its lines counted, its 40 sessions read
// and handed on, and its digest written, its Digester's folds next to
// nothing.
func BenchmarkDigestingAHeavyDay(b *testing.B) {
	dir, history := benchDirs(b)
	now := time.Date(2026, 10, 7, 13, 12, 0, 0, time.Local)
	if err := os.WriteFile(filepath.Join(dir, "requests-2026-10-06.jsonl"), heavyLines(b, "2026-10-06", 25000), 0o600); err != nil {
		b.Fatal(err)
	}
	summariseAt(dir, history, now)
	l := digestingAt(dir, history, now, &folding{})

	for b.Loop() {
		b.StopTimer()
		removeDigests(b, dir)
		b.StartTimer()
		l.DigestEnded(b.Context(), now)
	}
}

// removeDigests removes every digest the ledger in dir holds.
func removeDigests(b *testing.B, dir string) {
	b.Helper()
	digests, err := filepath.Glob(filepath.Join(dir, "digest-*"))
	if err != nil {
		b.Fatal(err)
	}
	for _, path := range digests {
		if err := os.Remove(path); err != nil {
			b.Fatal(err)
		}
	}
}
