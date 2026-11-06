package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

const (
	// appName is the app's folder, at the zip's root.
	appName = "Switchboard.app"
	// executable is the app's program, within its folder.
	executable = "Contents/MacOS/Switchboard"
	// sumName is the file beside the app holding the SHA-256 of the zip it
	// was written from.
	sumName = "." + appName + ".sum"
	// stagingPattern names the hidden folder beside the app that a new one is
	// unzipped into, and the one there renamed out into.
	stagingPattern = "." + appName + ".new-*"
	// lsregister registers an app with Launch Services, without which macOS
	// refuses its notifications, asking nobody, and unregisters one.
	lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	// registerTimeout bounds registering the app, or unregistering it, which
	// takes well under a second.
	registerTimeout = 10 * time.Second
	// lockName is the file beside the app whose lock writers take turns
	// under; lockWait is how long one waits for it, and lockPoll how often it
	// tries for it meanwhile.
	lockName = "." + appName + ".lock"
	lockWait = 30 * time.Second
	lockPoll = 100 * time.Millisecond
	// askWait bounds asking macOS whether the app may post: a little past
	// the 300 seconds the app gives the user to answer.
	askWait = 310 * time.Second
)

// app is switchboard's notifications' app, embedded zipped, written out,
// registered and asked about once a process, as ready says.
type app struct {
	zipped []byte
	// ask is how long it waits for the answer to whether it may post.
	ask time.Duration
	// rename renames a file, as os.Rename does: tests replace it to have
	// putting the app in place fail.
	rename func(oldpath, newpath string) error

	// started is set by the first to ready the app, and readied closed once
	// that's done, exe then its executable, "" where it couldn't be written
	// out and registered.
	started atomic.Bool
	readied chan struct{}
	exe     string
}

// newApp returns the app zipped, or nil where nothing's zipped.
func newApp(zipped []byte) *app {
	if len(zipped) == 0 {
		return nil
	}
	return &app{zipped: zipped, ask: askWait, rename: os.Rename, readied: make(chan struct{})}
}

// ready returns the app's executable, as readyApp readies it, once a process,
// under the context of the first to ask for it: any other waits for that,
// until its own context ends, when it returns that context's error, as the
// first does where its context cuts the readying short. It returns "" where
// there's no app to post through, as none is embedded, or it couldn't be
// written out and registered.
func (d Desktop) ready(ctx context.Context) (string, error) {
	a := d.app
	if a == nil {
		return "", nil
	}
	if a.started.CompareAndSwap(false, true) {
		exe, err := d.readyApp(ctx, a)
		a.exe = exe
		close(a.readied)
		if err != nil {
			return "", err
		}
	}
	// Once readied, the app is posted through whatever ctx says, which a
	// select of both, choosing at random, wouldn't promise.
	select {
	case <-a.readied:
		return a.exe, nil
	default:
	}
	select {
	case <-a.readied:
		return a.exe, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// readyApp writes the app out and registers it, then has it ask macOS whether
// it may post, waiting for the answer, so the first post waits on the
// question, returning its executable: "" where it can't be written out or
// registered, which is logged, the posts then going through osascript. All of
// it ends once ctx does, returning ctx's error, a readying cut short that
// warns of nothing, and the executable where only the answer was still to
// come.
func (d Desktop) readyApp(ctx context.Context, a *app) (string, error) {
	exe, err := d.writeOut(ctx, a)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		logger.Warn("can't write out the notifications' app, so notifications are posted through osascript", "error", err)
		return "", nil
	}
	return exe, d.authorize(ctx, exe, a.ask)
}

// writeOut writes the app out, where what's there isn't it already, and
// registers it, returning its executable, holding the lock beside it
// throughout, so a second switchboard writing it out at once finds it written
// and only registers it; it sweeps away first what writes cut short left. It
// registers the app even where it was there already, as Launch Services can
// lose an app it knew, as when its database is reset, and refuses its
// notifications then, asking nobody.
func (d Desktop) writeOut(ctx context.Context, a *app) (string, error) {
	path, err := d.appPath()
	if err != nil {
		return "", err
	}
	unlock, err := lockBeside(ctx, path, d.lockWait)
	if err != nil {
		return "", err
	}
	defer unlock()
	if err := sweep(filepath.Dir(path)); err != nil {
		return "", err
	}
	if err := writeApp(a.zipped, path, a.rename); err != nil {
		return "", err
	}
	if err := d.launchServices(ctx, "-f", path); err != nil {
		return "", fmt.Errorf("register %s with Launch Services: %w", path, err)
	}
	return filepath.Join(path, executable), nil
}

// authorize has the app at exe ask macOS whether it may post, which asks the
// user the first time, waiting at most wait for the answer, or until ctx
// ends, which ends the asking and returns ctx's error. Allowed or not, it's
// answered: a post not allowed fails as ErrNotAllowed. One that fails
// otherwise, or isn't answered in time, is logged, and the posts go on
// through the app.
func (d Desktop) authorize(ctx context.Context, exe string, wait time.Duration) error {
	asking, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	err := d.run(asking, d.env, exe, "--authorize")
	switch {
	case err == nil || notAllowedBy(err):
		return nil
	case ctx.Err() != nil:
		return ctx.Err()
	case asking.Err() != nil:
		err = fmt.Errorf("not answered within %s: %w", wait, err)
	}
	logger.Warn("can't ask macOS whether Switchboard may post notifications; posting through its app all the same", "error", err)
	return nil
}

// Uninstall removes the app written out, as the service's uninstall does:
// under the lock beside it, so no switchboard writes it meanwhile, it sweeps
// away what writes cut short left, unregisters the app from Launch Services,
// where it's there, and removes it, with its sum, then lets the lock go and
// removes that. One that can't be unregistered is removed all the same, and
// the error says so. Where the lock isn't had, it removes nothing; where the
// app's directory isn't there, there's nothing to remove.
func (d Desktop) Uninstall(ctx context.Context) error {
	if d.goos != "darwin" {
		return nil
	}
	path, err := d.appPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Dir(path)); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	unlock, err := lockBeside(ctx, path, d.lockWait)
	if err != nil {
		return fmt.Errorf("remove the notifications' app: %w", err)
	}
	errs := d.remove(ctx, path)
	unlock()
	if err := errors.Join(append(errs, removeFile(lockPath(path)))...); err != nil {
		return fmt.Errorf("remove the notifications' app: %w", err)
	}
	return nil
}

// remove sweeps away what writes cut short left beside the app at path,
// unregisters it from Launch Services, where it's there, and removes it, with
// its sum, returning what failed.
func (d Desktop) remove(ctx context.Context, path string) []error {
	errs := []error{sweep(filepath.Dir(path))}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		if err := d.launchServices(ctx, "-u", path); err != nil {
			errs = append(errs, fmt.Errorf("unregister %s from Launch Services: %w", path, err))
		}
	}
	return append(errs, os.RemoveAll(path), removeFile(sumPath(path)))
}

// removeFile removes the file at path, where it's there.
func removeFile(path string) error {
	if err := os.Remove(path); !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// launchServices runs lsregister with flag on the app at path.
func (d Desktop) launchServices(ctx context.Context, flag, path string) error {
	ctx, cancel := context.WithTimeout(ctx, registerTimeout)
	defer cancel()
	return d.run(ctx, d.env, lsregister, flag, path)
}

// lockBeside takes the lock beside the app at path, making its directory
// where it isn't there, waiting at most wait while another holds it, and
// taking none once ctx has ended.
func lockBeside(ctx context.Context, path string, wait time.Duration) (unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("make the notifications' app's directory: %w", err)
	}
	f, err := os.OpenFile(lockPath(path), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open the notifications' app's lock: %w", err)
	}
	deadline := time.Now().Add(wait)
	for ctx.Err() == nil {
		taken, err := tryLock(f)
		switch {
		case err != nil:
			_ = f.Close()
			return nil, fmt.Errorf("lock the notifications' app, %s: %w", f.Name(), err)
		case taken:
			return func() { _ = f.Close() }, nil
		case !time.Now().Before(deadline):
			_ = f.Close()
			return nil, fmt.Errorf("another switchboard held the notifications' app's lock, %s, past %s", f.Name(), wait)
		}
		select {
		case <-ctx.Done():
		case <-time.After(min(lockPoll, time.Until(deadline))):
		}
	}
	_ = f.Close()
	return nil, fmt.Errorf("wait for the notifications' app's lock: %w", ctx.Err())
}

// sweep removes the folders writes cut short left in dir, beside the app: no
// other writer is mid-write while the lock is held.
func sweep(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("look for what writes of the notifications' app cut short left: %w", err)
	}
	for _, e := range entries {
		// Match fails only on a malformed pattern, which stagingPattern isn't.
		if left, _ := filepath.Match(stagingPattern, e.Name()); !left {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("remove what a write of the notifications' app cut short left: %w", err)
		}
	}
	return nil
}

// writeApp writes the app zipped out at path, unless what's there is it
// already, as the sum beside it says: it's unzipped into a hidden folder
// beside path, the one there renamed out into that folder, so a running app
// is never written into, and the new one renamed in, then the folder
// removed, and the sum written. Its directory must be there, as the lock
// beside the app makes it.
func writeApp(zipped []byte, path string, rename func(oldpath, newpath string) error) error {
	sum := sumOf(zipped)
	if current(path, sum) {
		return nil
	}
	staging, err := os.MkdirTemp(filepath.Dir(path), stagingPattern)
	if err != nil {
		return fmt.Errorf("make a folder to unzip the notifications' app in: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	if err := unzip(zipped, staging); err != nil {
		return err
	}
	if err := swapIn(filepath.Join(staging, appName), path, filepath.Join(staging, "old"), rename); err != nil {
		return err
	}
	if err := atomicfile.Write(sumPath(path), []byte(sum+"\n"), 0o644); err != nil {
		return fmt.Errorf("note which notifications' app is written out: %w", err)
	}
	return nil
}

// swapIn renames the app at path out, to aside, where there's one, then the
// one fresh in to path, renaming the one aside back where that fails.
func swapIn(fresh, path, aside string, rename func(oldpath, newpath string) error) error {
	err := rename(path, aside)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("move the notifications' app there out of the way: %w", err)
	}
	movedAside := err == nil
	if err := rename(fresh, path); err != nil {
		err = fmt.Errorf("put the notifications' app in place: %w", err)
		if movedAside {
			if back := rename(aside, path); back != nil {
				err = errors.Join(err, fmt.Errorf("put the one there back: %w", back))
			}
		}
		return err
	}
	return nil
}

// current reports whether the app at path is the one zipped with the given
// sum: the sum beside it is that, and its executable is there to be run.
func current(path, sum string) bool {
	held, err := os.ReadFile(sumPath(path))
	return err == nil && strings.TrimSpace(string(held)) == sum && runnable(filepath.Join(path, executable))
}

// sumPath is where the sum of the app at path is kept, beside it.
func sumPath(path string) string {
	return filepath.Join(filepath.Dir(path), sumName)
}

// lockPath is where the lock of the app at path is, beside it.
func lockPath(path string) string {
	return filepath.Join(filepath.Dir(path), lockName)
}

// sumOf is the SHA-256 of zipped, in hex.
func sumOf(zipped []byte) string {
	sum := sha256.Sum256(zipped)
	return hex.EncodeToString(sum[:])
}
