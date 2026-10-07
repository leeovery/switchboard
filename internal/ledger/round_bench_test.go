package ledger_test

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
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
		summary := filepath.Join(dir, "day-"+date+".json")
		if err := os.WriteFile(summary, fmt.Appendf(nil, `{"version":1,"day":"%s","lines":%d}`+"\n", date, lines), 0o600); err != nil {
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
	l := ledger.Open(dir, 400*24*time.Hour, func() time.Time { return now }, noReadings, logs.For("router"))

	for b.Loop() {
		l.SummariseEnded(now)
	}
}

// heavyLine is the format of a line of a heavy day: its time, request's id,
// session, timings, size, answer's id, blocks and usage given, then its
// limits.
const heavyLine = `{"at":"2026-10-06T%02d:%02d:%02d.%03dZ","request":"%08x","kind":"message","session":"%s","dir":"~/Code/project",` +
	`"model":"claude-opus-5-5","account":"work","reason":"sticky","status":200,"attempts":1,"first_ms":%d,"total_ms":%d,` +
	`"agent":"claude-cli/2.1.0 (external, sdk-cli)","betas":["claude-code-20250219","context-1m-2025-08-07","interleaved-thinking-2025-05-14",` +
	`"fine-grained-tool-streaming-2025-05-14","context-management-2025-06-27"],"shape":{"bytes":%d,"messages":%d,"system":3,"tools":31,` +
	`"max_tokens":32000,"thinking":{"type":"enabled","budget_tokens":31999},"stream":true,"output_config":{"effort":"high"},` +
	`"context_management":{"edits":["clear_thinking_20251015"]}},"answer":{"id":"req_011C%016x","model":"claude-opus-5-5","stop":"tool_use",` +
	`"blocks":{"thinking":1,"text":1,"tool_use":%d},"tools":["Bash","Read","Edit","Grep","Glob","Write"]},"usage":{"input_tokens":%d,` +
	`"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":%d},` +
	`"output_tokens":%d,"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard"},"limits":{"status":"allowed",` +
	`"fallback-percentage":"0.5","overage-status":"rejected","overage-disabled-reason":"org_level_disabled",%s}}` + "\n"

// heavyDay returns a day of n of the ledger's lines, compressed, each of a
// long session's requests, as a heavy day's are: some 1.9 KB apiece, of 40
// sessions, their ids, timings and counts varied, the windows' use rising
// through the day, and the same every run.
func heavyDay(b *testing.B, n int) []byte {
	b.Helper()
	random := rand.New(rand.NewPCG(1, 2))
	sessions := make([]string, 40)
	for i := range sessions {
		sessions[i] = fmt.Sprintf("%016x%016x", random.Uint64(), random.Uint64())
	}
	var day bytes.Buffer
	w := gzip.NewWriter(&day)
	for i := range n {
		if _, err := fmt.Fprintf(w, heavyLine, i/680%24, i/11%60, i%60, random.IntN(1000), random.Uint32(), sessions[random.IntN(len(sessions))],
			random.IntN(5000), random.IntN(90000), random.IntN(900000), random.IntN(400), random.Uint64(), random.IntN(4)+1, random.IntN(100),
			random.IntN(20000), random.IntN(400000), random.IntN(20000), random.IntN(8000), limitsOf(i)); err != nil {
			b.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		b.Fatal(err)
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
