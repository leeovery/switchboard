package router

import (
	"bufio"
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"iter"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
)

const (
	// historyDirName is the readings history's directory in the state
	// directory.
	historyDirName = "history"
	// historyQueue is how many takings in of readings, an answer's or a
	// probe's each, can wait to be written. Past that, their readings are
	// dropped from the history rather than hold the router up.
	historyQueue = 1024
	// historyDay is the layout of a day's date in its files' names.
	historyDay = "2006-01-02"
	// compressAfter is how long after its day ends a day's file of the
	// history is compressed: today's and yesterday's never are, so the
	// router appends to plain files, and reads them back as it starts, in the
	// normal run of things.
	compressAfter = 48 * time.Hour
	// pruneLook is how often the history looks at whether a new day has
	// come, and its files are to be pruned, with no reading to write.
	pruneLook = time.Hour
	// historyLineMax is the longest line of the history read back: a
	// reading's is a couple of hundred bytes, and a longer line, which no
	// reading makes, is skipped without being held.
	historyLineMax = 4096
)

// source is where a reading came from, as the readings history gives it.
type source string

const (
	fromAnswer source = "answer"
	fromProbe  source = "probe"
	fromPrime  source = "prime"
)

// reading is a line of the readings history: a window of an account as read
// when its reading changed, and where the reading came from. It holds the
// account's id and nothing else of the account: never its token or label.
// Fields may be added to it, never renamed.
type reading struct {
	At          time.Time    `json:"at"`
	Account     string       `json:"account"`
	Window      string       `json:"window"`
	Utilization float64      `json:"utilization"`
	ResetsAt    time.Time    `json:"resets_at,omitzero"`
	Status      quota.Status `json:"status,omitempty"`
	Source      source       `json:"source"`
}

// readingsOf are windows of the account with the given id, read at a time
// from a source, as lines of the readings history.
func readingsOf(id string, windows []quota.Window, at time.Time, from source) []reading {
	readings := make([]reading, len(windows))
	for i, w := range windows {
		readings[i] = reading{At: at, Account: id, Window: w.Key, Utilization: w.Utilization, ResetsAt: w.ResetsAt, Status: w.Status, Source: from}
	}
	return readings
}

// window is the reading as the window it read.
func (r reading) window() quota.Window {
	return quota.Window{Key: r.Window, Utilization: r.Utilization, ResetsAt: r.ResetsAt, Status: r.Status}
}

// valid reports whether the reading, as the history holds it, can be taken
// up: it names an account and a window, and reads a use that can be.
func (r reading) valid() bool {
	return r.Account != "" && r.Window != "" && !r.At.IsZero() &&
		!math.IsNaN(r.Utilization) && !math.IsInf(r.Utilization, 0) && r.Utilization >= 0
}

// history is the readings history: each change to an account's windows, as
// the router takes its readings in, a JSON line in a file a day, by local
// date, in the state directory, compressed once its day ended two days
// before, and kept as long as the config says. It's for looking back at how
// the accounts were used, and for the recent rates, which the router takes up
// from it as it starts. Noting a reading never holds the router up: the
// readings queue for run's goroutine, which writes them. A write that fails
// is logged, once until one succeeds, and the reading goes unwritten: the
// history never stands in routing's way.
type history struct {
	now func() time.Time
	// keep is how long a day's file is kept, from the end of its day.
	keep time.Duration
	// dir is where the files are, set once the router has its state
	// directory, before run starts.
	dir    string
	opened atomic.Bool
	queue  chan []reading
	// dropping is set once a reading is dropped for the queue being full,
	// until one is written.
	dropping atomic.Bool
	// files is held to write while the files are pruned and compressed, and to
	// read while they're read, as GET /history reads them on goroutines of its
	// own. Appending holds neither: it's on run's goroutine, as pruning is, and
	// a reader of a file appended to meanwhile finds at worst its last line
	// cut short, which it skips.
	files sync.RWMutex
	// warned holds the files a read has warned of, as files that can't be
	// read or read short, so each is warned of once a run.
	warned filesWarned

	// Only run's goroutine touches what follows.
	failing bool
	// unmarshalled is set once a reading couldn't be put as a line, which is
	// logged once.
	unmarshalled bool
	// pruned is the day the files were last pruned on.
	pruned string
}

// newHistory returns a history kept as settings say, by now's clock: a zero
// Keep keeps a day's file for config.DefaultKeep.
func newHistory(settings config.History, now func() time.Time) *history {
	return &history{now: now, keep: cmp.Or(settings.Keep, config.DefaultKeep), queue: make(chan []reading, historyQueue)}
}

// open has the history kept in dir from now on.
func (h *history) open(dir string) {
	h.dir = dir
	h.opened.Store(true)
}

// note queues readings for run to write. It never waits: with the history
// not yet opened, or the queue full, they're dropped.
func (h *history) note(readings []reading) {
	if len(readings) == 0 || !h.opened.Load() {
		return
	}
	select {
	case h.queue <- readings:
	default:
		if !h.dropping.Swap(true) {
			logger.Warn("readings history fell behind; readings dropped from it")
		}
	}
}

// run writes the readings queued, pruning the files as it starts and on each
// day after, until ctx ends, when it writes those still queued. It makes the
// history's directory private as it starts.
func (h *history) run(ctx context.Context) {
	h.makePrivate()
	h.prune()
	look := time.NewTicker(pruneLook)
	defer look.Stop()
	for {
		select {
		case readings := <-h.queue:
			h.write(readings)
		case <-look.C:
			h.pruneDaily()
		case <-ctx.Done():
			h.drain()
			return
		}
	}
}

// makePrivate makes the history's directory, when it isn't there, and makes
// it private, the user's alone, logging why when it can't.
func (h *history) makePrivate() {
	err := os.MkdirAll(h.dir, 0o700)
	if err == nil {
		err = os.Chmod(h.dir, 0o700)
	}
	if err != nil {
		logger.Warn("can't make the readings history private", "dir", h.dir, "error", err)
	}
}

// drain writes the readings still queued.
func (h *history) drain() {
	for {
		select {
		case readings := <-h.queue:
			h.write(readings)
		default:
			return
		}
	}
}

// write appends readings to their day's file, pruning first on a day the
// files haven't been pruned on, and logging a failure once until a write
// succeeds.
func (h *history) write(readings []reading) {
	h.pruneDaily()
	err := h.append(readings)
	switch {
	case err != nil && !h.failing:
		logger.Warn("can't write the readings history; readings go unwritten until it can", "dir", h.dir, "error", err)
	case err == nil && h.failing:
		logger.Info("writing the readings history again", "dir", h.dir)
	}
	h.failing = err != nil
	if err == nil {
		h.dropping.Store(false)
	}
}

// append appends readings, a line each, to the plain file of the local day
// each was read on, making the directory, private, should it have gone.
func (h *history) append(readings []reading) error {
	if err := os.MkdirAll(h.dir, 0o700); err != nil {
		return err
	}
	for day, lines := range h.byDay(readings) {
		if err := appendLines(h.path(plainFile(day)), lines); err != nil {
			return err
		}
	}
	return nil
}

// byDay groups readings, as JSON lines, by the local day each was read on. A
// reading that can't be put as a line, as one of a use that isn't a number,
// goes unwritten, logged once.
func (h *history) byDay(readings []reading) map[string][]byte {
	days := make(map[string][]byte)
	for _, r := range readings {
		line, err := json.Marshal(r)
		if err != nil {
			if !h.unmarshalled {
				logger.Warn("readings history can't hold a reading; it goes unwritten", "account", r.Account, "window", r.Window, "error", err)
			}
			h.unmarshalled = true
			continue
		}
		day := r.At.Local().Format(historyDay)
		days[day] = append(append(days[day], line...), '\n')
	}
	return days
}

// appendLines appends lines to the file at path, making it, the user's alone,
// when it isn't there. The file is opened to append, so each write lands
// whole at its end.
func appendLines(path string, lines []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(lines)
	return errors.Join(err, f.Close())
}

// dayFile is one of the history's two files of a local day, by the day's
// date, as historyDay lays it out: its plain file, readings-<date>.jsonl,
// which the router appends the day's readings to, and its compressed file,
// readings-<date>.jsonl.gz, which holds them once the day ended two days
// before, a gzip member for each time lines were added to it.
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

// name is the file's name.
func (f dayFile) name() string {
	name := "readings-" + f.date + ".jsonl"
	if f.compressed {
		name += ".gz"
	}
	return name
}

// dayFileNamed returns the history's file with the given name, and the local
// day it holds, reporting false for a name that isn't a history file's.
func dayFileNamed(name string) (dayFile, time.Time, bool) {
	date, ok := strings.CutPrefix(name, "readings-")
	date, compressed := strings.CutSuffix(date, ".gz")
	date, dated := strings.CutSuffix(date, ".jsonl")
	if !ok || !dated {
		return dayFile{}, time.Time{}, false
	}
	day, err := time.ParseInLocation(historyDay, date, time.Local)
	if err != nil {
		return dayFile{}, time.Time{}, false
	}
	return dayFile{date: date, compressed: compressed}, day, true
}

// path returns where the history's file f is.
func (h *history) path(f dayFile) string {
	return filepath.Join(h.dir, f.name())
}

// pruneDaily prunes the history's files on a day they haven't been pruned on.
func (h *history) pruneDaily() {
	if h.now().Local().Format(historyDay) != h.pruned {
		h.prune()
	}
}

// prune removes the history's files past keeping, and compresses those of the
// days done with, as tend says, leaving anything else in the directory alone.
func (h *history) prune() {
	h.files.Lock()
	defer h.files.Unlock()
	now := h.now()
	h.pruned = now.Local().Format(historyDay)
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("can't prune the readings history", "dir", h.dir, "error", err)
		}
		return
	}
	for _, e := range entries {
		if f, day, ok := dayFileNamed(e.Name()); ok {
			h.tend(f, day, now)
		}
	}
}

// tend removes the history's file f, of the local day given, once the day
// ended keep or more before now, or when it's a day after tomorrow, as a
// clock once set ahead named it, which would crowd out the real ones; and
// compresses it, a plain file, once the day ended two days or more before
// now. What it can't do is logged, and left to the next prune.
func (h *history) tend(f dayFile, day, now time.Time) {
	switch ended := now.Sub(day.AddDate(0, 0, 1)); {
	case ended >= h.keep || afterTomorrow(day, now):
		if err := os.Remove(h.path(f)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("can't prune the readings history", "file", f.name(), "error", err)
		}
	case ended >= compressAfter && !f.compressed:
		if err := h.compress(f.date); err != nil {
			logger.Warn("can't compress the readings history", "file", f.name(), "error", err)
		}
	}
}

// compress moves the lines of the plain file of the local day with the given
// date into the day's compressed file: it writes that afresh, whole, as the
// one there, if any, followed by the lines as a gzip member of their own, and
// then removes the plain file. Lines compressed already, as plainCompressed
// says, aren't added again. One that fails leaves the plain file.
func (h *history) compress(date string) error {
	held, err := h.readDay(date)
	if err != nil {
		return err
	}
	if !held.plainCompressed() {
		member, err := gzipped(held.plain)
		if err != nil {
			return err
		}
		if err := atomicfile.Write(h.path(compressedFile(date)), append(held.written, member...), 0o600); err != nil {
			return err
		}
	}
	return os.Remove(h.path(plainFile(date)))
}

// dayHeld is what the two files of a local day hold.
type dayHeld struct {
	// plain is the plain file's lines.
	plain []byte
	// written is the compressed file as it's written, and compressed the lines
	// it gives, its members' one after another.
	written, compressed []byte
}

// readDay returns what the history's files of the local day with the given
// date hold: nothing of a file that isn't there.
func (h *history) readDay(date string) (dayHeld, error) {
	plain, err := os.ReadFile(h.path(plainFile(date)))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return dayHeld{}, err
	}
	written, compressed, err := readCompressed(h.path(compressedFile(date)))
	if err != nil {
		return dayHeld{}, err
	}
	return dayHeld{plain: plain, written: written, compressed: compressed}, nil
}

// plainCompressed reports whether the day's compressed file ends with its
// plain file's lines: they've been compressed, and the plain file holds none
// of its own, as when the router stopped once it had written the compressed
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
		return nil, nil, err
	}
	lines, err = io.ReadAll(members)
	return written, lines, err
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

// readBack returns the readings the history's two newest days hold, taken at
// or before now, one at a time, in the order they came: each day's after the
// older's, from the files dayFiles gives. The newest days are by the dates
// the files' names give, as a change of time zone can put today's file under
// another date than the clock's, but for those of a day after tomorrow. A
// window's readings since the half hour before now, and its baseline before
// that, are among them unless it has been quiet since before the older day,
// and then it has no recent rate to go by. A line that doesn't read as a
// reading that can be is left out.
func (h *history) readBack(now time.Time) iter.Seq[reading] {
	return func(yield func(reading) bool) {
		h.files.RLock()
		defer h.files.RUnlock()
		skipped := h.readDays(h.newest(now, 2), func(r reading) bool {
			return r.At.After(now) || yield(r)
		})
		if skipped > 0 {
			logger.Warn("readings history lines unread", "lines", skipped)
		}
	}
}

// windowReadings returns the readings the history holds of the window with
// the given key, of the accounts with the given ids, by account, in the order
// they came: those of the local days from the day before from's to the day
// after to's, as a change of time zone can put a reading under a date beside
// its own. It holds the files from pruning and compressing while it reads
// them, and reads none for no account, or before the history is opened.
func (h *history) windowReadings(key string, ids []string, from, to time.Time) map[string][]reading {
	if len(ids) == 0 || !h.opened.Load() {
		return nil
	}
	h.files.RLock()
	defer h.files.RUnlock()
	readings := make(map[string][]reading)
	h.readDays(datesFrom(startOfDay(from).AddDate(0, 0, -1), to.Local().AddDate(0, 0, 1)), func(r reading) bool {
		if r.Window == key && slices.Contains(ids, r.Account) {
			readings[r.Account] = append(readings[r.Account], r)
		}
		return true
	})
	return readings
}

// newest returns the dates of the history's n newest days at now, oldest
// first, by the dates its files' names give, but for those of a day after
// tomorrow.
func (h *history) newest(now time.Time, n int) []string {
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("can't read the readings history", "dir", h.dir, "error", err)
		}
		return nil
	}
	var dates []string
	for _, e := range entries {
		if f, day, ok := dayFileNamed(e.Name()); ok && !afterTomorrow(day, now) {
			dates = append(dates, f.date)
		}
	}
	slices.Sort(dates)
	dates = slices.Compact(dates)
	return dates[max(len(dates)-n, 0):]
}

// readDays hands take each reading the history holds of the local days with
// the given dates, in the order they came, each day's after the day's before,
// from the files dayFiles gives, until take reports false, and returns how
// many of the lines didn't read as a reading. The files must be held to read.
func (h *history) readDays(dates []string, take func(reading) bool) (bad int) {
	for _, date := range dates {
		for _, f := range h.dayFiles(date) {
			skipped, more := h.readFile(f, take)
			bad += skipped
			if !more {
				return bad
			}
		}
	}
	return bad
}

// datesFrom returns the dates, as historyDay lays them out, of the local days
// from first's to last's.
func datesFrom(first, last time.Time) []string {
	var dates []string
	for day := startOfDay(first); !day.After(last); day = day.AddDate(0, 0, 1) {
		dates = append(dates, day.Format(historyDay))
	}
	return dates
}

// startOfDay returns when the local day t falls on starts.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Local().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

// dayFiles returns those of the history's files of the local day with the
// given date that hold its lines, in the order the lines came: its compressed
// file, then its plain one, which holds those added since the day was
// compressed, as after the clock was set back to it; but the compressed file
// alone when the plain file's lines are compressed already, as
// plainCompressed says. When the two can't be read to tell, both are read.
func (h *history) dayFiles(date string) []dayFile {
	plain, compressed := plainFile(date), compressedFile(date)
	switch hasPlain, hasCompressed := h.has(plain), h.has(compressed); {
	case hasPlain && hasCompressed:
		if held, err := h.readDay(date); err == nil && held.plainCompressed() {
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

// has reports whether the history's file f is there, or may be: one that
// can't be looked at is read, for the reading to say why it can't.
func (h *history) has(f dayFile) bool {
	_, err := os.Stat(h.path(f))
	return !errors.Is(err, fs.ErrNotExist)
}

// afterTomorrow reports whether day, a local day as dayFileNamed gives it, is
// after the day after now's: a file of it was named by a clock set ahead.
func afterTomorrow(day, now time.Time) bool {
	y, m, d := now.Local().Date()
	return day.After(time.Date(y, m, d+1, 0, 0, 0, 0, time.Local))
}

// readFile hands take each reading the history's file f holds, in the order
// of its lines, until take reports false, and returns how many of its lines
// didn't read as a reading, and whether take wanted more. A line longer than
// historyLineMax is skipped without being held. A file that can't be read
// holds none, which is warned of but for one that isn't there, and one
// damaged, as a compressed file cut short, the lines before the damage, which
// is warned of too: each once a run, as warn says.
func (h *history) readFile(f dayFile, take func(reading) bool) (bad int, more bool) {
	src, err := h.openFile(f)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			h.warn("can't read the readings history", f, err)
		}
		return 0, true
	}
	defer func() { _ = src.Close() }()
	lines := bufio.NewReaderSize(src, historyLineMax)
	for {
		line, err := lines.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			bad++
			if err = skipLine(lines); err == nil {
				continue
			}
			line = nil
		}
		if len(line) > 0 {
			var r reading
			switch {
			case json.Unmarshal(line, &r) != nil || !r.valid():
				bad++
			case !take(r):
				return bad, false
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				h.warn("readings history read short", f, err)
			}
			return bad, true
		}
	}
}

// openFile opens the history's file f to read the lines it holds: through
// gzip when it's compressed, which reads its members as one stream.
func (h *history) openFile(f dayFile) (io.ReadCloser, error) {
	file, err := os.Open(h.path(f))
	if err != nil {
		return nil, err
	}
	if !f.compressed {
		return file, nil
	}
	members, err := gzip.NewReader(file)
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return readCloser{Reader: members, Closer: file}, nil
}

// readCloser reads through one thing, and closes another, as a file read
// through gzip is.
type readCloser struct {
	io.Reader
	io.Closer
}

// warn logs msg of the history's file f, saying why, the first time a read of
// it fails this run.
func (h *history) warn(msg string, f dayFile, err error) {
	if h.warned.first(f.name()) {
		logger.Warn(msg, "file", f.name(), "error", err)
	}
}

// filesWarned holds, by name, the history's files a read has warned of, so
// each is warned of once a run, not at every GET /history: a file that can't
// be read, or is damaged, doesn't mend itself. It's safe for concurrent use,
// as reads of the history are.
type filesWarned struct {
	mu   sync.Mutex
	told map[string]bool
}

// first reports whether the file with the given name is warned of for the
// first time.
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

// skipLine reads past the rest of a line too long to hold, reporting why it
// stopped short of the line's end, if it did.
func skipLine(lines *bufio.Reader) error {
	for {
		if _, err := lines.ReadSlice('\n'); !errors.Is(err, bufio.ErrBufferFull) {
			return err
		}
	}
}
