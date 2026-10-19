package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/readings"
)

// Reader reads the request ledger back where it lies, in the state directory,
// with no router: its lines, and its days' summaries, today's and those of
// the days not summarised since lines came to be filed under them summarised
// from their lines, with the readings history beside them, as they're read.
type Reader struct {
	days days
	now  func() time.Time
	// unlisted is how its listings of the readings history went.
	unlisted failing
}

// NewReader returns a reader of the ledger in the state directory stateDir,
// by now's clock, which summarises days with the accounts' caps as caps gives
// them, what it can't read logged to logger.
func NewReader(stateDir string, now func() time.Time, caps Caps, logger *slog.Logger) *Reader {
	d, _ := daysIn(stateDir, caps, logger)
	return &Reader{days: d, now: now}
}

// daysIn returns the ledger's days in the state directory stateDir, as they
// lie, summarised with the readings history beside them, in history, and the
// accounts' caps as caps gives them, what can't be read of them logged to
// logger.
func daysIn(stateDir string, caps Caps, logger *slog.Logger) (d days, history *dayfile.Files) {
	history = readings.Files(readings.Dir(stateDir), logger)
	return days{files: filesIn(Dir(stateDir), logger), history: HistoryIn(history), caps: caps, logger: logger}, history
}

// HistoryIn returns the readings history in files, as readings.Files gives
// them, as the ledger summarises its days with it: one that holds no readings
// where files is nil, as before the router opens its history.
func HistoryIn(files *dayfile.Files) History {
	return filedHistory{files: files}
}

// filedHistory is the readings history in files, as HistoryIn says.
type filedHistory struct {
	files *dayfile.Files
}

func (h filedHistory) Between(from, to time.Time, faults *dayfile.Faults) iter.Seq[readings.Reading] {
	if h.files == nil {
		return readNone
	}
	return readings.Between(h.files, from, to, faults)
}

func (h filedHistory) Dates() ([]string, error) {
	if h.files == nil {
		return nil, nil
	}
	return h.files.All()
}

// reaching returns what tells how far the readings history holds the
// readings a summary of the local day with the given date reads, as
// readingsHeld.reach says, from list's listing of its files, taken once, as
// it's first asked: before any read of them, or after, never as one is under
// way, as readingAhead says. Where they can't be listed, which is warned of
// as unlisted says, it tells of every day that the history holds them all,
// as everyReach says.
func (d days) reaching(list func() ([]string, error), unlisted *failing) func(date string) reach {
	listing := sync.OnceValue(func() func(date string) reach {
		dates, err := list()
		unlisted.note(d.logger, "can't read the readings history", err)
		if err != nil {
			return func(string) reach { return everyReach }
		}
		return readingsHeld(dates).reach
	})
	return func(date string) reach { return listing()(date) }
}

// failing is how listings of a directory, as the readings history's, went,
// so one that fails is warned of once, until one succeeds, not at each
// listing, as at each look a dashboard takes, or each round. It's safe for
// concurrent use.
type failing struct {
	warned atomic.Bool
}

// note notes how a listing went, err its failure, nil where it succeeded,
// warning of the first to fail since one last succeeded to logger, as msg
// and args, attributes before the error's, say.
func (f *failing) note(logger *slog.Logger, msg string, err error, args ...any) {
	if err == nil {
		f.warned.Store(false)
		return
	}
	if !f.warned.Swap(true) {
		logger.Warn(msg, slices.Concat(args, []any{"error", err})...)
	}
}

// Held is a line as the ledger holds it: what it reads as, and its JSON, as
// it was written, any field a later release added to it included.
type Held struct {
	Line
	JSON json.RawMessage
}

// filedHeld is a line as the ledger holds it, and under, the date of the day
// whose files hold it: a date beside the one it arrived on where a change of
// time zone filed it there.
type filedHeld struct {
	Held
	under string
}

// Lines returns the lines of the requests that arrived from from on, until
// now, oldest first, by when each arrived, whichever day's file each is in,
// plain or compressed, as dayfile.Read reads them, holding none of another
// time: a day's are written as their requests end, which a long one does
// after others that arrived later. It reads the days from where start says.
// A line that doesn't read as one, as one cut short, is passed over, and a
// damaged compressed file read up to the damage: how many lines couldn't be
// read is logged.
func (r *Reader) Lines(from time.Time) iter.Seq[Held] {
	return func(yield func(Held) bool) {
		now := r.now()
		within := func(h filedHeld) bool { return !h.At.Before(from) && !h.At.After(now) }
		r.days.lines(dayfile.Dates(r.days.start(from, now), now), heldIn, within, func(h filedHeld) bool { return yield(h.Held) }, nil)
	}
}

// lines hands take each line of the files of the local days with the given
// dates, given oldest first, as decode reads it, that within reports true
// of, oldest first, by when each arrived, whichever day's file it's in, as
// dayfile.Read hands them, each with the date of that day, as filedHeld has
// it, until take reports false, noting in faults, where they're given, the
// days whose files it couldn't read for now, as dayfile.Faults says; and
// warns of how many of them couldn't be read, args, attributes before the
// count, saying of what.
func (d days) lines(dates []string, decode func([]byte) (Held, bool), within, take func(filedHeld) bool, faults *dayfile.Faults, args ...any) {
	filed := func(date string, data []byte) (filedHeld, bool) {
		h, ok := decode(data)
		return filedHeld{Held: h, under: date}, ok
	}
	if unread := dayfile.ReadDated(d.files, dates, filed, arrived, within, take, faults); unread > 0 {
		d.logger.Warn("request ledger lines unread", slices.Concat(args, []any{"lines", unread})...)
	}
}

// start returns where a read of the ledger's days from from on, at now,
// starts: where the ledger begins, as Begins says, where that's after from,
// as no day before it holds anything, so a ledger that holds no day before
// tomorrow reads today's alone; and at from where the ledger can't be looked
// at to tell, which is warned of.
func (d days) start(from, now time.Time) time.Time {
	first, err := d.first()
	return d.startFrom(first, err, from, now)
}

// startFrom returns where a read of the ledger's days from from on, at now,
// starts, as start says, the first day the ledger holds as first and err,
// first's failure, give it.
func (d days) startFrom(first time.Time, err error, from, now time.Time) time.Time {
	began, _, err := Begins(first, err, now)
	switch {
	case err != nil:
		d.logger.Warn("can't read the request ledger", "dir", d.files.Dir, "error", err)
		return from
	case began.After(from):
		return began
	}
	return from
}

// Begins returns where the ledger begins, at now, the first day it holds as
// first and err, its failure, give it, as FirstDay gives them: the start of
// that day; but where it holds none, or none before tomorrow, the start of
// now's day, as a read of its days then reads today's alone, none reporting
// it holds none yet, for a reader that holds where it began to ask again;
// failing, but for holding none, where it can't be looked at to tell. It's
// where every reader of the ledger, its days and their plans' prices alike,
// takes it to begin.
func Begins(first time.Time, err error, now time.Time) (began time.Time, none bool, failed error) {
	switch {
	case errors.Is(err, fs.ErrNotExist), err == nil && first.After(now):
		return dayfile.DayStart(now.Local(), 0), true, nil
	case err != nil:
		return time.Time{}, false, err
	}
	return first, false, nil
}

// first returns the start of the first local day the ledger holds, as listed
// gives it.
func (d days) first() (time.Time, error) {
	listing, err := d.listed()
	if err != nil {
		return time.Time{}, err
	}
	start, _, _ := dayfile.Day(listing.first)
	return start, nil
}

// dayListing is what a listing of the ledger's directory finds, as
// days.listed gives it: the dates of the local days it holds lines of,
// oldest first, each once; the date of the first day it holds, of its lines
// or its summaries; and the dates of the days it holds files built of the
// lines of, digests, parts files or heads, oldest first, each once.
type dayListing struct {
	lines []string
	first string
	built []string
}

// listed returns what one listing of the ledger's directory finds, as
// dayListing has it: the first day it holds the earliest its files of lines,
// and its summaries, are of, by the dates their names give. It lists the
// directory as dayfile.Files.List does, never halfway through a prune. It
// fails with fs.ErrNotExist where it holds no day, giving the files built of
// lines it holds all the same, as those are its to delete.
func (d days) listed() (dayListing, error) {
	entries, err := d.files.List()
	if err != nil {
		return dayListing{}, err
	}
	var listing dayListing
	var summaries []string
	for _, e := range entries {
		if date, ok := d.files.DateOf(e.Name()); ok {
			listing.lines = append(listing.lines, date)
		} else if date, ok := datedName(e.Name(), summaryPrefix, summarySuffix); ok {
			summaries = append(summaries, date)
		} else if date, ok := builtName(e.Name()); ok {
			listing.built = append(listing.built, date)
		}
	}
	slices.Sort(listing.built)
	listing.built = slices.Compact(listing.built)
	held := append(slices.Clone(listing.lines), summaries...)
	if len(held) == 0 {
		return dayListing{built: listing.built}, fmt.Errorf("the request ledger holds no day: %w", fs.ErrNotExist)
	}
	slices.Sort(listing.lines)
	listing.lines, listing.first = slices.Compact(listing.lines), slices.Min(held)
	return listing, nil
}

// listedNoting returns what a listing of the ledger's directory finds, as
// listed does, reporting false where it can't be listed, or holds no day:
// how the listing went noted by unlisted, as failing.note says, so a
// listing that fails is warned of once, until one succeeds, but one that
// finds no day never is.
func (d days) listedNoting(unlisted *failing) (dayListing, bool) {
	listing, err := d.listed()
	failed := err
	if errors.Is(err, fs.ErrNotExist) {
		failed = nil
	}
	unlisted.note(d.logger, "can't read the request ledger", failed, "dir", d.files.Dir)
	return listing, err == nil
}

// heldIn returns the line a line of the ledger holds, as it's held, reporting
// false for one that doesn't read as a line.
func heldIn(data []byte) (Held, bool) {
	line, ok := lineIn(data)
	if !ok {
		return Held{}, false
	}
	return Held{Line: line, JSON: slices.Clone(data)}, true
}

// arrived is when the request h holds the line of arrived.
func arrived(h filedHeld) time.Time {
	return h.At
}

// Days returns the summaries of the local days from from's to today's, oldest
// first, every one of them from where start says, so the last is today's: the
// summary the ledger holds of a day, as it's held, where it stands, as stands
// says; else the day summarised from its lines and the readings history, as
// they're read, as of today as far as it has gone, knowing no less than any
// summary held of it, as knowing says, the history read ahead for them all,
// as readingAhead says, of those days alone whose summaries read them, as
// reach.reads says. A day of no requests is a summary of the accounts the
// readings history tells of alone, of none where it tells of none.
func (r *Reader) Days(from time.Time) []Summary {
	now := r.now()
	dates := dayfile.Span(r.days.start(from, now), now)
	days := make([]Summary, len(dates))
	var due []dueDay
	var at []int
	reachOf := r.days.reaching(r.days.history.Dates, &r.unlisted)
	for i, date := range dates {
		summary, stands := r.standing(date)
		if stands {
			days[i] = *summary
			continue
		}
		due, at = append(due, dueDay{date: date, held: summary, asOf: now, reach: reachOf(date)}), append(at, i)
	}
	for k, summary := range readingAhead(r.days.history, due, r.summarised, nil) {
		days[at[k]] = summary
	}
	return days
}

// standing returns the summary the ledger holds of the local day with the
// given date, reporting false where the day is to be summarised from its
// lines: the ledger holds no summary of it, or one that can't be read as the
// day's, as one damaged or of another day, which is warned of, both nil; or
// one that doesn't stand, as stands says, after a look at the day's files as
// glance gives it, given for the day's summary from its lines to know no
// less. One whose day has no lines stands, whatever its version, as the
// router alone summarises it again. One that can't be told to stand, as its
// day's files can't be looked at or its lines counted, is taken as it's
// held, which is warned of too. It marks no summary, as summaries are the
// router's to write.
func (r *Reader) standing(date string) (*Summary, bool) {
	held, ok := r.days.readable(date)
	if !ok {
		return nil, false
	}
	stat, none, marked, err := r.days.glance(date, held.stamp)
	stands := none || marked
	if err == nil && !stands {
		stands, err = r.days.stands(held.Summary, stat)
	}
	if err != nil {
		r.days.logger.Warn("can't read the request ledger", "day", date, "error", err)
		return &held.Summary, true
	}
	return &held.Summary, stands
}

// readable returns the summary the ledger holds of the local day with the
// given date, with its stamp, as held gives it, reporting false where the day
// is to be summarised from its lines: the ledger holds no summary of it, or
// one that can't be read as the day's, as one damaged or of another day,
// which is warned of.
func (d days) readable(date string) (stamped, bool) {
	held, err := d.held(date)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return stamped{}, false
	case err != nil:
		d.logger.Warn("can't read the request ledger's summary of a day; summarising it from its lines", "day", date, "error", err)
		return stamped{}, false
	}
	return held, true
}

// summarised returns the summary of the day due from its lines and the
// readings history, as read reads it, knowing no less than the one held of
// it, where there's one, as knowing says: of those of its files that can be
// read, where one can't, which is warned of.
func (r *Reader) summarised(day dueDay, read *ahead) Summary {
	summary, err := r.days.summarise(day, read)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		r.days.logger.Warn("can't read the request ledger", "day", day.date, "error", err)
	}
	return summary
}
