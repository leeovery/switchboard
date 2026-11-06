package notify

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

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
