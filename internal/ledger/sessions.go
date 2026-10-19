package ledger

import (
	"cmp"
	"encoding/json"
	"errors"
	"io/fs"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// Sessions returns the sessions it reads, by their ids, each as SessionFrom
// has one as of the first of the near days, as sessionsLookup.nearDays gives
// them, its lines those that arrived by now: today's sessions, those with a
// request that arrived today, and those with the given ids. It reads the near
// days' files, every session's, finding today's sessions among them; then
// gives each session as sessionsLookup.asOf does: resting as rest says of
// it, by its id, as Rest has it, on a state the digest of a day before the
// near days naming it holds, or on none, read whole, each on the newest
// state that stands for it where rest is nil; its lines those of the days
// naming it after that one, as their summaries, as Days gives them, name it,
// or might, as Summary.naming says, and the near days'. Each day's files are
// read once, the days of each read side by side, as days reads them, and no
// day before the near days looked at where it reads no session. take, where
// it's given, is handed each line read that arrived by now, of whichever
// session, with the date of the day whose files hold it, the near days'
// first.
func (f *Follower) Sessions(ids []string, rest func(id string) Rest, take func(date string, l Line)) map[string]SessionFrom {
	read, l, near := f.readingNear(take)
	read.days(f.days, near, nil)
	return f.asOf(l, near, wanting(read.today(dayfile.DayStart(read.now.Local(), 0)), ids), rest, read)
}

// SessionsOf returns the sessions with the given ids alone, as Sessions gives
// them, but none of today's others: the near days' files read for them alone,
// side by side with those of the days before them naming one of them, or
// that might, so a session that ran today for weeks has none of its days
// read for another's, and no day waits on another. take, where it's given,
// is handed each line read, as Sessions hands it.
func (f *Follower) SessionsOf(ids []string, rest func(id string) Rest, take func(date string, l Line)) map[string]SessionFrom {
	read, l, near := f.readingNear(take)
	return f.asOf(l, near, wanting(make(map[string]bool), ids), rest, read)
}

// readingNear returns a read of sessions as Sessions reads them, at now,
// holding none yet, handing take each line read, where it's given; a lookup
// of sessions by the follower's own look at the ledger's days; and the dates
// of the near days at now, oldest first, as the lookup finds them.
func (f *Follower) readingNear(take func(date string, l Line)) (*sessionsRead, *sessionsLookup, []string) {
	read, l := newSessionsRead(f.now(), take), f.lookup()
	return read, l, l.nearDays(read.now)
}

// asOf returns the sessions wanted as of the first of the near days, as l
// finds them through read, their lines the near days' after whatever they
// rest on, as rest says of each, as sessionsLookup.asOf gives them: none, and
// no day before the near days looked at, where none is wanted.
func (f *Follower) asOf(l *sessionsLookup, near []string, wanted map[string]bool, rest func(id string) Rest, read *sessionsRead) map[string]SessionFrom {
	var dates []string
	if len(wanted) > 0 {
		dates = f.datesBefore(near[0], read.now)
	}
	return l.asOf(dates, near, wanted, rest, read)
}

// wanting is wanted, the sessions a read wants by their ids, wanting those
// with the given ids too.
func wanting(wanted map[string]bool, ids []string) map[string]bool {
	for _, id := range ids {
		wanted[id] = true
	}
	return wanted
}

// datesBefore returns the dates of the local days before the one with the
// given date, from where the ledger begins at now, as days.datesBefore gives
// them, the first day it holds as firstDay gives it, holding the follower as
// it looks.
func (f *Follower) datesBefore(date string, now time.Time) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	first, err := f.firstDay(now)
	return f.days.datesBefore(date, first, err, now)
}

// datesBefore returns the dates of the local days before the one with the
// given date, oldest first, from where the ledger begins at now, as Begins
// says, the first day it holds as first and err give it: so none before
// today where it holds no day; and none where it can't be looked at to tell,
// which is warned of.
func (d days) datesBefore(date string, first time.Time, err error, now time.Time) []string {
	began, _, err := Begins(first, err, now)
	if err != nil {
		d.logger.Warn("can't read the request ledger", "dir", d.files.Dir, "error", err)
		return nil
	}
	before, _, _ := dayfile.Day(date)
	return dayfile.Span(began, dayfile.DayStart(before, -1))
}

// sessionsRead is a read of sessions' days as Sessions reads them: the lines
// held so far, by session, each with the date of the day whose files hold it,
// and its place among those read; how many have been read; the dates of the
// days whose files it has read, and of those one of whose files couldn't be
// read; the lines handed on to take, where it's given; and the time no line
// arrived after.
type sessionsRead struct {
	take     func(date string, l Line)
	now      time.Time
	held     map[string][]filed
	read     int
	daysRead map[string]bool
	failed   map[string]bool
}

// newSessionsRead returns a read of sessions' days at now, holding none yet,
// handing take each line read, where it's given.
func newSessionsRead(now time.Time, take func(date string, l Line)) *sessionsRead {
	return &sessionsRead{take: take, now: now, held: make(map[string][]filed), daysRead: make(map[string]bool), failed: make(map[string]bool)}
}

// filed is a line read, as the read of its day holds it, with the date of the
// day whose files hold it, and its place among the lines read.
type filed struct {
	*Line
	date  string
	place int
}

// days reads the files of the local days with the given dates, but those it
// has read before, as day reads each, side by side, as readAhead reads them,
// as many ahead as Go runs goroutines at once, more gaining nothing; and
// takes in what each read in the order given, as reading one after another
// would, each day as soon as those before it are, and let go once taken in:
// take handed each line, and held each line of a session that arrived by
// now, every session's where keep is nil, else those of the sessions it
// holds; and the day noted among those that failed where one of its files
// couldn't be read. A day's files are read once: what a first read held of
// them is held still, as the near days, read for every session's lines, hold
// those of the sessions the lookup's read of them would keep.
func (r *sessionsRead) days(d days, dates []string, keep map[string]bool) {
	var unread []string
	for _, date := range dates {
		if !r.daysRead[date] {
			r.daysRead[date] = true
			unread = append(unread, date)
		}
	}
	readAhead(unread, runtime.GOMAXPROCS(0), func(date string) dayRead { return r.day(d, date, keep) }, func(date string, read dayRead) {
		if read.failed {
			r.failed[date] = true
		}
		for _, l := range read.lines {
			if r.take != nil {
				r.take(date, *l)
			}
			if kept(l, keep) {
				r.held[l.Session] = append(r.held[l.Session], filed{Line: l, date: date, place: r.read})
			}
			r.read++
		}
	})
}

// readAhead reads the days with the given dates, as read reads each, side by
// side, and hands take what each read in the order given, each as soon as
// those before it are handed on: never more than ahead read ahead of the one
// handed on, the next begun as one is taken, so a slow day holds no more
// than ahead read after it, never a long keep's every day. ahead is one or
// more.
func readAhead[T any](dates []string, ahead int, read func(date string) T, take func(date string, got T)) {
	got := make([]chan T, len(dates))
	var reads sync.WaitGroup
	defer reads.Wait()
	begin := func(i int) {
		if i < len(dates) {
			got[i] = make(chan T, 1)
			reads.Go(func() { got[i] <- read(dates[i]) })
		}
	}
	for i := range ahead {
		begin(i)
	}
	for i, date := range dates {
		day := <-got[i]
		begin(i + ahead)
		take(date, day)
	}
}

// dayRead is what a read of a local day's files gave, as day reads them: the
// lines to be taken in, each where the read of sessions holds it, so none is
// copied as it's taken in; and whether one of the files couldn't be read.
type dayRead struct {
	lines  []*Line
	failed bool
}

// day returns the lines of the files of the local day with the given date
// that arrived by now, in the order the files hold them, to be taken in as
// days takes them in: every one, where take is to be handed them, else only
// those kept, as kept has it, the others read for their sessions alone, as
// keptLineIn reads them; and whether one of the files couldn't be read, none
// failing where the day has none. How many of its lines couldn't be read is
// warned of, as is a file that can't be read.
func (r *sessionsRead) day(d days, date string, keep map[string]bool) dayRead {
	decode := lineIn
	if r.take == nil && keep != nil {
		decode = keptLineIn(keep)
	}
	var lines []*Line
	_, unread, err := dayfile.ReadDay(d.files, date, decode, func(l Line) bool {
		if !l.At.After(r.now) && (r.take != nil || kept(&l, keep)) {
			lines = append(lines, new(l))
		}
		return true
	})
	if unread > 0 {
		d.logger.Warn("request ledger lines unread", "day", date, "lines", unread)
	}
	failed := err != nil && !errors.Is(err, fs.ErrNotExist)
	if failed {
		d.logger.Warn("can't read the request ledger", "day", date, "error", err)
	}
	return dayRead{lines: lines, failed: failed}
}

// kept reports whether a read of sessions holds the line l: one of a
// session, of any where keep is nil, else of one it holds.
func kept(l *Line, keep map[string]bool) bool {
	return l.Session != "" && (keep == nil || keep[l.Session])
}

// keptLineIn returns what reads a line of the ledger as the line it holds,
// reporting false for one that doesn't read as a line, as lineIn does, but
// reads a line of a session keep doesn't hold for its session alone,
// giving none of the rest, so a read of a few sessions over a busy day
// decodes theirs alone.
func keptLineIn(keep map[string]bool) func(data []byte) (Line, bool) {
	return func(data []byte) (Line, bool) {
		var of struct {
			Session string `json:"session"`
		}
		if json.Unmarshal(data, &of) != nil {
			return Line{}, false
		}
		if !keep[of.Session] {
			return Line{}, true
		}
		return lineIn(data)
	}
}

// today returns the sessions of the lines held with a request, as Counted
// counts one, that arrived from start on, by their ids.
func (r *sessionsRead) today(start time.Time) map[string]bool {
	sessions := make(map[string]bool)
	for id, lines := range r.held {
		if slices.ContainsFunc(lines, func(l filed) bool { return l.Counted() && !l.At.Before(start) }) {
			sessions[id] = true
		}
	}
	return sessions
}

// oldestFirst returns the lines of held, sorting it oldest first, by when
// each arrived, then the date of the day whose files hold it, and its place
// among those read: each copied once, from where the read held it.
func oldestFirst(held []filed) []Line {
	slices.SortFunc(held, func(a, b filed) int {
		return cmp.Or(a.At.Compare(b.At), strings.Compare(a.date, b.date), cmp.Compare(a.place, b.place))
	})
	lines := make([]Line, len(held))
	for i, h := range held {
		lines[i] = *h.Line
	}
	return lines
}
