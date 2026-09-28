package logs

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestRollsOverBeforeAWriteWouldPassTheCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "router.log")
	log, err := openLog(path, 100, 3)
	if err != nil {
		t.Fatal(err)
	}
	// Each record is 30 bytes, so a file holds three: a fourth would take it
	// to 120.
	for i := range 20 {
		if _, err := fmt.Fprintf(log, "record %02d %s\n", i, strings.Repeat("x", 19)); err != nil {
			t.Fatal(err)
		}
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}

	want := map[string][]int{
		"router.log":   {18, 19},
		"router.log.1": {15, 16, 17},
		"router.log.2": {12, 13, 14},
		"router.log.3": {9, 10, 11},
	}
	if got := fileNames(t, dir); !slices.Equal(got, slices.Sorted(maps.Keys(want))) {
		t.Fatalf("files = %v, want the log and three rolled over", got)
	}
	for name, records := range want {
		file := filepath.Join(dir, name)
		if got := recordNumbers(t, file); !slices.Equal(got, records) {
			t.Errorf("%s holds records %v, want %v", name, got, records)
		}
		info, err := os.Stat(file)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > 100 {
			t.Errorf("%s is %d bytes, over the cap of 100", name, info.Size())
		}
		if mode := info.Mode().Perm(); mode != fileMode {
			t.Errorf("%s has mode %v, want %v", name, mode, fs.FileMode(fileMode))
		}
	}
}

func TestWritesARecordLargerThanTheCapWhole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "router.log")
	log, err := openLog(path, 10, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []string{"a record longer than the cap\n", "and another\n"} {
		if _, err := log.Write([]byte(record)); err != nil {
			t.Fatal(err)
		}
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string]string{"router.log": "and another\n", "router.log.1": "a record longer than the cap\n"} {
		if got := readFile(t, filepath.Join(dir, name)); got != want {
			t.Errorf("%s holds %q, want %q", name, got, want)
		}
	}
}

func TestRollsOverAFullLogWhenOpened(t *testing.T) {
	dir := t.TempDir()
	path := RoleCLI.Path(dir)
	full, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	if err := full.Truncate(maxSize); err != nil {
		t.Fatal(err)
	}
	if err := full.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rotated(path, 1), []byte("older\n"), fileMode); err != nil {
		t.Fatal(err)
	}

	Init(Options{Dir: dir, Role: RoleCLI})
	For("test").Info("after")
	Close(0)

	if info, err := os.Stat(rotated(path, 1)); err != nil || info.Size() != maxSize {
		t.Errorf("cli.log.1 = %v, %v; want the full log, %d bytes", info, err, maxSize)
	}
	if got := readFile(t, rotated(path, 2)); got != "older\n" {
		t.Errorf("cli.log.2 holds %q, want what was cli.log.1", got)
	}
	if got := readFile(t, path); !strings.Contains(got, `msg=after`) || strings.Count(got, "\n") != 1 {
		t.Errorf("cli.log holds %q, want only the record logged after opening it", got)
	}
}

func TestAppendsToALogWithRoom(t *testing.T) {
	dir := t.TempDir()
	path := RoleCLI.Path(dir)
	if err := os.WriteFile(path, []byte("earlier\n"), fileMode); err != nil {
		t.Fatal(err)
	}

	Init(Options{Dir: dir, Role: RoleCLI})
	For("test").Info("after")
	Close(0)

	if got := readFile(t, path); !strings.HasPrefix(got, "earlier\n") || !strings.Contains(got, "msg=after") {
		t.Errorf("cli.log holds %q, want the record appended", got)
	}
	if _, err := os.Stat(rotated(path, 1)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("cli.log.1: %v, want no such file", err)
	}
}

func TestMovesOnWhenAnotherProcessRollsTheLogOver(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cli.log")
	first := mustOpenLog(t, path, 30)
	second := mustOpenLog(t, path, 30)

	write(t, first, "first 1", "first 2", "first 3")
	write(t, second, "second 1") // It would take the log past 30 bytes, so rolls it over.
	write(t, first, "first 4")   // Its file is now cli.log.1: it moves on.

	for name, want := range map[string]string{
		"cli.log":   "second 1\nfirst 4\n",
		"cli.log.1": "first 1\nfirst 2\nfirst 3\n",
	} {
		if got := readFile(t, filepath.Join(dir, name)); got != want {
			t.Errorf("%s holds %q, want %q", name, got, want)
		}
	}
}

func TestRollsOverOnceWhenProcessesFindTheLogFullAtOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cli.log")
	first := mustOpenLog(t, path, 30)
	second := mustOpenLog(t, path, 30)
	write(t, first, "first 1", "first 2", "first 3")
	full, err := first.file.Stat()
	if err != nil {
		t.Fatal(err)
	}

	first.roll(full)
	write(t, first, "first 4")
	second.roll(full) // It found the same file full, but first has rolled it over since.
	write(t, second, "second 1")

	for name, want := range map[string]string{
		"cli.log":   "first 4\nsecond 1\n",
		"cli.log.1": "first 1\nfirst 2\nfirst 3\n",
	} {
		if got := readFile(t, filepath.Join(dir, name)); got != want {
			t.Errorf("%s holds %q, want %q", name, got, want)
		}
	}
	if _, err := os.Stat(rotated(path, 2)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("cli.log.2: %v, want no such file: the log rolled over once", err)
	}
}

func TestRecreatesALogThatsDeleted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.log")
	log := mustOpenLog(t, path, 100)
	write(t, log, "before")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	write(t, log, "after")

	if got := readFile(t, path); got != "after\n" {
		t.Errorf("router.log holds %q, want the record written after it was deleted", got)
	}
}

func TestProcessesSharingALogLoseNoRecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cli.log")
	const writers, records = 4, 100
	var wg sync.WaitGroup
	for w := range writers {
		// Enough rolled-over files that none is dropped.
		log, err := openLog(path, 500, 100)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = log.Close() })
		wg.Go(func() {
			for r := range records {
				if _, err := fmt.Fprintf(log, "writer %d record %03d\n", w, r); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()

	line := regexp.MustCompile(`^writer \d record \d{3}\n$`)
	seen := make(map[string]int)
	for _, name := range fileNames(t, dir) {
		for l := range strings.Lines(readFile(t, filepath.Join(dir, name))) {
			if !line.MatchString(l) {
				t.Errorf("%s holds %q, want whole records", name, l)
			}
			seen[l]++
		}
	}
	for w := range writers {
		for r := range records {
			if l := fmt.Sprintf("writer %d record %03d\n", w, r); seen[l] != 1 {
				t.Errorf("%q logged %d times, want once", l, seen[l])
			}
		}
	}
}

func TestWritesAfterCloseFail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli.log")
	log := mustOpenLog(t, path, 100)
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	if _, err := log.Write([]byte("late\n")); !errors.Is(err, os.ErrClosed) {
		t.Errorf("Write() after Close() error = %v, want %v", err, os.ErrClosed)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a write after Close() made %s: %v", path, err)
	}
}

func TestInitLeavesTheLogToTheLevel(t *testing.T) {
	dir := t.TempDir()
	Init(Options{Dir: dir, Level: slog.LevelWarn})
	For("test").Info("below the level")
	Close(0)

	if got := readFile(t, RoleCLI.Path(dir)); got != "" {
		t.Errorf("cli.log holds %q, want nothing", got)
	}
}

func mustOpenLog(t *testing.T, path string, maxSize int64) *logFile {
	t.Helper()
	log, err := openLog(path, maxSize, 3)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	return log
}

// write writes each record to log as a line.
func write(t *testing.T, log *logFile, records ...string) {
	t.Helper()
	for _, record := range records {
		if _, err := log.Write([]byte(record + "\n")); err != nil {
			t.Fatal(err)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// fileNames lists the files in dir, sorted.
func fileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// recordNumbers reads the numbers of the records in the file at path.
func recordNumbers(t *testing.T, path string) []int {
	t.Helper()
	var numbers []int
	for line := range strings.Lines(readFile(t, path)) {
		var n int
		if _, err := fmt.Sscanf(line, "record %d ", &n); err != nil {
			t.Fatalf("%s holds %q: %v", path, line, err)
		}
		numbers = append(numbers, n)
	}
	return numbers
}
