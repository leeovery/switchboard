package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/readings"
)

// importingConfig is the config of a router of testConfigured's on start's
// clock, listening on a port of its own, its state directory one of the
// test's own, short enough to hold its control socket.
func importingConfig(t *testing.T) Config {
	t.Helper()
	dir, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cfg := testConfig(at(start), &stubProber{}, testTokens.Read)
	cfg.StateDir, cfg.Listen = dir, "127.0.0.1:0"
	return cfg
}

// runImporting runs r until the test ends, returning once its control API
// answers, with a client of it, and what stops it, which returns once Run
// has.
func runImporting(t *testing.T, r *Router) (*Client, func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan error, 1)
	go func() { finished <- r.run(ctx) }()
	stop := sync.OnceValue(func() error {
		cancel()
		return <-finished
	})
	t.Cleanup(func() { _ = stop() })
	client := NewClient(SocketPath(r.cfg.StateDir))
	waitFor(t, "the router answers", func() bool {
		_, err := client.Health(t.Context())
		return err == nil
	})
	return client, stop
}

// stageFile writes data as the file at path, making its directory.
func stageFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// readingOn is a reading of work's session taken at at, as a line of the
// readings history.
func readingOn(at time.Time) string {
	return `{"at":"` + at.UTC().Format(time.RFC3339) + `","account":"work","window":"5h","utilization":0.4,"status":"allowed","source":"answer"}` + "\n"
}

// heldImporting is a router of testConfigured's, run in a state directory of
// the test's own, with a day of readings staged to import, and its readings
// history's writer held at its first round, as by a round that takes a while,
// until release is called: so an import waits on it.
type heldImporting struct {
	r      *Router
	client *Client
	// dir is the router's state directory, and date the date of the day
	// staged.
	dir, date string
	release   func()
	// stop stops the router, and returns once Run has.
	stop func() error
}

// newHeldImporting runs a router as heldImporting says, until the test ends,
// returning once its control API answers, and its writer is held.
func newHeldImporting(t *testing.T) *heldImporting {
	t.Helper()
	cfg := importingConfig(t)
	h := &heldImporting{r: newTestRouterOf(t, cfg), dir: cfg.StateDir, date: dayfile.DateOf(start.AddDate(0, 0, -10))}
	staged := readings.Files(readings.Dir(filepath.Join(ImportDir(h.dir), "1")), logger)
	stageFile(t, staged.CompressedPath(h.date), gzipOf(t, readingOn(start.AddDate(0, 0, -10))))
	busy, released := make(chan struct{}), make(chan struct{})
	h.release = sync.OnceFunc(func() { close(released) })
	// The writer goes free as the test ends, before its router is stopped,
	// which waits on it.
	h.r.history.pruning = func(time.Time) {
		close(busy)
		select {
		case <-released:
		case <-t.Context().Done():
		}
	}
	h.client, h.stop = runImporting(t, h.r)
	<-busy
	return h
}

// importing starts an import, and returns what it fails with once it's
// answered, once the router has it under way.
func (h *heldImporting) importing(t *testing.T) <-chan error {
	t.Helper()
	answered := make(chan error, 1)
	go func() {
		_, err := h.client.Import(t.Context())
		answered <- err
	}()
	waitFor(t, "the import is under way", h.r.imports.running.Load)
	return answered
}

// filesUnder returns each file under the directories given, by its path, as
// it's written and when it was last modified.
func filesUnder(t *testing.T, dirs ...string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
			if err != nil || !e.Type().IsRegular() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			info, err := e.Info()
			if err != nil {
				return err
			}
			files[path] = string(data) + "@" + info.ModTime().String()
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
	}
	return files
}

// waitFor waits a few seconds at most for done to report true.
func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("gave up waiting until %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAnImportAskedForWhileOneRunsIsRefusedChangingNothing(t *testing.T) {
	logstest.Capture(t)
	h := newHeldImporting(t)
	first := h.importing(t)
	dirs := []string{ImportDir(h.dir), filepath.Join(h.dir, "ledger"), filepath.Join(h.dir, "history")}
	before := filesUnder(t, dirs...)

	_, err := h.client.Import(t.Context())
	if d, ok := errors.AsType[declined](err); !ok || d.status != http.StatusConflict || d.reason != "an import is running already" {
		t.Errorf("Import() while one runs: %v, want a 409 saying an import is running already", err)
	}
	if after := filesUnder(t, dirs...); !maps.Equal(after, before) {
		t.Errorf("once the second import was refused, the files were\n%v\nwant them as they were\n%v", after, before)
	}
	h.release()
	if err := <-first; err != nil {
		t.Errorf("the import running: %v, want it done once the writer it waited on was free", err)
	}
	if _, err := os.Stat(ImportDir(h.dir)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("once the import running was done, what was staged: %v, want it removed", err)
	}
}

func TestARouterStoppingMidImportCutsItShortWithoutWaitingOnIt(t *testing.T) {
	logstest.Capture(t)
	h := newHeldImporting(t)
	answered := h.importing(t)

	stopped := make(chan error, 1)
	go func() { stopped <- h.stop() }()
	select {
	case err := <-answered:
		if d, ok := errors.AsType[declined](err); !ok || d.status != http.StatusServiceUnavailable {
			t.Errorf("Import() as the router stopped: %v, want a 503 saying it stopped partway", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the import went unanswered as the router stopped, the writer it waits on held: want it cut short, answering why")
	}
	waitFor(t, "the import's handler returns, the writer still held", func() bool { return !h.r.imports.running.Load() })
	h.release()
	if err := <-stopped; err != nil {
		t.Errorf("Run() = %v, want nil", err)
	}
	if _, err := os.Stat(ImportDir(h.dir)); err != nil {
		t.Errorf("once the import was cut short, what was staged: %v, want it left for the next import to remove", err)
	}
	if _, err := os.Stat(readings.Files(readings.Dir(h.dir), logger).CompressedPath(h.date)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("once the import was cut short, the day staged: %v, want it never taken in, the writer never having taken the work up", err)
	}
}

func TestTheRouterSaysWhetherAnImportIsRunning(t *testing.T) {
	logstest.Capture(t)
	h := newHeldImporting(t)
	running := func(when string, want bool) {
		t.Helper()
		if got, err := h.client.ImportRunning(t.Context()); err != nil || got != want {
			t.Errorf("%s, ImportRunning() = %v, %v, want %v", when, got, err, want)
		}
	}

	running("before an import", false)
	answered := h.importing(t)
	running("as one runs", true)
	h.release()
	if err := <-answered; err != nil {
		t.Fatalf("Import() = %v", err)
	}
	running("once it's done", false)
}

// pastKeeping stages a folder to import into a router whose ledger keeps its
// lines 30 days, and its readings history 400, of a day sixty days before
// start's: its readings, and those of the day before its week, so a round
// summarises the day from them; and its one request's line, past keeping, so
// its summary staged, of 2 lines, is their only record. It returns the
// router's config, and where the router's summary of the day goes.
func pastKeeping(t *testing.T) (cfg Config, summary string) {
	t.Helper()
	cfg = importingConfig(t)
	cfg.Ledger.Keep = 30 * 24 * time.Hour
	day := start.AddDate(0, 0, -60)
	date, weekBefore := dayfile.DateOf(day), dayfile.DateOf(day.AddDate(0, 0, -9))
	// The Mac's ledger holds an older day's summary, so a round summarises
	// the days after from their readings, where the readings history covers
	// them.
	older := dayfile.DateOf(day.AddDate(0, 0, -10))
	stageFile(t, ledger.SummaryFile(ledger.Dir(cfg.StateDir), older), []byte(`{"version":6,"day":"`+older+`","lines":0}`+"\n"))
	folder := filepath.Join(ImportDir(cfg.StateDir), "1")
	stagedReadings := readings.Files(readings.Dir(folder), logger)
	stageFile(t, stagedReadings.CompressedPath(weekBefore), gzipOf(t, readingOn(day.AddDate(0, 0, -9))))
	stageFile(t, stagedReadings.CompressedPath(date), gzipOf(t, readingOn(day)))
	stageFile(t, ledger.Files(ledger.Dir(folder), logger).CompressedPath(date),
		gzipOf(t, `{"at":"`+day.UTC().Format(time.RFC3339)+`","request":"staged","kind":"message","account":"work","status":200}`+"\n"))
	stageFile(t, ledger.SummaryFile(ledger.Dir(folder), date), []byte(`{"version":6,"day":"`+date+`","lines":2,"accounts":[{"account":"work"}]}`+"\n"))
	return cfg, ledger.SummaryFile(ledger.Dir(cfg.StateDir), date)
}

// linesCounted returns how many lines the summary at path counts.
func linesCounted(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var summary ledger.Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("the summary at %s, %s: %v", path, data, err)
	}
	return summary.Lines
}

func TestASummaryARoundWritesOfADayAsItsFolderComesInGivesWayToTheOneStaged(t *testing.T) {
	logstest.Capture(t)
	cfg, summary := pastKeeping(t)
	r := newTestRouterOf(t, cfg)
	var wrote bool
	r.imports.between = func() {
		r.ledger.summariseSome(r.cfg.Now())
		_, err := os.Stat(summary)
		wrote = err == nil
	}
	client, _ := runImporting(t, r)

	imported, err := client.Import(t.Context())
	if err != nil || imported.Summaries != 1 {
		t.Errorf("Import() = %+v, %v, want the day's summary copied in", imported, err)
	}
	if !wrote {
		t.Fatal("the round between the readings and the ledger's days summarised nothing of the day, want one of its readings alone")
	}
	if got := linesCounted(t, summary); got != 2 {
		t.Errorf("once imported, the day's summary counts %d lines, want the one staged, of its 2 requests, in place of the round's of its readings alone", got)
	}
}

func TestAnImportRetriedAfterOneCutShortTakesTheSummaryStagedOverARoundsBetween(t *testing.T) {
	logstest.Capture(t)
	cfg, summary := pastKeeping(t)
	first := newTestRouterOf(t, cfg)
	client, stop := runImporting(t, first)
	// The router stops as the folder's readings are in, cutting the import
	// short before its ledger's days.
	first.imports.between = func() {
		go func() { _ = stop() }()
		<-first.imports.ctx.Done()
	}
	_, err := client.Import(t.Context())
	if d, ok := errors.AsType[declined](err); !ok || d.status != http.StatusServiceUnavailable {
		t.Fatalf("Import() as the router stopped: %v, want a 503 saying it stopped partway", err)
	}
	if err := stop(); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	client, _ = runImporting(t, newTestRouterOf(t, cfg))
	waitFor(t, "the router started again summarises the day", func() bool {
		_, err := os.Stat(summary)
		return err == nil
	})
	if got := linesCounted(t, summary); got != 0 {
		t.Fatalf("before the import is retried, the day's summary counts %d lines, want one of its readings alone", got)
	}
	imported, err := client.Import(t.Context())
	if err != nil || imported.Summaries != 1 {
		t.Errorf("Import() retried = %+v, %v, want the day's summary copied in", imported, err)
	}
	if got := linesCounted(t, summary); got != 2 {
		t.Errorf("once the import was retried, the day's summary counts %d lines, want the one staged, of its 2 requests, in place of the round's of its readings alone", got)
	}
}

func TestAnImportAStoppedWriterFailsIsToldAsTheRouterStopping(t *testing.T) {
	_, err := newImports().run(func(context.Context) (Imported, error) {
		return Imported{}, fmt.Errorf("import the readings history of 2026-09-18: %w", dayfile.ErrStopped)
	})
	if !errors.Is(err, errImportStopped) || importStatus(err) != http.StatusServiceUnavailable {
		t.Errorf("run() of an import a stopped writer failed = %v, answered %d, want it told as the router stopping, a 503", err, importStatus(err))
	}
}

func TestClientImportWaitsForTheAnswerHoweverLongItTakes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ln := servePipes(t, &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(time.Hour)
			_, _ = io.WriteString(w, `{"folders": 1, "days": {"added": 27, "merged": 3, "past_keep": 12}, "lines": 1204, "summaries": 27}`)
		})})

		got, err := ln.client().Import(t.Context())
		want := Imported{Folders: 1, Days: ImportedDays{Added: 27, Merged: 3, PastKeep: 12}, Lines: 1204, Summaries: 27}
		if err != nil || got != want {
			t.Errorf("Import() = %+v, %v, an hour on, want %+v", got, err, want)
		}
	})
}
