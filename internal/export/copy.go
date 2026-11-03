package export

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
)

// coarsestTimes is how far apart a file's modification time and the one a
// folder's file system keeps of it can be: FAT keeps them to 2 seconds, and
// exFAT, SMB shares and NAS drives no finer than it.
const coarsestTimes = 2 * time.Second

// lineEnding ends each line a compressed day's file holds.
var lineEnding = []byte{'\n'}

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
// read, as another process may prune them between the two, as tests do there.
type dayLines struct {
	src, dst *dayfile.Files
	looked   func(date string)
}

// export exports the day with the given date into the folder, reporting
// whether it wrote it: its compressed file copied, where that's the day's
// only file, as copyFile copies it; else a snapshot of its lines, as
// snapshot writes one. Where the day's files can't be read, it fails with an
// unreadable.
func (d dayLines) export(date string) (bool, error) {
	stat, err := d.src.Stat(date)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// Pruned since it was listed: the folder keeps what it was given.
		return false, nil
	case err != nil:
		return false, unreadable{err}
	}
	if d.looked != nil {
		d.looked(date)
	}
	if stat.PlainModified.IsZero() {
		return copyFile(d.src.CompressedPath(date), d.dst.CompressedPath(date))
	}
	return d.snapshot(date, stat)
}

// snapshot writes the lines of the day with the given date, whose files
// looked as stat says before they were read, as the day's compressed file in
// the folder, a snapshot of them, reporting whether it wrote it: not where
// the folder's is a snapshot of the plain file as it is still, as
// snapshotted says, nor where the day has gone. The snapshot's header holds
// the plain file's size and modification time, as markOf marks them, and the
// file takes the time too. Its lines are those ReadDay reads, each once,
// whichever of the day's files holds it, but for a line that doesn't read as
// JSON, as one cut short at the plain file's end, still being written, which
// the next export takes up whole. The plain file is looked at before its
// lines are read, so a line appended as they're read changes it, and the
// next export writes the day again. Where the day's files can't be read, it
// fails with an unreadable.
func (d dayLines) snapshot(date string, stat dayfile.DayStat) (bool, error) {
	dst, mark := d.dst.CompressedPath(date), markOf(stat)
	if snapshotted(dst, mark) {
		return false, nil
	}
	var read error
	err := writeFile(dst, stat.PlainModified, func(w io.Writer) error {
		member := gzip.NewWriter(w)
		member.Comment = mark
		var written error
		_, _, read = dayfile.ReadDay(d.src, date, jsonLine, func(line []byte) bool {
			if _, written = member.Write(line); written == nil {
				_, written = member.Write(lineEnding)
			}
			return written == nil
		})
		if read != nil {
			return read
		}
		return errors.Join(written, member.Close())
	})
	switch {
	case errors.Is(read, fs.ErrNotExist):
		return false, nil
	case read != nil:
		return false, unreadable{read}
	}
	return err == nil, err
}

// markOf is what a snapshot's header says of the plain file it was taken
// of, as stat looks at it: its size and its modification time, to the
// nanosecond, which the folder's file system needn't keep.
func markOf(stat dayfile.DayStat) string {
	return fmt.Sprintf("switchboard plain %d %d", stat.PlainSize, stat.PlainModified.UnixNano())
}

// snapshotted reports whether the file at path is a snapshot its header
// marks as mark says, reading its header alone: a file in a folder synced
// from elsewhere may be fetched as it's read.
func snapshotted(path, mark string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	header, err := gzip.NewReader(file)
	return err == nil && header.Comment == mark
}

// jsonLine takes the line, where it reads as JSON, as every line written
// does, but for one cut short.
func jsonLine(line []byte) ([]byte, bool) {
	return line, json.Valid(line)
}

// copyFile copies the file at src to dst, byte for byte, with its
// modification time, reporting whether it wrote it: not where dst has the
// same size already, and a modification time as near as coarsestTimes, nor
// where src has gone, as a day pruned since it was listed has. Where src
// can't be read, it fails with an unreadable.
func copyFile(src, dst string) (bool, error) {
	file, err := os.Open(src)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, unreadable{err}
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return false, unreadable{err}
	}
	if was, err := os.Stat(dst); err == nil && was.Size() == info.Size() && near(was.ModTime(), info.ModTime()) {
		return false, nil
	}
	err = writeFile(dst, info.ModTime(), func(w io.Writer) error {
		_, err := io.Copy(w, readFailing{file})
		return err
	})
	return err == nil, err
}

// near reports whether a and b are as near as coarsestTimes.
func near(a, b time.Time) bool {
	return a.Sub(b).Abs() <= coarsestTimes
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
