package ledger_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/readings"
)

// BenchmarkARoundOverAYearOfHeavyDays times a round of the ledger over 400
// days that have ended, each summarised, as every round but the first after
// a day ends finds them: each of 16,000 lines, some 30 MB of them, 1.4 MB
// compressed, as heavy a day as a busy owner's, and its summary standing, as
// the router leaves it. It writes some 570 MB.
func BenchmarkARoundOverAYearOfHeavyDays(b *testing.B) {
	const days, lines = 400, 16000
	dir := b.TempDir()
	now := time.Date(2026, 10, 7, 13, 12, 0, 0, time.Local)
	day := heavyDay(b, lines)
	for back := range days {
		date := now.AddDate(0, 0, -1-back).Format(time.DateOnly)
		compressed := filepath.Join(dir, "requests-"+date+".jsonl.gz")
		if err := os.WriteFile(compressed, day, 0o600); err != nil {
			b.Fatal(err)
		}
		summary := summaryFile(dir, date)
		if err := os.WriteFile(summary, []byte(noRequests(date, lines)+"\n"), 0o600); err != nil {
			b.Fatal(err)
		}
		info, err := os.Stat(compressed)
		if err != nil {
			b.Fatal(err)
		}
		if err := os.Chtimes(summary, time.Time{}, info.ModTime()); err != nil {
			b.Fatal(err)
		}
	}
	l := ledger.Open(dir, 400*24*time.Hour, func() time.Time { return now }, noReadings, caps, logs.For("router"))

	for b.Loop() {
		l.SummariseEnded(now)
	}
}

// BenchmarkSummarisingAYearOfHeavyDaysAgain times what a release that moves
// the summaries' version on costs, once: the router's first round under it,
// over 400 days that have ended, the ledger's keep and the readings
// history's, each a heavy day, as heavyDay gives it, with a day of the
// history's readings beside it, as heavyReadings gives them, its summary of
// the version before, as that release's router left it, so each is
// summarised again from its lines and its readings; and a reader's read of
// the days before that round, which summarises each again as it reads it, at
// every read, as the router alone writes summaries. It writes some 600 MB.
func BenchmarkSummarisingAYearOfHeavyDaysAgain(b *testing.B) {
	const days, lines, read = 400, 16000, 5000
	now := time.Date(2026, 10, 7, 13, 12, 0, 0, time.Local)
	state := b.TempDir()
	dir, history := ledger.Dir(state), readings.Dir(state)
	for _, d := range []string{dir, history} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			b.Fatal(err)
		}
	}
	day := heavyDay(b, lines)
	for back := range days {
		date := now.AddDate(0, 0, -1-back).Format(time.DateOnly)
		if err := os.WriteFile(filepath.Join(dir, "requests-"+date+".jsonl.gz"), day, 0o600); err != nil {
			b.Fatal(err)
		}
	}
	// The readings of today, of each day, and of the week before the first
	// and the day before that, as its summary reads them.
	for back := range days + 9 {
		start := dayfile.DayStart(now, -back)
		name := filepath.Join(history, "readings-"+start.Format(time.DateOnly)+".jsonl.gz")
		if err := os.WriteFile(name, gzipped(b, string(heavyReadings(b, start, read))), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	round := func() {
		ledger.Open(dir, 400*24*time.Hour, func() time.Time { return now }, historyIn(history), caps, logs.For("router")).SummariseEnded(now)
	}
	round()
	older := olderSummaries(b, dir)

	b.Run("the router's round", func(b *testing.B) {
		for b.Loop() {
			b.StopTimer()
			older()
			b.StartTimer()
			round()
		}
	})
	b.Run("a reader's read before it", func(b *testing.B) {
		older()
		from := now.AddDate(0, 0, -days)
		for b.Loop() {
			readerAt(state, now).Days(from)
		}
	})
}

// olderSummaries returns what writes the summaries the ledger in dir holds
// back as the release before version 6 wrote them, whole and marked: of
// version 5, their models' hours never counted, and stamped a second sooner,
// as that release stamped them.
func olderSummaries(tb testing.TB, dir string) (write func()) {
	tb.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "day-*.json"))
	if err != nil {
		tb.Fatal(err)
	}
	type written struct {
		path  string
		data  []byte
		stamp time.Time
	}
	held := make([]written, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			tb.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			tb.Fatal(err)
		}
		var s ledger.Summary
		if err := json.Unmarshal(data, &s); err != nil {
			tb.Fatal(err)
		}
		s.Version = 5
		for _, a := range s.Accounts {
			for i := range a.Models {
				a.Models[i].Hours = nil
			}
		}
		if data, err = json.Marshal(s); err != nil {
			tb.Fatal(err)
		}
		held = append(held, written{path: path, data: append(data, '\n'), stamp: info.ModTime().Add(-time.Second)})
	}
	return func() {
		for _, w := range held {
			if err := os.WriteFile(w.path, w.data, 0o600); err != nil {
				tb.Fatal(err)
			}
			if err := os.Chtimes(w.path, time.Time{}, w.stamp); err != nil {
				tb.Fatal(err)
			}
		}
	}
}

// heavyLine is the format of a line of a heavy day: its date and time,
// request's id, session, timings, size, answer's id, blocks and usage given,
// then its limits.
const heavyLine = `{"at":"%sT%02d:%02d:%02d.%03dZ","request":"%08x","kind":"message","session":"%s","dir":"~/Code/project",` +
	`"model":"claude-opus-5-5","account":"work","reason":"sticky","status":200,"attempts":1,"first_ms":%d,"total_ms":%d,` +
	`"agent":"claude-cli/2.1.0 (external, sdk-cli)","betas":["claude-code-20250219","context-1m-2025-08-07","interleaved-thinking-2025-05-14",` +
	`"fine-grained-tool-streaming-2025-05-14","context-management-2025-06-27"],"shape":{"bytes":%d,"messages":%d,"system":3,"tools":31,` +
	`"max_tokens":32000,"thinking":{"type":"enabled","budget_tokens":31999},"stream":true,"output_config":{"effort":"high"},` +
	`"context_management":{"edits":["clear_thinking_20251015"]}},"answer":{"id":"req_011C%016x","model":"claude-opus-5-5","stop":"tool_use",` +
	`"blocks":{"thinking":1,"text":1,"tool_use":%d},"tools":["Bash","Read","Edit","Grep","Glob","Write"]},"usage":{"input_tokens":%d,` +
	`"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":%d},` +
	`"output_tokens":%d,"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard"},"limits":{"status":"allowed",` +
	`"fallback-percentage":"0.5","overage-status":"rejected","overage-disabled-reason":"org_level_disabled",%s}}` + "\n"

// heavyDay returns a day of n of the ledger's lines, compressed, as
// heavyLines gives them of 6 October 2026.
func heavyDay(b *testing.B, n int) []byte {
	b.Helper()
	var day bytes.Buffer
	w := gzip.NewWriter(&day)
	if _, err := w.Write(heavyLines(b, "2026-10-06", n)); err != nil {
		b.Fatal(err)
	}
	if err := w.Close(); err != nil {
		b.Fatal(err)
	}
	return day.Bytes()
}

// heavyLines returns n of the ledger's lines of the day with the given date,
// each of a long session's requests, as a heavy day's are: some 1.9 KB
// apiece, of 40 sessions, their ids, timings and counts varied, the windows'
// use rising through the day, and the same every run.
func heavyLines(b *testing.B, date string, n int) []byte {
	b.Helper()
	random := rand.New(rand.NewPCG(1, 2))
	sessions := make([]string, 40)
	for i := range sessions {
		sessions[i] = fmt.Sprintf("%016x%016x", random.Uint64(), random.Uint64())
	}
	var day bytes.Buffer
	for i := range n {
		if _, err := fmt.Fprintf(&day, heavyLine, date, i/680%24, i/11%60, i%60, random.IntN(1000), random.Uint32(), sessions[random.IntN(len(sessions))],
			random.IntN(5000), random.IntN(90000), random.IntN(900000), random.IntN(400), random.Uint64(), random.IntN(4)+1, random.IntN(100),
			random.IntN(20000), random.IntN(400000), random.IntN(20000), random.IntN(8000), limitsOf(i)); err != nil {
			b.Fatal(err)
		}
	}
	return day.Bytes()
}

// limitsOf returns the limits the ith of a heavy day's lines gives, but its
// status: each window's use, rising through the day, reset, status and
// threshold.
func limitsOf(i int) string {
	var limits []string
	for k, window := range []string{"5h", "7d", "7d_oi", "7d_opus", "7d_sonnet"} {
		limits = append(limits, fmt.Sprintf(`"%[1]s-utilization":"0.%02[2]d","%[1]s-reset":"%[3]d","%[1]s-status":"allowed","%[1]s-surpassed-threshold":"0.%[4]d"`,
			window, (i/160+k*7)%100, 1791320400+3600*(i/680), k+5))
	}
	return strings.Join(limits, ",")
}
