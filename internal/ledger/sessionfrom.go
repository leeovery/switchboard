package ledger

import (
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// SessionFrom is a session as of the start of a local day, as the ledger
// finds it for one who folds its lines into its state, the router's digester
// and a reader alike: Base, the state it rests on, as the digest of an
// earlier day holds it, nil where it rests on none; and Lines, its lines
// after the base, read raw, by the days' files, every one where it rests on
// none, oldest first, as oldestFirst orders them. restsOn is what a
// state folded from it rests on, as a digest's entry keeps it, and
// sessionsLookup.restsOn gives it: nil where that can't be told, so the
// state isn't one to keep. A session's state as of the end of a day is
// folded from it as of the start of the day after, which reads the day's own
// files, so the day is among what the state rests on, as the days naming the
// session up to its own must be for it to stand.
type SessionFrom struct {
	Base    *SessionBase
	Lines   []Line
	restsOn map[string]int
}

// SessionsWhole returns the sessions whose lines are given, by their ids,
// each as SessionFrom has one read whole, resting on no base, its lines every
// one given: as a read of a ledger that holds no digest gives them.
func SessionsWhole(lines map[string][]Line) map[string]SessionFrom {
	sessions := make(map[string]SessionFrom, len(lines))
	for id, held := range lines {
		sessions[id] = SessionFrom{Lines: held}
	}
	return sessions
}

// SessionBase is the state a session rests on, as the digest of the local
// day with the date Date holds it: its entry there, the days the state was
// folded from, and the state, its builder's, carried as it's held.
type SessionBase struct {
	Date string
	DigestEntry
}

// daysLook is a look at the ledger's days, as one who asks for sessions as of
// a day has its own, the router's writer and a reader alike, of the version
// of the sessions' states it reads: naming gives those of the sessions
// wanted the local day with the given date names among its sessions' ids, or
// might, by their ids; kept the lines the ledger keeps of a day, as
// digestGrounds' kept gives them; digest the digest the ledger holds of a
// day, as days.digest reads it, and head its head alone, as days.digestHead
// reads it, each reporting false where it holds none that reads.
type daysLook interface {
	naming(date string, wanted map[string]bool) map[string]bool
	kept(date string) (lines int, known bool)
	digest(date string) (Digest, bool)
	head(date string) (DigestHead, bool)
}

// lookup returns a lookup of sessions by the follower's own look at the
// ledger's days, as followerLook has it, having looked at none yet.
func (f *Follower) lookup() *sessionsLookup {
	return newSessionsLookup(f.days, followerLook{f: f})
}

// followerLook is a Follower's look at the ledger's days, each as follow
// finds it, and the digests of them of the version it reads, holding the
// follower as it looks.
type followerLook struct {
	f *Follower
}

// digest gives the digest the ledger holds of the local day with the given
// date, of the version the follower reads, as days.digest reads it,
// reporting false where it holds none that reads.
func (l followerLook) digest(date string) (Digest, bool) {
	digest, err := l.f.days.digest(date, l.f.body)
	return digest, err == nil
}

// head gives the head of the digest the ledger holds of the local day with
// the given date, as Follower.digestHead holds it.
func (l followerLook) head(date string) (DigestHead, bool) {
	l.f.mu.Lock()
	defer l.f.mu.Unlock()
	return l.f.digestHead(date)
}

// naming gives those of the sessions wanted the local day with the given
// date names, or might, as Follower.naming says.
func (l followerLook) naming(date string, wanted map[string]bool) map[string]bool {
	l.f.mu.Lock()
	defer l.f.mu.Unlock()
	return l.f.naming(date, wanted)
}

// kept gives the lines the ledger keeps of the local day with the given
// date, as digestGrounds' kept gives them: none of a day with no files,
// whatever its summary counted; the Lines of its summary where its files
// hold exactly as many, as follow finds; and none known where its files
// can't be looked at, or it has no summary that stands, or one that stands
// for more lines than they hold, as after one was lost to damage, as a read
// of them reads fewer than a state folded from them did.
func (l followerLook) kept(date string) (int, bool) {
	l.f.mu.Lock()
	defer l.f.mu.Unlock()
	d := l.f.follow(date)
	switch {
	case d.look.err != nil:
		return 0, false
	case d.look.none:
		return 0, true
	case d.exact:
		return d.held.Lines, true
	}
	return 0, false
}

// writerLook is the router's writer's look at the ledger's days, made for
// one of the digesting's passes: each day found once, as it's first asked
// of, as find finds it; and its digest read, and its digest's head, of the
// sessions' states of the version body, as it's asked of.
type writerLook struct {
	days   days
	body   int
	looked map[string]writtenDay
	find   func(date string) writtenDay
}

// digest gives the digest the ledger holds of the local day with the given
// date, as days.digest reads it, reporting false where it holds none that
// reads.
func (l writerLook) digest(date string) (Digest, bool) {
	digest, err := l.days.digest(date, l.body)
	return digest, err == nil
}

// head gives the head of the digest the ledger holds of the local day with
// the given date, as days.digestHead reads it, reporting false where it
// holds none that reads.
func (l writerLook) head(date string) (DigestHead, bool) {
	head, err := l.days.digestHead(date, l.body)
	return head, err == nil
}

// naming gives those of the sessions wanted the local day with the given
// date names, or might, as writtenDay.naming says.
func (l writerLook) naming(date string, wanted map[string]bool) map[string]bool {
	return l.day(date).naming(wanted)
}

// kept gives the lines the ledger keeps of the local day with the given
// date, as writtenDay.kept says.
func (l writerLook) kept(date string) (int, bool) {
	return l.day(date).kept()
}

// day returns the local day with the given date as the look finds it,
// finding it where it hasn't yet.
func (l writerLook) day(date string) writtenDay {
	day, looked := l.looked[date]
	if !looked {
		day = l.find(date)
		l.looked[date] = day
	}
	return day
}

// writtenDay is a local day as the writer's look finds it, as written finds
// it: look, its summary's file and its files, as looked at; ids, the
// sessions it names, sorted, each once, and every, whether it might name any
// other, as named gives them; and lines, the lines it keeps, where known, as
// kept gives them. It holds nothing more of its summary, as a day held for
// the router's life.
type writtenDay struct {
	look  look
	ids   []string
	every bool
	lines int
	known bool
}

// written returns the local day with the given date as the writer's look
// finds it, its summary's file and its files as l looks at them, its
// summary read now: the sessions it names, as its summary names them, as
// Summary.named says, where it stands, or the day has no files; none of a
// day with neither files nor a summary that reads; and any, where it has
// files without a summary that stands, as only a read of them would tell.
// And the lines it keeps: none of a day with no files; the Lines of its
// summary where its files hold exactly as many, as counting tells; and none
// known else, as followerLook's kept gives them.
func (d days) written(date string, l look) writtenDay {
	day := writtenDay{look: l, every: !l.none, known: l.none}
	held, err := d.held(date)
	if err != nil {
		return day
	}
	stands, exact := l.none, false
	if l.err == nil && !l.none {
		s, e, err := d.counting(held.Summary, l.files)
		stands, exact = err == nil && s, err == nil && e
	}
	if stands {
		day.ids, day.every = held.named()
	}
	if exact {
		day.lines, day.known = held.Lines, true
	}
	return day
}

// named returns the ids of the sessions the day names, sorted, each once,
// and whether it might name any other, as written finds them.
func (w writtenDay) named() (ids []string, every bool) {
	return w.ids, w.every
}

// naming gives those of the sessions wanted the day names, or might, as
// named tells them, by their ids.
func (w writtenDay) naming(wanted map[string]bool) map[string]bool {
	return namingAmong(wanted, w.ids, w.every)
}

// kept gives the lines the ledger keeps of the day, where they're known, as
// written finds them.
func (w writtenDay) kept() (int, bool) {
	return w.lines, w.known
}

// sessionsLookup is a lookup of sessions as of a local day: the ledger's
// days, and the asker's look at them; and, held so each day is looked at,
// its digest read and its digest's head read once, the lines the ledger kept
// of each day as the look first gave them, each day's digest, nil where it
// holds none that reads, and each day's digest's head, as the look first
// gave them.
type sessionsLookup struct {
	days    days
	look    daysLook
	kept    map[string]keptLines
	digests map[string]*Digest
	heads   map[string]headLooked
}

// headLooked is the head of a day's digest as a look gave it: held false
// where the ledger holds none that reads.
type headLooked struct {
	head DigestHead
	held bool
}

// newSessionsLookup returns a lookup of sessions in the ledger's days d, by
// the asker's own look at them, having looked at none yet.
func newSessionsLookup(d days, look daysLook) *sessionsLookup {
	return &sessionsLookup{days: d, look: look, kept: make(map[string]keptLines), digests: make(map[string]*Digest), heads: make(map[string]headLooked)}
}

// asOf returns the sessions wanted as of the start of a local day, as
// SessionFrom has them, by their ids: none of one resting on none none of
// whose days holds a line of it. dates are those of the days before that
// day, from where the ledger begins, oldest first, as datesBefore gives
// them; and near those of the near days, from that day on, oldest first,
// whose lines are every session's after whatever it rests on, as a reader's
// sessions read today's. Each session rests on the newest state the digest
// of a day naming it holds, of the lookup's version, that stands for it, as
// standsFor says, but one whole reports true of, where it's given, which
// rests on none; and its lines are those of the days naming it after that
// one, then the near days', read through read, by the days' files, each
// day's once for every session wanting them, as days reads them. It rests on
// none, its lines those of every day naming it and the near days', where one
// of the days naming it can't be told, as told says; and where, once the days
// after its base are read, one of them couldn't be read whole, or one of the
// lines it hands on after its base arrived before the base's day ended, as a
// change of time zone files a line under a later date than its own: a state
// sealed at that day's end can't take it in. take, where read hands one each
// line read, is handed them in the order they're read, not oldest first: the
// days read for the sessions' bases, oldest first, but those read before,
// then any before them read once a session's base falls.
func (l *sessionsLookup) asOf(dates, near []string, wanted map[string]bool, whole func(id string) bool, read *sessionsRead) map[string]SessionFrom {
	asked := l.namingEach(dates, near, wanted)
	for id, s := range asked {
		if whole == nil || !whole(id) {
			l.rest(id, s)
		}
	}
	l.readFor(read, asked, wanted)
	for id, s := range asked {
		if s.base != nil && s.unsettled(read.held[id], read.failed) {
			s.base = nil
		}
	}
	l.readFor(read, asked, wanted)
	return l.from(asked, read)
}

// nearDays returns the dates of the near days at now, oldest first, whose
// files a reader's sessions read raw, every session's: today's, and the day
// after's, as a clock set back files a line under a day after today's; and
// the day before's, as a change of time zone files a line under the date
// beside its own, while its digest doesn't stand, as standing says. One whose
// digest stands holds no line that arrived after it ended, nor any filed
// since, so none of today's: the sessions it names read it as a day before
// the near days.
func (l *sessionsLookup) nearDays(now time.Time) []string {
	dates := dayfile.Dates(now, now)
	if l.standing(dates[0]) {
		return dates[1:]
	}
	return dates
}

// standing reports whether the digest the ledger holds of the local day with
// the given date stands, as digestGrounds.stands judges its head alone.
func (l *sessionsLookup) standing(date string) bool {
	head, held := l.head(date)
	return held && l.grounds().stands(head, DigestEntry{})
}

// keptLines are the lines the ledger keeps of a day, as daysLook's kept gives
// them.
type keptLines struct {
	lines int
	known bool
}

// sessionAsked is a session as a lookup finds it: naming, the dates of the
// days before the one it's asked as of naming it, oldest first, as the
// asker's look tells; near, those of the near days, every session's lines
// read of them; base, the state it rests on, nil for none; and ended, when
// its base's day ended, as the digest's head says.
type sessionAsked struct {
	naming []string
	near   []string
	base   *SessionBase
	ended  time.Time
}

// namingEach returns the sessions wanted, by their ids, each with the dates
// of those of the days with the given dates naming it, or that might, as the
// look tells, in the order given, and of the near days: no day looked at
// where none is wanted.
func (l *sessionsLookup) namingEach(dates, near []string, wanted map[string]bool) map[string]*sessionAsked {
	asked := make(map[string]*sessionAsked, len(wanted))
	for id := range wanted {
		asked[id] = &sessionAsked{near: near}
	}
	if len(wanted) == 0 {
		return asked
	}
	for _, date := range dates {
		for id := range l.look.naming(date, wanted) {
			asked[id].naming = append(asked[id].naming, date)
		}
	}
	return asked
}

// rest rests the session asked, with the given id, on the newest state the
// digest of a day naming it holds that stands for it, as standsFor says: on
// none where a day naming it can't be told, as told says, as what of its
// lines that day holds can't be.
func (l *sessionsLookup) rest(id string, s *sessionAsked) {
	if !l.told(s.naming) {
		return
	}
	for i, date := range slices.Backward(s.naming) {
		digest := l.digest(date)
		if digest == nil {
			continue
		}
		if entry, ok := digest.Sessions[id]; ok && l.standsFor(digest.Head, entry, s.naming[:i+1]) {
			s.base, s.ended = &SessionBase{Date: date, DigestEntry: entry}, digest.Head.End
			return
		}
	}
}

// standsFor reports whether entry, a session's in the digest whose head is
// head, stands, as digestGrounds.stands says, and rests on exactly the days
// naming the session up to its own, naming, as restsOnEach says.
func (l *sessionsLookup) standsFor(head DigestHead, entry DigestEntry, naming []string) bool {
	return restsOnEach(entry, naming) && l.grounds().stands(head, entry)
}

// restsOnEach reports whether entry, a session's in a digest, rests on
// exactly the days naming the session up to the digest's own, naming: a line
// of the session filed under a day it never ran on, as after a clock set
// back, has that day's summary name it, and a read of its lines then reads
// that day, whose line a state that doesn't rest on it never took in.
func restsOnEach(entry DigestEntry, naming []string) bool {
	return len(entry.RestsOn) == len(naming) && !slices.ContainsFunc(naming, func(date string) bool {
		_, rests := entry.RestsOn[date]
		return !rests
	})
}

// grounds are the grounds the lookup judges a digest on, as it finds the
// ledger.
func (l *sessionsLookup) grounds() digestGrounds {
	return digestGrounds{kept: l.keptOn, heads: l.head}
}

// told reports whether the lines the ledger keeps of each of the days with
// the given dates can be told, as kept tells them.
func (l *sessionsLookup) told(dates []string) bool {
	return !slices.ContainsFunc(dates, func(date string) bool {
		_, known := l.keptOn(date)
		return !known
	})
}

// keptOn gives the lines the ledger keeps of the local day with the given
// date, as the look gave them when the lookup first asked.
func (l *sessionsLookup) keptOn(date string) (int, bool) {
	k, asked := l.kept[date]
	if !asked {
		k.lines, k.known = l.look.kept(date)
		l.kept[date] = k
	}
	return k.lines, k.known
}

// head gives the head of the digest the ledger holds of the local day with
// the given date, as the look gives it, where it hasn't yet: reporting false
// where it holds none that reads. None of the digest's sessions is read for
// it.
func (l *sessionsLookup) head(date string) (DigestHead, bool) {
	h, asked := l.heads[date]
	if !asked {
		h.head, h.held = l.look.head(date)
		l.heads[date] = h
	}
	return h.head, h.held
}

// digest returns the digest the ledger holds of the local day with the given
// date, as the look gives it, where it hasn't yet: nil where it holds none
// that reads.
func (l *sessionsLookup) digest(date string) *Digest {
	digest, read := l.digests[date]
	if !read {
		if held, ok := l.look.digest(date); ok {
			digest = &held
		}
		l.digests[date] = digest
	}
	return digest
}

// readFor reads through read the files of the days whose lines the sessions
// asked are read from, as reads gives them, but those read before, as days
// passes them over, holding the lines of those wanted, oldest day first.
func (l *sessionsLookup) readFor(read *sessionsRead, asked map[string]*sessionAsked, wanted map[string]bool) {
	dates := make(map[string]bool)
	for _, s := range asked {
		for _, date := range s.reads() {
			dates[date] = true
		}
	}
	read.days(l.days, slices.Sorted(maps.Keys(dates)), wanted)
}

// reads returns the dates of the days whose lines are read for the session,
// oldest first: those naming it after its base's, every one where it rests on
// none, then the near days.
func (s *sessionAsked) reads() []string {
	naming := s.naming
	if s.base != nil {
		naming = naming[slices.Index(naming, s.base.Date)+1:]
	}
	return slices.Concat(naming, s.near)
}

// unsettled reports whether the session, resting on a base, can't, once the
// days after the base are read, held its lines read, and failed the dates of
// the days that couldn't be read whole: one of those days is among them; or
// one of its lines read there arrived before its base's day ended.
func (s *sessionAsked) unsettled(held []filed, failed map[string]bool) bool {
	reads := s.reads()
	if slices.ContainsFunc(reads, func(date string) bool { return failed[date] }) {
		return true
	}
	return slices.ContainsFunc(filedOn(held, reads), func(l filed) bool { return l.At.Before(s.ended) })
}

// from returns the sessions asked as SessionFrom has them, by their ids, each
// with its lines of those read held, the dates of the days that couldn't be
// read whole failed among it: none of one resting on none that none of them
// are of. The read lets go of each session's lines once they're given, so
// what it read and what's given are held side by side for one session at a
// time, never for every one.
func (l *sessionsLookup) from(asked map[string]*sessionAsked, read *sessionsRead) map[string]SessionFrom {
	sessions := make(map[string]SessionFrom, len(asked))
	for id, s := range asked {
		held := filedOn(read.held[id], s.reads())
		delete(read.held, id)
		if s.base == nil && len(held) == 0 {
			continue
		}
		from := SessionFrom{Base: s.base, restsOn: l.restsOn(s, read.failed)}
		if len(held) > 0 {
			from.Lines = oldestFirst(held)
		}
		sessions[id] = from
	}
	return sessions
}

// restsOn returns what a state folded from the session asked rests on, as a
// digest's entry keeps it: each day naming it, its base's and those its lines
// were read from, by its date, with the lines the ledger kept of it as kept
// gave them before any of their lines were read, so a day whose lines change
// since stops the state standing, as digestGrounds.stands says. It gives
// none where one of those days can't be told, as told says, or one read for
// it, failed among them, couldn't be read whole: the state couldn't be told
// to hold what a read of its lines holds. Nor where its lines are read of
// near days too: a state folded from today's lines, which go on growing, is
// none to keep.
func (l *sessionsLookup) restsOn(s *sessionAsked, failed map[string]bool) map[string]int {
	if len(s.near) > 0 || !l.told(s.naming) || slices.ContainsFunc(s.reads(), func(date string) bool { return failed[date] }) {
		return nil
	}
	restsOn := make(map[string]int, len(s.naming))
	for _, date := range s.naming {
		restsOn[date], _ = l.keptOn(date)
	}
	return restsOn
}

// filedOn returns those of held filed under the days with the given dates,
// afresh, held left as it is.
func filedOn(held []filed, dates []string) []filed {
	on := make(map[string]bool, len(dates))
	for _, date := range dates {
		on[date] = true
	}
	return slices.DeleteFunc(slices.Clone(held), func(l filed) bool { return !on[l.date] })
}
