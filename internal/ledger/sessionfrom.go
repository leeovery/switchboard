package ledger

import (
	"slices"
	"time"
)

// SessionFrom is a session as of the start of a local day, as the ledger
// finds it for one who folds its lines into its state, the router's digester
// and a reader alike: Base, the state it rests on, as the digest of an
// earlier day holds it, nil where it rests on none; and Lines, its lines
// after the base, read raw, by the days' files, every one where it rests on
// none, oldest first, as sessionsRead.of orders them. restsOn is what a
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

// SessionBase is the state a session rests on, as the digest of the local
// day with the date Date holds it: its entry there, the days the state was
// folded from, and the state, its builder's, carried as it's held.
type SessionBase struct {
	Date string
	DigestEntry
}

// daysLook is a look at the ledger's days, as one who asks for sessions as of
// a day has its own, the router's writer and a reader alike: naming gives
// those of the sessions wanted the local day with the given date names among
// its sessions' ids, or might, by their ids; and kept the lines the ledger
// keeps of a day, as digestGrounds' kept gives them.
type daysLook interface {
	naming(date string, wanted map[string]bool) map[string]bool
	kept(date string) (lines int, known bool)
}

// sessionsAsOf returns the sessions with the given ids as of the start of
// the local day with the given date, by the follower's own look at the
// ledger's days, as followerLook has it, their states of the version body,
// as sessionsAsOf has them: take, where it's given, handed each line read,
// with the date of the day whose files hold it, in the order they're read,
// not oldest first: the days read for the sessions' bases, oldest first,
// then any before them read once a session's base falls.
func (f *Follower) sessionsAsOf(ids []string, date string, body int, take func(date string, l Line)) map[string]SessionFrom {
	read := newSessionsRead(f.now(), take)
	f.mu.Lock()
	dates := f.datesBefore(date, read.now)
	f.mu.Unlock()
	return sessionsAsOf(f.days, followerLook{f: f}, dates, ids, body, read)
}

// followerLook is a Follower's look at the ledger's days, each as follow
// finds it, holding the follower as it looks.
type followerLook struct {
	f *Follower
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
// of, as find finds it.
type writerLook struct {
	looked map[string]writtenDay
	find   func(date string) writtenDay
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

// sessionsAsOf returns the sessions with the given ids as of the start of a
// local day, as SessionFrom has them, by their ids: none of one no day
// names, or of one resting on none whose days hold none of its lines. dates
// are those of the days before that day, from where the ledger begins,
// oldest first, as datesBefore gives them, and look the asker's own look at
// them. Each session rests on the newest state the digest of a day naming it
// holds, of the version body, that stands for it, as standsFor says; and its
// lines are those of the days naming it after that one, read through read,
// by the days' files, each day's once for every session wanting them. It
// rests on none, its lines those of every day naming it, where one of those
// days can't be told, as told says; and where, once the days after its base
// are read, one of them couldn't be read whole, or one of the lines it hands
// on after its base arrived before the base's day ended, as a change of time
// zone files a line under a later date than its own: a state sealed at that
// day's end can't take it in.
func sessionsAsOf(d days, look daysLook, dates, ids []string, body int, read *sessionsRead) map[string]SessionFrom {
	l := &sessionsLookup{days: d, look: look, body: body, kept: make(map[string]keptLines), digests: make(map[string]*Digest), read: make(map[string]bool)}
	wanted := wanting(make(map[string]bool), ids)
	asked := make(map[string]*sessionAsked)
	for id, naming := range namingEach(look, dates, wanted) {
		asked[id] = &sessionAsked{naming: naming}
		l.rest(id, asked[id])
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

// sessionsLookup is a lookup of sessions as of a local day, as sessionsAsOf
// makes one: the ledger's days, the asker's look at them, and the version of
// the states its sessions rest on; and, held so each day is looked at, its
// digest read and its files read once, the lines the ledger kept of each day
// as the look first gave them, each day's digest, nil where it holds none
// that reads, and the days whose files have been read.
type sessionsLookup struct {
	days    days
	look    daysLook
	body    int
	kept    map[string]keptLines
	digests map[string]*Digest
	read    map[string]bool
}

// keptLines are the lines the ledger keeps of a day, as daysLook's kept gives
// them.
type keptLines struct {
	lines int
	known bool
}

// sessionAsked is a session as a lookup finds it: naming, the dates of the
// days naming it, oldest first, as the asker's look tells; base, the state it
// rests on, nil for none; and ended, when its base's day ended, as the
// digest's head says.
type sessionAsked struct {
	naming []string
	base   *SessionBase
	ended  time.Time
}

// namingEach returns the dates of the days among those given naming each of
// the sessions wanted that any of them names, or might, as look tells, in
// the order given, by the sessions' ids: none, and no day looked at, where
// none is wanted.
func namingEach(look daysLook, dates []string, wanted map[string]bool) map[string][]string {
	naming := make(map[string][]string)
	if len(wanted) == 0 {
		return naming
	}
	for _, date := range dates {
		for id := range look.naming(date, wanted) {
			naming[id] = append(naming[id], date)
		}
	}
	return naming
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
// the given date, as digest reads it, reporting false where it holds none
// that reads.
func (l *sessionsLookup) head(date string) (DigestHead, bool) {
	if digest := l.digest(date); digest != nil {
		return digest.Head, true
	}
	return DigestHead{}, false
}

// digest returns the digest the ledger holds of the local day with the given
// date, its sessions' states of the lookup's version, as days.digest reads
// it, where it hasn't yet: nil where it holds none that reads.
func (l *sessionsLookup) digest(date string) *Digest {
	digest, read := l.digests[date]
	if !read {
		if held, err := l.days.digest(date, l.body); err == nil {
			digest = &held
		}
		l.digests[date] = digest
	}
	return digest
}

// readFor reads through read the files of the days whose lines the sessions
// asked are read from, as reads gives them, but those read before, holding
// the lines of those wanted, oldest day first.
func (l *sessionsLookup) readFor(read *sessionsRead, asked map[string]*sessionAsked, wanted map[string]bool) {
	var dates []string
	for _, s := range asked {
		for _, date := range s.reads() {
			if !l.read[date] {
				l.read[date] = true
				dates = append(dates, date)
			}
		}
	}
	slices.Sort(dates)
	read.days(l.days, dates, wanted)
}

// reads returns the dates of the days whose lines are read for the session,
// oldest first: those naming it after its base's, every one where it rests on
// none.
func (s *sessionAsked) reads() []string {
	if s.base == nil {
		return s.naming
	}
	return s.naming[slices.Index(s.naming, s.base.Date)+1:]
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
// are of.
func (l *sessionsLookup) from(asked map[string]*sessionAsked, read *sessionsRead) map[string]SessionFrom {
	sessions := make(map[string]SessionFrom, len(asked))
	for id, s := range asked {
		held := filedOn(read.held[id], s.reads())
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
// to hold what a read of its lines holds.
func (l *sessionsLookup) restsOn(s *sessionAsked, failed map[string]bool) map[string]int {
	if !l.told(s.naming) || slices.ContainsFunc(s.reads(), func(date string) bool { return failed[date] }) {
		return nil
	}
	restsOn := make(map[string]int, len(s.naming))
	for _, date := range s.naming {
		restsOn[date], _ = l.keptOn(date)
	}
	return restsOn
}

// filedOn returns those of held filed under the days with the given dates.
func filedOn(held []filed, dates []string) []filed {
	on := make(map[string]bool, len(dates))
	for _, date := range dates {
		on[date] = true
	}
	var lines []filed
	for _, l := range held {
		if on[l.date] {
			lines = append(lines, l)
		}
	}
	return lines
}
