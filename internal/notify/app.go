package notify

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/config"
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
	// refuses its notifications, asking nobody.
	lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	// registerTimeout bounds registering the app, which takes well under a
	// second.
	registerTimeout = 10 * time.Second
	// lockName is the file beside the app whose lock writers take turns
	// under; lockWait is how long one waits for it, and lockPoll how often it
	// tries for it meanwhile.
	lockName = "." + appName + ".lock"
	lockWait = 30 * time.Second
	lockPoll = 100 * time.Millisecond
)

// app is switchboard's notifications' app, embedded zipped, and where it's
// written out: once a process, as ready says.
type app struct {
	zipped []byte
	// path returns where it's written out.
	path func() (string, error)
	// wait is how long it waits for another switchboard writing it out.
	wait time.Duration
	// rename renames a file, as os.Rename does: tests replace it to have
	// putting the app in place fail.
	rename func(oldpath, newpath string) error

	once sync.Once
	// exe is the app's executable once it's written out and registered, ""
	// where it couldn't be.
	exe string
}

// newApp returns the app zipped, written out where getenv and homeDir find
// it, as config.AppPath does, or nil where nothing's zipped.
func newApp(zipped []byte, getenv func(string) string, homeDir func() (string, error)) *app {
	if len(zipped) == 0 {
		return nil
	}
	return &app{
		zipped: zipped,
		path:   func() (string, error) { return config.AppPath(getenv, homeDir) },
		wait:   lockWait,
		rename: os.Rename,
	}
}

// ready returns the app's executable, written out and registered, and
// whether it is: it's done once a process, any other asking meanwhile
// waiting for it. One that can't be is logged once, and none is ready for
// the rest of the process, which posts through osascript.
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
		a.exe = exe
	})
	return a.exe, a.exe != ""
}

// writeOut writes the app out, where what's there isn't it already, and
// registers it, returning its executable, holding the lock beside it
// throughout, so a second switchboard writing it out at once finds it written
// and only registers it. It registers the app even where it was there
// already, as Launch Services can lose an app it knew, as when its database
// is reset, and refuses its notifications then, asking nobody.
func (d Desktop) writeOut(a *app) (string, error) {
	path, err := a.path()
	if err != nil {
		return "", err
	}
	unlock, err := lockBeside(path, a.wait)
	if err != nil {
		return "", err
	}
	defer unlock()
	if err := writeApp(a.zipped, path, a.rename); err != nil {
		return "", err
	}
	if err := d.register(path); err != nil {
		return "", err
	}
	return filepath.Join(path, executable), nil
}

// lockBeside takes the lock beside the app at path, making its directory
// where it isn't there, waiting at most wait while another holds it.
func lockBeside(path string, wait time.Duration) (unlock func(), err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("make the notifications' app's directory: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, lockName), os.O_RDWR|os.O_CREATE, 0o600)
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

// register registers the app at path with Launch Services.
func (d Desktop) register(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), registerTimeout)
	defer cancel()
	if err := d.run(ctx, d.env, lsregister, "-f", path); err != nil {
		return fmt.Errorf("register %s with Launch Services: %w", path, err)
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

// sumOf is the SHA-256 of zipped, in hex.
func sumOf(zipped []byte) string {
	sum := sha256.Sum256(zipped)
	return hex.EncodeToString(sum[:])
}

// unzip unzips zipped into dir, each file with the permissions it was zipped
// with, refusing an entry that would land outside dir, a link, or anything
// else but a plain file or folder, and a zip that holds no app.
func unzip(zipped []byte, dir string) error {
	r, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil {
		return fmt.Errorf("read the notifications' app's zip: %w", err)
	}
	for _, f := range r.File {
		if err := extract(f, dir); err != nil {
			return fmt.Errorf("unzip the notifications' app: %w", err)
		}
	}
	if !runnable(filepath.Join(dir, appName, executable)) {
		return fmt.Errorf("the notifications' app's zip holds no %s that can be run", filepath.Join(appName, executable))
	}
	return nil
}

// runnable reports whether the file at path is a plain file that can be run.
func runnable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0
}

// extract unzips f into dir.
func extract(f *zip.File, dir string) error {
	name := filepath.FromSlash(f.Name)
	if !filepath.IsLocal(name) {
		return fmt.Errorf("%q would land outside its folder", f.Name)
	}
	target := filepath.Join(dir, name)
	mode := f.Mode()
	switch {
	case mode.IsDir():
		return os.MkdirAll(target, 0o755)
	case mode&fs.ModeSymlink != 0:
		return fmt.Errorf("%q is a link", f.Name)
	case !mode.IsRegular():
		return fmt.Errorf("%q is neither a plain file nor a folder", f.Name)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return writeFile(f, target, mode.Perm())
}

// writeFile writes what f holds to a new file at target, with the
// permissions perm, whatever the umask.
func writeFile(f *zip.File, target string, perm fs.FileMode) error {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	if err == nil {
		err = dst.Chmod(perm)
	}
	return errors.Join(err, dst.Close())
}
