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
// exporting with exportInto, the history's writers tended at once, the home
// directory /Users/test, and no restart coming.
func testExports(dir string, exportInto func(dir string) (export.Result, error)) *exports {
	x := &exports{
		every:      time.Hour,
		now:        at(start),
		homeDir:    func() (string, error) { return "/Users/test", nil },
		exportInto: exportInto,
		tend:       tended,
		restarting: noRestart,
		running:    make(map[string]time.Time),
	}
	x.configure(config.Export{Dir: dir})
	return x
}

// tended has the history's writers tended at once, as none is kept.
func tended(context.Context) error { return nil }

// noRestart is a router that isn't to restart.
func noRestart() bool { return false }

// exportNow makes a round as round does, but on the caller's goroutine,
// returning once its export has ended.
func (x *exports) exportNow(ctx context.Context) {
	if target, began := x.begin(); began {
		defer x.end(target.dir)
		x.export(ctx, target)
	}
}

// idle reports whether no export runs, into any folder.
func (x *exports) idle() bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	return len(x.running) == 0
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
	const dir = "/Users/test/Documents/Switchboard"
	folder := filepath.Join(dir, "mac-0123456789abcdef")
	failed := func(op, path string, err error) error {
		return fmt.Errorf("can't export into %s: %w", dir, &fs.PathError{Op: op, Path: path, Err: err})
	}
	tests := []struct {
		name string
		err  error
		// refused is set where macOS refusing the router the folder is
		// what's said, else the error's own words.
		refused bool
	}{
		{name: "macOS refusing a file made in the folder", err: failed("open", filepath.Join(folder, ".export.json.123"), syscall.EPERM), refused: true},
		{name: "macOS refusing a folder made in it", err: failed("mkdir", filepath.Join(folder, "ledger"), syscall.EPERM), refused: true},
		{name: "macOS refusing a look at the folder", err: failed("stat", dir, syscall.EPERM), refused: true},
		{name: "macOS refusing the folder's listing", err: failed("readdir", filepath.Join(folder, "ledger"), syscall.EPERM), refused: true},
		{
			name: "a rename onto a file the Finder has locked",
			err: fmt.Errorf("can't export into %s: %w", dir, &os.LinkError{
				Op: "rename", Old: filepath.Join(folder, ".export.json.123"), New: filepath.Join(folder, "export.json"), Err: syscall.EPERM,
			}),
		},
		{name: "a time set on a file made already", err: failed("chtimes", filepath.Join(folder, ".export.json.123"), syscall.EPERM)},
		{name: "a file opened outside the folder", err: failed("open", "/Users/test/.local/state/switchboard/install-id", syscall.EPERM)},
		{name: "a folder named alike beside it", err: failed("open", dir+"-old/export.json", syscall.EPERM)},
		{name: "a file that can't be written", err: failed("open", filepath.Join(folder, ".export.json.123"), syscall.EACCES)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			x := testExports(dir, func(string) (export.Result, error) { return export.Result{}, tt.err })

			x.exportNow(t.Context())

			want := tt.err.Error()
			if tt.refused {
				want = macOSRefused(dir)
			}
			if got := x.report().Error; got != want {
				t.Errorf("report().Error = %q, want %q", got, want)
			}
		})
	}
}

func TestAFolderTheExportCantHaveIsSaidPlainly(t *testing.T) {
	tests := []struct {
		name string
		// folder is the folder to export into, in root, made as the case
		// has it.
		folder func(t *testing.T, root string) string
		// want is what the export into dir says, its Mac's folder at folder.
		want func(dir, folder string) string
	}{
		{
			name:   "not there, nor the folder it goes in",
			folder: func(_ *testing.T, root string) string { return filepath.Join(root, "Volumes", "Backup", "Switchboard") },
			want: func(dir, _ string) string {
				return "can't export into " + dir + ": it isn't there, nor the folder it goes in"
			},
		},
		{
			name: "not there, and not to be made where it goes, as a disk not mounted in /Volumes",
			folder: func(t *testing.T, root string) string {
				readOnly(t, root)
				return filepath.Join(root, "Backup")
			},
			want: func(dir, _ string) string {
				return "can't export into " + dir + ": it isn't there, and can't be made: mkdir " + dir + ": permission denied"
			},
		},
		{
			name: "there, but not to be written",
			folder: func(t *testing.T, root string) string {
				readOnly(t, root)
				return root
			},
			want: func(dir, folder string) string {
				return "can't export into " + dir + ": mkdir " + folder + ": permission denied"
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
			x := newExports(cfg, tended, noRestart)

			x.exportNow(t.Context())

			id, err := export.InstallID(state)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := x.report(), (status.Export{Dir: dir, Error: tt.want(dir, export.Folder(dir, id))}); got != want {
				t.Errorf("report() = %+v, want %+v", got, want)
			}
		})
	}
}

// readOnly makes the folder at dir one that can't be written, as /Volumes,
// which only the system writes, is to the user, until the test ends.
func readOnly(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

func TestAnExportIntoAFolderNotThereWritesNothing(t *testing.T) {
	logstest.Capture(t)
	state, root := t.TempDir(), t.TempDir()
	parent := filepath.Join(root, "Volumes", "Backup")
	cfg := testConfig(at(start), &stubProber{}, testTokens.Read)
	cfg.StateDir, cfg.Export = state, config.Export{Dir: filepath.Join(parent, "Switchboard")}
	x := newExports(cfg, tended, noRestart)

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
	waitFor(t, "the export ends", func() bool { return x.idle() })
	x.round(t.Context())
	held.await(t)
	if got := held.asked.Load(); got != 2 {
		t.Errorf("once the export ended, it's been asked %d times, want 2", got)
	}
	waitFor(t, "the export ends", func() bool { return x.idle() })
}

func TestTheExportsExportOnceTheRouterHasFirstLooked(t *testing.T) {
	logstest.Capture(t)
	held := newHeldExport(t)
	x := testExports("/Volumes/Backup/Switchboard", held.into)
	ctx, cancel := context.WithCancel(t.Context())
	looked, ran := make(chan struct{}), make(chan struct{})

	go func() {
		defer close(ran)
		x.run(ctx, looked)
	}()
	close(looked)

	held.await(t)
	cancel()
	<-ran
	held.release()
	waitFor(t, "the export ends", func() bool { return x.idle() })
}

func TestTheExportsStoppingBeforeTheRouterHasLookedExportNothing(t *testing.T) {
	logstest.Capture(t)
	fake := &exportAs{}
	x := testExports("/Volumes/Backup/Switchboard", fake.into)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	x.run(ctx, make(chan struct{}))

	waitFor(t, "no export runs", func() bool { return x.idle() })
	if got := fake.folders(); len(got) > 0 {
		t.Errorf("exported into %q before the router first looked, want nothing", got)
	}
}

func TestARouterStartingWithARestartDueStartsNoExport(t *testing.T) {
	logstest.Capture(t)
	s := newStartedFrom(t)
	cfg := exportingConfig(t)
	cfg.ConfigFile = Watch(s.config)
	binary := filepath.Join(t.TempDir(), "switchboard")
	stageFile(t, binary, []byte("switchboard\n"))
	cfg.Binary, cfg.Supervised = Watch(binary), true
	cfg.Args, cfg.Environ = []string{binary, "serve"}, []string{"HOME=/Users/test"}
	// Long enough that an export begun as the router starts is asked well
	// before its first look.
	cfg.WatchEvery = 500 * time.Millisecond
	execs := make(chan string, 1)
	cfg.Exec = func(path string, _, _ []string) error {
		execs <- path
		return nil
	}
	r := newTestRouterOf(t, cfg)
	held := newHeldExport(t)
	r.exports.exportInto = held.into
	// The config file changes as the router starts, so its first look finds
	// a restart due.
	writeFile(t, s.kept, twoAccounts)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)

	go func() { finished <- r.run(ctx) }()

	select {
	case err := <-finished:
		if err != nil {
			t.Errorf("Run() = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("gave up waiting for the router to restart, with a restart due from its first look")
	}
	select {
	case path := <-execs:
		if path != binary {
			t.Errorf("the router replaced itself with %s, want %s", path, binary)
		}
	default:
		t.Error("the router stopped without replacing itself, want it restarting")
	}
	held.release()
	waitFor(t, "no export runs", func() bool { return r.exports.idle() })
	if got := held.asked.Load(); got != 0 {
		t.Errorf("an export was asked %d times, want none: the router it becomes exports as it starts", got)
	}
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

func TestAnExportIntoAFolderIsntHeldOffByOneIntoTheFolderBefore(t *testing.T) {
	logstest.Capture(t)
	const was, other = "/Volumes/Backup/Switchboard", "/Volumes/Other/Switchboard"
	held, fake := newHeldExport(t), &exportAs{}
	x := testExports(was, func(dir string) (export.Result, error) {
		if dir == was {
			return held.into(dir)
		}
		return fake.into(dir)
	})
	x.round(t.Context())
	held.await(t)

	x.configure(config.Export{Dir: other})
	x.exportNow(t.Context())
	if got := fake.folders(); !slices.Equal(got, []string{other}) {
		t.Errorf("with the export into %s held, the next round exported into %q, want %s", was, got, other)
	}
	if got, want := x.report(), (status.Export{Dir: other, At: start}); got != want {
		t.Errorf("report() = %+v, want %+v", got, want)
	}

	held.release()
	waitFor(t, "the export held ends", func() bool { return x.idle() })
	if got, want := x.report(), (status.Export{Dir: other, At: start}); got != want {
		t.Errorf("once the export held ends, report() = %+v, want %+v, nothing kept of it", got, want)
	}
}

func TestTheReportSaysWhenTheExportUnderWayBegan(t *testing.T) {
	logstest.Capture(t)
	const dir = "/Users/test/Documents/Switchboard"
	clock := &setClock{now: start}
	held := newHeldExport(t)
	x := testExports(dir, held.into)
	x.now = clock.read

	x.round(t.Context())
	held.await(t)
	clock.set(start.Add(time.Hour))
	if got, want := x.report(), (status.Export{Dir: dir, Running: start}); got != want {
		t.Errorf("with the export held, as on macOS's prompt, report() = %+v, want %+v", got, want)
	}

	held.release()
	waitFor(t, "the export ends", func() bool { return x.idle() })
	if got, want := x.report(), (status.Export{Dir: dir, At: start.Add(time.Hour)}); got != want {
		t.Errorf("once the export ends, report() = %+v, want %+v", got, want)
	}
}

func TestARouterAboutToRestartStartsNoExport(t *testing.T) {
	tests := []struct {
		name         string
		restarting   bool
		want         status.Export
		wantExported []string
	}{
		{name: "a restart coming", restarting: true, want: status.Export{Dir: "/Volumes/Backup/Switchboard"}},
		{name: "none coming", want: status.Export{Dir: "/Volumes/Backup/Switchboard", At: start}, wantExported: []string{"/Volumes/Backup/Switchboard"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			fake := &exportAs{}
			x := testExports("/Volumes/Backup/Switchboard", fake.into)
			x.restarting = func() bool { return tt.restarting }

			x.exportNow(t.Context())

			if got := fake.folders(); !slices.Equal(got, tt.wantExported) {
				t.Errorf("exported into %q, want %q", got, tt.wantExported)
			}
			if got := x.report(); got != tt.want {
				t.Errorf("report() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestARestartComesWhereTheRouterTakesItUpItself(t *testing.T) {
	tests := []struct {
		name       string
		supervised bool
		// change changes what the router, running a copy of its own, was
		// started from, as the case has it, and has it look, or try to
		// restart.
		change func(t *testing.T, s startedFrom, r *restarts)
		want   bool
	}{
		{name: "none due", supervised: true, change: func(_ *testing.T, _ startedFrom, r *restarts) { r.look() }},
		{name: "one due", supervised: true, change: configChanged, want: true},
		{
			name: "one due, waiting for a moment with no request in flight", supervised: true,
			change: func(t *testing.T, s startedFrom, r *restarts) {
				r.inFlight.begin()
				t.Cleanup(r.inFlight.end)
				configChanged(t, s, r)
			},
			want: true,
		},
		{name: "one due to a router run by hand", change: configChanged},
		{
			name: "one due that its binary holds", supervised: true,
			change: func(t *testing.T, s startedFrom, r *restarts) {
				askedAt(t, r, binaryAnswer{refusal: "the config won't do"})
				configChanged(t, s, r)
				putOff(t, r)
			},
		},
		{
			name: "one due with its binary gone", supervised: true,
			change: func(t *testing.T, s startedFrom, r *restarts) {
				remove(t, s.binary)
				configChanged(t, s, r)
			},
		},
		{
			name: "an upgrade that can't be copied in", supervised: true,
			change: func(t *testing.T, s startedFrom, r *restarts) {
				relink(t, s.binary, s.version("1.1"))
				chmod(t, s.version("1.1"), 0o000)
				r.look()
				putOff(t, r)
			},
		},
		{
			name: "one under way", supervised: true,
			change: func(_ *testing.T, _ startedFrom, r *restarts) { r.begin(restartForConfig, nil) },
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			s, own := newRunningCopy(t)
			r := restartsFor(Config{
				ConfigFile: Watch(s.config),
				Binary:     Watch(own),
				From:       WatchFrom(s.binary, own),
				Zone:       Watch(s.zone),
				Supervised: tt.supervised,
				Now:        at(start),
			}, newInFlight(), unheeded)

			tt.change(t, s, r)
			if got := r.coming(); got != tt.want {
				t.Errorf("coming() = %v, want %v", got, tt.want)
			}
		})
	}
}

// putOff has r try to restart, failing the test where it does.
func putOff(t *testing.T, r *restarts) {
	t.Helper()
	if r.restart(t.Context()) {
		t.Fatal("restart() = true, want it put off")
	}
}

// configChanged changes the config file of s into another valid config, and
// has r look at it.
func configChanged(t *testing.T, s startedFrom, r *restarts) {
	t.Helper()
	writeFile(t, s.kept, twoAccounts)
	r.look()
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
	if err := r.tendHistory(t.Context()); err != nil {
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
	tests := []struct {
		name       string
		supervised bool
		// wantExported is whether the next round exports into the folder:
		// not where the change makes a restart due that the router takes up
		// itself, as the router it becomes exports as it starts.
		wantExported bool
	}{
		{name: "the service's router", supervised: true},
		{name: "a router run by hand", wantExported: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			s := newStartedFrom(t)
			cfg := testConfig(at(start), &stubProber{}, testTokens.Read)
			cfg.ConfigFile, cfg.Supervised = Watch(s.config), tt.supervised
			r := newTestRouterOf(t, cfg)
			fake := &exportAs{}
			r.exports.exportInto, r.exports.tend = fake.into, tended
			dir := filepath.Join(t.TempDir(), "Switchboard")

			writeFile(t, s.kept, "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"personal\"\n\n[[account]]\nid = \"side\"\n\n[export]\ndir = \""+dir+"\"\n")
			r.upkeep.look()
			if got, want := r.exports.report(), (status.Export{Dir: dir}); got != want {
				t.Errorf("once the config file names a folder, report() = %+v, want %+v", got, want)
			}
			r.exports.exportNow(t.Context())

			if got := len(fake.folders()) > 0; got != tt.wantExported {
				t.Errorf("the next round exported into %q, want an export %v", fake.folders(), tt.wantExported)
			}
		})
	}
}

// exportingConfig is the config of a router run as importingConfig's is,
// exporting into a folder of the test's own, which looks at what it was
// started from often, so its first export comes soon after it starts.
func exportingConfig(t *testing.T) Config {
	t.Helper()
	cfg := importingConfig(t)
	cfg.Export = config.Export{Dir: filepath.Join(t.TempDir(), "Switchboard")}
	cfg.WatchEvery = 50 * time.Millisecond
	return cfg
}

// stopsWithin is how long a router's stop, or its restart in place, may take
// with nothing in flight, waiting on no export: a few milliseconds on a
// machine at rest, so one that waited on an export held for seconds would
// take more.
const stopsWithin = time.Second

func TestTheRouterStoppingWaitsOnNoExport(t *testing.T) {
	logstest.Capture(t)
	cfg := exportingConfig(t)
	r := newTestRouterOf(t, cfg)
	held := newHeldExport(t)
	r.exports.exportInto = held.into
	_, stop := runImporting(t, r)
	// Released before the router's stopped as the test ends, so a stop that
	// waits on it fails the test rather than hang it.
	t.Cleanup(held.release)
	held.await(t)

	stopped := make(chan error, 1)
	go func() { stopped <- stop() }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("Run() = %v, want nil", err)
		}
	case <-time.After(stopsWithin):
		t.Fatalf("the router's stop took more than %v, waiting on the export held", stopsWithin)
	}
	held.release()
	waitFor(t, "the export ends", func() bool { return r.exports.idle() })
}

func TestTheRouterRestartingInPlaceWaitsOnNoExport(t *testing.T) {
	logstest.Capture(t)
	cfg := exportingConfig(t)
	binary := filepath.Join(t.TempDir(), "switchboard")
	stageFile(t, binary, []byte("switchboard\n"))
	cfg.Binary, cfg.Supervised = Watch(binary), true
	cfg.Args, cfg.Environ = []string{binary, "serve"}, []string{"HOME=/Users/test"}
	execs := make(chan string, 1)
	cfg.Exec = func(path string, _, _ []string) error {
		execs <- path
		return nil
	}
	r := newTestRouterOf(t, cfg)
	held := newHeldExport(t)
	r.exports.exportInto = held.into
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- r.run(ctx) }()
	client := NewClient(SocketPath(cfg.StateDir))
	waitFor(t, "the router answers", func() bool {
		_, err := client.Health(t.Context())
		return err == nil
	})
	held.await(t)

	if _, err := client.Restart(t.Context()); err != nil {
		t.Fatalf("Restart() = %v", err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Errorf("Run() = %v, want nil", err)
		}
	case <-time.After(stopsWithin):
		t.Fatalf("the router's restart in place took more than %v, waiting on the export held", stopsWithin)
	}
	select {
	case path := <-execs:
		if path != binary {
			t.Errorf("the router replaced itself with %s, want %s", path, binary)
		}
	default:
		t.Error("the router stopped without replacing itself, want it handing over and exec'ing its binary")
	}
	held.release()
	waitFor(t, "the export ends", func() bool { return r.exports.idle() })
}
