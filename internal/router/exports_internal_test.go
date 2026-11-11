package router

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
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

// testExports are the service's router's exports into the folder dir names,
// by start's clock, exporting with exportInto, the history's writers tended
// at once, the home directory /Users/test, and no restart coming.
func testExports(dir string, exportInto func(dir string) (export.Result, error)) *exports {
	x := &exports{
		every:      time.Hour,
		now:        at(start),
		homeDir:    func() (string, error) { return "/Users/test", nil },
		exportInto: exportInto,
		tend:       tended,
		restarting: noRestart,
		moving:     noMove,
		supervised: true,
		running:    make(map[string]time.Time),
		underWay:   newInFlight(),
	}
	x.configure(config.Export{Dir: dir})
	return x
}

// tended has the history's writers tended at once, as none is kept.
func tended(context.Context) error { return nil }

// noRestart is a router that isn't to restart.
func noRestart() bool { return false }

// noMove is a router whose history waits on no move.
func noMove() bool { return false }

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
			name: "a rename refused onto a file that isn't locked",
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

func TestMacOSRefusingTheRouterSaysWhatToAllow(t *testing.T) {
	const dir = "/Users/test/Documents/Switchboard"
	refused := fmt.Errorf("can't export into %s: %w", dir, &fs.PathError{Op: "mkdir", Path: filepath.Join(dir, "mac-0123456789abcdef"), Err: syscall.EPERM})
	tests := []struct {
		name       string
		supervised bool
		want       string
	}{
		{name: "the service's router", supervised: true, want: macOSRefused(dir)},
		{
			name: "a router run by hand, which macOS counts as part of its terminal",
			want: "macOS refused access to " + dir + ": allow the terminal app the router runs in, in System Settings → Privacy & Security",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			cfg := testConfig(at(start), &stubProber{}, testTokens.Read)
			cfg.Export, cfg.Supervised = config.Export{Dir: dir}, tt.supervised
			x := newExports(cfg, tended, noRestart, noMove)
			x.exportInto = func(string) (export.Result, error) { return export.Result{}, refused }

			x.exportNow(t.Context())

			if got := x.report().Error; got != tt.want {
				t.Errorf("report().Error = %q, want %q", got, tt.want)
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
			x := newExports(cfg, tended, noRestart, noMove)

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
	x := newExports(cfg, tended, noRestart, noMove)

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

func TestWhatAnExportPassesOverIsWarnedOfOnceAndFailsNoExport(t *testing.T) {
	const (
		dir    = "/Volumes/Backup/Switchboard"
		folder = dir + "/mac-0123456789abcdef"
	)
	kinds := []struct {
		name string
		// met is what an export that meets what's warned of did.
		met    export.Result
		warned []string
	}{
		{
			name:   "a day's file it couldn't read",
			met:    export.Result{Unread: []export.DayError{{What: "the request ledger of 2026-09-26", Date: "2026-09-26", Err: errors.New("permission denied")}}},
			warned: []string{"level=WARN", `msg="couldn't export the request ledger of 2026-09-26; passed over"`, `error="permission denied"`},
		},
		{
			name: "a file a write cut short left that it couldn't remove",
			met:  export.Result{Unremoved: []export.Leftover{{Path: folder + "/ledger/.requests-2026-09-26.jsonl.gz.123", Err: errors.New("permission denied")}}},
			warned: []string{
				"level=WARN", `msg="can't remove a file an export cut short left; trying again at each round"`,
				"path=" + folder + "/ledger/.requests-2026-09-26.jsonl.gz.123", `error="permission denied"`,
			},
		},
	}
	steps := []struct {
		name       string
		met        bool
		wantWarned int
	}{
		{name: "met", met: true, wantWarned: 1},
		{name: "met again", met: true, wantWarned: 1},
		{name: "not met", wantWarned: 1},
		{name: "met once more", met: true, wantWarned: 2},
	}
	for _, kind := range kinds {
		t.Run(kind.name, func(t *testing.T) {
			log := logstest.Capture(t)
			fake := &exportAs{}
			x := testExports(dir, fake.into)
			for _, step := range steps {
				var r export.Result
				if step.met {
					r = kind.met
				}
				r.Folder, r.AsTheyWere = folder, 1
				fake.set(r, nil)
				x.exportNow(t.Context())

				if got, want := x.report(), (status.Export{Dir: dir, At: start}); got != want {
					t.Errorf("%s, report() = %+v, want %+v", step.name, got, want)
				}
				if got := timesLogged(log, kind.warned...); got != step.wantWarned {
					t.Errorf("%s, it's warned of %d times, want %d; log reads\n%s", step.name, got, step.wantWarned, log)
				}
				if log.Has(exportFailed) {
					t.Errorf("%s, log reads\n%s\nwant no failed export", step.name, log)
				}
			}
		})
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
	cfg, binary, execs := replacingConfig(t)
	cfg.ConfigFile = Watch(s.config)
	// Long enough that an export begun as the router starts is asked well
	// before its first look.
	cfg.WatchEvery = 500 * time.Millisecond
	r := newTestRouterOf(t, cfg)
	held := newHeldExport(t)
	r.exports.exportInto = held.into
	// The config file changes as the router starts, so its first look finds
	// a restart due.
	writeFile(t, s.kept, twoAccounts)

	finished := runRouter(t, r)

	wantReplaced(t, finished, execs, binary, 5*time.Second)
	held.release()
	waitFor(t, "no export runs", func() bool { return r.exports.idle() })
	if got := held.asked.Load(); got != 0 {
		t.Errorf("an export was asked %d times, want none: the router it becomes exports as it starts", got)
	}
}

func TestTheFirstLookEndsOnceARestartDueIsKnown(t *testing.T) {
	logstest.Capture(t)
	s := newStartedFrom(t)
	cfg := testConfig(at(start), &stubProber{}, testTokens.Read)
	cfg.ConfigFile, cfg.Supervised = Watch(s.config), true
	r := newTestRouterOf(t, cfg)
	firstLook := r.upkeep.firstLook()
	var lookedAsDue bool
	r.upkeep.restarts.emit = func(Event) {
		select {
		case <-firstLook:
			lookedAsDue = true
		default:
		}
	}
	writeFile(t, s.kept, twoAccounts)

	r.upkeep.look()

	if lookedAsDue {
		t.Error("the first look ended before the restart it found due was known, want it ending after")
	}
	select {
	case <-firstLook:
	default:
		t.Error("the first look hasn't ended, want it ended once upkeep has looked")
	}
	if !r.restartComing() {
		t.Error("restartComing() = false after the first look, want a restart coming, as the config file changed")
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

func TestNoExportStartsWhileTheHistoryWaitsOnItsMove(t *testing.T) {
	logstest.Capture(t)
	fake := &exportAs{}
	x := testExports("/Volumes/Backup/Switchboard", fake.into)
	moving := true
	x.moving = func() bool { return moving }

	x.exportNow(t.Context())
	if got := fake.folders(); len(got) != 0 {
		t.Errorf("while the history waited on its move, exported into %q, want none", got)
	}
	moving = false
	x.exportNow(t.Context())
	if got, want := fake.folders(), []string{"/Volumes/Backup/Switchboard"}; !slices.Equal(got, want) {
		t.Errorf("once the history was moved, exported into %q, want %q", got, want)
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

func TestARestartDueWaitsForTheExportUnderWayToEnd(t *testing.T) {
	logstest.Capture(t)
	s := newStartedFrom(t)
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)
	held := newHeldExport(t)
	x := testExports("/Volumes/Backup/Switchboard", held.into)
	r.exports = x
	x.round(t.Context())
	held.await(t)

	configChanged(t, s, r)
	if !r.coming() {
		t.Error("coming() = false with the export under way, want the restart coming, so no other export begins")
	}
	select {
	case <-r.ready():
		t.Fatal("ready() closed with an export under way, want it waiting for the export to end")
	default:
	}
	putOff(t, r)

	held.release()
	select {
	case <-r.ready():
	case <-time.After(5 * time.Second):
		t.Fatal("ready() didn't close once the export ended")
	}
	if !r.restart(t.Context()) {
		t.Error("restart() = false once the export ended, want the router restarting")
	}
}

func TestARestartAskedForGoesOverNoExport(t *testing.T) {
	const dir = "/Volumes/Backup/Switchboard"
	tests := []struct {
		name string
		// running is whether an export runs as the restart is asked for;
		// elsewhere whether the config has moved the exports on to another
		// folder since it began; and gone whether the router's binary is
		// gone, which refuses the restart.
		running, elsewhere, gone bool
		// wantErr is why the restart is refused, the router started from s:
		// "" for none.
		wantErr func(s startedFrom) string
		// wantExported is whether a round after exports: not while the
		// restart cleared holds every export off.
		wantExported bool
	}{
		{
			name: "refused while an export runs", running: true,
			wantErr:      func(startedFrom) string { return "an export into " + dir + " is under way: try again once it ends" },
			wantExported: true,
		},
		{
			name: "refused while an export runs into a folder no longer configured", running: true, elsewhere: true,
			wantErr:      func(startedFrom) string { return "an export into " + dir + " is under way: try again once it ends" },
			wantExported: true,
		},
		{name: "cleared with none running", wantErr: func(startedFrom) string { return "" }},
		{
			name: "refused for another reason", gone: true,
			wantErr: func(s startedFrom) string {
				return "the router's binary, " + s.binary + ", isn't there, as for a moment while an upgrade moves it on: try again once it is"
			},
			wantExported: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			s := newStartedFrom(t)
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)
			held := newHeldExport(t)
			x := testExports(dir, held.into)
			r.exports = x
			if tt.running {
				x.round(t.Context())
				held.await(t)
			}
			if tt.elsewhere {
				x.configure(config.Export{Dir: "/Volumes/Other/Switchboard"})
			}
			if tt.gone {
				remove(t, s.binary)
			}

			_, err := r.restartable(t.Context())
			if got, want := fmt.Sprint(err), cmp.Or(tt.wantErr(s), "<nil>"); got != want {
				t.Errorf("restartable() = %s, want %s", got, want)
			}
			held.release()
			waitFor(t, "no export runs", func() bool { return x.idle() })
			before := held.asked.Load()
			x.exportNow(t.Context())
			if got := held.asked.Load() > before; got != tt.wantExported {
				t.Errorf("a round after exported %v, want %v", got, tt.wantExported)
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

func TestARestartReportsTheExportUnderWayOnlyWhereThatHoldsIt(t *testing.T) {
	const dir = "/Volumes/Backup/Switchboard"
	tests := []struct {
		name       string
		supervised bool
		// held is whether the binary refuses to start as the restart is
		// tried, before the export begins, which holds the restart.
		held bool
		want string
	}{
		{name: "the service's router, waiting for the export", supervised: true, want: dir},
		{name: "a router run by hand, which waits for nothing"},
		{name: "held, its binary refusing to start, which the export ending gets no further", supervised: true, held: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newStartedFrom(t)
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), tt.supervised, newInFlight(), at(start), unheeded)
			exporting := newHeldExport(t)
			x := testExports(dir, exporting.into)
			r.exports = x
			askedOf(r, binaryAnswer{refusal: "invalid config"})
			relink(t, s.binary, s.version("1.1"))
			if tt.held {
				r.look()
				r.restart(t.Context())
			}
			x.round(t.Context())
			exporting.await(t)
			r.look()

			got := r.report()
			if got.Exporting != tt.want || got.ByHand == tt.supervised || (got.Held != "") != tt.held {
				t.Errorf("report() = %+v, want the restart waiting on the export into %q", got, tt.want)
			}
			if logged := log.Has(`msg="restart due; restarting once the export under way ends`); logged != (tt.want != "") {
				t.Errorf("log reads\n%s\nwant the restart due logged as waiting on the export: %v", log, tt.want != "")
			}
		})
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

func TestARestartDueMidExportWaitsForTheExportToEnd(t *testing.T) {
	log := logstest.Capture(t)
	s := newStartedFrom(t)
	cfg, binary, execs := replacingConfig(t)
	cfg.ConfigFile = Watch(s.config)
	r := newTestRouterOf(t, cfg)
	held := newHeldExport(t)
	r.exports.exportInto = held.into
	finished := runRouter(t, r)
	held.await(t)

	writeFile(t, s.kept, twoAccounts)
	waitFor(t, "the restart falls due", func() bool { return r.upkeep.restartDue().Due() })
	// Looks enough for a restart that didn't wait on the export to have come.
	select {
	case err := <-finished:
		t.Fatalf("the router stopped with an export under way, Run() = %v, want it waiting until the export ends", err)
	case <-time.After(10 * cfg.WatchEvery):
	}
	if _, err := NewClient(SocketPath(cfg.StateDir)).Health(t.Context()); err != nil {
		t.Errorf("Health() while the restart waits = %v, want the router answering", err)
	}
	if !log.Has("level=INFO", `msg="restart due; restarting once the export under way ends and no request is in flight"`, "dir="+cfg.Export.Dir) {
		t.Errorf("log reads\n%s\nwant the restart due logged as waiting on the export", log)
	}
	held.release()

	wantReplaced(t, finished, execs, binary, 5*time.Second)
}

func TestTheStatusSaysWhichExportARestartWaitsFor(t *testing.T) {
	tests := []struct {
		name string
		// kept is whether the config change that makes the restart due keeps
		// the folder the export runs into, else dropping [export].
		kept bool
	}{
		{name: "the folder kept", kept: true},
		{name: "the folder dropped"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			s := newStartedFrom(t)
			cfg, _, _ := replacingConfig(t)
			cfg.ConfigFile = Watch(s.config)
			r := newTestRouterOf(t, cfg)
			held := newHeldExport(t)
			r.exports.exportInto = held.into
			runRouter(t, r)
			client := NewClient(SocketPath(cfg.StateDir))
			held.await(t)
			changed := twoAccounts
			if tt.kept {
				changed += fmt.Sprintf("\n[export]\ndir = %q\n", cfg.Export.Dir)
			}
			writeFile(t, s.kept, changed)
			waitFor(t, "the restart falls due", func() bool { return r.upkeep.restartDue().Due() })
			configured := ""
			if tt.kept {
				configured = cfg.Export.Dir
			}
			waitFor(t, "the exports take the config up", func() bool { return r.exports.report().Dir == configured })

			doc, err := client.Status(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if got := doc.Restart.Exporting; got != cfg.Export.Dir {
				t.Errorf("the document's restart waits on the export into %q, want %q", got, cfg.Export.Dir)
			}
			text := doc.Restart.Text(time.Now())
			if !strings.Contains(text, "once the export into "+cfg.Export.Dir+" ends") || strings.Contains(text, "restarts it now") {
				t.Errorf("Restart.Text() = %q, want it waiting on the export, offering no service restart", text)
			}
			_, err = client.Restart(t.Context())
			if under, ok := errors.AsType[ExportUnderWay](err); !ok || under.Dir != cfg.Export.Dir {
				t.Errorf("Restart() = %v, want it refused for the export into %s", err, cfg.Export.Dir)
			}
		})
	}
}

func TestARestartAskedForWhileAnExportRunsIsRefused(t *testing.T) {
	logstest.Capture(t)
	cfg, binary, execs := replacingConfig(t)
	r := newTestRouterOf(t, cfg)
	held := newHeldExport(t)
	r.exports.exportInto = held.into
	finished := runRouter(t, r)
	client := NewClient(SocketPath(cfg.StateDir))
	held.await(t)

	_, err := client.Restart(t.Context())
	under, typed := errors.AsType[ExportUnderWay](err)
	if want := "an export into " + cfg.Export.Dir + " is under way: try again once it ends"; !typed || under.Dir != cfg.Export.Dir || err.Error() != want {
		t.Errorf("Restart() with an export under way = %v, want ExportUnderWay, %q", err, want)
	}
	if _, err := client.Health(t.Context()); err != nil {
		t.Errorf("Health() once the restart was refused = %v, want the router answering", err)
	}
	held.release()
	waitFor(t, "the export ends", func() bool { return r.exports.idle() })
	if _, err := client.Restart(t.Context()); err != nil {
		t.Fatalf("Restart() once the export ended = %v", err)
	}

	wantReplaced(t, finished, execs, binary, stopsWithin)
}

// replacingConfig is the config of a supervised router run as
// exportingConfig's is, which replaces itself with its binary, returned, as
// it restarts, telling execs of the path it execs.
func replacingConfig(t *testing.T) (Config, string, chan string) {
	t.Helper()
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
	return cfg, binary, execs
}

// runRouter runs r until the test ends, returning what Run's error comes on
// as it returns.
func runRouter(t *testing.T, r *Router) <-chan error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	finished := make(chan error, 1)
	go func() { finished <- r.run(ctx) }()
	return finished
}

// wantReplaced waits up to within for a router's Run to return, as finished
// tells, failing the test where it doesn't, or where it hadn't replaced
// itself with binary, as execs tells.
func wantReplaced(t *testing.T, finished <-chan error, execs <-chan string, binary string, within time.Duration) {
	t.Helper()
	select {
	case err := <-finished:
		if err != nil {
			t.Errorf("Run() = %v, want nil", err)
		}
	case <-time.After(within):
		t.Fatalf("gave up waiting %v for the router to restart", within)
	}
	select {
	case path := <-execs:
		if path != binary {
			t.Errorf("the router replaced itself with %s, want %s", path, binary)
		}
	default:
		t.Error("the router stopped without replacing itself, want it handing over and exec'ing its binary")
	}
}

func TestTheRoutersDocumentSaysHowTheExportsStand(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		want status.Export
	}{
		{name: "into a folder", dir: "/Volumes/Backup/Switchboard", want: status.Export{Dir: "/Volumes/Backup/Switchboard", At: start}},
		{name: "into none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			cfg := testConfig(at(start), &stubProber{}, testTokens.Read)
			cfg.Export = config.Export{Dir: tt.dir}
			r := newTestRouterOf(t, cfg)
			r.exports.exportInto, r.exports.tend = (&exportAs{}).into, tended

			r.exports.exportNow(t.Context())

			doc := r.Status()
			if doc.Export != tt.want {
				t.Errorf("Status().Export = %+v, want %+v", doc.Export, tt.want)
			}
			data, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := bytes.Contains(data, []byte(`"export":`)), tt.dir != ""; got != want {
				t.Errorf("the document reads\n%s\nwant an export %v", data, want)
			}
		})
	}
}
