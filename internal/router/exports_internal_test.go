package router

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
)

// The lines the router logs of its exports.
const (
	exportFailed  = `msg="can't export the history; trying again at each round"`
	exportedAgain = `msg="exporting the history again"`
)

// macOSRefused is what an export macOS refused the folder dir says.
func macOSRefused(dir string) string {
	return "macOS refused access to " + dir + ": allow switchboard in System Settings → Privacy & Security"
}

// exportAs is an export into a folder that does as it's told: it gives
// result and err, and records the folders it's asked to export into.
type exportAs struct {
	mu     sync.Mutex
	result export.Result
	err    error
	asked  []string
}

func (e *exportAs) into(dir string) (export.Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.asked = append(e.asked, dir)
	return e.result, e.err
}

// set has the export give result and err from now on.
func (e *exportAs) set(result export.Result, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.result, e.err = result, err
}

// folders are the folders the export has been asked to export into.
func (e *exportAs) folders() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.asked)
}

// testExports are exports into the folder dir names, by start's clock,
// exporting with exportInto, the history's writers' rounds made at once, and
// the home directory /Users/test.
func testExports(dir string, exportInto func(dir string) (export.Result, error)) *exports {
	x := &exports{
		every:      time.Hour,
		now:        at(start),
		homeDir:    func() (string, error) { return "/Users/test", nil },
		exportInto: exportInto,
		rounds:     func(context.Context) error { return nil },
	}
	x.configure(config.Export{Dir: dir})
	return x
}

// timesLogged is how many of the lines log holds hold every one of parts.
func timesLogged(log *logstest.Log, parts ...string) int {
	n := 0
	for _, line := range log.Lines() {
		if containsEvery(line, parts) {
			n++
		}
	}
	return n
}

func containsEvery(line string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(line, part) {
			return false
		}
	}
	return true
}

func TestAnExportsFailureIsLoggedOnceUntilOneSucceeds(t *testing.T) {
	log := logstest.Capture(t)
	const dir = "/Volumes/Backup/Switchboard"
	failure := fmt.Errorf("can't export into %s: %w", dir, &fs.PathError{Op: "mkdir", Path: dir, Err: syscall.EACCES})
	done := export.Result{Folder: dir + "/mac-0123456789abcdef", Written: 3}
	steps := []struct {
		name       string
		err        error
		want       status.Export
		wantFailed int
		wantAgain  int
	}{
		{name: "failing", err: failure, want: status.Export{Dir: dir, Error: failure.Error()}, wantFailed: 1},
		{name: "failing again", err: failure, want: status.Export{Dir: dir, Error: failure.Error()}, wantFailed: 1},
		{name: "succeeding", want: status.Export{Dir: dir, At: start}, wantFailed: 1, wantAgain: 1},
		{name: "succeeding again", want: status.Export{Dir: dir, At: start}, wantFailed: 1, wantAgain: 1},
		{name: "failing once more", err: failure, want: status.Export{Dir: dir, At: start, Error: failure.Error()}, wantFailed: 2, wantAgain: 1},
	}
	fake := &exportAs{}
	x := testExports(dir, fake.into)
	for _, step := range steps {
		fake.set(done, step.err)
		x.exportNow(t.Context())

		if got := x.report(); got != step.want {
			t.Errorf("%s, report() = %+v, want %+v", step.name, got, step.want)
		}
		if got := timesLogged(log, "level=WARN", exportFailed, "dir="+dir); got != step.wantFailed {
			t.Errorf("%s, the failure is logged %d times, want %d; log reads\n%s", step.name, got, step.wantFailed, log)
		}
		if got := timesLogged(log, "level=INFO", exportedAgain, "dir="+dir); got != step.wantAgain {
			t.Errorf("%s, the export succeeding again is logged %d times, want %d; log reads\n%s", step.name, got, step.wantAgain, log)
		}
	}
}

func TestAnExportsFailureSaysItsCauseInPlainWords(t *testing.T) {
	tests := []struct {
		name string
		// folder is the folder to export into, in root, made as the case
		// has it.
		folder func(t *testing.T, root string) string
		// exportInto, where it's set, exports in place of the router's own
		// exporter.
		exportInto func(dir string) (export.Result, error)
		want       func(dir string) string
	}{
		{
			name:   "macOS refusing the router the folder",
			folder: func(_ *testing.T, root string) string { return filepath.Join(root, "Documents", "Switchboard") },
			exportInto: func(dir string) (export.Result, error) {
				return export.Result{}, fmt.Errorf("can't export into %s: %w", dir, &fs.PathError{Op: "open", Path: filepath.Join(dir, "export.json"), Err: syscall.EPERM})
			},
			want: macOSRefused,
		},
		{
			name:   "the folder not there, nor the one it goes in, as a disk not mounted",
			folder: func(_ *testing.T, root string) string { return filepath.Join(root, "Volumes", "Backup", "Switchboard") },
			want: func(dir string) string {
				return "can't export into " + dir + ": it isn't there, nor the folder it goes in"
			},
		},
		{
			name: "anything else, as a folder that can't be written",
			folder: func(t *testing.T, root string) string {
				if err := os.Chmod(root, 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
				return filepath.Join(root, "Switchboard")
			},
			want: func(dir string) string {
				return "can't export into " + dir + ": mkdir " + dir + ": permission denied"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			state := t.TempDir()
			dir := tt.folder(t, t.TempDir())
			cfg := testConfig(at(start), &stubProber{}, testTokens.Read)
			cfg.StateDir, cfg.Export = state, config.Export{Dir: dir}
			x := newExports(cfg, func(context.Context) error { return nil })
			if tt.exportInto != nil {
				x.exportInto = tt.exportInto
			}

			x.exportNow(t.Context())

			if got, want := x.report(), (status.Export{Dir: dir, Error: tt.want(dir)}); got != want {
				t.Errorf("report() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestAnExportIntoAFolderNotThereWritesNothing(t *testing.T) {
	logstest.Capture(t)
	state, root := t.TempDir(), t.TempDir()
	parent := filepath.Join(root, "Volumes", "Backup")
	cfg := testConfig(at(start), &stubProber{}, testTokens.Read)
	cfg.StateDir, cfg.Export = state, config.Export{Dir: filepath.Join(parent, "Switchboard")}
	x := newExports(cfg, func(context.Context) error { return nil })

	x.exportNow(t.Context())

	for _, path := range []string{filepath.Join(root, "Volumes"), filepath.Join(state, "install-id")} {
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("after the export, %s: %v, want it not there", path, err)
		}
	}
}

func TestAnExportIntoAFolderInTheHomeDirectory(t *testing.T) {
	tests := []struct {
		name    string
		homeDir func() (string, error)
		// want is the folder exported into, "" for none; wantDir the folder
		// reported, and wantError why the export failed.
		want, wantDir, wantError string
	}{
		{
			name:    "the home directory found",
			homeDir: func() (string, error) { return "/Users/test", nil },
			want:    "/Users/test/Dropbox/Switchboard", wantDir: "/Users/test/Dropbox/Switchboard",
		},
		{
			name:      "the home directory not found",
			homeDir:   func() (string, error) { return "", errors.New("$HOME is not defined") },
			wantDir:   "~/Dropbox/Switchboard",
			wantError: "find the home directory export.dir is in: $HOME is not defined",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			fake := &exportAs{}
			x := testExports("", fake.into)
			x.homeDir = tt.homeDir
			x.configure(config.Export{Dir: "~/Dropbox/Switchboard"})

			x.exportNow(t.Context())

			var want []string
			if tt.want != "" {
				want = []string{tt.want}
			}
			if got := fake.folders(); !slices.Equal(got, want) {
				t.Errorf("exported into %q, want %q", got, want)
			}
			if got := x.report(); got.Dir != tt.wantDir || got.Error != tt.wantError {
				t.Errorf("report() = %+v, want the folder %q, failing as %q", got, tt.wantDir, tt.wantError)
			}
		})
	}
}

func TestADayAnExportCouldntReadIsWarnedOfOnceAndFailsNoExport(t *testing.T) {
	log := logstest.Capture(t)
	const dir = "/Volumes/Backup/Switchboard"
	unread := export.DayError{What: "the request ledger of 2026-09-26", Date: "2026-09-26", Err: errors.New("permission denied")}
	warned := []string{"level=WARN", `msg="couldn't export the request ledger of 2026-09-26; passed over"`, `error="permission denied"`}
	steps := []struct {
		name       string
		unread     []export.DayError
		wantWarned int
	}{
		{name: "passed over", unread: []export.DayError{unread}, wantWarned: 1},
		{name: "passed over again", unread: []export.DayError{unread}, wantWarned: 1},
		{name: "read", wantWarned: 1},
		{name: "passed over once more", unread: []export.DayError{unread}, wantWarned: 2},
	}
	fake := &exportAs{}
	x := testExports(dir, fake.into)
	for _, step := range steps {
		fake.set(export.Result{Folder: dir + "/mac-0123456789abcdef", AsTheyWere: 1, Unread: step.unread}, nil)
		x.exportNow(t.Context())

		if got, want := x.report(), (status.Export{Dir: dir, At: start}); got != want {
			t.Errorf("%s, report() = %+v, want %+v", step.name, got, want)
		}
		if got := timesLogged(log, warned...); got != step.wantWarned {
			t.Errorf("%s, the day passed over is warned of %d times, want %d; log reads\n%s", step.name, got, step.wantWarned, log)
		}
		if log.Has(exportFailed) {
			t.Errorf("%s, log reads\n%s\nwant no failed export", step.name, log)
		}
	}
}

// heldExport is an export that holds, once it's asked, until it's released,
// as a write into a folder macOS guards waits on its prompt, counting the
// times it's asked.
type heldExport struct {
	asked    atomic.Int32
	entered  chan struct{}
	released chan struct{}
	release  func()
}

func newHeldExport(t *testing.T) *heldExport {
	h := &heldExport{entered: make(chan struct{}, 8), released: make(chan struct{})}
	h.release = sync.OnceFunc(func() { close(h.released) })
	t.Cleanup(h.release)
	return h
}

func (h *heldExport) into(string) (export.Result, error) {
	h.asked.Add(1)
	h.entered <- struct{}{}
	<-h.released
	return export.Result{}, nil
}

// await waits for the export to be asked.
func (h *heldExport) await(t *testing.T) {
	t.Helper()
	select {
	case <-h.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("gave up waiting for the export to be asked")
	}
}

func TestARoundWhileAnExportRunsStartsNone(t *testing.T) {
	logstest.Capture(t)
	held := newHeldExport(t)
	x := testExports("/Volumes/Backup/Switchboard", held.into)

	x.round(t.Context())
	held.await(t)
	x.round(t.Context())
	x.round(t.Context())
	if got := held.asked.Load(); got != 1 {
		t.Errorf("with the export still running, it's been asked %d times, want 1", got)
	}

	held.release()
	waitFor(t, "the export ends", func() bool { return !x.running.Load() })
	x.round(t.Context())
	held.await(t)
	if got := held.asked.Load(); got != 2 {
		t.Errorf("once the export ended, it's been asked %d times, want 2", got)
	}
	waitFor(t, "the export ends", func() bool { return !x.running.Load() })
}

func TestTheExportsExportAsTheyStart(t *testing.T) {
	logstest.Capture(t)
	held := newHeldExport(t)
	x := testExports("/Volumes/Backup/Switchboard", held.into)
	ctx, cancel := context.WithCancel(t.Context())
	ran := make(chan struct{})

	go func() {
		defer close(ran)
		x.run(ctx)
	}()

	held.await(t)
	cancel()
	<-ran
	held.release()
	waitFor(t, "the export ends", func() bool { return !x.running.Load() })
}

func TestTheConfigIsTakenUpFromTheNextRound(t *testing.T) {
	const (
		was   = "/Volumes/Backup/Switchboard"
		other = "/Volumes/Other/Switchboard"
	)
	tests := []struct {
		name string
		// from is the folder the exports went by, "" for none, and to the
		// one they go by after.
		from, to string
		// want is how they stand once configured, and wantExported the
		// folders exported into at the next round.
		want         status.Export
		wantExported []string
	}{
		{name: "set", to: was, want: status.Export{Dir: was}, wantExported: []string{was}},
		{name: "changed", from: was, to: other, want: status.Export{Dir: other}, wantExported: []string{other}},
		{name: "removed", from: was},
		{name: "left as it was", from: was, to: was, want: status.Export{Dir: was, At: start}, wantExported: []string{was}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			fake := &exportAs{}
			x := testExports(tt.from, fake.into)
			x.exportNow(t.Context())
			before := len(fake.folders())

			x.configure(config.Export{Dir: tt.to})
			if got := x.report(); got != tt.want {
				t.Errorf("once configured, report() = %+v, want %+v", got, tt.want)
			}
			x.exportNow(t.Context())
			if got := fake.folders()[before:]; !slices.Equal(got, tt.wantExported) {
				t.Errorf("the next round exported into %q, want %q", got, tt.wantExported)
			}
		})
	}
}

func TestAnExportOfAFolderNoLongerConfiguredIsntKept(t *testing.T) {
	logstest.Capture(t)
	const was, other = "/Volumes/Backup/Switchboard", "/Volumes/Other/Switchboard"
	held := newHeldExport(t)
	x := testExports(was, held.into)
	x.round(t.Context())
	held.await(t)

	x.configure(config.Export{Dir: other})
	held.release()
	waitFor(t, "the export ends", func() bool { return !x.running.Load() })

	if got, want := x.report(), (status.Export{Dir: other}); got != want {
		t.Errorf("report() = %+v, want %+v, nothing kept of the export into %s", got, want, was)
	}
}

// exportedKinds are the kinds of the history's files of lines, each by the
// files it keeps in root, a state directory or a Mac's folder.
var exportedKinds = []struct {
	name  string
	files func(root string) *dayfile.Files
}{
	{name: "the request ledger", files: func(root string) *dayfile.Files { return ledger.Files(ledger.Dir(root), logger) }},
	{name: "the router's events", files: func(root string) *dayfile.Files { return events.Files(ledger.Dir(root), logger) }},
	{name: "the readings history", files: func(root string) *dayfile.Files { return readings.Files(readings.Dir(root), logger) }},
}

// setClock is a clock a test sets, read from any goroutine.
type setClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *setClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *setClock) set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

// runHistory opens the history r keeps, its readings, its events and its
// request ledger, in its state directory, and runs their writers until the
// test ends, returning once each has made its first round.
func runHistory(t *testing.T, r *Router) {
	t.Helper()
	r.openHistory()
	r.openLedger(ledger.Dir(r.cfg.StateDir))
	ctx, cancel := context.WithCancel(context.Background())
	var running sync.WaitGroup
	running.Go(func() { r.history.run(ctx) })
	running.Go(func() { r.ledger.run(ctx) })
	running.Go(func() { r.eventFiles.Run(ctx) })
	t.Cleanup(func() {
		cancel()
		running.Wait()
	})
	if err := r.roundsNow(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestAnExportFollowsTheRoundsCompress(t *testing.T) {
	for _, kind := range exportedKinds {
		t.Run(kind.name, func(t *testing.T) {
			logstest.Capture(t)
			clock := &setClock{now: start}
			cfg := testConfig(clock.read, &stubProber{}, testTokens.Read)
			cfg.StateDir = t.TempDir()
			cfg.Export = config.Export{Dir: filepath.Join(t.TempDir(), "Switchboard")}
			r := newTestRouterOf(t, cfg)
			date := dayfile.DateOf(start.AddDate(0, 0, -2))
			held := kind.files(cfg.StateDir)
			compressed := held.CompressedPath(date)
			plain := strings.TrimSuffix(compressed, ".gz")
			stageFile(t, plain, []byte(readingOn(start.AddDate(0, 0, -2))))
			runHistory(t, r)

			// A day later, the day is the round's to compress, as the
			// writers' own hourly rounds haven't come.
			clock.set(start.AddDate(0, 0, 1))
			r.exports.exportNow(t.Context())

			if _, err := os.Lstat(plain); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("after the round, the day's plain file: %v, want it compressed", err)
			}
			want, err := os.ReadFile(compressed)
			if err != nil {
				t.Fatal(err)
			}
			id, err := export.InstallID(cfg.StateDir)
			if err != nil {
				t.Fatal(err)
			}
			copied := kind.files(export.Folder(cfg.Export.Dir, id)).CompressedPath(date)
			if got, err := os.ReadFile(copied); err != nil || !bytes.Equal(got, want) {
				t.Errorf("the folder's file of the day reads %q (%v), want the compressed file, %q", got, err, want)
			}
			if got := r.exports.report(); got.Error != "" || !got.At.Equal(start.AddDate(0, 0, 1)) {
				t.Errorf("report() = %+v, want an export that succeeded", got)
			}
		})
	}
}

func TestAConfigChangeIsHandedToTheExports(t *testing.T) {
	logstest.Capture(t)
	s := newStartedFrom(t)
	cfg := testConfig(at(start), &stubProber{}, testTokens.Read)
	cfg.ConfigFile = Watch(s.config)
	r := newTestRouterOf(t, cfg)
	dir := filepath.Join(t.TempDir(), "Switchboard")

	writeFile(t, s.kept, "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"personal\"\n\n[[account]]\nid = \"side\"\n\n[export]\ndir = \""+dir+"\"\n")
	r.upkeep.look()

	if got, want := r.exports.report(), (status.Export{Dir: dir}); got != want {
		t.Errorf("once the config file names a folder, report() = %+v, want %+v", got, want)
	}
}

func TestTheRouterStoppingWaitsOnNoExport(t *testing.T) {
	logstest.Capture(t)
	cfg := importingConfig(t)
	cfg.Export = config.Export{Dir: filepath.Join(t.TempDir(), "Switchboard")}
	r := newTestRouterOf(t, cfg)
	held := newHeldExport(t)
	r.exports.exportInto = held.into
	_, stop := runImporting(t, r)
	held.await(t)

	stopped := make(chan error, 1)
	go func() { stopped <- stop() }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("Run() = %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the router's stop is waiting on the export held")
	}
	held.release()
	waitFor(t, "the export ends", func() bool { return !r.exports.running.Load() })
}
