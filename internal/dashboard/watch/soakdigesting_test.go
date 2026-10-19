package watch

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// soakDigesting has the router's digesting of the soak's world timed, as
// TestTheRoutersDigestingOverASoaksWorld says.
var soakDigesting = flag.Bool("soak.digesting", false, "time the router's digesting of the soak's world, -soak.days of it: none skips it")

// heapObjects is the runtime's metric of the bytes the heap's objects take,
// live or not yet swept: what the process holds of them at a moment.
const heapObjects = "/memory/classes/heap/objects:bytes"

// TestTheRoutersDigestingOverASoaksWorld times the router's digesting of the
// soak's world, -soak.days of it, every finished day digested as the router
// leaves them, at the soak's start, each pass made at once, unpaced, as
// DigestEnded makes it: the first pass of a ledger opened afresh, as after
// the router restarts, which reads every digest to judge it; a pass that
// finds nothing changed, as every kick finds them but the first after a day
// ends; yesterday's digest built, as after the round that summarises it;
// yesterday's built with the long session's parts before it damaged in
// place, so it rests on none, every line of each day it ran handed the
// day's builder, as a session whose points can't take a day in is read
// whole again; yesterday's built so with every session's; and a backfill of
// every day, as after the release that brings digests in. Each is reported
// with the most the heap took above what it held, collected, as it began,
// and leaves a digest of every day the world's router digested.
func TestTheRoutersDigestingOverASoaksWorld(t *testing.T) {
	if !*soakDigesting {
		t.Skip("the digesting's timing runs only as -soak.digesting asks")
	}
	began := time.Now()
	w := newSoakWorld(t, soakStart, *soakDays, ownersPace)
	t.Logf("%d days at %d requests a day, written and digested in %.1fs", *soakDays, ownersPace.rate, time.Since(began).Seconds())
	dir := ledger.Dir(w.state)
	yesterday := dayfile.DateOf(dayfile.DayStart(soakStart, -1))
	left, parts := digestsIn(t, dir, "digest-*"), digestsIn(t, dir, "parts-*")
	t.Logf("%d digests, %.1f KB of them, and %d parts files, %.1f KB", len(left), float64(sizeOf(t, left))/(1<<10), len(parts), float64(sizeOf(t, parts))/(1<<10))
	var l *ledger.Ledger
	passes := []struct {
		name   string
		before func()
	}{
		{"the first pass after a start", func() { l = w.opened() }},
		{"a pass finding nothing changed", func() {}},
		{"yesterday's digest built", func() { removeDigests(t, dir, "digest-"+yesterday+"*") }},
		{"yesterday's, the long one whole", func() {
			damagePartsOf(t, dir, busySession(busyLong), yesterday)
			removeDigests(t, dir, "digest-"+yesterday+"*")
		}},
		{"yesterday's, every session whole", func() {
			damageParts(t, dir, yesterday)
			removeDigests(t, dir, "digest-"+yesterday+"*")
		}},
		{"a backfill of every day", func() { removeDigests(t, dir, "digest-*"); l = w.opened() }},
	}
	for _, p := range passes {
		p.before()
		took, peak := measured(func() { l.DigestEnded(t.Context(), soakStart) })
		t.Logf("%-32s took %9v, the heap at most %7.1f MB above", p.name, took.Round(100*time.Microsecond), mb(peak))
		if built := digestsIn(t, dir, "digest-*"); len(built) != len(left) {
			t.Errorf("%s left %d digests, want the %d the world's router left", p.name, len(built), len(left))
		}
	}
}

// measured runs do, returning how long it took and the most the heap's
// objects took meanwhile above what they took, collected, as it began, as
// sampled every millisecond.
func measured(do func()) (time.Duration, uint64) {
	runtime.GC()
	base := heapNow()
	done, peak := make(chan struct{}), make(chan uint64)
	go func() {
		most := base
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				peak <- max(most, heapNow()) - base
				return
			case <-tick.C:
				most = max(most, heapNow())
			}
		}
	}()
	began := time.Now()
	do()
	took := time.Since(began)
	close(done)
	return took, <-peak
}

// heapNow is the bytes the heap's objects take now, as heapObjects has it.
func heapNow() uint64 {
	sample := []metrics.Sample{{Name: heapObjects}}
	metrics.Read(sample)
	return sample[0].Value.Uint64()
}

// removeDigests deletes the digests in the ledger's directory dir whose
// names match pattern.
func removeDigests(t *testing.T, dir, pattern string) {
	t.Helper()
	for _, path := range digestsIn(t, dir, pattern) {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}

// damageParts writes over every parts file in the ledger's directory dir but
// the one of the day with the given date with as many zeros, so no part in
// them reads, by its CRC, and no day is due, as the digesting looks at their
// sizes alone.
func damageParts(t *testing.T, dir, but string) {
	t.Helper()
	for _, path := range digestsIn(t, dir, "parts-*") {
		if filepath.Base(path) == "parts-"+but+".gz" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, make([]byte, info.Size()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// damagePartsOf writes zeros over the part of the session with the given id
// in the parts file of every day in the ledger's directory dir but the one
// with the given date, where the day's digest points at one, so it doesn't
// read, by its CRC, and no day is due, the files as long.
func damagePartsOf(t *testing.T, dir, id, but string) {
	t.Helper()
	for _, path := range digestsIn(t, dir, "digest-*") {
		date := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "digest-"), ".json.gz")
		if date == but {
			continue
		}
		var digest struct {
			Sessions map[string]struct {
				Part *struct {
					Offset int64 `json:"offset"`
					Length int64 `json:"length"`
				} `json:"part"`
			} `json:"sessions"`
		}
		if err := json.Unmarshal(gunzippedFile(t, path), &digest); err != nil {
			t.Fatal(err)
		}
		part := digest.Sessions[id].Part
		if part == nil {
			continue
		}
		f, err := os.OpenFile(filepath.Join(dir, "parts-"+date+".gz"), os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteAt(make([]byte, part.Length), part.Offset)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

// gunzippedFile is the file at path, inflated.
func gunzippedFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return plain
}

// sizeOf is the bytes the files at paths take.
func sizeOf(t *testing.T, paths []string) uint64 {
	t.Helper()
	var size uint64
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		size += uint64(info.Size())
	}
	return size
}

// digestsIn returns the paths of the digests in the ledger's directory dir
// whose names match pattern.
func digestsIn(t *testing.T, dir, pattern string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatal(err)
	}
	return paths
}
