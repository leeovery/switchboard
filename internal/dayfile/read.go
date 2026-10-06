package dayfile

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"slices"
	"sync"
	"time"
)

// Newest returns the dates of the n newest days at now, oldest first, by the
// dates the files' names give, but for those of a day after tomorrow: a file
// of one was named by a clock set ahead.
func (f *Files) Newest(now time.Time, n int) []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	entries, err := os.ReadDir(f.Dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			f.Logger.Warn("can't read the "+f.Name, "dir", f.Dir, "error", err)
		}
		return nil
	}
	var dates []string
	for _, e := range entries {
		if file, day, ok := f.named(e.Name()); ok && !afterTomorrow(day, now) {
			dates = append(dates, file.date)
		}
	}
	slices.Sort(dates)
	dates = slices.Compact(dates)
	return dates[max(len(dates)-n, 0):]
}

// afterTomorrow reports whether day, a local day as named gives it, is after
// the day after now's.
func afterTomorrow(day, now time.Time) bool {
	y, m, d := now.Local().Date()
	return day.After(time.Date(y, m, d+1, 0, 0, 0, 0, time.Local))
}

// Dates returns the dates of the local days whose files may hold lines of
// times from from to to: from the day before from's to the day after to's,
// as a change of time zone can file a line under a date beside its own.
func Dates(from, to time.Time) []string {
	return datesFrom(noon(from.Local()).AddDate(0, 0, -1), noon(to.Local()).AddDate(0, 0, 1))
}

// datesFrom returns the dates, as dateLayout lays them out, of the days from
// first's to last's, in first's time zone. It steps a day at a time from
// noon: where the clocks change at midnight, a step from midnight would land
// on a date twice, and leave out the last.
func datesFrom(first, last time.Time) []string {
	end := noon(last.In(first.Location()))
	var dates []string
	for day := noon(first); !day.After(end); day = day.AddDate(0, 0, 1) {
		dates = append(dates, day.Format(dateLayout))
	}
	return dates
}

// noon returns noon of the day t falls on, in t's time zone.
func noon(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 12, 0, 0, 0, t.Location())
}

// Read hands take each line the files of the local days with the given dates
// hold that decode makes a T of, in the order they came, each day's after the
// day's before, from the files dayFiles gives, until take reports false, and
// returns how many of the lines were unread: too long to hold, or made
// nothing of. decode is handed each line without its line ending, and mustn't
// keep it. A file that can't be read holds none, which is warned of but for
// one that isn't there, and one damaged, as a compressed file cut short, the
// lines before the damage, which is warned of too: each once until it reads
// to its end again, as filesWarned says.
func Read[T any](f *Files, dates []string, decode func(line []byte) (T, bool), take func(T) bool) (unread int) {
	undecoded := 0
	long := f.read(dates, func(line []byte) bool {
		v, ok := decode(line)
		if !ok {
			undecoded++
			return true
		}
		return take(v)
	})
	return long + undecoded
}

// read hands take each line the files of the local days with the given dates
// hold, as Read does, until take reports false, and returns how many of the
// lines were too long to hold.
func (f *Files) read(dates []string, take func(line []byte) bool) (long int) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, date := range dates {
		for _, file := range f.dayFiles(date) {
			skipped, more := f.readFile(file, take)
			long += skipped
			if !more {
				return long
			}
		}
	}
	return long
}

// dayFiles returns those of the files of the local day with the given date
// that hold its lines, in the order the lines came: its compressed file, then
// its plain one, which holds those added since the day was compressed, as
// after the clock was set back to it; but the compressed file alone when the
// plain file's lines are compressed already, as plainCompressed says. When
// the two can't be read to tell, both are read.
func (f *Files) dayFiles(date string) []dayFile {
	plain, compressed := plainFile(date), compressedFile(date)
	switch hasPlain, hasCompressed := f.has(plain), f.has(compressed); {
	case hasPlain && hasCompressed:
		if held, err := f.readDay(date); err == nil && held.plainCompressed() {
			return []dayFile{compressed}
		}
		return []dayFile{compressed, plain}
	case hasCompressed:
		return []dayFile{compressed}
	case hasPlain:
		return []dayFile{plain}
	}
	return nil
}

// has reports whether the file is there, or may be: one that can't be looked
// at is read, for the reading to say why it can't.
func (f *Files) has(file dayFile) bool {
	_, err := os.Stat(f.path(file))
	return !errors.Is(err, fs.ErrNotExist)
}

// readFile hands take each line the file holds, in order, without its line
// ending, until take reports false, and returns how many of its lines were
// longer than LineMax, which are skipped without being held, and whether take
// wanted more. A file that can't be read, or is damaged, is warned of as Read
// says.
func (f *Files) readFile(file dayFile, take func(line []byte) bool) (long int, more bool) {
	src, err := f.open(file)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			f.warn("can't read the "+f.Name, file, err)
		}
		return 0, true
	}
	defer func() { _ = src.Close() }()
	lines := bufio.NewReaderSize(src, f.LineMax)
	for {
		line, err := lines.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			long++
			if err = skipLine(lines); err == nil {
				continue
			}
			line = nil
		}
		if len(line) > 0 && !take(bytes.TrimSuffix(line, []byte("\n"))) {
			return long, false
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				f.warned.forget(file.name(f.Prefix))
			} else {
				f.warn(f.Name+" read short", file, err)
			}
			return long, true
		}
	}
}

// open opens the file to read the lines it holds: through gzip when it's
// compressed, which reads its members as one stream.
func (f *Files) open(file dayFile) (io.ReadCloser, error) {
	src, err := os.Open(f.path(file))
	if err != nil {
		return nil, err
	}
	if !file.compressed {
		return src, nil
	}
	members, err := gzip.NewReader(src)
	if err != nil {
		return nil, errors.Join(err, src.Close())
	}
	return readCloser{Reader: members, Closer: src}, nil
}

// readCloser reads through one thing, and closes another, as a file read
// through gzip is.
type readCloser struct {
	io.Reader
	io.Closer
}

// skipLine reads past the rest of a line too long to hold, reporting why it
// stopped short of the line's end, if it did.
func skipLine(lines *bufio.Reader) error {
	for {
		if _, err := lines.ReadSlice('\n'); !errors.Is(err, bufio.ErrBufferFull) {
			return err
		}
	}
}

// warn logs msg of the file, saying why, the first time a read of it fails
// since it last read to its end.
func (f *Files) warn(msg string, file dayFile, err error) {
	name := file.name(f.Prefix)
	if f.warned.first(name) {
		f.Logger.Warn(msg, "file", name, "error", err)
	}
}

// filesWarned holds, by name, the files a read has warned of, so each is
// warned of once until it reads to its end again, not at every read: a file
// that can't be read, or is damaged, seldom mends itself, but one that does
// is warned of afresh should it fail again. It's safe for concurrent use, as
// reads of the files are.
type filesWarned struct {
	mu   sync.Mutex
	told map[string]bool
}

// first reports whether the file with the given name is warned of for the
// first time since it last read to its end.
func (w *filesWarned) first(name string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.told[name] {
		return false
	}
	if w.told == nil {
		w.told = make(map[string]bool)
	}
	w.told[name] = true
	return true
}

// forget notes that the file with the given name read to its end: a read of
// it that fails after is warned of again.
func (w *filesWarned) forget(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.told, name)
}
