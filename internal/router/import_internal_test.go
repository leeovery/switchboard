package router

import (
	"context"
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
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/readings"
)

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
	// busy closes once the writer is held.
	busy    chan struct{}
	release func()
	// stop stops the router, and returns once Run has.
	stop func() error
}

// newHeldImporting runs a router as heldImporting says, until the test ends,
// returning once its control API answers.
func newHeldImporting(t *testing.T) *heldImporting {
	t.Helper()
	dir, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cfg := testConfig(at(start), &stubProber{}, testTokens.Read)
	cfg.StateDir, cfg.Listen = dir, "127.0.0.1:0"
	h := &heldImporting{r: newTestRouterOf(t, cfg), client: NewClient(SocketPath(dir)), dir: dir, date: dayfile.DateOf(start.AddDate(0, 0, -10)), busy: make(chan struct{})}
	staged := readings.Files(readings.Dir(filepath.Join(ImportDir(dir), "1")), logger)
	if err := os.MkdirAll(staged.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	line := `{"at":"` + start.AddDate(0, 0, -10).Format(time.RFC3339) + `","account":"work","window":"5h","utilization":0.4,"status":"allowed","source":"answer"}` + "\n"
	if err := os.WriteFile(staged.CompressedPath(h.date), gzipOf(t, line), 0o600); err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	h.release = sync.OnceFunc(func() { close(released) })
	h.r.history.pruning = func(time.Time) {
		close(h.busy)
		<-released
	}
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan error, 1)
	go func() { finished <- h.r.run(ctx) }()
	h.stop = sync.OnceValue(func() error {
		cancel()
		return <-finished
	})
	t.Cleanup(func() {
		h.release()
		_ = h.stop()
	})
	waitFor(t, "the router answers", func() bool {
		_, err := h.client.Health(t.Context())
		return err == nil
	})
	<-h.busy
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
