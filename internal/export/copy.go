package export

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
)

// lineEnding ends each line a compressed day's file holds.
var lineEnding = []byte{'\n'}

// dayLines are the files a day of one kind of line in the state directory,
// src, and where the Mac's folder holds them, dst.
type dayLines struct {
	src, dst *dayfile.Files
}

// export exports the day with the given date into the folder, reporting
// whether it wrote it: its compressed file copied, where that's the day's
// only file, as copyFile copies it; else its lines compressed, as compress
// compresses them.
func (d dayLines) export(date string) (bool, error) {
	stat, err := d.src.Stat(date)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// Pruned since it was listed: the folder keeps what it was given.
		return false, nil
	case err != nil:
		return false, err
	case stat.PlainModified.IsZero():
		return copyFile(d.src.CompressedPath(date), d.dst.CompressedPath(date))
	}
	return d.compress(date, stat.PlainModified)
}

// compress writes the lines of the day with the given date, whose plain file
// was last modified at modified, as the day's compressed file in the folder,
// modified then too, reporting whether it wrote it: not where the folder's
// is modified then already, as the plain file is unchanged since. Its lines
// are those ReadDay reads, each once, whichever of the day's files holds it,
// but for a line that doesn't read as JSON, as one cut short at the plain
// file's end, still being written, which the next export takes up whole.
// modified is looked at before the lines are read, so a line appended as
// they're read changes it, and the next export writes the day again.
func (d dayLines) compress(date string, modified time.Time) (bool, error) {
	dst := d.dst.CompressedPath(date)
	if info, err := os.Stat(dst); err == nil && info.ModTime().Equal(modified) {
		return false, nil
	}
	var read error
	err := writeFile(dst, modified, func(w io.Writer) error {
		member := gzip.NewWriter(w)
		var written error
		_, _, read = dayfile.ReadDay(d.src, date, jsonLine, func(line []byte) bool {
			if _, written = member.Write(line); written == nil {
				_, written = member.Write(lineEnding)
			}
			return written == nil
		})
		return errors.Join(read, written, member.Close())
	})
	if errors.Is(read, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// jsonLine takes the line, where it reads as JSON, as every line written
// does, but for one cut short.
func jsonLine(line []byte) ([]byte, bool) {
	return line, json.Valid(line)
}

// copyFile copies the file at src to dst, byte for byte, with its
// modification time, reporting whether it wrote it: not where dst has the
// same size and modification time already, nor where src has gone, as a day
// pruned since it was listed has.
func copyFile(src, dst string) (bool, error) {
	file, err := os.Open(src)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if was, err := os.Stat(dst); err == nil && was.Size() == info.Size() && was.ModTime().Equal(info.ModTime()) {
		return false, nil
	}
	err = writeFile(dst, info.ModTime(), func(w io.Writer) error {
		_, err := io.Copy(w, file)
		return err
	})
	return err == nil, err
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
