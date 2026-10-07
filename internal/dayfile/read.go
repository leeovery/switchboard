package dayfile

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/linescan"
)

// Newest returns the dates of the n newest days at now, oldest first, by the
// dates the files' names give, but for those of a day after tomorrow: a file
// of one was named by a clock set ahead.
func (f *Files) Newest(now time.Time, n int) []string {
	dates := f.dates(func(day time.Time) bool { return !afterTomorrow(day, now) })
	return dates[max(len(dates)-n, 0):]
}

// Ended returns the dates of the days the files hold lines of that ended at
// least ago before now, oldest first, by the dates the files' names give.
func (f *Files) Ended(now time.Time, ago time.Duration) []string {
	return f.dates(func(day time.Time) bool { return now.Sub(endOf(day)) >= ago })
}

// dates returns the dates of the local days the files are of, each once,
// oldest first, but for those whose day, by its start, keep reports false
// of.
func (f *Files) dates(keep func(day time.Time) bool) []string {
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
		if file, day, ok := f.named(e.Name()); ok && keep(day) {
			dates = append(dates, file.date)
		}
	}
	slices.Sort(dates)
	return slices.Compact(dates)
}

// afterTomorrow reports whether day, the start of a local day as named gives
// it, is after the day after now's.
func afterTomorrow(day, now time.Time) bool {
	return day.After(DayStart(now.Local(), 1))
}

// Dates returns the dates of the local days whose files may hold lines of
// times from from to to: from the day before from's to the day after to's,
// as a change of time zone can file a line under a date beside its own.
func Dates(from, to time.Time) []string {
	return datesFrom(noon(from.Local()).AddDate(0, 0, -1), noon(to.Local()).AddDate(0, 0, 1))
}

// Span returns the dates of the local days from first's to last's, oldest
// first, as the files' names give them: none where last's comes before.
func Span(first, last time.Time) []string {
	return datesFrom(first.Local(), last.Local())
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

// Read hands take each line the files of the local days with the given
// dates, given oldest first, hold that decode makes a T of, oldest first by
// the time at gives it, whichever day's file it's in, until take reports
// false, and returns how many of the lines were unread: too long to hold, or
// made nothing of. decode is handed each line without its line ending, and
// mustn't keep it. A change of time zone files a line under a date beside its
// own, never further, so ordering them holds no more than a day or two of
// them at a time. A file that can't be read holds none, which is warned of,
// and one damaged, as a compressed file cut short, the lines before the
// damage, which is warned of too: each once until it reads to its end again,
// as filesWarned says. The files are read as openDay opens them.
func Read[T any](f *Files, dates []string, decode func(line []byte) (T, bool), at func(T) time.Time, take func(T) bool) (unread int) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	lines := inOrder[T]{at: at, take: take}
	for _, date := range dates {
		opened, failed := f.openDay(date)
		for _, e := range failed {
			f.warn("can't read the "+f.Name, e.file, e.err)
		}
		_, skipped := readOpened(f, opened, decode, lines.hold)
		unread += skipped
		// A later date's file holds no line of a day before this one.
		if start, _, ok := Day(date); ok && !lines.handOn(start) {
			return unread
		}
	}
	lines.handOnAll()
	return unread
}

// ReadDay hands take each line the files of the local day with the given
// date hold that decode makes a T of, in the order they came, until take
// reports false, and returns how many lines the files hold, as far as it
// read, and how many of those were unread, as Read says. Where one of the
// files can't be opened, it fails, saying why, without warning of it, once it
// has read those that could be; and with fs.ErrNotExist where the day has
// none, its lines pruned, or never written. A file damaged is read up to the
// damage, warned of as Read says, and the files are read as openDay opens
// them.
func ReadDay[T any](f *Files, date string, decode func(line []byte) (T, bool), take func(T) bool) (lines, unread int, err error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	opened, failed := f.openDay(date)
	if len(opened) == 0 && len(failed) == 0 {
		return 0, 0, fmt.Errorf("the %s holds no lines of %s: %w", f.Name, date, fs.ErrNotExist)
	}
	lines, unread = readOpened(f, opened, decode, take)
	return lines, unread, f.failure(failed)
}

// Count returns how many lines the files of the local day with the given
// date hold, read or not, failing as ReadDay does.
func (f *Files) Count(date string) (int, error) {
	lines, _, err := ReadDay(f, date, func([]byte) (struct{}, bool) { return struct{}{}, true }, func(struct{}) bool { return true })
	return lines, err
}

// readOpened hands take each line the files opened hold that decode makes a
// T of, in order, until take reports false, and returns how many lines they
// hold, as far as it read, and how many of those were unread. It closes the
// files.
func readOpened[T any](f *Files, opened []openFile, decode func(line []byte) (T, bool), take func(T) bool) (lines, unread int) {
	defer closeAll(opened)
	undecoded := 0
	for _, file := range opened {
		handed, long, more := f.readFile(file, func(line []byte) bool {
			v, ok := decode(line)
			if !ok {
				undecoded++
				return true
			}
			return take(v)
		})
		lines += handed + long
		unread += long
		if !more {
			break
		}
	}
	return lines, unread + undecoded
}

// readFile hands take each line the file holds, in order, without its line
// ending, until take reports false, and returns how many of its lines it
// handed take, how many it skipped as too long to hold, as LineMax says,
// without holding them, and whether take wanted more. A file damaged is
// warned of as Read says.
func (f *Files) readFile(file openFile, take func(line []byte) bool) (handed, long int, more bool) {
	lines := linescan.New(file, f.LineMax)
	for lines.Scan() {
		handed++
		if !take(lines.Bytes()) {
			return handed, lines.Long(), false
		}
	}
	if err := lines.Err(); err != nil {
		f.warn(f.Name+" read short", file.dayFile, err)
	} else {
		f.warned.forget(file.name(f.Prefix))
	}
	return handed, lines.Long(), true
}

// inOrder hands on what's read, oldest first by the time at gives each, once
// nothing read after it can come before it.
type inOrder[T any] struct {
	at   func(T) time.Time
	take func(T) bool
	held []T
}

// hold holds v until it can be handed on.
func (o *inOrder[T]) hold(v T) bool {
	o.held = append(o.held, v)
	return true
}

// handOn hands take, oldest first, what's held of times before until, and
// reports whether take wanted more.
func (o *inOrder[T]) handOn(until time.Time) bool {
	o.sort()
	ready, _ := slices.BinarySearchFunc(o.held, until, func(v T, t time.Time) int { return o.at(v).Compare(t) })
	for _, v := range o.held[:ready] {
		if !o.take(v) {
			return false
		}
	}
	o.held = slices.Delete(o.held, 0, ready)
	return true
}

// handOnAll hands take, oldest first, all that's held, until it reports
// false.
func (o *inOrder[T]) handOnAll() {
	o.sort()
	for _, v := range o.held {
		if !o.take(v) {
			return
		}
	}
}

// sort orders what's held oldest first, those of one time in the order they
// were read.
func (o *inOrder[T]) sort() {
	slices.SortStableFunc(o.held, func(a, b T) int { return o.at(a).Compare(o.at(b)) })
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
// at is opened, for the opening to say why it can't be.
func (f *Files) has(file dayFile) bool {
	_, err := os.Stat(f.path(file))
	return !errors.Is(err, fs.ErrNotExist)
}

// openFile is one of a day's files, opened to read the lines it holds.
type openFile struct {
	dayFile
	io.ReadCloser
}

// fileError is why one of a day's files couldn't be opened.
type fileError struct {
	file dayFile
	err  error
}

// gone reports whether the file wasn't there to be opened.
func (e fileError) gone() bool {
	return errors.Is(e.err, fs.ErrNotExist)
}

// openDay opens the files of the local day with the given date that hold its
// lines, in the order the lines came, as dayFiles lists them, as openListed
// does.
func (f *Files) openDay(date string) (opened []openFile, failed []fileError) {
	return f.openListed(date, f.dayFiles(date))
}

// openListed opens the files listed of the local day with the given date:
// those it opened, in order, and why each other couldn't be. One gone by the
// time it's opened, as when another process compressed the day once it was
// listed, has the day's files listed and opened again, once, so none of its
// lines is missed, nor read twice; one gone then is passed over. A file once
// opened reads as it was, whatever becomes of it after.
func (f *Files) openListed(date string, listed []dayFile) (opened []openFile, failed []fileError) {
	opened, failed = f.openAll(listed)
	if slices.ContainsFunc(failed, fileError.gone) {
		closeAll(opened)
		opened, failed = f.openAll(f.dayFiles(date))
	}
	return opened, slices.DeleteFunc(failed, fileError.gone)
}

// openAll opens each of files: those it opened, in order, and why each other
// couldn't be.
func (f *Files) openAll(files []dayFile) (opened []openFile, failed []fileError) {
	for _, file := range files {
		src, err := f.open(file)
		if err != nil {
			failed = append(failed, fileError{file: file, err: err})
			continue
		}
		opened = append(opened, openFile{dayFile: file, ReadCloser: src})
	}
	return opened, failed
}

// closeAll closes files.
func closeAll(files []openFile) {
	for _, file := range files {
		_ = file.Close()
	}
}

// failure is the error of the files that couldn't be opened: nil for none.
func (f *Files) failure(failed []fileError) error {
	errs := make([]error, len(failed))
	for i, e := range failed {
		errs[i] = fmt.Errorf("read %s: %w", e.file.name(f.Prefix), e.err)
	}
	return errors.Join(errs...)
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
