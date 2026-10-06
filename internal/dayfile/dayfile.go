// Package dayfile keeps files a day of lines, as the readings history and the
// request ledger keep theirs, in a directory of their own: a plain file each
// local day, <prefix>-<date>.jsonl, which lines are appended to, compressed,
// as <prefix>-<date>.jsonl.gz, once its day ended two days before, and
// removed once its day is past keeping. A file named for a day after
// tomorrow, as a clock once set ahead names one, stays until its day is past
// keeping too, and anything else in the directory is left alone. Read reads
// the files back, either form, passing over what it can't read; a Writer
// writes to them on a goroutine of its own, so noting what's to be written
// never waits.
package dayfile

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

const (
	// dateLayout is the layout of a day's date in its files' names.
	dateLayout = time.DateOnly
	// compressAfter is how long after its day ends a day's file is
	// compressed: today's and yesterday's never are, so lines are appended
	// to plain files, and read back from them, in the normal run of things.
	compressAfter = 48 * time.Hour
)

// Files are the files a day of one kind of line, in a directory, by the
// prefix their names begin with: what can't be done with them is logged
// under their name. They're safe for concurrent use: pruning waits for reads
// and appends, and they for it.
type Files struct {
	// Dir is the directory they're in.
	Dir string
	// Prefix begins each file's name, as "readings" begins
	// readings-2026-09-28.jsonl.
	Prefix string
	// Name is what they're called in what's logged of them, as "readings
	// history".
	Name string
	// LineMax is the longest line read back, its line ending included: a
	// longer one is skipped without being held.
	LineMax int
	// Logger logs what can't be done with them.
	Logger *slog.Logger

	// mu is held to write while the files are pruned and compressed, and to
	// read while they're read or appended to: a reader of a file appended to
	// meanwhile finds at worst its last line cut short.
	mu sync.RWMutex
	// warned holds the files a read has warned of, as files that can't be
	// read or read short, so each is warned of once until it reads again.
	warned filesWarned
}

// Lines are lines to append to the files, by the date of the local day each
// is filed under, as Add files them.
type Lines map[string][]byte

// Add files line, which holds no line ending, under the local day at falls
// on.
func (l Lines) Add(at time.Time, line []byte) {
	date := dateOf(at)
	l[date] = append(append(l[date], line...), '\n')
}

// dateOf is the date of the local day t falls on, as the files' names give
// it.
func dateOf(t time.Time) string {
	return t.Local().Format(dateLayout)
}

// Append appends lines to the plain files of their days, making the
// directory, private, should it have gone, and each file, the user's alone,
// when it isn't there. A file is opened to append, and written a day's lines
// at once, so they land whole at its end. One left ending in a line cut
// short, as a crash or a power cut partway through a write leaves one, has
// that line ended first: the day's lines would run on from it, and neither
// it nor the first of them would read.
func (f *Files) Append(lines Lines) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return err
	}
	for date, day := range lines {
		if err := appendLines(f.path(plainFile(date)), day); err != nil {
			return err
		}
	}
	return nil
}

// appendLines appends lines to the file at path, making it, the user's alone,
// when it isn't there, and ending first the line it ends in, should that be
// cut short.
func appendLines(path string, lines []byte) error {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	err = endCutShort(file)
	if err == nil {
		_, err = file.Write(lines)
	}
	return errors.Join(err, file.Close())
}

// endCutShort ends the line file ends in, opened to read and append, where
// it's cut short: the file holds something, and its last byte isn't a line
// ending.
func endCutShort(file *os.File) error {
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		return err
	}
	var last [1]byte
	if _, err := file.ReadAt(last[:], info.Size()-1); err != nil || last[0] == '\n' {
		return err
	}
	_, err = file.Write([]byte{'\n'})
	return err
}

// makePrivate makes the files' directory, when it isn't there, and makes it
// private, the user's alone, logging why when it can't.
func (f *Files) makePrivate() {
	err := os.MkdirAll(f.Dir, 0o700)
	if err == nil {
		err = os.Chmod(f.Dir, 0o700)
	}
	if err != nil {
		f.Logger.Warn("can't make the "+f.Name+" private", "dir", f.Dir, "error", err)
	}
}

// dayFile is one of the two files of a local day, by the day's date, as
// dateLayout lays it out: its plain file, <prefix>-<date>.jsonl, which lines
// are appended to, and its compressed file, <prefix>-<date>.jsonl.gz, which
// holds them once the day ended two days before, a gzip member for each time
// lines were added to it.
type dayFile struct {
	date       string
	compressed bool
}

// plainFile is the plain file of the local day with the given date.
func plainFile(date string) dayFile {
	return dayFile{date: date}
}

// compressedFile is the compressed file of the local day with the given date.
func compressedFile(date string) dayFile {
	return dayFile{date: date, compressed: true}
}

// name is the file's name, among files whose names begin with prefix.
func (d dayFile) name(prefix string) string {
	name := prefix + "-" + d.date + ".jsonl"
	if d.compressed {
		name += ".gz"
	}
	return name
}

// path returns where the file is.
func (f *Files) path(file dayFile) string {
	return filepath.Join(f.Dir, file.name(f.Prefix))
}

// named returns the file with the given name, and the local day it holds,
// reporting false for a name that isn't one of the files'.
func (f *Files) named(name string) (dayFile, time.Time, bool) {
	date, ok := strings.CutPrefix(name, f.Prefix+"-")
	date, compressed := strings.CutSuffix(date, ".gz")
	date, dated := strings.CutSuffix(date, ".jsonl")
	if !ok || !dated {
		return dayFile{}, time.Time{}, false
	}
	day, err := time.ParseInLocation(dateLayout, date, time.Local)
	if err != nil {
		return dayFile{}, time.Time{}, false
	}
	return dayFile{date: date, compressed: compressed}, day, true
}

// prune removes the files past keeping at now, those of days that ended keep
// or more before, and compresses those of the days done with, as tend says,
// leaving anything else in the directory alone.
func (f *Files) prune(now time.Time, keep time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entries, err := os.ReadDir(f.Dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			f.Logger.Warn("can't prune the "+f.Name, "dir", f.Dir, "error", err)
		}
		return
	}
	for _, e := range entries {
		if file, day, ok := f.named(e.Name()); ok {
			f.tend(file, day, now, keep)
		}
	}
}

// tend removes the file, of the local day given, once the day ended keep or
// more before now, and compresses it, a plain file, once the day ended two
// days or more before now. A file of a day after tomorrow, as a clock once
// set ahead names one, stays until its day is past keeping too: the clock may
// be the one that's wrong, set back, and Newest passes such a file over
// meanwhile. What it can't do is logged, and left to the next prune.
func (f *Files) tend(file dayFile, day, now time.Time, keep time.Duration) {
	switch ended := now.Sub(day.AddDate(0, 0, 1)); {
	case ended >= keep:
		if err := os.Remove(f.path(file)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			f.Logger.Warn("can't prune the "+f.Name, "file", file.name(f.Prefix), "error", err)
		}
	case ended >= compressAfter && !file.compressed:
		if err := f.compress(file.date); err != nil {
			f.Logger.Warn("can't compress the "+f.Name, "file", file.name(f.Prefix), "error", err)
		}
	}
}

// compress moves the lines of the plain file of the local day with the given
// date into the day's compressed file: it writes that afresh, whole, as the
// one there, if any, followed by the lines as a gzip member of their own, and
// then removes the plain file. Lines compressed already, as plainCompressed
// says, aren't added again. One that fails leaves the plain file.
func (f *Files) compress(date string) error {
	held, err := f.readDay(date)
	if err != nil {
		return err
	}
	if !held.plainCompressed() {
		member, err := gzipped(held.plain)
		if err != nil {
			return err
		}
		if err := atomicfile.Write(f.path(compressedFile(date)), append(held.written, member...), 0o600); err != nil {
			return err
		}
	}
	return os.Remove(f.path(plainFile(date)))
}

// dayHeld is what the two files of a local day hold.
type dayHeld struct {
	// plain is the plain file's lines.
	plain []byte
	// written is the compressed file as it's written, and compressed the lines
	// it gives, its members' one after another.
	written, compressed []byte
}

// readDay returns what the files of the local day with the given date hold:
// nothing of a file that isn't there.
func (f *Files) readDay(date string) (dayHeld, error) {
	plain, err := os.ReadFile(f.path(plainFile(date)))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return dayHeld{}, err
	}
	written, compressed, err := readCompressed(f.path(compressedFile(date)))
	if err != nil {
		return dayHeld{}, err
	}
	return dayHeld{plain: plain, written: written, compressed: compressed}, nil
}

// plainCompressed reports whether the day's compressed file ends with its
// plain file's lines: they've been compressed, and the plain file holds none
// of its own, as when the writer stopped once it had written the compressed
// file, before it removed the plain one.
func (d dayHeld) plainCompressed() bool {
	return bytes.HasSuffix(d.compressed, d.plain)
}

// readCompressed returns what the compressed file at path holds, as it's
// written and as the lines it gives, its members' one after another: nothing
// when it isn't there.
func readCompressed(path string) (written, lines []byte, err error) {
	written, err = os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	members, err := gzip.NewReader(bytes.NewReader(written))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	lines, err = io.ReadAll(members)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	return written, lines, nil
}

// gzipped returns data compressed, as a gzip member of its own.
func gzipped(data []byte) ([]byte, error) {
	var member bytes.Buffer
	w := gzip.NewWriter(&member)
	_, err := w.Write(data)
	if err = errors.Join(err, w.Close()); err != nil {
		return nil, err
	}
	return member.Bytes(), nil
}
