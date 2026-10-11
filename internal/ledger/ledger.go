// Package ledger is the request ledger: a line for each request the router
// routes, written as the router finishes with it, which holds everything
// about the request but its content, in files a day, as internal/dayfile
// keeps them: requests-<date>.jsonl, compressed two days after its day ends,
// and removed once its day is past keeping; and a summary of each day once it
// has ended, day-<date>.json beside them, written again while lines come to be
// filed under the day, knowing no less each time, and kept for good, marked
// with the day's files as they were, so a day untouched since is never read
// again to tell. A Reader reads them back where they lie, with no router,
// from the first day the ledger holds, a Follower as they grow, for one who
// looks at them again and again, and a Table prices what they hold.
package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/redact"
)

const (
	// dirName is the ledger's directory in the state directory.
	dirName = "ledger"
	// queue is how many lines can wait to be written: past that, they're
	// dropped rather than hold a request up. It's eight times the readings
	// history's, as a line dropped is a request never counted.
	queue = 8192
	// lineMax is the longest line read back: a request's is a kilobyte or
	// so, and the longest written, every text, list and count at its most and
	// each character escaped, runs to some 450 KiB.
	lineMax = 1 << 20
	// summariseAfter is how long after its day ends a day is summarised: a
	// request still in flight as the day ends has its line filed under the
	// day once it's done, which an hour leaves it time for.
	summariseAfter = time.Hour
	// summaryPrefix and summarySuffix begin and end the name of a day's
	// summary: day-<date>.json.
	summaryPrefix, summarySuffix = "day-", ".json"
	// postponeFor is how long a day of no lines whose readings can't all be
	// read for now is left for a later round, from the first round that left
	// it: a file of them that can't be opened may be put right, but one that
	// never is mustn't hold the day back for good.
	postponeFor = 24 * time.Hour
)

// Dir is the ledger's directory in the state directory stateDir.
func Dir(stateDir string) string {
	return filepath.Join(stateDir, dirName)
}

// filesIn are the ledger's files of lines in dir, what can't be done with
// them logged to logger.
func filesIn(dir string, logger *slog.Logger) *dayfile.Files {
	return &dayfile.Files{Dir: dir, Prefix: "requests", Name: "request ledger", LineMax: lineMax, Logger: logger}
}

// summaryFile is where the ledger in dir keeps the summary of the local day
// with the given date.
func summaryFile(dir, date string) string {
	return filepath.Join(dir, summaryPrefix+date+summarySuffix)
}

// summaryDate returns the date of the local day the file with the given name
// is the summary of, reporting false for a name that isn't a summary's.
func summaryDate(name string) (string, bool) {
	date, prefixed := strings.CutPrefix(name, summaryPrefix)
	date, suffixed := strings.CutSuffix(date, summarySuffix)
	_, _, dated := dayfile.Day(date)
	return date, prefixed && suffixed && dated
}

// Ledger writes the lines noted to it to its files, on Run's goroutine, and
// keeps them as long as it was opened to: noting a line never waits, as a
// dayfile.Writer says. A line is written as JSON, filed under the local day
// its request arrived on, and never holds anything shaped like a token. On
// its writer's round, it summarises the days that have ended, as
// SummariseEnded says.
type Ledger struct {
	writer *dayfile.Writer[*Line]
	days   days
	// summarising is held while the days are summarised, as they are from
	// the readings history's writer's goroutine as well as Run's.
	summarising sync.Mutex
	// left are the summaries the last round left as they are, of days with no
	// lines that don't read as their days', each as its file looked then, by
	// its date: so it's warned of once until its file changes. postponed are
	// the days the last round left for a later one as their readings
	// couldn't all be read for now, each as of the round that first left it,
	// by its date: so it's warned of once, until it's summarised, and
	// summarised from what can be read once postponeFor has gone. Each round
	// keeps those of its own alone, so a day no longer due is let go of, and
	// warned of again should it come to be. Only a round, holding
	// summarising, touches them.
	left      map[string]fs.FileInfo
	postponed map[string]time.Time
	// unlisted is how the rounds' listings of the readings history went.
	unlisted failing
	// looked, where it's set, is called with the date of each day a round
	// summarises once it has looked at the day's files, before it reads them:
	// a prune may come between the two, as tests have it there.
	looked func(date string)
	// unwritable is set once a line couldn't be put as JSON, which is logged
	// once. Only Run's goroutine touches it.
	unwritable bool
}

// Open returns the ledger in dir, which keeps a day's lines for keep once the
// day has ended, by now's clock, and longer where its summary isn't written,
// or doesn't stand, as settled says, and summarises its days with the
// readings history, and the accounts' caps as caps gives them, logging what
// can't be done with them to logger.
func Open(dir string, keep time.Duration, now func() time.Time, history History, caps Caps, logger *slog.Logger) *Ledger {
	l := &Ledger{days: days{files: filesIn(dir, logger), history: history, caps: caps, logger: logger}}
	l.writer = dayfile.NewWriter(l.days.files, l.lines, dayfile.WriterOptions{Queue: queue, Keep: keep, Now: now, Items: "lines", Round: l.SummariseEnded,
		Spare: func(date string) bool { return !l.days.settled(date) }})
	return l
}

// Note queues line for Run to write, which has it from then on. It never
// waits: with the queue full, the line is dropped.
func (l *Ledger) Note(line *Line) {
	l.writer.Note(line)
}

// Run writes the lines noted, and keeps the ledger's files, as its writer's
// Run does, until ctx ends, when it writes what's still queued.
func (l *Ledger) Run(ctx context.Context) {
	l.writer.Run(ctx)
}

// lines returns line as the ledger's lines, filed under the local day its
// request arrived on: none, should it not be put as JSON, which is logged
// once.
func (l *Ledger) lines(line *Line) dayfile.Lines {
	written := line.written()
	data, err := json.Marshal(written)
	if err != nil {
		if !l.unwritable {
			l.days.logger.Warn("request ledger can't hold a line; it goes unwritten", "request", written.Request, "error", err)
		}
		l.unwritable = true
		return nil
	}
	lines := make(dayfile.Lines)
	// JSON escapes none of a token's characters, so a token anywhere in the
	// line shows whole, to be hidden, and what hides it needs no escaping.
	lines.Add(written.At, []byte(redact.Text(string(data))))
	return lines
}

// SummariseEnded summarises each day that ended summariseAfter or more before
// now whose lines the ledger holds, or whose readings the readings history
// does, as ended says, and has no summary that stands, as standing says, the
// readings history read ahead for them all, as readingAhead says: the summary
// knows no less than any it replaces, as made says, and is written whole, the
// user's alone, over any there, and marked, as mark says. A day that can't be
// summarised, as one of its files can't be opened, which a summary of it in
// part would be taken for whole, or whose readings alone couldn't be read for
// now, as writeSummary says, is logged, as written says, and left to the next
// round. It lists the ledger's directory and the history's once each, the
// history's before it reads any of its files; and where the history can't be
// listed, it summarises no day, as listedReadings says. It's safe to call
// from any goroutine: the readings history's writer calls it before it
// prunes the readings the summaries need.
func (l *Ledger) SummariseEnded(now time.Time) {
	l.summarising.Lock()
	defer l.summarising.Unlock()
	held, ok := l.listedReadings()
	if !ok {
		return
	}
	left := make(map[string]fs.FileInfo)
	var due []dueDay
	for _, date := range l.ended(now, held) {
		stands, summary, err := l.days.standing(date, held.reach)
		switch {
		case errors.Is(err, errLeftAsItIs):
			left[date] = l.warnLeft(date, err)
		case err != nil:
			l.days.logger.Warn("can't summarise the request ledger", "day", date, "error", err)
		case !stands:
			due = append(due, dueDay{date: date, held: summary, asOf: now, reach: held.reach(date)})
		}
	}
	postponed := make(map[string]time.Time)
	for i, err := range readingAhead(l.days.history, due, l.writeSummary, held.holds) {
		if first, ok := l.written(due[i], err); ok {
			postponed[due[i].date] = first
		}
	}
	l.left, l.postponed = left, postponed
}

// listedReadings returns one listing of the readings history's files,
// reporting false where they can't be listed, which is warned of once, until
// they can be: a listing that failed tells of no day the history holds no
// readings of, so a round summarises none of its days without one, as each
// may read them.
func (l *Ledger) listedReadings() (readingsHeld, bool) {
	dates, err := l.days.history.Dates()
	l.unlisted.note(l.days.logger, "can't read the readings history; the request ledger's days are summarised once it can be", err)
	return dates, err == nil
}

// written logs what became of the summary of the day due, as writeSummary's
// err says, and reports, where it's left for a later round as its readings
// can't all be read for now, as of when it was first left: warned of then
// alone, as it may be for a while. Any other failure is warned of at each
// round.
func (l *Ledger) written(day dueDay, err error) (first time.Time, postponed bool) {
	if !errors.Is(err, errReadingsUnread) {
		if err != nil {
			l.days.logger.Warn("can't summarise the request ledger", "day", day.date, "error", err)
		}
		return time.Time{}, false
	}
	first, warned := l.postponed[day.date]
	if !warned {
		l.days.logger.Warn("can't summarise the request ledger", "day", day.date, "error", err)
		first = day.asOf
	}
	return first, true
}

// errLeftAsItIs is the error of a summary of a day with no lines that doesn't
// read as the day's, left as it is, as what it held can't be had again.
var errLeftAsItIs = errors.New("its lines are gone, so it's left as it is")

// warnLeft warns of the summary of the local day with the given date, left
// as it is, as err says: once, until its file looks otherwise than the last
// round left it. It returns how its file looks.
func (l *Ledger) warnLeft(date string, err error) fs.FileInfo {
	info, statErr := os.Stat(summaryFile(l.days.files.Dir, date))
	if statErr != nil {
		info = nil
	}
	if was, warned := l.left[date]; !warned || !dayfile.SameAs(was, info) {
		l.days.logger.Warn("can't read the request ledger's summary of a day; its lines are gone, so it's left as it is", "day", date, "error", err)
	}
	return info
}

// ended returns the dates of the local days a round at now summarises, each
// that ended summariseAfter or more before now, oldest first: those the
// ledger holds lines of; and, from the first day it holds on, as nothing
// before it is in it, those whose readings the readings history covers, as
// held, one listing of its files, tells, as reach says, requests or not.
// Those it walks from the first day the history holds readings of, where
// that's later, as it covers none before.
func (l *Ledger) ended(now time.Time, held readingsHeld) []string {
	lines, first, err := l.days.listed()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		l.days.logger.Warn("can't read the request ledger", "dir", l.days.files.Dir, "error", err)
		return nil
	}
	lastDay := dayfile.DayStart(now.Add(-summariseAfter).Local(), -1)
	last := dayfile.DateOf(lastDay)
	dates := slices.DeleteFunc(lines, func(date string) bool { return date > last })
	if len(held) > 0 {
		from, _, _ := dayfile.Day(max(first, held[0]))
		for _, date := range dayfile.Span(from, lastDay) {
			if held.reach(date).covered {
				dates = append(dates, date)
			}
		}
	}
	slices.Sort(dates)
	return slices.Compact(dates)
}

// dueDay is a local day to be summarised, by its date, as of asOf, as
// Summarise says, and the summary held of it, which the day's summary
// replaces: nil for none. reach is how far the readings history holds the
// readings its summary reads, as readingsHeld.reach says.
type dueDay struct {
	date  string
	held  *Summary
	asOf  time.Time
	reach reach
}

// errReadingsUnread is the error of a day of no lines whose readings couldn't
// be read for now.
var errReadingsUnread = errors.New("its readings can't all be read for now; it's left for a later round")

// writeSummary writes the summary of the day due, from its lines and the
// readings history, as read reads it, knowing no less than the one held, as
// made says, marked with its files as they were before they were read: of
// its readings alone, where it has no files, as a day of no requests, or one
// whose lines were pruned. Of those it writes none where a summary is there
// that couldn't be read as the day's, as what it held can't be had again;
// and none where the read couldn't read the readings for now, as ahead.whole
// says, failing, so the next round tries again, until the day has been left
// for postponeFor, as overdue says, when it writes it from what could be
// read, which is warned of. It writes none where the day's lines go between
// its look at them and its read, as to a prune, so the next round finds them
// gone; and fails where one of the day's files can't be opened, writing
// none.
func (l *Ledger) writeSummary(day dueDay, read *ahead) error {
	stat, err := l.days.files.Stat(day.date)
	none := errors.Is(err, fs.ErrNotExist)
	if err != nil && !none {
		return err
	}
	if l.looked != nil {
		l.looked(day.date)
	}
	summary, err := l.days.summarise(day, read)
	gone := errors.Is(err, fs.ErrNotExist)
	unread := none && !read.whole(day.date)
	switch {
	case err != nil && !gone:
		return err
	case gone && !none, none && day.held == nil && l.days.holdsSummary(day.date):
		return nil
	case unread && !l.overdue(day):
		return errReadingsUnread
	}
	if err := l.days.mark(summary, stat); err != nil {
		return err
	}
	if unread {
		l.days.logger.Warn("summarised a day of the request ledger from what of its readings could be read; the rest couldn't be for a day", "day", day.date)
		return nil
	}
	l.days.logger.Info("summarised a day of the request ledger", "day", day.date, "lines", summary.linesCounted())
	return nil
}

// overdue reports whether the day due was first left for a later round, as
// its readings couldn't all be read, postponeFor or more before it's due.
func (l *Ledger) overdue(day dueDay) bool {
	first, postponed := l.postponed[day.date]
	return postponed && !day.asOf.Before(first.Add(postponeFor))
}

// days are the ledger's days where they lie, in its directory: their lines,
// and their summaries beside them, summarised with the readings history, and
// the accounts' caps, what can't be done with them logged.
type days struct {
	files   *dayfile.Files
	history History
	caps    Caps
	logger  *slog.Logger
}

// errNotTheDaysSummary is the error of a summary held of a day that doesn't
// read as the day's.
var errNotTheDaysSummary = errors.New("not the day's summary")

// stamped is a summary the ledger holds, as it reads, and as it's written,
// with its stamp: its file's modification time, which mark sets from when the
// day's compressed file was last modified, as the summary was marked, as
// stampOf says.
type stamped struct {
	Summary
	written []byte
	stamp   time.Time
}

// held returns the summary the ledger holds of the local day with the given
// date, with its stamp, failing with fs.ErrNotExist where it holds none, and
// with errNotTheDaysSummary where what it holds doesn't read as the day's, as
// one cut short, or whose day is another's.
func (d days) held(date string) (stamped, error) {
	data, stamp, err := readStamped(summaryFile(d.files.Dir, date))
	if err != nil {
		return stamped{}, err
	}
	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return stamped{}, fmt.Errorf("%w: %w", errNotTheDaysSummary, err)
	}
	if summary.Day != date {
		return stamped{}, fmt.Errorf("%w: it's of %q", errNotTheDaysSummary, summary.Day)
	}
	return stamped{Summary: summary, written: data, stamp: stamp}, nil
}

// readStamped returns what the file at path holds, and when it was last
// modified, from one opening of it, so the two are of the same file.
func readStamped(path string) ([]byte, time.Time, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, time.Time{}, err
	}
	data, err := io.ReadAll(file)
	return data, info.ModTime(), err
}

// standing reports whether the summary the ledger holds of the local day
// with the given date stands, as stands says, marked afresh, as remark says,
// where it does, so the next look counts none of the day's lines; and gives
// the summary held where it doesn't, for the one that replaces it to know no
// less: none where it holds none, or one that doesn't read as the day's,
// which is warned of, so the day is summarised from its lines alone. Of a
// day with no lines, its lines pruned, or none written, one that doesn't
// read as the day's is left as it is, never replaced by one of the readings
// alone, as what it held can't be had again: it fails with errLeftAsItIs. A
// look at the summary's file and the day's files, as glance gives it,
// reading none of them, tells of most days: they're as they were when it was
// marked, or the day has no lines, and its summary reads none of the readings
// history's readings, as reachOf tells, as reach.reads says, so there's nothing
// to summarise it again from. The summary is read only where the look can't
// tell, so one damaged in place, its file keeping its stamp, stands until a
// read of it tells. It fails where the summary can't be read, or the day's
// lines counted, to tell.
func (d days) standing(date string, reachOf func(date string) reach) (stands bool, held *Summary, err error) {
	info, err := os.Stat(summaryFile(d.files.Dir, date))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil, nil
	case err != nil:
		return false, nil, err
	}
	stat, none, marked, err := d.glance(date, info.ModTime())
	switch {
	case err != nil:
		return false, nil, err
	case marked || none && !reachOf(date).reads(none):
		return true, nil, nil
	}
	h, err := d.held(date)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil, nil
	case errors.Is(err, errNotTheDaysSummary) && none:
		return false, nil, fmt.Errorf("%w: %w", errLeftAsItIs, err)
	case errors.Is(err, errNotTheDaysSummary):
		d.logger.Warn("can't read the request ledger's summary of a day; summarising it from its lines", "day", date, "error", err)
		return false, nil, nil
	case err != nil:
		return false, nil, err
	}
	if stands, err = d.stands(h.Summary, stat); stands {
		d.remark(h, stat)
	}
	return stands, &h.Summary, err
}

// glance looks at the files of the local day with the given date, reading
// none of them, as the router and a reader each look at them for whether its
// summary stands without a read of it: stat is the look, none reports the day
// has neither file, its lines pruned, or none written, and marked that its
// files are as they were when its summary, stamped at stamp, was marked, as
// unchanged says. It fails where they can't be looked at.
func (d days) glance(date string, stamp time.Time) (stat dayfile.DayStat, none, marked bool, err error) {
	stat, err = d.files.Stat(date)
	none = errors.Is(err, fs.ErrNotExist)
	if err != nil && !none {
		return stat, false, false, err
	}
	return stat, none, unchanged(date, stat, stamp), nil
}

// holdsSummary reports whether the ledger holds a summary of the local day
// with the given date, readable or not.
func (d days) holdsSummary(date string) bool {
	_, err := os.Stat(summaryFile(d.files.Dir, date))
	return err == nil
}

// stands reports whether held, a summary the ledger holds, stands, its day's
// files as stat looks at them: they're as they were when it was marked, as
// marked says, or they hold no more lines than it was made from, counted to
// tell, so a day whose lines haven't changed since costs a look at its
// files, never a read of them. A line can come to be filed under a day after
// it's summarised, as one of a request in flight past the hour the day is
// given, or one filed after a change of time zone, or a clock set ahead and
// set right again; but none is taken away while they're kept, so fewer means
// some were lost since, as to a damaged file, or pruned before others came to
// be filed under the day, as after a clock set back, and the summary made
// from more stands. One whose day's lines are pruned stands, as does one of a
// day none were written for, but one of an older version than the ledger
// writes, which doesn't, so its day is summarised again, once, as a summary
// of this version: from its lines while they're kept, and from its readings
// while those are. It fails where the lines can't be counted, as when one of
// the day's files can't be opened.
func (d days) stands(held Summary, stat dayfile.DayStat) (bool, error) {
	if stands, told := standsUncounted(held, stat); told {
		return stands, nil
	}
	lines, err := d.files.Count(held.Day)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, nil
	case err != nil:
		return false, err
	}
	return lines <= held.Lines, nil
}

// settled reports whether the summary of the local day with the given date
// is written, and stands, as stands says, so the day's lines can be pruned:
// a day's lines go no sooner, so none goes uncounted, as while a round can't
// summarise it. One with no summary, or one that can't be read as the day's,
// or can't be told to stand, isn't; but one whose files have gone since has
// nothing left to keep.
func (d days) settled(date string) bool {
	held, err := d.held(date)
	if err != nil {
		return false
	}
	stat, err := d.files.Stat(date)
	if err != nil {
		return errors.Is(err, fs.ErrNotExist)
	}
	stands, err := d.stands(held.Summary, stat)
	return err == nil && stands
}

// standsUncounted reports whether held, a summary the ledger holds, stands,
// its day's files as stat looks at them, without its day's lines counted to
// tell, and whether that tells, as stands says: one of an older version than
// the ledger writes doesn't, and one marked with them does.
func standsUncounted(held Summary, stat dayfile.DayStat) (stands, told bool) {
	switch {
	case held.Version < summaryVersion:
		return false, true
	case marked(held, stat):
		return true, true
	}
	return false, false
}

// marked reports whether a day's files, as stat looks at them, are as they
// were when summary was marked: of the sizes its bytes hold.
func marked(summary Summary, stat dayfile.DayStat) bool {
	return summary.Bytes == bytesOf(stat)
}

// unchanged reports, by a look at the files of the local day with the given
// date that reads none of them, stat, whether they're as they were when the
// day's summary was marked, its stamp the one given, and the summary is of
// the version the ledger writes, without the summary read to tell: the day
// has no plain file, which lines are appended to, and the summary's stamp is
// the one stampOf gives, as mark stamped it. Compressing a day writes its
// compressed file anew, as it prunes, once a day, never twice in a tick of
// the coarsest clock a file system keeps, so the time changes as the file
// does.
func unchanged(date string, stat dayfile.DayStat, stamp time.Time) bool {
	return stat.PlainSize == 0 && stampOf(date, stat).Equal(stamp)
}

// stampOf returns the stamp of a summary of the version the ledger writes of
// the local day with the given date, its files as stat looks at them: when
// its compressed file was last modified, or, where it has neither file, as
// one of no requests, when the day started; a second later for each version
// past the first, so the stamp holds the summary's version too, and a look
// tells one of an older version, which is summarised again, as on a file
// system that keeps whole seconds, as HFS+ does. A day with a plain file
// alone gives none. A summary of version 1 was stamped at the time itself,
// and one of version 2 of a day with neither file, not at all.
func stampOf(date string, stat dayfile.DayStat) time.Time {
	var at time.Time
	switch {
	case !stat.CompressedModified.IsZero():
		at = stat.CompressedModified
	case stat.PlainSize == 0:
		at, _, _ = dayfile.Day(date)
	default:
		return time.Time{}
	}
	return at.Add((summaryVersion - 1) * time.Second)
}

// mark writes summary, whole, the user's alone, over any there, marked with
// stat, a look at its day's files as they were before the lines it's of were
// read: their sizes as its bytes, as marked checks, and its stamp, as
// stampOf gives it, and unchanged checks.
func (d days) mark(summary Summary, stat dayfile.DayStat) error {
	summary.Bytes = bytesOf(stat)
	data, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	return d.write(summary.Day, data, stat)
}

// remark marks held, which stands, afresh with stat, where its day's files
// changed since it was marked, as when its day was compressed, so its lines
// aren't counted again: it rewrites its bytes alone, every other field as
// it's written, those a later release added among them, so a release before
// that one never drops them from a summary of that release's version; and
// stamps it, where they didn't change, so the next look needn't read it. One
// that can't be marked is warned of, and its lines are counted again at the
// next round.
func (d days) remark(held stamped, stat dayfile.DayStat) {
	if marked(held.Summary, stat) {
		d.stamp(held.Day, stat)
		return
	}
	data, err := rebytes(held.written, bytesOf(stat))
	if err == nil {
		err = d.write(held.Day, data, stat)
	}
	if err != nil {
		d.logger.Warn("can't mark the request ledger's summary of a day; its lines are counted again at the next round", "day", held.Day, "error", err)
	}
}

// write writes data, a summary of the local day with the given date, as its
// summary, whole, the user's alone, over any there, stamped as stat gives
// it.
func (d days) write(date string, data []byte, stat dayfile.DayStat) error {
	if err := atomicfile.Write(summaryFile(d.files.Dir, date), append(data, '\n'), 0o600); err != nil {
		return err
	}
	d.stamp(date, stat)
	return nil
}

// rebytes returns summary, as it's written, with b as its bytes: in place of
// those it holds, or after its lines, where it holds none, every other field
// as it's written, in its order.
func rebytes(summary []byte, b Bytes) ([]byte, error) {
	fields, err := fieldsOf(summary)
	if err != nil {
		return nil, err
	}
	value, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(fields, func(f field) bool { return f.name == "bytes" })
	if i < 0 {
		i = slices.IndexFunc(fields, func(f field) bool { return f.name == "lines" }) + 1
		fields = slices.Insert(fields, i, field{name: "bytes"})
	}
	fields[i].value = value
	return objectOf(fields), nil
}

// field is a field of a JSON object: its name, and its value, as it's
// written.
type field struct {
	name  string
	value json.RawMessage
}

// fieldsOf returns the fields of object, a JSON object, in its order.
func fieldsOf(object []byte) ([]field, error) {
	dec := json.NewDecoder(bytes.NewReader(object))
	open, err := dec.Token()
	switch {
	case err != nil:
		return nil, err
	case open != json.Delim('{'):
		return nil, errNotAnObject
	}
	var fields []field
	for dec.More() {
		name, err := dec.Token()
		if err != nil {
			return nil, err
		}
		f := field{name: name.(string)}
		if err := dec.Decode(&f.value); err != nil {
			return nil, err
		}
		fields = append(fields, f)
	}
	return fields, nil
}

// errNotAnObject is the error of JSON that isn't an object.
var errNotAnObject = errors.New("not an object")

// objectOf returns fields as a JSON object, in their order.
func objectOf(fields []field) []byte {
	object := []byte{'{'}
	for i, f := range fields {
		if i > 0 {
			object = append(object, ',')
		}
		// A string always marshals.
		name, _ := json.Marshal(f.name)
		object = append(append(append(object, name...), ':'), f.value...)
	}
	return append(object, '}')
}

// stamp sets the modification time of the summary of the local day with the
// given date to its stamp, as stampOf gives it, the day's files as stat
// looked at them as it was marked: leaving it as it is where the day has a
// plain file alone. A summary that can't be stamped is warned of, and read
// again at the next round.
func (d days) stamp(date string, stat dayfile.DayStat) {
	if err := os.Chtimes(summaryFile(d.files.Dir, date), time.Time{}, stampOf(date, stat)); err != nil {
		d.logger.Warn("can't stamp the request ledger's summary of a day; it's read again at the next round", "day", date, "error", err)
	}
}

// summarise returns the summary of the day due from its lines, as many as
// ReadDay counts, and the readings history, as history reads it ahead, where
// the day's reach says its summary reads them, as Summarise makes it,
// knowing no less than the summary held of it, which it replaces, where
// there's one, as made says; warning of how many of the lines couldn't be
// read as one, where any couldn't. Where one of the day's files can't be
// opened, it fails, giving the summary of those that could be all the same;
// and with fs.ErrNotExist where the day has none, giving the summary of its
// readings alone.
func (d days) summarise(day dueDay, history *ahead) (Summary, error) {
	var lines, unread int
	var read error
	summary, err := Summarise(day.date, func(yield func(Line) bool) {
		lines, unread, read = dayfile.ReadDay(d.files, day.date, lineIn, yield)
	}, func(from, to time.Time) iter.Seq[readings.Reading] {
		if !day.reach.reads(errors.Is(read, fs.ErrNotExist)) {
			return readNone
		}
		return history.readings(from, to)
	}, day.held, d.caps, day.asOf)
	if err != nil {
		return Summary{}, err
	}
	if unread > 0 {
		d.logger.Warn("request ledger lines unread", "day", day.date, "lines", unread)
	}
	return summary.made(lines, day.held), read
}

// lineIn returns the line a line of the ledger holds, reporting false for one
// that doesn't read as a line, as one cut short.
func lineIn(data []byte) (Line, bool) {
	var line Line
	if json.Unmarshal(data, &line) != nil {
		return Line{}, false
	}
	return line, true
}
