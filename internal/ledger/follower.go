package ledger

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/readings"
)

// Follower reads the request ledger back where it lies, with no router, as
// it grows, for one who looks at it again and again, as the dashboard does:
// today's lines read once, then tailed, each look reading only what the
// days' files gained since the last read ended; each day's summary held once
// read, until its stamp or its sizes change; today, and any day not
// summarised since lines came to be filed under it, a running tally of its
// lines as they're tailed, beside the readings history, read as it grows too;
// and a session's days found by the summaries' session ids. What it gives is
// what a Reader gives, reading them whole. It's safe for concurrent use.
type Follower struct {
	days    days
	history *dayfile.Files
	now     func() time.Time

	mu       sync.Mutex
	listing  heldListing[dayListing]
	readings heldListing[[]string]
	unlisted failing
	followed map[string]*followedDay
	today    todaysLines
}

// NewFollower returns a follower of the ledger in the state directory
// stateDir, by now's clock, which summarises days with the accounts' caps as
// caps gives them, what it can't read logged to logger.
func NewFollower(stateDir string, now func() time.Time, caps Caps, logger *slog.Logger) *Follower {
	d, history := daysIn(stateDir, caps, logger)
	return &Follower{days: d, history: history, now: now, followed: make(map[string]*followedDay)}
}

// heldListing is what a listing of a directory found, found, or why it
// found nothing, err, held while the directory looks as dir did before it
// was listed: none held where dir is nil.
type heldListing[T any] struct {
	dir   fs.FileInfo
	found T
	err   error
}

// listed returns what list finds of the directory at path, at now: as it was
// last found, where the directory looks as it did then, which a file added
// to it, or removed, changes; else listing it again. A listing that fails,
// but for finding nothing, is never held.
func (h *heldListing[T]) listed(path string, now time.Time, list func() (T, error)) (T, error) {
	dir, err := os.Stat(path)
	if err == nil && h.dir != nil && dayfile.SameAs(h.dir, dir) {
		return h.found, h.err
	}
	found, listErr := list()
	*h = heldListing[T]{found: found, err: listErr}
	// A file system that keeps whole seconds, as HFS+ does, gives a directory
	// changed again within the second it was listed in no time of its own: a
	// listing of one changed in the last second isn't held.
	if err == nil && (listErr == nil || errors.Is(listErr, fs.ErrNotExist)) && now.Sub(dir.ModTime()) > time.Second {
		h.dir = dir
	}
	return found, listErr
}

// dayListing is what a listing of the ledger's directory finds, as
// days.listed gives it: the dates of the local days it holds lines of,
// oldest first, and the date of the first day it holds, of its lines or its
// summaries.
type dayListing struct {
	lines []string
	first string
}

// listed returns what a listing of the ledger's directory finds, at now,
// held as heldListing.listed says while the directory looks as it did.
func (f *Follower) listed(now time.Time) (dayListing, error) {
	return f.listing.listed(f.days.files.Dir, now, func() (dayListing, error) {
		lines, first, err := f.days.listed()
		return dayListing{lines: lines, first: first}, err
	})
}

// firstDay returns the start of the first local day the ledger holds, as
// days.first does, at now, held as listed says.
func (f *Follower) firstDay(now time.Time) (time.Time, error) {
	held, err := f.listed(now)
	if err != nil {
		return time.Time{}, err
	}
	start, _, _ := dayfile.Day(held.first)
	return start, nil
}

// FirstDay returns the start of the first local day the ledger holds, at
// now, as Days reads from it: it fails with fs.ErrNotExist where it holds
// none.
func (f *Follower) FirstDay(now time.Time) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.firstDay(now)
}

// errNoLines is why a ledger that holds no day's lines can't say the first
// day it holds lines of.
var errNoLines = fmt.Errorf("the request ledger holds no day's lines: %w", fs.ErrNotExist)

// FirstLinesDay returns the start of the first local day the ledger holds
// lines of, at now, held as listed says: the first a day's lines can be read
// back from, a summary of an earlier day, whose lines are pruned, aside. It
// fails with fs.ErrNotExist where it holds no day's lines.
func (f *Follower) FirstLinesDay(now time.Time) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	held, err := f.listed(now)
	if err == nil && len(held.lines) == 0 {
		err = errNoLines
	}
	if err != nil {
		return time.Time{}, err
	}
	start, _, _ := dayfile.Day(held.lines[0])
	return start, nil
}

// readingsDates returns the dates of the local days the readings history
// holds readings of, as its Dates gives them, at now, held as
// heldListing.listed says while its directory looks as it did.
func (f *Follower) readingsDates(now time.Time) ([]string, error) {
	return f.readings.listed(f.history.Dir, now, f.days.history.Dates)
}

// Days returns the summaries of the local days from from's to today's, as
// Reader.Days gives them: each day's held, as it was read, while its summary
// and its files look as they did, and today's, and any other's summarised
// from its lines, as of now. The summaries are shared with later calls, so
// they're never to be changed.
func (f *Follower) Days(from time.Time) []Summary {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	reachOf := f.days.reaching(func() ([]string, error) { return f.readingsDates(now) }, &f.unlisted)
	dates := f.dates(from, now)
	summaries := make([]Summary, len(dates))
	for i, date := range dates {
		summaries[i] = f.summaryOf(date, now, reachOf)
	}
	return summaries
}

// DaysOn returns the summaries of the local days with the dates given,
// oldest first, each once, as Days gives them from the earliest: none of a
// date Days gives none of, before the first day the ledger holds or after
// today's. Only those days' files are looked at, however long ago they
// were.
func (f *Follower) DaysOn(dates []string) []Summary {
	wanted := slices.Compact(slices.Sorted(slices.Values(dates)))
	if len(wanted) == 0 {
		return nil
	}
	from, _, ok := dayfile.Day(wanted[0])
	if !ok {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	reachOf := f.days.reaching(func() ([]string, error) { return f.readingsDates(now) }, &f.unlisted)
	first, today := dayfile.DateOf(f.start(from, now)), dayfile.DateOf(now)
	var summaries []Summary
	for _, date := range wanted {
		if date >= first && date <= today {
			summaries = append(summaries, f.summaryOf(date, now, reachOf))
		}
	}
	return summaries
}

// dates returns the dates of the local days from from's to today's that a
// read of the ledger's days gives, at now, from where start says.
func (f *Follower) dates(from, now time.Time) []string {
	return dayfile.Span(f.start(from, now), now)
}

// start returns where a read of the ledger's days from from on, at now,
// starts, as days.start says, the first day the ledger holds as firstDay
// gives it: now, without a look at the ledger, where from is today.
func (f *Follower) start(from, now time.Time) time.Time {
	if dayfile.DayStart(from.Local(), 0).Equal(dayfile.DayStart(now.Local(), 0)) {
		return now
	}
	first, err := f.firstDay(now)
	return f.days.startFrom(first, err, from, now)
}

// summaryOf returns the summary of the local day with the given date, as
// Reader.Days gives it, at now: the summary the ledger holds of it, where it
// stands, as follow finds; else the day summarised from its lines as they
// grow, and the readings history, where the day's reach, as reachOf tells it,
// says its summary reads them, as reach.reads says, its files as follow last
// looked at them.
func (f *Follower) summaryOf(date string, now time.Time, reachOf func(date string) reach) Summary {
	d := f.follow(date)
	if d.stands {
		return *d.held
	}
	live := d.liveDay(f)
	live.read(f.days.logger)
	return live.summary(f, d.held, now, reachOf(date).reads(d.look.none))
}

// names reports whether the local day with the given date, as Days gives it,
// names any of the sessions wanted among its sessions' ids, or might, as
// naming says.
func (f *Follower) names(date string, wanted map[string]bool) bool {
	return len(f.naming(date, wanted)) > 0
}

// naming returns those of the sessions wanted the local day with the given
// date, as Days gives it, names among its sessions' ids, or might, as
// Summary.naming says, by their ids: from the summary the ledger holds of
// it, where it stands, as follow finds; else from its lines as they grow,
// and the summary held of it, which the day's knows no less than. It never
// reads the readings history, which names no session.
func (f *Follower) naming(date string, wanted map[string]bool) map[string]bool {
	d := f.follow(date)
	if d.stands {
		return d.held.naming(wanted)
	}
	live := d.liveDay(f)
	live.read(f.days.logger)
	named := live.naming(wanted)
	if d.held != nil {
		maps.Copy(named, d.held.naming(wanted))
	}
	return named
}

// follow returns the local day with the given date as the follower follows
// it, whether the summary the ledger holds of it stands found afresh, as
// standing says, only where the summary's file or the day's files look
// otherwise than they did.
func (f *Follower) follow(date string) *followedDay {
	d, ok := f.followed[date]
	if !ok {
		d = &followedDay{date: date}
		f.followed[date] = d
	}
	if l := f.lookAt(date); !d.looked || !l.same(d.look) {
		f.standing(d, l)
	}
	return d
}

// followedDay is a day of the ledger as a Follower last found it, by its
// date: look, its summary's file and its files, as last looked at; held, the
// summary the ledger holds of it, as read then, nil where it holds none, or
// one that can't be read as the day's; whether held stands, as standing
// says, or is taken as it's held; and exact, whether the day has files that
// hold exactly the lines held counts, as they look as they did when it was
// marked, or are counted to tell. live is the day summarised from its lines
// as they grow, where held doesn't stand, or stands only as they were counted
// to tell.
type followedDay struct {
	date   string
	look   look
	looked bool
	held   *Summary
	stands bool
	exact  bool
	live   *liveDay
}

// liveDay returns the day summarised from its lines as they grow, starting
// it where there's none yet.
func (d *followedDay) liveDay(f *Follower) *liveDay {
	if d.live == nil {
		d.live = &liveDay{date: d.date, lines: tailed[Line]{Tail: dayfile.NewTail(f.days.files, d.date, lineIn), date: d.date}, tally: make(tally)}
	}
	return d.live
}

// standing finds whether the summary the ledger holds of the day stands, as
// Reader.standing does, its summary's file and its files as l looks at them:
// counting the day's lines, where only a count tells, as the day's live
// lines are read, kept to count again as they grow; letting go of them where
// it stands without.
func (f *Follower) standing(d *followedDay, l look) {
	d.look, d.looked = l, true
	d.held, d.stands, d.exact = nil, false, false
	held, ok := f.days.readable(d.date)
	if !ok {
		return
	}
	d.held = &held.Summary
	stands, told, err := l.tells(held)
	switch {
	case err != nil:
		f.days.logger.Warn("can't read the request ledger", "day", d.date, "error", err)
		d.stands = true
	case told:
		d.stands, d.exact = stands, stands && !l.none
	default:
		d.stands, d.exact = d.liveDay(f).counting(f.days.logger, held.Lines)
		return
	}
	if d.stands {
		d.live = nil
	}
}

// look is a look at a day's summary's file and its files that reads none of
// them: summary, nil where there's none, or it can't be looked at; and
// files, the day's files as a dayfile.DayStat looks at them, none set where
// it has none, and err, where they can't be looked at.
type look struct {
	summary fs.FileInfo
	files   dayfile.DayStat
	none    bool
	err     error
}

// lookAt looks at the summary's file and the files of the local day with
// the given date.
func (f *Follower) lookAt(date string) look {
	var l look
	if info, err := os.Stat(summaryFile(f.days.files.Dir, date)); err == nil {
		l.summary = info
	}
	l.files, l.err = f.days.files.Stat(date)
	if errors.Is(l.err, fs.ErrNotExist) {
		l.none, l.err = true, nil
	}
	return l
}

// same reports whether l looks as other does: the same summary's file, as
// long and last modified at the same time, and the day's files of the same
// sizes, the compressed one last modified at the same time, or none, or
// neither to be looked at.
func (l look) same(other look) bool {
	return dayfile.SameAs(l.summary, other.summary) && l.none == other.none && (l.err == nil) == (other.err == nil) &&
		l.files.PlainSize == other.files.PlainSize && l.files.CompressedSize == other.files.CompressedSize &&
		l.files.CompressedModified.Equal(other.files.CompressedModified)
}

// tells reports whether held, the summary the ledger holds of a day whose
// files l looks at, stands, as stands says, and whether the look tells,
// without its lines counted, as Reader.standing has it: the day has no
// lines, or its files are as they were when it was marked, as unchanged and
// standsUncounted say, or it's of an older version. It fails where the day's
// files can't be looked at.
func (l look) tells(held stamped) (stands, told bool, err error) {
	switch {
	case l.err != nil:
		return false, false, l.err
	case l.none || unchanged(held.Day, l.files, held.stamp):
		return true, true, nil
	}
	stands, told = standsUncounted(held.Summary, l.files)
	return stands, told, nil
}

// liveDay is a day summarised from its lines, and the readings history
// beside them, as they're read as they grow: its lines tailed, and tallied as
// they're read, tallied counting those that read as lines; and the readings
// history of it, and of the week before it, read as it's first wanted.
type liveDay struct {
	date    string
	lines   tailed[Line]
	tally   tally
	tallied int
	history *readings.Day
}

// read reads what the day's lines gained since the last read, tallying
// them, and reports false where one of its files couldn't be opened, as
// tailed says.
func (d *liveDay) read(logger *slog.Logger) bool {
	return d.lines.read(logger, func() { d.tally, d.tallied = make(tally), 0 }, func(l Line) {
		d.tally.line(&l)
		d.tallied++
	})
}

// naming returns those of the sessions wanted the day's lines, as last read,
// the line the plain file ends in, cut short, among them, are of, as a
// summary of them counts its sessions, by their ids.
func (d *liveDay) naming(wanted map[string]bool) map[string]bool {
	named := make(map[string]bool)
	for _, a := range d.tally {
		for id := range a.sessions {
			if wanted[id] {
				named[id] = true
			}
		}
	}
	if cut, ok := d.lines.Cut(); ok && wanted[cut.CountedSession()] {
		named[cut.CountedSession()] = true
	}
	return named
}

// counting reports whether the day's files hold no more lines than held, as
// they're read now, and whether they hold exactly as many: no more, but not
// exactly, where they can't all be read, as its summary is then taken as
// it's held.
func (d *liveDay) counting(logger *slog.Logger, held int) (noMore, exactly bool) {
	if !d.read(logger) {
		return true, false
	}
	lines, _ := d.lines.Lines()
	return lines <= held, lines == held
}

// summary returns the day summarised from its lines as last read, and the
// readings history as it's read now, where reads reports its summary reads
// them, as of asOf, as Reader.Days summarises a day from its lines, knowing
// no less than held, where it's given: its tally, and the line the day's
// plain file ends in, cut short, where it reads as one, as a read of the day
// whole would read it. So the minutes at each account's cap and at a limit,
// and each window's last reading standing, move on with the clock, as
// summarising the day afresh moves them.
func (d *liveDay) summary(f *Follower, held *Summary, asOf time.Time, reads bool) Summary {
	t, n := d.tally.clone(), d.tallied
	if cut, ok := d.lines.Cut(); ok {
		t.line(&cut)
		n++
	}
	lines, _ := d.lines.Lines()
	read := func() iter.Seq[readings.Reading] {
		if !reads {
			return readNone
		}
		if d.history == nil {
			start, end, _ := dayfile.Day(d.date)
			d.history = readings.NewDay(f.history, start.Add(-readingsBefore), start, end)
		}
		return slices.Values(d.history.Read())
	}
	return t.day(d.date, n, read, held, f.days.caps, asOf).made(lines, held)
}

// tailed is the ledger's files of a local day, by its date, read as they
// grow, as a dayfile.Tail reads them: warned is how many of their lines that
// couldn't be read were last warned of.
type tailed[T any] struct {
	*dayfile.Tail[T]
	date   string
	warned int
}

// read reads what the day's files gained, as the Tail's Read does, and
// warns of how many of their lines couldn't be read, where more can't than
// were warned of before.
func (t *tailed[T]) read(logger *slog.Logger, afresh func(), take func(T)) bool {
	ok := t.Read(afresh, take)
	_, unread := t.Lines()
	if unread > t.warned {
		logger.Warn("request ledger lines unread", "day", t.date, "lines", unread)
	}
	t.warned = unread
	return ok
}

// Mark is how far one who reads today's lines has read them, as Today gives
// it: the zero Mark has read none.
type Mark struct {
	gen, read int
}

// Today returns the lines of the requests that arrived today, until now, read
// since mark, oldest first, by when each arrived, then the date of the day's
// file each is in, and its place there, as Reader.Lines orders them, and the
// Mark they're read to. Each is the line it reads as alone, none of its JSON
// held, as the dashboard that follows them never reads it. A verb that writes
// lines as the ledger holds them reads them through a Reader. Where mark is
// the zero Mark, or one of today's lines as they were before they were read
// afresh, as when the day turned since, or one of the days' files was
// replaced, or shrunk, they're every one of today's, as Reader.Lines gives
// them from today's start, and afresh reports it, for those read before to be
// let go of. Asking with a mark lets go of the lines given before it, so the
// follower holds only those given since the mark last asked with, and gives
// them again to one who asks with it again: a mark before it, or the zero
// Mark, has every one of today's given afresh, those let go of read again
// from the days' files, and every one it gives let go of. A line still being
// written, cut short, is given once
// it's whole. The days' files are read as they grow, those of the day
// before's to the day after's, as a change of time zone files a line under a
// date beside its own.
func (f *Follower) Today(mark Mark) (lines []Held, next Mark, afresh bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t := &f.today
	t.read(f.days, f.now())
	given, afresh := t.since(f.days, mark)
	slices.SortFunc(given, byArrival)
	lines = make([]Held, len(given))
	for i, a := range given {
		lines[i] = a.Held
	}
	return lines, t.mark(), afresh
}

// Day returns the lines of the requests that arrived on the local day with
// the given date, by now, oldest first, by when each arrived, then the date
// of the day's file each is in, and its place there, as Today orders them:
// read whole, at each call, from the day's files and those of the days either
// side of it, as a change of time zone files a line under a date beside its
// own, and a request that ends after midnight is its arrival day's. Each is
// the line it reads as alone, none of its JSON held, as Today gives them. It
// gives none of a date that isn't one, or of a day whose files hold none.
// How many lines couldn't be read is warned of.
func (f *Follower) Day(date string) []Held {
	start, end, ok := dayfile.Day(date)
	if !ok {
		return nil
	}
	now := f.now()
	within := func(h Held) bool { return !h.At.Before(start) && h.At.Before(end) && !h.At.After(now) }
	var lines []Held
	f.days.lines(dayfile.Dates(start, start), lineHeldIn, within, func(h Held) bool {
		lines = append(lines, h)
		return true
	}, "day", date)
	return lines
}

// todaysLines are today's lines as a Follower reads them as they grow:
// those of the requests that arrived from today's start, start, on, in the
// files of the day before's to the day after's, as Reader.Lines reads them,
// by their dates. given are those given from base on, in the order they were
// read, of those arrived by when they were, base counting those given before
// them, let go of as a mark later than base was asked with, or as every one
// of today's was given. pending are those read that arrived later;
// gen counts the times today's lines began to be given afresh.
type todaysLines struct {
	start   time.Time
	files   map[string]*arrivals
	given   []*placed
	base    int
	pending []*placed
	gen     int
}

// arrivals are the files of a local day as today's lines read them as they
// grow, holding none of their lines: taken counts every line read since they
// were last read afresh, and kept those of them that arrived from today's
// start on.
type arrivals struct {
	tailed[Held]
	taken, kept int
}

// placed is a line read, as Today and Session order them: the date of the
// day's files it's in, and its place among the lines read of them.
type placed struct {
	Held
	date  string
	place int
}

// byArrival orders lines read as Today does: by when each arrived, then the
// date of the day's file each is in, and its place there.
func byArrival(a, b *placed) int {
	return cmp.Or(a.At.Compare(b.At), strings.Compare(a.date, b.date), cmp.Compare(a.place, b.place))
}

// read reads what the files of today's lines gained by now, giving those that
// arrived by now, and giving every one afresh, as begin does, where the day
// turned to now's, or a file was read afresh.
func (t *todaysLines) read(d days, now time.Time) {
	if t.turn(now) {
		t.begin(d, now)
		return
	}
	var gained []*placed
	for _, date := range dayfile.Dates(t.start, now) {
		read, afresh := t.file(d, date).read(d.logger, t.start)
		if afresh {
			t.begin(d, now)
			return
		}
		gained = append(gained, read...)
	}
	t.give(append(gained, t.pending...), now)
}

// turn has today's lines turn to now's day where it's another, as after
// midnight, or a clock set back, reporting whether it is.
func (t *todaysLines) turn(now time.Time) bool {
	start := dayfile.DayStart(now.Local(), 0)
	if start.Equal(t.start) {
		return false
	}
	t.start = start
	return true
}

// file returns the files of the local day with the given date as today's
// lines read them, none read yet where they're new to them.
func (t *todaysLines) file(d days, date string) *arrivals {
	file, ok := t.files[date]
	if !ok {
		file = &arrivals{Tail: dayfile.NewTail(d.files, date, lineHeldIn), date: date}
		t.files[date] = file
	}
	return file
}

// lineHeldIn returns the line a line of the ledger holds, as Today and Day
// give it, none of its JSON held, reporting false for one that doesn't read
// as a line.
func lineHeldIn(data []byte) (Held, bool) {
	line, ok := lineIn(data)
	if !ok {
		return Held{}, false
	}
	return Held{Line: line}, true
}

// read reads what the day's files gained, returning those that arrived from
// start on, and whether it read the files afresh where they'd given any such
// before: a first read of them, or one afresh of files that gave none, lets
// go of nothing.
func (a *arrivals) read(logger *slog.Logger, start time.Time) (gained []*placed, afresh bool) {
	a.tailed.read(logger, func() {
		afresh = a.kept > 0
		a.taken, a.kept, gained = 0, 0, nil
	}, a.placing(start, &a.taken, func(line *placed) {
		gained = append(gained, line)
		a.kept++
	}))
	return gained, afresh
}

// placing returns what takes each line read of the day's files, in the order
// they hold them, counted by taken, which counts every one, handing keep
// those that arrived from start on, each placed where taken counted it: so a
// read of them as they grow, and one again from their start, place them
// alike.
func (a *arrivals) placing(start time.Time, taken *int, keep func(*placed)) func(Held) {
	return func(h Held) {
		if !h.At.Before(start) {
			keep(&placed{Held: h, date: a.date, place: *taken})
		}
		*taken++
	}
}

// readAgain reads again, from the start of the day's files, the lines read
// of them that arrived from start on, as far as they were read, but those
// held reports true of, by their places. Where it can't read as far, which is
// warned of where a file can't be read, the files are read afresh at their
// next read, so the lines it couldn't give are given then.
func (a *arrivals) readAgain(d days, start time.Time, held func(place int) bool) []*placed {
	if a.kept == 0 {
		return nil
	}
	var lines []*placed
	read := 0
	take := a.placing(start, &read, func(line *placed) {
		if !held(line.place) {
			lines = append(lines, line)
		}
	})
	_, _, err := dayfile.ReadDay(d.files, a.date, lineHeldIn, func(h Held) bool {
		take(h)
		return read < a.taken
	})
	if err != nil {
		d.logger.Warn("can't read the request ledger", "day", a.date, "error", err)
	}
	if err != nil || read < a.taken {
		a.Tail = dayfile.NewTail(d.files, a.date, lineHeldIn)
	}
	return lines
}

// begin gives today's lines afresh: every one of the days' files, read again
// from their start, that arrived by now, each file's unread lines warned of
// only where more can't be read than were last warned of.
func (t *todaysLines) begin(d days, now time.Time) {
	was := t.files
	t.files = make(map[string]*arrivals)
	var all []*placed
	for _, date := range dayfile.Dates(t.start, now) {
		file := t.file(d, date)
		if before, ok := was[date]; ok {
			file.warned = before.warned
		}
		read, _ := file.read(d.logger, t.start)
		all = append(all, read...)
	}
	t.given, t.base, t.pending, t.gen = nil, 0, nil, t.gen+1
	t.give(all, now)
}

// give gives those of lines that arrived by now, after those given before,
// and holds the others pending until they have.
func (t *todaysLines) give(lines []*placed, now time.Time) {
	t.pending = nil
	for _, line := range lines {
		if line.At.After(now) {
			t.pending = append(t.pending, line)
		} else {
			t.given = append(t.given, line)
		}
	}
}

// since returns the lines given since mark, and whether they're every one of
// today's, given afresh, as Today says: those given since a mark of them as
// they're given now, letting go of those before it; else every one, those
// let go of read again, letting go of every one.
func (t *todaysLines) since(d days, mark Mark) ([]*placed, bool) {
	from, afresh := mark.read, mark.gen != t.gen
	if afresh {
		from = 0
	}
	if !afresh && from >= t.base {
		t.letGo(from)
		return slices.Clone(t.given), false
	}
	var whole []*placed
	if from < t.base {
		whole = t.readAgain(d)
	}
	whole = append(whole, t.given...)
	t.letGo(t.base + len(t.given))
	return whole, true
}

// letGo lets go of the lines given before read, counted as a Mark's read
// counts them.
func (t *todaysLines) letGo(read int) {
	if read > t.base {
		// Cloned, as the array behind a slice of it would hold on to every line
		// let go of.
		t.given, t.base = slices.Clone(t.given[read-t.base:]), read
	}
}

// mark is the Mark of today's lines read to the last given.
func (t *todaysLines) mark() Mark {
	return Mark{gen: t.gen, read: t.base + len(t.given)}
}

// readAgain reads again the lines given before base, let go of: those the
// days' files gave as far as they've been read, but those still held, given
// since or pending. As far as they've been read, whatever they gained since,
// so the next read gives what they gained, each line once.
func (t *todaysLines) readAgain(d days) []*placed {
	type spot struct {
		date  string
		place int
	}
	held := make(map[spot]bool, len(t.given)+len(t.pending))
	for _, line := range slices.Concat(t.given, t.pending) {
		held[spot{line.date, line.place}] = true
	}
	var gone []*placed
	for date, file := range t.files {
		gone = append(gone, file.readAgain(d, t.start, func(place int) bool { return held[spot{date, place}] })...)
	}
	return gone
}

// Session returns the lines of the session with the given id that arrived by
// now, newest first, in the order Reader.Lines gives them turned about, read
// as far back as the caller goes on, of the days' files Reader.Lines reads, as
// linesDates gives them. They're read of its days alone, those whose
// summaries, as Days gives them, name it among their sessions' ids, or, never
// having read them, might, as names tells without the readings history, and
// any after today's, which none summarises, as after a change of time zone,
// or a clock set back: so a session's quota checks, which a summary counts
// toward no session, are passed over on a day it made no other request.
func (f *Follower) Session(id string) iter.Seq[Held] {
	return func(yield func(Held) bool) {
		now := f.now()
		lines := newestFirst{take: yield}
		unread := 0
		defer func() {
			if unread > 0 {
				f.days.logger.Warn("request ledger lines unread", "lines", unread)
			}
		}()
		f.mu.Lock()
		dates := f.linesDates(now)
		f.mu.Unlock()
		wanted := map[string]bool{id: true}
		for _, date := range slices.Backward(dates) {
			start, end, _ := dayfile.Day(date)
			if start.After(now) || f.namesLocked(date, wanted) {
				unread += f.readSession(date, id, now, &lines)
			}
			// No earlier day's file holds a line of a later day than this one.
			if !lines.handOn(end) {
				return
			}
		}
		lines.handOn(time.Time{})
	}
}

// linesDates returns the dates of the local days whose files Reader.Lines
// reads of every line, at now, as dayfile.Dates gives them: from the day
// before the one the ledger begins on, as Begins says, to the day after
// today's; or the days either side of today's, and today's, where the ledger
// can't be looked at to tell, which is warned of.
func (f *Follower) linesDates(now time.Time) []string {
	first, err := f.firstDay(now)
	start, _, err := Begins(first, err, now)
	if err != nil {
		f.days.logger.Warn("can't read the request ledger", "dir", f.days.files.Dir, "error", err)
		start = now
	}
	return dayfile.Dates(start, now)
}

// namesLocked reports whether the local day with the given date names any of
// the sessions wanted, as names says, holding the follower as it looks.
func (f *Follower) namesLocked(date string, wanted map[string]bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.names(date, wanted)
}

// naming returns those of the sessions wanted the summary names among those
// its accounts' requests were of, or might, by their ids: one that counts
// sessions it doesn't name, as Unnamed says, as one of version 1, might name
// any, so every one.
func (s Summary) naming(wanted map[string]bool) map[string]bool {
	if s.Unnamed() > 0 {
		return maps.Clone(wanted)
	}
	named := make(map[string]bool)
	for _, a := range s.Accounts {
		for _, id := range a.SessionIDs {
			if wanted[id] {
				named[id] = true
			}
		}
	}
	return named
}

// readSession holds in lines those of the local day with the given date
// that are of the session with the given id, and arrived by now, as ReadDay
// reads them, and returns how many lines of the day couldn't be read: none
// of a day pruned, and those of the files that could be read of one whose
// other can't be, which is warned of.
func (f *Follower) readSession(date, id string, now time.Time, lines *newestFirst) (unread int) {
	_, unread, err := dayfile.ReadDay(f.days.files, date, sessionHeldIn(id), func(h Held) bool {
		if h.Session == id && !h.At.After(now) {
			lines.hold(h, date)
		}
		return true
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		f.days.logger.Warn("can't read the request ledger", "day", date, "error", err)
	}
	return unread
}

// sessionHeldIn returns what reads a line of the ledger as the line it
// holds, reporting false for one that doesn't read as a line, as heldIn does,
// but reads a line of another session than the one with the given id for
// its session alone, giving none of the rest, as keptLineIn does, so a read
// of a session over a busy day decodes its own lines alone; and holds the
// JSON of its own.
func sessionHeldIn(id string) func(data []byte) (Held, bool) {
	kept := keptLineIn(map[string]bool{id: true})
	return func(data []byte) (Held, bool) {
		line, ok := kept(data)
		if !ok || line.Session != id {
			return Held{Line: line}, ok
		}
		return Held{Line: line, JSON: slices.Clone(data)}, true
	}
}

// newestFirst hands take, newest first, as Session orders them, the lines
// held, each once no line read after it can come before it. held counts
// every line held, its place among them.
type newestFirst struct {
	take  func(Held) bool
	lines []*placed
	held  int
}

// hold holds h, a line of the day's file with the given date, after those
// held before.
func (n *newestFirst) hold(h Held, date string) {
	n.lines = append(n.lines, &placed{Held: h, date: date, place: n.held})
	n.held++
}

// handOn hands take, newest first, those held that arrived from until on,
// and reports whether take wanted more.
func (n *newestFirst) handOn(until time.Time) bool {
	slices.SortFunc(n.lines, func(a, b *placed) int { return byArrival(b, a) })
	ready := slices.IndexFunc(n.lines, func(a *placed) bool { return a.At.Before(until) })
	if ready < 0 {
		ready = len(n.lines)
	}
	for _, a := range n.lines[:ready] {
		if !n.take(a.Held) {
			return false
		}
	}
	n.lines = slices.Delete(n.lines, 0, ready)
	return true
}
