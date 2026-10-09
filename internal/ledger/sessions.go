package ledger

import (
	"cmp"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// Sessions returns the lines of the sessions it reads that arrived by now, by
// each one's id, oldest first, as Session reads one's: today's sessions, those
// with a request that arrived today, and those with the given ids. It reads
// the files of today and of the days either side of it, every session's, as a
// change of time zone files a line under the date beside its own, and a clock
// set back one under a day after today's; then those of the days before them
// whose summaries, as Days gives them, name any of the sessions it reads among
// theirs, or might, as Summary.names says: each day's files once, and no
// summary where it reads no session. take, where it's given, is handed each
// line read that arrived by now, of whichever session, with the date of the
// day whose files hold it.
func (f *Follower) Sessions(ids []string, take func(date string, l Line)) map[string][]Line {
	now := f.now()
	read := sessionsRead{take: take, now: now, held: make(map[string][]filed)}
	near := dayfile.Dates(now, now)
	for _, date := range near {
		read.day(f, date, nil)
	}
	wanted := read.today(dayfile.DayStart(now.Local(), 0))
	for _, id := range ids {
		wanted[id] = true
	}
	for _, date := range f.namingBefore(near[0], wanted, now) {
		read.day(f, date, wanted)
	}
	return read.of(wanted)
}

// namingBefore returns the dates of the local days before the one with the
// given date whose summaries, as Days gives them at now, name any of the
// sessions wanted among theirs, or might, as Summary.names says: none, and no
// summary read, where none is wanted. It gives none where the ledger holds no
// day, or can't be looked at to tell, which is warned of.
func (f *Follower) namingBefore(date string, wanted map[string]bool, now time.Time) []string {
	if len(wanted) == 0 {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	first, err := f.firstDay(now)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			f.days.logger.Warn("can't read the request ledger", "dir", f.days.files.Dir, "error", err)
		}
		return nil
	}
	before, _, _ := dayfile.Day(date)
	var dates []string
	for _, d := range dayfile.Span(first, dayfile.DayStart(before, -1)) {
		summary := f.summaryOf(d, now)
		for id := range wanted {
			if summary.names(id) {
				dates = append(dates, d)
				break
			}
		}
	}
	return dates
}

// sessionsRead is a read of sessions' days as Sessions reads them: the lines
// held so far, by session, each with the date of the day whose files hold it,
// and its place among those read; how many have been read; the lines handed
// on to take, where it's given; and the time no line arrived after.
type sessionsRead struct {
	take func(date string, l Line)
	now  time.Time
	held map[string][]filed
	read int
}

// filed is a line read, with the date of the day whose files hold it, and its
// place among the lines read.
type filed struct {
	Line
	date  string
	place int
}

// day reads the files of the local day with the given date, holding the lines
// of a session that arrived by now: every session's where keep is nil, else
// those of the sessions it holds. How many of its lines couldn't be read is
// warned of, as is a file that can't be read.
func (r *sessionsRead) day(f *Follower, date string, keep map[string]bool) {
	_, unread, err := dayfile.ReadDay(f.days.files, date, lineIn, func(l Line) bool {
		if l.At.After(r.now) {
			return true
		}
		if r.take != nil {
			r.take(date, l)
		}
		if l.Session != "" && (keep == nil || keep[l.Session]) {
			r.held[l.Session] = append(r.held[l.Session], filed{Line: l, date: date, place: r.read})
		}
		r.read++
		return true
	})
	if unread > 0 {
		f.days.logger.Warn("request ledger lines unread", "day", date, "lines", unread)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		f.days.logger.Warn("can't read the request ledger", "day", date, "error", err)
	}
}

// today returns the sessions of the lines held with a request, a message,
// that arrived from start on, by their ids.
func (r *sessionsRead) today(start time.Time) map[string]bool {
	sessions := make(map[string]bool)
	for id, lines := range r.held {
		if slices.ContainsFunc(lines, func(l filed) bool { return l.Kind == KindMessage && !l.At.Before(start) }) {
			sessions[id] = true
		}
	}
	return sessions
}

// of returns the lines held of the sessions wanted, by each one's id, oldest
// first, by when each arrived, then the date of the day whose files hold it,
// and its place among those read.
func (r *sessionsRead) of(wanted map[string]bool) map[string][]Line {
	sessions := make(map[string][]Line)
	for id := range wanted {
		held := r.held[id]
		if len(held) == 0 {
			continue
		}
		slices.SortFunc(held, func(a, b filed) int {
			return cmp.Or(a.At.Compare(b.At), strings.Compare(a.date, b.date), cmp.Compare(a.place, b.place))
		})
		lines := make([]Line, len(held))
		for i, h := range held {
			lines[i] = h.Line
		}
		sessions[id] = lines
	}
	return sessions
}
