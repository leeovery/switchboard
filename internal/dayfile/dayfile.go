// Package dayfile keeps files a day of lines, as the readings history, the
// request ledger and the router's events keep theirs, in a directory of their
// own: a plain file each local day, <prefix>-<date>.jsonl, which lines are
// appended to, compressed, as <prefix>-<date>.jsonl.gz, once its day ended
// two days before, and removed once its day is past keeping, which it never
// is when they're kept Forever. A file named for a day after tomorrow, as a
// clock once set ahead names one, stays until its day is past keeping too, and
// anything else in the directory is left alone. Read reads the files back,
// either form, oldest first, passing over what it can't read, ReadFiled as
// they were filed, a day at a time, and ReadDay a day's, saying what it can't,
// and a Tail a day's as they grow, reading only what they gained since its
// last read; a Writer writes to them on a goroutine of its own, so noting
// what's to be written never waits, and makes a round of them every hour, and
// before each prune, which what else is kept of the days, as the request
// ledger's summaries, can be kept on, on every round, or just before each
// prune.
package dayfile

import (
	"errors"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// dateLayout is the layout of a day's date in its files' names; and
	// monthLayout, of the month a day falls in, as a date's leads with it.
	dateLayout  = time.DateOnly
	monthLayout = "2006-01"
	// compressAfter is how many days after its day ends a day's file is
	// compressed, as doneWith counts them: today's and yesterday's never
	// are, so lines are appended to plain files, and read back from them, in
	// the normal run of things.
	compressAfter = 2
)

// Forever, as how long the files are kept, keeps every day's, however long
// ago the day ended: it's the longest a time.Duration holds.
const Forever time.Duration = math.MaxInt64

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
	// LineMax is how much of a line is read back at most, its line ending
	// included: a line that runs to it without its ending, a file's last
	// included, is skipped without being held.
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
	// opened, where it's set, is called as each of a day's files is opened
	// to be read, before the next is: another process may compress the day
	// between the two, as tests do there.
	opened func(dayFile)
}

// Lines are lines to append to the files, by the date of the local day each
// is filed under, as Add files them.
type Lines map[string][]byte

// Add files line, which holds no line ending, under the local day at falls
// on.
func (l Lines) Add(at time.Time, line []byte) {
	date := DateOf(at)
	l[date] = append(append(l[date], line...), '\n')
}

// DateOf is the date of the local day t falls on, as the files' names give
// it.
func DateOf(t time.Time) string {
	return CivilOf(t).Format(dateLayout)
}

// Civil is the day with the given date, as the files' names give it, as a
// date in UTC, where every date has its midnight and its 24 hours, for
// counting days and naming them. It reports false for a date that isn't one.
func Civil(date string) (time.Time, bool) {
	day, err := time.Parse(dateLayout, date)
	return day, err == nil
}

// CivilOf is the local day t falls on, as Civil gives a day.
func CivilOf(t time.Time) time.Time {
	return civilIn(t, time.Local)
}

// civilIn is the day t falls on in loc, as Civil gives a day.
func civilIn(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// DaysBetween counts the days from the one from falls on to the one to falls
// on, both in loc, however long the days between ran, as where the clocks
// change: negative where to's day is before from's.
func DaysBetween(from, to time.Time, loc *time.Location) int {
	return int(civilIn(to, loc).Sub(civilIn(from, loc)) / (24 * time.Hour))
}

// MonthOf is the month the day, as Civil gives one, falls in, as a date
// leads with it, as 2026-10.
func MonthOf(day time.Time) string {
	return day.Format(monthLayout)
}

// MonthStart is the first day of the month given as MonthOf gives one, as
// Civil gives a day. It reports false for a month that isn't one.
func MonthStart(month string) (time.Time, bool) {
	first, err := time.Parse(monthLayout, month)
	return first, err == nil
}

// Day returns when the local day with the given date, as the files' names
// give it, starts and ends: at its first instant, and the next day's, as
// DayStart gives them. It reports false for a date that isn't one.
func Day(date string) (start, end time.Time, ok bool) {
	return dayIn(date, time.Local)
}

// Bounds are when a local day starts and ends, as Day gives them.
type Bounds struct {
	Start, End time.Time
}

// DayBounds returns the bounds of the local day with the given date, as Day
// gives them. It reports false for a date that isn't one.
func DayBounds(date string) (Bounds, bool) {
	start, end, ok := Day(date)
	return Bounds{Start: start, End: end}, ok
}

// Holds reports whether t falls on the day b bounds: at its start or after,
// and before its end, the next day's start, which falls on the next day.
func (b Bounds) Holds(t time.Time) bool {
	return !t.Before(b.Start) && t.Before(b.End)
}

// FileDates returns the dates of the days whose files may hold lines of the
// day b bounds, as Dates gives them of its start: its own, and those of the
// days either side of it, as a change of time zone can file a line under a
// date beside its own.
func (b Bounds) FileDates() []string {
	return Dates(b.Start, b.Start)
}

// dayIn returns when the day with the given date starts and ends in loc, as
// Day says.
func dayIn(date string, loc *time.Location) (start, end time.Time, ok bool) {
	day, ok := Civil(date)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	y, m, d := day.Date()
	noon := time.Date(y, m, d, 12, 0, 0, 0, loc)
	return DayStart(noon, 0), DayStart(noon, 1), true
}

// DayStart returns when the day days after the one t falls on starts, or
// days before it for days less than none, in t's time zone: at its first
// instant, when the clocks first read its midnight, as firstRead says, or
// went forward over it, as they do at midnight in some zones as summer time
// starts.
func DayStart(t time.Time, days int) time.Time {
	y, m, d := noon(t).AddDate(0, 0, days).Date()
	return firstRead(y, m, d, 0, 0, t.Location())
}

// TimeOfDay returns when, on the day t falls on, in t's time zone, the clocks
// first read hour:minute, as firstRead says, or went forward over it.
func TimeOfDay(t time.Time, hour, minute int) time.Time {
	y, m, d := t.Date()
	return firstRead(y, m, d, hour, minute, t.Location())
}

// firstRead returns the instant the clocks in loc first read the given date
// at hour:minute: the first of two, where they went back over it, so it came
// twice; and where they went forward over it, the instant they did.
func firstRead(y int, m time.Month, d, hour, minute int, loc *time.Location) time.Time {
	asked := time.Date(y, m, d, hour, minute, 0, 0, time.UTC)
	t := time.Date(y, m, d, hour, minute, 0, 0, loc)
	began, ended := t.ZoneBounds()
	switch read := reading(t); {
	case read.Before(asked):
		// time.Date puts a time the clocks went forward over in the zone
		// before, west of UTC, which ended as they did.
		return ended
	case read.After(asked):
		// East of UTC, it puts it in the zone after, which began as they did.
		return began
	}
	// time.Date picks the second of two such times, east of UTC: where the
	// zone before had read it as it ended, it read it first.
	if before := began.Add(-time.Nanosecond); !reading(before).Before(asked) {
		_, was := before.Zone()
		return asked.Add(-time.Duration(was) * time.Second).In(loc)
	}
	return t
}

// reading is what the clocks read at t, in its time zone, as a time in UTC,
// where every date and time of day comes once.
func reading(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
}

// endOf returns when the local day that starts at start ends.
func endOf(start time.Time) time.Time {
	return DayStart(start, 1)
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
// it's cut short, as cutShort says.
func endCutShort(file *os.File) error {
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		return err
	}
	var last [1]byte
	if _, err := file.ReadAt(last[:], info.Size()-1); err != nil || !cutShort(last[:]) {
		return err
	}
	_, err = file.Write([]byte{'\n'})
	return err
}

// cutShort reports whether lines end in a line cut short: they hold
// something, and their last byte isn't a line ending.
func cutShort(lines []byte) bool {
	return len(lines) > 0 && lines[len(lines)-1] != '\n'
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

// DateOf returns the date of the local day the file with the given name holds
// the lines of, as its name gives it, reporting false for a name that isn't
// one of the files'.
func (f *Files) DateOf(name string) (string, bool) {
	file, _, ok := f.named(name)
	return file.date, ok
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
	day, _, ok := Day(date)
	if !ok {
		return dayFile{}, time.Time{}, false
	}
	return dayFile{date: date, compressed: compressed}, day, true
}

// prune removes the files past keeping at now, as pastKeeping says, none
// when keep is Forever, but those of the days spared reports true of, where
// it's given, and compresses those of the days done with, as tend says,
// leaving anything else in the directory alone. spared is asked holding the
// files, so it takes nothing of them.
func (f *Files) prune(now time.Time, keep time.Duration, spared func(date string) bool) {
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
			f.tend(file, day, now, keep, spared != nil && spared(file.date))
		}
	}
}

// tend removes the file, of the local day given, once the day is past
// keeping at now, as pastKeeping says, never when keep is Forever, nor where
// it's spared, and compresses it, a plain file, once the day is done with at
// now, as doneWith says. A file of a day after tomorrow, as a clock once set
// ahead names one, stays until its day is past keeping too: the clock may be
// the one that's wrong, set back, and Newest passes such a file over
// meanwhile. What it can't do is logged, and left to the next prune.
func (f *Files) tend(file dayFile, day, now time.Time, keep time.Duration, spared bool) {
	switch {
	case keep != Forever && !spared && !now.Before(pastKeeping(day, keep)):
		if err := os.Remove(f.path(file)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			f.Logger.Warn("can't prune the "+f.Name, "file", file.name(f.Prefix), "error", err)
		}
	case !now.Before(doneWith(day)) && !file.compressed:
		if err := f.compress(file.date); err != nil {
			f.Logger.Warn("can't compress the "+f.Name, "file", file.name(f.Prefix), "error", err)
		}
	}
}

// doneWith returns when the day that starts at day, in its time zone, is done
// with, so its file is compressed: at the start of the day compressAfter's
// days after the day ends, as DayStart counts days, wherever the clocks change
// between.
func doneWith(day time.Time) time.Time {
	return DayStart(endOf(day), compressAfter)
}

// pastKeeping returns when the files of the day that starts at day, in its
// time zone, are past keeping, kept as long as keep says: at the start of the
// day keep's days after the day ends, as DayStart counts days, wherever the
// clocks change between, so always at a day's start. Part of a day counts as
// a whole one, so the files are never kept less than keep.
func pastKeeping(day time.Time, keep time.Duration) time.Time {
	return DayStart(endOf(day), keptDays(keep))
}

// keptDays are how many days after a day ends its files are kept, kept as
// long as keep says, as pastKeeping counts them: part of a day counts as a
// whole one.
func keptDays(keep time.Duration) int {
	const whole = 24 * time.Hour
	days := keep / whole
	if keep%whole > 0 {
		days++
	}
	return int(days)
}

// KeptUntil returns the date of the last local day the files of the local day
// with the given date are kept on, kept as long as keep says: the day of the
// last instant before they're past keeping, as a prune removes them from
// then. It reports false where keep is Forever, which keeps them for good, or
// the date isn't one.
func KeptUntil(date string, keep time.Duration) (string, bool) {
	return keptUntilIn(date, keep, time.Local)
}

// keptUntilIn returns the date of the last day in loc the files of the day in
// loc with the given date are kept on, as KeptUntil says.
func keptUntilIn(date string, keep time.Duration, loc *time.Location) (string, bool) {
	day, _, ok := dayIn(date, loc)
	if !ok || keep == Forever {
		return "", false
	}
	return pastKeeping(day, keep).Add(-time.Nanosecond).In(loc).Format(dateLayout), true
}

// OldestKept returns when the oldest local day whose files are kept at now
// starts, kept as long as keep says: the day whose files KeptUntil keeps
// until now's, so a prune at now leaves its files, and removes those of the
// days before it. It reports false where keep is Forever, which keeps every
// day's.
func OldestKept(now time.Time, keep time.Duration) (time.Time, bool) {
	return oldestKeptIn(now, keep, time.Local)
}

// oldestKeptIn returns when the oldest day in loc whose files are kept at now
// starts, as OldestKept says.
func oldestKeptIn(now time.Time, keep time.Duration, loc *time.Location) (time.Time, bool) {
	if keep == Forever {
		return time.Time{}, false
	}
	return DayStart(now.In(loc), -keptDays(keep)), true
}
