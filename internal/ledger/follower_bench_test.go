package ledger_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

// BenchmarkReadingAHeavyDayAsItGrows times a read of today, a heavy day of
// 16,000 lines, some 30 MB, with a week of the readings history behind it,
// 5,000 readings a day, some 6 MB, as a Reader reads it whole, and as a
// Follower reads it after its first read: its summary, as Days gives it, and
// its lines, as Today gives them. Before each read, a line and a reading are
// written, as a look finds them every 5 seconds on a heavy day.
func BenchmarkReadingAHeavyDayAsItGrows(b *testing.B) {
	now := time.Date(2026, 10, 6, 23, 30, 0, 0, time.Local)
	state := heavyState(b, now)
	start := startOfDay(now)
	grow := func() {
		holdLines(b, ledger.Dir(state), now.Format(time.DateOnly), asked("written", now))
		holdReadings(b, readings.Dir(state), now.Format(time.DateOnly), workRead(now, "5h", 0.5, now.Add(time.Hour), quota.StatusAllowed))
	}
	b.Run("its summary, read whole", func(b *testing.B) {
		for b.Loop() {
			grow()
			readerAt(state, now).Days(start)
		}
	})
	b.Run("its summary, followed", func(b *testing.B) {
		follower := followerOf(state, &testClock{at: now}, caps)
		follower.Days(start)
		for b.Loop() {
			grow()
			follower.Days(start)
		}
	})
	b.Run("its lines, read whole", func(b *testing.B) {
		for b.Loop() {
			grow()
			for range readerAt(state, now).Lines(start) {
			}
		}
	})
	b.Run("its lines, followed", func(b *testing.B) {
		follower := followerOf(state, &testClock{at: now}, caps)
		var p page
		p.read(follower)
		for b.Loop() {
			grow()
			p.read(follower)
		}
	})
}

// heavyState returns a state directory of the benchmark's own, its ledger
// holding a heavy day, the one now falls on, as heavyLines gives it, and its
// readings history holding the week before it and the day, compressed but for
// the two days before it, as the history keeps them.
func heavyState(b *testing.B, now time.Time) string {
	b.Helper()
	state := b.TempDir()
	dir, history := ledger.Dir(state), readings.Dir(state)
	for _, d := range []string{dir, history} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			b.Fatal(err)
		}
	}
	today := now.Format(time.DateOnly)
	if err := os.WriteFile(filepath.Join(dir, "requests-"+today+".jsonl"), heavyLines(b, today, 16000), 0o600); err != nil {
		b.Fatal(err)
	}
	for back := 8; back >= 0; back-- {
		day := dayfile.DayStart(now, -back)
		name := "readings-" + day.Format(time.DateOnly) + ".jsonl"
		data := heavyReadings(b, day, 5000)
		if back > 2 {
			name, data = name+".gz", gzipped(b, string(data))
		}
		if err := os.WriteFile(filepath.Join(history, name), data, 0o600); err != nil {
			b.Fatal(err)
		}
	}
	return state
}

// heavyReadings returns n of the readings history's lines of the day that
// starts at start, spread through it: two accounts' session, week and Opus
// week, their use rising and resetting as the day goes.
func heavyReadings(b *testing.B, start time.Time, n int) []byte {
	b.Helper()
	accounts, windows := []string{"work", "personal"}, []string{"5h", "7d", "7d_opus"}
	var day bytes.Buffer
	for i := range n {
		at := start.Add(time.Duration(i) * 24 * time.Hour / time.Duration(n))
		r := readings.Reading{At: at.UTC(), Account: accounts[i%2], Key: windows[i/2%3], Utilization: float64(i%500) / 500,
			ResetsAt: at.Truncate(5 * time.Hour).Add(5 * time.Hour).UTC(), Status: quota.StatusAllowed, Source: readings.FromAnswer}
		day.WriteString(readingJSON(b, r) + "\n")
	}
	return day.Bytes()
}
