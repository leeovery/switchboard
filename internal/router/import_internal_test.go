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
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/readings"
)

// importingConfig is the config of a router of testConfigured's on start's
// clock, listening on a port of its own, its state directory one of the
// test's own, short enough to hold its control socket, its accounts record
// noting the first pool, as one whose history's moves are done, as a router
// imports only once they are, notes it.
func importingConfig(t *testing.T) Config {
	t.Helper()
	dir, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := identity.NewFile(dir).Update(func(r *identity.Record) error {
		r.Pools = []identity.PoolEntry{{ID: "q7mt", Added: start, First: true}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
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

// stageOwn gives the folder staged first in the state directory given the
// export.json of that install's own export, as one exported before
// switchboard made ids, and returns the folder.
func stageOwn(t *testing.T, stateDir string) string {
	t.Helper()
	folder := filepath.Join(ImportDir(stateDir), "1")
	install, err := export.InstallID(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	stageFile(t, filepath.Join(folder, "export.json"), []byte(`{"version":1,"install":"`+install+`","exported_at":"2026-09-20T09:30:00Z"}`+"\n"))
	return folder
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
	staged := readings.Files(readings.Dir(stageOwn(t, h.dir)), logger)
	stageFile(t, staged.CompressedPath(h.date), gzipOf(t, readingOn(start.AddDate(0, 0, -10))))
	busy, released := make(chan struct{}), make(chan struct{})
	h.release = sync.OnceFunc(func() { close(released) })
	// The writer goes free as the test ends, before its router is stopped,
	// which waits on it.
	h.r.history.pruning = func(time.Time, time.Duration) {
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

func TestAnImportAskedForWhileTheHistoryWaitsOnItsMoveIsRefusedTakingNothingIn(t *testing.T) {
	tests := []struct {
		name   string
		moving func(cfg *Config)
		said   string
	}{
		{
			name:   "onto the accounts' ids",
			moving: func(cfg *Config) { cfg.Move = &answeringMover{answers: []error{idmove.ErrAwaitingRestart}} },
			said:   prose.HistoryMoving,
		},
		{
			name:   "to pools",
			moving: func(cfg *Config) { cfg.PoolMove = &answeringPoolMover{answers: []error{idmove.ErrAwaitingPools}} },
			said:   prose.HistoryMovingPools,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			cfg := importingConfig(t)
			tt.moving(&cfg)
			r := newTestRouterOf(t, cfg)
			day := start.AddDate(0, 0, -10)
			staged := readings.Files(readings.Dir(filepath.Join(ImportDir(cfg.StateDir), "1")), logger)
			stageFile(t, staged.CompressedPath(dayfile.DateOf(day)), gzipOf(t, readingOn(day)))
			client, _ := runImporting(t, r)

			_, err := client.Import(t.Context())
			if d, ok := errors.AsType[declined](err); !ok || d.status != http.StatusServiceUnavailable || !strings.Contains(d.reason, tt.said) {
				t.Errorf("Import() while the history waits on its move: %v, want a 503 saying %q", err, tt.said)
			}
			if _, err := os.Stat(readings.Files(readings.Dir(cfg.StateDir), logger).CompressedPath(dayfile.DateOf(day))); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("the day staged, refused: %v, want it not taken in", err)
			}
			if _, err := os.Stat(ImportDir(cfg.StateDir)); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("once the import was refused, what was staged: %v, want it removed", err)
			}
		})
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
	stageFile(t, ledger.SummaryFile(ledger.Dir(cfg.StateDir), older), []byte(`{"version":7,"day":"`+older+`","lines":0}`+"\n"))
	folder := stageOwn(t, cfg.StateDir)
	stagedReadings := readings.Files(readings.Dir(folder), logger)
	stageFile(t, stagedReadings.CompressedPath(weekBefore), gzipOf(t, readingOn(day.AddDate(0, 0, -9))))
	stageFile(t, stagedReadings.CompressedPath(date), gzipOf(t, readingOn(day)))
	stageFile(t, ledger.Files(ledger.Dir(folder), logger).CompressedPath(date),
		gzipOf(t, `{"at":"`+day.UTC().Format(time.RFC3339)+`","request":"staged","kind":"message","account":"work","status":200}`+"\n"))
	stageFile(t, ledger.SummaryFile(ledger.Dir(folder), date), []byte(`{"version":7,"day":"`+date+`","lines":2,"accounts":[{"account":"work"}]}`+"\n"))
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
		r.history.pruning(r.cfg.Now(), r.history.keep)
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

func TestAnAccountsRecordStagedThatDisagreesIsAConflict(t *testing.T) {
	if got := importStatus(fmt.Errorf("nothing was imported, as %w", identity.ErrConflict)); got != http.StatusConflict {
		t.Errorf("importStatus() of an accounts record that disagrees = %d, want 409", got)
	}
}

// A folder exported before pools, where the Mac's record notes no first pool
// to give it, is the Mac's fault, not the folder's.
func TestAFolderBeforePoolsWithNoFirstPoolToGiveItIsTheMacsFault(t *testing.T) {
	cfg := importingConfig(t)
	if err := identity.NewFile(cfg.StateDir).Update(func(r *identity.Record) error {
		r.Pools = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	folder := stageOwn(t, cfg.StateDir)
	_, err := newTestRouterOf(t, cfg).joinStaged([]string{folder})
	if got := importStatus(err); !errors.Is(err, idmove.ErrNoFirstPool) || got != http.StatusInternalServerError {
		t.Errorf("joinStaged() = %v, answered %d, want no first pool, 500", err, got)
	}
}

// A folder whose accounts can't be told is answered as the folder's fault: a
// 422 where its record doesn't read, or it's another Mac's from before
// switchboard made ids; a 409, to try again, where its record hasn't synced.
func TestAFolderWhoseAccountsCantBeToldIsAnsweredAsTheFolders(t *testing.T) {
	const otherInstall = `"install":"fedcba9876543210","exported_at":"2026-09-20T09:30:00Z"`
	tests := []struct {
		name     string
		manifest string
		record   string
		want     int
	}{
		{name: "a record that doesn't read", manifest: `{"version":1,` + otherInstall + `,"accounts":true}`, record: "not a record\n", want: http.StatusUnprocessableEntity},
		{name: "another Mac's from before ids", manifest: `{"version":1,` + otherInstall + `}`, want: http.StatusUnprocessableEntity},
		{name: "a record that hasn't synced", manifest: `{"version":1,` + otherInstall + `,"accounts":true}`, want: http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := importingConfig(t)
			folder := filepath.Join(ImportDir(cfg.StateDir), "1")
			stageFile(t, filepath.Join(folder, "export.json"), []byte(tt.manifest+"\n"))
			if tt.record != "" {
				stageFile(t, identity.NewFile(folder).Path(), []byte(tt.record))
			}
			_, err := newTestRouterOf(t, cfg).joinStaged([]string{folder})
			if got := importStatus(err); err == nil || got != tt.want {
				t.Errorf("joinStaged() = %v, answered %d, want %d", err, got, tt.want)
			}
		})
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
