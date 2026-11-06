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
	"sync"
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
	// wait is how long it waits for another switchboard writing it out, and
	// ask how long for the answer to whether it may post.
	wait, ask time.Duration
	// rename renames a file, as os.Rename does: tests replace it to have
	// putting the app in place fail.
	rename func(oldpath, newpath string) error

	once sync.Once
	// exe is the app's executable once it's written out and registered, ""
	// where it couldn't be.
	exe string
}

// newApp returns the app zipped, or nil where nothing's zipped.
func newApp(zipped []byte) *app {
	if len(zipped) == 0 {
		return nil
	}
	return &app{zipped: zipped, wait: lockWait, ask: askWait, rename: os.Rename}
}

// ready returns the app's executable, written out and registered, and
// whether it is, having asked macOS whether it may post, waiting for the
// answer: it's done once a process, any other asking meanwhile waiting for
// it, so the first post waits on the question. One that can't be written out
// or registered is logged once, and none is ready for the rest of the
// process, which posts through osascript.
func (d Desktop) ready() (string, bool) {
	a := d.app
	if a == nil {
		return "", false
	}
	a.once.Do(func() {
		exe, err := d.writeOut(a)
		if err != nil {
			logger.Warn("can't write out the notifications' app, so notifications are posted through osascript", "error", err)
			return
		}
		d.authorize(exe, a.ask)
		a.exe = exe
	})
	return a.exe, a.exe != ""
}

// writeOut writes the app out, where what's there isn't it already, and
// registers it, returning its executable, holding the lock beside it
// throughout, so a second switchboard writing it out at once finds it written
// and only registers it; it sweeps away first what writes cut short left. It
// registers the app even where it was there already, as Launch Services can
// lose an app it knew, as when its database is reset, and refuses its
// notifications then, asking nobody.
func (d Desktop) writeOut(a *app) (string, error) {
	path, err := d.appPath()
	if err != nil {
		return "", err
	}
	unlock, err := lockBeside(path, a.wait)
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
	if err := d.launchServices("-f", path); err != nil {
		return "", fmt.Errorf("register %s with Launch Services: %w", path, err)
	}
	return filepath.Join(path, executable), nil
}

// authorize has the app at exe ask macOS whether it may post, which asks the
// user the first time, waiting at most wait for the answer. Allowed or not,
// it's answered: a post not allowed fails as ErrNotAllowed. One that fails
// otherwise, or isn't answered in time, is logged, and the posts go on
// through the app.
func (d Desktop) authorize(exe string, wait time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	err := d.run(ctx, d.env, exe, "--authorize")
	if err == nil || notAllowedBy(err) {
		return
	}
	if ctx.Err() != nil {
		err = fmt.Errorf("not answered within %s: %w", wait, err)
	}
	logger.Warn("can't ask macOS whether Switchboard may post notifications; posting through its app all the same", "error", err)
}

// Uninstall unregisters the app written out from Launch Services and removes
// it, with its sum and its lock, as the service's uninstall does: where
// there's no app, it does nothing. One that can't be unregistered is logged,
// and removed all the same.
func (d Desktop) Uninstall() error {
	if d.goos != "darwin" {
		return nil
	}
	path, err := d.appPath()
	if err != nil {
		return err
	}
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err := d.launchServices("-u", path); err != nil {
		logger.Warn("can't unregister the notifications' app from Launch Services; removing it all the same", "path", path, "error", err)
	}
	errs := []error{os.RemoveAll(path)}
	for _, beside := range []string{sumPath(path), lockPath(path)} {
		if err := os.Remove(beside); !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("remove the notifications' app: %w", err)
	}
	return nil
}

// launchServices runs lsregister with flag on the app at path.
func (d Desktop) launchServices(flag, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), registerTimeout)
	defer cancel()
	return d.run(ctx, d.env, lsregister, flag, path)
}

// lockBeside takes the lock beside the app at path, making its directory
// where it isn't there, waiting at most wait while another holds it.
func lockBeside(path string, wait time.Duration) (unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("make the notifications' app's directory: %w", err)
	}
	f, err := os.OpenFile(lockPath(path), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open the notifications' app's lock: %w", err)
	}
	deadline := time.Now().Add(wait)
	for {
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
		time.Sleep(min(lockPoll, time.Until(deadline)))
	}
}

// sweep removes the folders writes cut short left in dir, beside the app: no
// other writer is mid-write while the lock is held.
func sweep(dir string) error {
	left, err := filepath.Glob(filepath.Join(dir, stagingPattern))
	if err != nil {
		return err
	}
	for _, folder := range left {
		if err := os.RemoveAll(folder); err != nil {
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
