package export

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
)

// outcome is what exporting one of a day's files came to.
type outcome int

const (
	// asItWas is a file the folder holds as it is already, passed over.
	asItWas outcome = iota
	// wrote is a file written into the folder.
	wrote
	// gone is a file gone from the state directory since it was listed, as
	// a prune removes a day's, of which the folder is given nothing.
	gone
)

// coarserTimes are the precisions coarser than the nanosecond a folder's
// file system may keep a file's modification time to, by truncating it:
// exFAT's 10 milliseconds, and the whole second of HFS+ and many network
// shares.
var coarserTimes = []time.Duration{10 * time.Millisecond, time.Second}

// unreadable is why a file of the state directory couldn't be read, so its
// day is passed over, as no write into the folder failed.
type unreadable struct {
	error
}

func (u unreadable) Unwrap() error {
	return u.error
}

// dayLines are the files a day of one kind of line in the state directory,
// src, and where the Mac's folder holds them, dst. looked, where it's set, is
// called with each day's date once its files are looked at, before they're
// read, as another process may prune them between the two, as tests do there;
// and wrap, where it's set, is given what a snapshot is written to, and
// returns what to write it to, as tests of writes that fail have it.
type dayLines struct {
	src, dst *dayfile.Files
	looked   func(date string)
	wrap     func(io.Writer) io.Writer
}

// export exports the day with the given date into the folder: its compressed
// file copied, where that's the day's only file, as copyFile copies it; else
// a snapshot of its lines, as snapshot writes one. Where the day's files
// can't be read, it fails with an unreadable.
func (d dayLines) export(date string) (outcome, error) {
	stat, err := d.src.Stat(date)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return gone, nil
	case err != nil:
		return asItWas, unreadable{err}
	}
	if d.looked != nil {
		d.looked(date)
	}
	if stat.PlainModified.IsZero() {
		return copyFile(d.src.CompressedPath(date), d.dst.CompressedPath(date))
	}
	return d.snapshot(date, stat)
}

// copyFile copies the file at src to dst, byte for byte, with its
// modification time: not where dst is as long, and its time is src's as its
// file system keeps it, as keptAs says; nor where src has gone, as a day
// pruned since it was listed has. Where src can't be read, it fails with an
// unreadable.
func copyFile(src, dst string) (outcome, error) {
	file, err := os.Open(src)
	if errors.Is(err, fs.ErrNotExist) {
		return gone, nil
	}
	if err != nil {
		return asItWas, unreadable{err}
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return asItWas, unreadable{err}
	}
	if was, err := os.Stat(dst); err == nil && was.Size() == info.Size() && keptAs(info.ModTime(), was.ModTime()) {
		return asItWas, nil
	}
	err = writeFile(dst, info.ModTime(), func(w io.Writer) error {
		_, err := io.Copy(w, readFailing{file})
		return err
	})
	if err != nil {
		return asItWas, err
	}
	return wrote, nil
}

// keptAs reports whether kept, a copy's modification time as its folder's
// file system keeps it, is modified, its original's: the same, or modified
// truncated to a coarser precision kept shows, as one of coarserTimes. A
// file system that truncates keeps a time no later than the one it's given,
// and one modified a second or more after another, as a summary of the
// next version is, truncates to a later time, so a change of that kind is
// never taken for none.
func keptAs(modified, kept time.Time) bool {
	if kept.Equal(modified) {
		return true
	}
	for _, precision := range coarserTimes {
		if kept.Equal(kept.Truncate(precision)) && modified.Truncate(precision).Equal(kept) {
			return true
		}
	}
	return false
}

// readFailing reads what r does, failing with an unreadable where it fails.
type readFailing struct {
	r io.Reader
}

func (f readFailing) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		err = unreadable{err}
	}
	return n, err
}

// writeFile writes what write writes to a file at path, the user's alone,
// last modified at modified, beside where it goes and renamed into place,
// making its directory, the user's alone, where it isn't there.
func writeFile(path string, modified time.Time, write func(w io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicfile.WriteModified(path, 0o600, modified, write)
}
