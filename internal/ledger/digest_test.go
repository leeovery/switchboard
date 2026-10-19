package ledger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// digestsIn returns what each file in dir whose name holds "digest",
// "parts" or "logday" holds, by its name.
func digestsIn(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	held := make(map[string][]byte)
	for _, e := range entries {
		if strings.Contains(e.Name(), "digest") || strings.Contains(e.Name(), "parts") || strings.Contains(e.Name(), "logday") {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			held[e.Name()] = data
		}
	}
	return held
}

// The ledger's reads, and its rounds, pass over the digests beside its days,
// their parts files, and their heads: those it writes, and a stray one, of a
// day before the first it holds, one named for no day, and one left half
// written. Where it begins, its days and its lines are as they'd be without
// them, nothing warned of; and a round summarises, compresses and prunes its
// days' lines, leaving every digest, parts file, and head, as it was.
func TestTheLedgersReadsAndRoundsPassOverItsDigests(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, history := stateDirs(t)
	// The 8th is done with by the 11th, so compressed, and the 5th past
	// keeping, kept three days once it ends, so pruned once summarised.
	const done = "2026-10-08"
	at := on(6, 10, 0)
	holdLines(t, dir, date, asked("1", on(0, 9, 0)))
	holdLines(t, dir, done, asked("2", on(3, 9, 0)))
	for _, day := range []string{date, done} {
		start, end, _ := dayfile.Day(day)
		if err := ledger.WriteDigest(dir, ledger.Digest{Head: ledger.DigestHead{Body: 1, Day: day, Start: start, End: end, SummaryLines: 1, Lines: 1}}); err != nil {
			t.Fatal(err)
		}
		if err := ledger.WriteParts(dir, day, map[string]json.RawMessage{"one": json.RawMessage(`{}`)}); err != nil {
			t.Fatal(err)
		}
		if err := ledger.WriteDayHead(dir, ledger.DayHead{Body: 1, Day: day, Start: start, End: end, SummaryLines: 1, Lines: 1, Side: json.RawMessage(`[]`)}); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, dir, "digest-2026-09-01.json.gz", []byte("not a digest"))
	writeFile(t, dir, "digest-someday.json.gz", gzipped(t, "{}"))
	writeFile(t, dir, ".digest-"+date+".json.gz.123456", []byte(`{"version":1,"bo`))
	writeFile(t, dir, "parts-2026-09-01.gz", []byte("not a parts file"))
	writeFile(t, dir, "parts-someday.gz", gzipped(t, "{}"))
	writeFile(t, dir, ".parts-"+date+".gz.123456", []byte(`{"tu`))
	writeFile(t, dir, "logday-2026-09-01.json.gz", []byte("not a head"))
	writeFile(t, dir, "logday-someday.json.gz", gzipped(t, "{}"))
	writeFile(t, dir, ".logday-"+date+".json.gz.123456", []byte(`{"version":1,"si`))
	digests := digestsIn(t, dir)

	follower := followerOf(state, &testClock{at: at}, caps)
	if first, err := follower.FirstDay(at); err != nil || !first.Equal(on(0, 0, 0)) {
		t.Errorf("FirstDay() = %v, %v; want the start of the 5th, the first day the ledger holds lines of", first, err)
	}
	if days := readerAt(state, at).Days(on(-60, 0, 0)); len(days) == 0 || days[0].Day != date {
		t.Errorf("Days() are %d, from %+v; want them from the 5th's", len(days), days)
	}
	var requests []string
	for held := range readerAt(state, at).Lines(on(-60, 0, 0)) {
		requests = append(requests, held.Request)
	}
	if want := []string{"1", "2"}; !slices.Equal(requests, want) {
		t.Errorf("Lines() are of requests %q, want %q", requests, want)
	}

	l := ledger.Open(dir, 3*24*time.Hour, func() time.Time { return at }, historyIn(history), caps, logs.For("router"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	l.Run(ctx)

	for name, want := range map[string]bool{
		"requests-" + date + ".jsonl": false, "day-" + date + ".json": true,
		"requests-" + done + ".jsonl": false, "requests-" + done + ".jsonl.gz": true, "day-" + done + ".json": true,
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); errors.Is(err, fs.ErrNotExist) == want {
			t.Errorf("after the round, %s is there %t, want %t", name, err == nil, want)
		}
	}
	after := digestsIn(t, dir)
	for name, was := range digests {
		if got, ok := after[name]; !ok || !bytes.Equal(got, was) {
			t.Errorf("after the round, %s holds %q, there %t; want it as it was", name, got, ok)
		}
	}
	if log.Has("level=WARN") {
		t.Errorf("log reads\n%s\nwant nothing warned of", log)
	}
}
