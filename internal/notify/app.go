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
)

// app is switchboard's notifications' app, embedded zipped, and where it's
// written out: once a process, as ready says.
type app struct {
	zipped []byte
	// path returns where it's written out.
	path func() (string, error)
	// rename renames a file, as os.Rename does: tests replace it to have
	// another writer win.
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
// registers it, returning its executable. It registers the app even where
// it was there already, as Launch Services can lose an app it knew, as when
// its database is reset, and refuses its notifications then, asking nobody.
func (d Desktop) writeOut(a *app) (string, error) {
	path, err := a.path()
	if err != nil {
		return "", err
	}
	if err := writeApp(a.zipped, path, a.rename); err != nil {
		return "", err
	}
	if err := d.register(path); err != nil {
		return "", err
	}
	return filepath.Join(path, executable), nil
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
// removed, and the sum written. One whose rename in fails takes the app
// another writer put there meanwhile, where the sum beside it is the zip's.
func writeApp(zipped []byte, path string, rename func(oldpath, newpath string) error) error {
	sum := sumOf(zipped)
	if current(path, sum) {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("make the notifications' app's directory: %w", err)
	}
	staging, err := os.MkdirTemp(dir, stagingPattern)
	if err != nil {
		return fmt.Errorf("make a folder to unzip the notifications' app in: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	if err := unzip(zipped, staging); err != nil {
		return err
	}
	if err := swapIn(filepath.Join(staging, appName), path, filepath.Join(staging, "old"), rename); err != nil {
		if current(path, sum) {
			return nil
		}
		return err
	}
	if err := atomicfile.Write(sumPath(path), []byte(sum+"\n"), 0o644); err != nil {
		return fmt.Errorf("note which notifications' app is written out: %w", err)
	}
	return nil
}

// swapIn renames the app at path out, to aside, where there's one, then the
// one fresh in to path.
func swapIn(fresh, path, aside string, rename func(oldpath, newpath string) error) error {
	if err := rename(path, aside); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("move the notifications' app there out of the way: %w", err)
	}
	if err := rename(fresh, path); err != nil {
		return fmt.Errorf("put the notifications' app in place: %w", err)
	}
	return nil
}

// current reports whether the app at path is the one zipped with the given
// sum: the sum beside it is that, and its executable is there.
func current(path, sum string) bool {
	held, err := os.ReadFile(sumPath(path))
	if err != nil || strings.TrimSpace(string(held)) != sum {
		return false
	}
	info, err := os.Stat(filepath.Join(path, executable))
	return err == nil && info.Mode().IsRegular()
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
	if info, err := os.Stat(filepath.Join(dir, appName, executable)); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("the notifications' app's zip holds no %s", filepath.Join(appName, executable))
	}
	return nil
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
