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

// Sessions returns the lines of the sessions it reads that arrived by now, by
// each one's id, oldest first, as Session reads one's: today's sessions, those
// with a request that arrived today, and those with the given ids. It reads
// the files of today and of the days either side of it, every session's, as a
// change of time zone files a line under the date beside its own, and a clock
// set back one under a day after today's; then those of the days before them
// whose summaries, as Days gives them, name any of the sessions it reads among
// theirs, or might, as Summary.names says: each day's files once, and no
// summary where it reads no session; the days of each read side by side, as
// days reads them. take, where it's given, is handed each line read that
// arrived by now, of whichever session, with the date of the day whose files
// hold it.
func (f *Follower) Sessions(ids []string, take func(date string, l Line)) map[string][]Line {
	read, near := f.readNear(take)
	return f.readNaming(read, near, read.today(dayfile.DayStart(read.now.Local(), 0)), ids)
}

// SessionsOf returns the lines of the sessions with the given ids alone, as
// Sessions reads them, but none of today's others: the files of today and
// of the days either side of it, and those of the days before them whose
// summaries name one of them, or might, all side by side, so a session that
// ran today for weeks has none of its days read for another's, and no day
// waits on another. take, where it's given, is handed each line read, as
// Sessions hands it.
func (f *Follower) SessionsOf(ids []string, take func(date string, l Line)) map[string][]Line {
	read, near := f.sessionsRead(take)
	wanted := wanting(make(map[string]bool), ids)
	read.days(f, append(f.namingBefore(near[0], wanted, read.now), near...), wanted)
	return read.of(wanted)
}

// sessionsRead is a read of sessions as Sessions reads them, holding none
// yet, handing take each line read, with the dates of today and of the days
// either side of it, oldest first.
func (f *Follower) sessionsRead(take func(date string, l Line)) (*sessionsRead, []string) {
	now := f.now()
	return &sessionsRead{take: take, now: now, held: make(map[string][]filed)}, dayfile.Dates(now, now)
}

// readNear reads the files of today and of the days either side of it,
// every session's, as Sessions reads them, returning the read and those
// days' dates, oldest first.
func (f *Follower) readNear(take func(date string, l Line)) (*sessionsRead, []string) {
	read, near := f.sessionsRead(take)
	read.days(f, near, nil)
	return read, near
}

// readNaming returns the lines read holds of the sessions wanted and those
// with the given ids, once it has read the files of the days before near
// whose summaries name any of them, or might, as namingBefore finds them.
func (f *Follower) readNaming(read *sessionsRead, near []string, wanted map[string]bool, ids []string) map[string][]Line {
	wanted = wanting(wanted, ids)
	read.days(f, f.namingBefore(near[0], wanted, read.now), wanted)
	return read.of(wanted)
}

// wanting is wanted, the sessions a read wants by their ids, wanting those
// with the given ids too.
func wanting(wanted map[string]bool, ids []string) map[string]bool {
	for _, id := range ids {
		wanted[id] = true
	}
	return wanted
}

// namingBefore returns the dates of the local days before the one with the
// given date whose summaries, as Days gives them at now, name any of the
// sessions wanted among theirs, or might, as Follower.names tells without the
// readings history: none, and no summary read, where none is wanted. It reads
// from where the ledger begins, as Begins says, so it gives none before today
// where the ledger holds no day; and none where it can't be looked at to
// tell, which is warned of.
func (f *Follower) namingBefore(date string, wanted map[string]bool, now time.Time) []string {
	if len(wanted) == 0 {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	first, err := f.firstDay(now)
	began, _, err := Begins(first, err, now)
	if err != nil {
		f.days.logger.Warn("can't read the request ledger", "dir", f.days.files.Dir, "error", err)
		return nil
	}
	before, _, _ := dayfile.Day(date)
	var dates []string
	for _, d := range dayfile.Span(began, dayfile.DayStart(before, -1)) {
		if f.names(d, wanted) {
			dates = append(dates, d)
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

// days reads the files of the local days with the given dates, as day reads
// each, side by side, as many at once as Go runs goroutines at once, and
// takes in what each read in the order given, as reading one after another
// would, each day as soon as those before it are, and let go once taken in:
// take handed each line, and held each line of a session that arrived by
// now, every session's where keep is nil, else those of the sessions it
// holds.
func (r *sessionsRead) days(f *Follower, dates []string, keep map[string]bool) {
	// running holds a place for each read under way, as many as Go runs at
	// once, as more would gain nothing. It bounds the reads, not what's held:
	// a day read waits in its channel, its lines held, till the days before
	// it are taken in, so a slow day holds those read after it.
	running := make(chan struct{}, runtime.GOMAXPROCS(0))
	read := make([]chan []Line, len(dates))
	var reads sync.WaitGroup
	defer reads.Wait()
	for i, date := range dates {
		read[i] = make(chan []Line, 1)
		reads.Go(func() {
			running <- struct{}{}
			defer func() { <-running }()
			read[i] <- r.day(f, date, keep)
		})
	}
	for i, date := range dates {
		for _, l := range <-read[i] {
			if r.take != nil {
				r.take(date, l)
			}
			if kept(&l, keep) {
				r.held[l.Session] = append(r.held[l.Session], filed{Line: l, date: date, place: r.read})
			}
			r.read++
		}
	}
}

// day returns the lines of the files of the local day with the given date
// that arrived by now, in the order the files hold them, to be taken in as
// days takes them in: every one, where take is to be handed them, else only
// those kept, as kept has it, the others read for their sessions alone, as
// keptLineIn reads them. How many of its lines couldn't be read is warned
// of, as is a file that can't be read.
func (r *sessionsRead) day(f *Follower, date string, keep map[string]bool) []Line {
	decode := lineIn
	if r.take == nil && keep != nil {
		decode = keptLineIn(keep)
	}
	var lines []Line
	_, unread, err := dayfile.ReadDay(f.days.files, date, decode, func(l Line) bool {
		if !l.At.After(r.now) && (r.take != nil || kept(&l, keep)) {
			lines = append(lines, l)
		}
		return true
	})
	if unread > 0 {
		f.days.logger.Warn("request ledger lines unread", "day", date, "lines", unread)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		f.days.logger.Warn("can't read the request ledger", "day", date, "error", err)
	}
	return lines
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
