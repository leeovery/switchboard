package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// Digester folds the sessions of a finished local day into their states as
// of its end, for the ledger to keep in the day's digest: Version is the
// version of the states it gives, as a digest of states of another doesn't
// stand; and Digest gives the state of each session day hands it, by its id,
// failing where it can't fold them, or ctx ends first. A session handed
// resting on a state it gives none of is handed it again read whole, in a
// call of its own; one it gives no state of then has no entry in the digest,
// which leaves the day due: it's built again whenever it's judged afresh, as
// at each start of the router.
type Digester interface {
	Version() int
	Digest(ctx context.Context, day DigestDay) (map[string]json.RawMessage, error)
}

// DigestDay is a finished local day as the ledger hands it to its Digester:
// Date, its date; Start and End, when it started and ended, in this time
// zone; and Sessions, the sessions its summary names, as of the day's end,
// by their ids, as the ledger finds them: each on the newest state the
// digest of an earlier day holds that stands for it, or none, and its lines
// after that, the day's own among them, read by the days' files. A session
// whose state can't be kept, as a day it rests on can't be told, isn't among
// them; nor is one resting on the day's own digest still, whose state there
// is its state as of the day's end.
type DigestDay struct {
	Date       string
	Start, End time.Time
	Sessions   map[string]SessionFrom
}

// Option is how Open opens a ledger, beyond what it's given.
type Option func(*options)

// options are what the options given to Open set: the Digester the ledger
// digests its days with, nil for none.
type options struct {
	digester Digester
}

// Digesting has the ledger digest each finished day with digester, as
// digesting says: none where it's nil, as where the option isn't given.
func Digesting(digester Digester) Option {
	return func(o *options) { o.digester = digester }
}

// digesting is the ledger's digesting of its days with its Digester: a pass
// over them at each kick, on a goroutine of Run's, as run makes it, or at
// once, on DigestEnded's caller's, as pass says. It holds, from pass to
// pass, each day as it last found it, the digests it judged not due, and the
// days whose digests couldn't be built, so a pass that finds nothing changed
// reads no digest, and a failure is warned of once.
type digesting struct {
	digester Digester
	days     days
	now      func() time.Time
	// kicks holds one kick at most: one waiting stands for every kick since
	// the pass under way began.
	kicks chan struct{}
	// passing is held while a pass is made, as Run's goroutine and
	// DigestEnded's caller may each make one. Only a pass touches what
	// follows: held, each day as find last found it, by its date; judged,
	// each day's digest last judged not due; warned, each day whose digest
	// couldn't be built, or pruned, as it looked then; and unlisted, how the
	// passes' listings of the ledger's directory went.
	passing  sync.Mutex
	held     map[string]writtenDay
	judged   map[string]judgement
	warned   map[string]look
	unlisted failing
}

// newDigesting returns the digesting of the ledger's days d with digester,
// by now's clock, having made no pass yet.
func newDigesting(digester Digester, d days, now func() time.Time) *digesting {
	return &digesting{digester: digester, days: d, now: now, kicks: make(chan struct{}, 1),
		held: make(map[string]writtenDay), judged: make(map[string]judgement), warned: make(map[string]look)}
}

// run makes a pass as it starts, paced, at the clock's now, and again at
// each kick, until ctx ends.
func (d *digesting) run(ctx context.Context) {
	for {
		d.pass(ctx, d.now(), true)
		select {
		case <-ctx.Done():
			return
		case <-d.kicks:
		}
	}
}

// kick has run make a pass once it's done with the one it's making, if any,
// never waiting.
func (d *digesting) kick() {
	select {
	case d.kicks <- struct{}{}:
	default:
	}
}

// pass makes a pass over the ledger's days at now, but none once ctx has
// ended: it deletes the digests of days without lines, as prune does, even
// where the ledger holds no day; lets go of what it holds of the days before
// where the ledger begins, as letGoBefore does; then digests each finished
// day that's due, as digestDue does, pausing after each, where paced, as
// long as it took to build. Where the ledger's directory can't be listed, it
// does none of these, as listedNoting says.
func (d *digesting) pass(ctx context.Context, now time.Time, paced bool) {
	d.passing.Lock()
	defer d.passing.Unlock()
	if ctx.Err() != nil {
		return
	}
	listing, ok := d.days.listedNoting(&d.unlisted)
	d.prune(listing)
	if !ok {
		return
	}
	d.letGoBefore(listing.first)
	first, _, _ := dayfile.Day(listing.first)
	d.digestDue(ctx, d.days.datesBefore(dayfile.DateOf(now), first, nil, now), now, paced)
}

// letGoBefore lets go of the days before the one with the given date, where
// the ledger begins, as a prune of its earlier days leaves it, and of the
// judgements of the digests resting on one of them, as a state resting on a
// pruned day stops standing. No other judgement is on a day before where the
// ledger begins, as holds judges none that rests on one due.
func (d *digesting) letGoBefore(first string) {
	maps.DeleteFunc(d.held, func(date string, _ writtenDay) bool { return date < first })
	maps.DeleteFunc(d.judged, func(_ string, j judgement) bool {
		for date := range j.on {
			if date < first {
				return true
			}
		}
		return false
	})
}

// prune deletes the digests listing finds of days it finds no lines of, their
// lines pruned, or never written, as a digest stands only beside its day's
// lines. One it can't delete is warned of, as warn says.
func (d *digesting) prune(listing dayListing) {
	for _, date := range listing.digests {
		if _, lined := slices.BinarySearch(listing.lines, date); lined {
			continue
		}
		if err := os.Remove(digestFile(d.days.files.Dir, date)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			d.warn(date, d.days.lookAt(date), "can't prune the request ledger's digest of a day", "error", err)
			continue
		}
		delete(d.judged, date)
		delete(d.warned, date)
	}
}

// digestDue digests each of the days with the given dates, those from where
// the ledger begins to yesterday, at now, oldest first, so a day built again
// has those resting on it built after it: each with files whose digest is
// due, as due says, so long as it's told, as kept tells, its summary standing
// for exactly the lines its files hold. It stops at a day that can't be
// told, as a day that might name any session leaves no state after it one
// to keep; that's warned of where days after it go undigested, as warn says,
// as it may last, as a day whose files can't be looked at. It looks at each
// day once, as find finds it, so it judges and builds each on the one look
// at the days before it. Where paced, it pauses after each build as long as
// the build took, so it never takes more than half a core. It digests none
// once ctx has ended.
func (d *digesting) digestDue(ctx context.Context, dates []string, now time.Time, paced bool) {
	view := d.view()
	for i, date := range dates {
		if ctx.Err() != nil {
			return
		}
		day := view.day(date)
		if _, known := day.kept(); !known {
			if i < len(dates)-1 {
				d.warn(date, day.look, "can't digest the request ledger's days after one whose lines can't be told against its summary")
			}
			return
		}
		if day.look.none || !d.due(dates[:i+1], view) {
			continue
		}
		began := time.Now()
		d.build(ctx, dates[:i+1], view, now)
		if paced {
			pause(ctx, time.Since(began))
		}
	}
}

// view returns a look at the ledger's days for one pass, finding each day
// as find does, and reading their digests of the version its Digester gives.
func (d *digesting) view() writerLook {
	return writerLook{days: d.days, body: d.digester.Version(), looked: make(map[string]writtenDay), find: d.find}
}

// due reports whether the digest of the local day with the given date, the
// last of dates, those of the days from where the ledger begins, is due, as
// view finds the days: none is held that reads, of the version its Digester
// gives, or the one held holds no longer, as holds says. One judged not due
// before isn't, without its digest read, while its file looks as it did
// then, as nothing it was judged on changed since, as forget and letGoBefore
// let go of those something did.
func (d *digesting) due(dates []string, view writerLook) bool {
	date := dates[len(dates)-1]
	file := d.days.digestInfo(date)
	if file == nil {
		return true
	}
	if j, judged := d.judged[date]; judged && dayfile.SameAs(j.digest, file) {
		return false
	}
	delete(d.judged, date)
	digest, ok := view.digest(date)
	if !ok || !holds(digest, dates, view) {
		return true
	}
	d.judged[date] = judgementOf(digest, file)
	return false
}

// holds reports whether digest, of the last of the days with the given
// dates, those from where the ledger begins, holds as view finds them: its
// head current, as digestGrounds.current says; each session its day names
// with an entry; and each entry resting on exactly the days naming its
// session up to its own, as restsOnEach says, each keeping the lines it did,
// as digestGrounds.restsOn says. What standing adds to these, that none of
// its day's lines, nor its neighbours', arrived on another's, it doesn't
// ask: a digest of a day where some did never stands, and building it again
// changes nothing.
func holds(digest Digest, dates []string, view writerLook) bool {
	grounds := digestGrounds{kept: view.kept}
	named, _ := view.day(digest.Head.Day).named()
	if !grounds.current(digest.Head) || !digest.entering(named) {
		return false
	}
	asked := newSessionsLookup(view.days, view).namingEach(dates, nil, wanting(make(map[string]bool), slices.Collect(maps.Keys(digest.Sessions))))
	for id, entry := range digest.Sessions {
		if !grounds.restsOn(entry) || !restsOnEach(entry, asked[id].naming) {
			return false
		}
	}
	return true
}

// entering reports whether the digest holds an entry of each of the sessions
// with the given ids.
func (d Digest) entering(ids []string) bool {
	return !slices.ContainsFunc(ids, func(id string) bool {
		_, entered := d.Sessions[id]
		return !entered
	})
}

// judgement is a day's digest as the digesting last judged it not due: its
// file, as it looked then; and on, the dates of the days whose lines it was
// judged on, its own and those its entries rest on.
type judgement struct {
	digest fs.FileInfo
	on     map[string]bool
}

// judgementOf returns the judgement of digest, its file as file looks at it.
func judgementOf(digest Digest, file fs.FileInfo) judgement {
	on := map[string]bool{digest.Head.Day: true}
	for _, e := range digest.Sessions {
		for date := range e.RestsOn {
			on[date] = true
		}
	}
	return judgement{digest: file, on: on}
}

// find returns the local day with the given date as the digesting found it
// last, where it looks as it did then, as look.same says; else as
// days.written finds it now, letting go of the judgements its change may
// have changed, as forget says. A day it never found is taken to have had
// neither files nor a summary, as no judgement walked it.
func (d *digesting) find(date string) writtenDay {
	l := d.days.lookAt(date)
	was, held := d.held[date]
	if !held {
		was = writtenDay{look: look{none: true}, known: true}
	} else if was.look.same(l) {
		return was
	}
	is := d.days.written(date, l)
	d.forget(date, was, is)
	d.held[date] = is
	return is
}

// forget lets go of the judgements the change of the local day with the
// given date, from was to is, may have changed: where what it names changed,
// or whether the lines it keeps can be told, those of each day from it on,
// as their sessions' days naming them are walked from where the ledger
// begins; else, where the lines it keeps changed, those judged on them.
func (d *digesting) forget(date string, was, is writtenDay) {
	wasNamed, wasEvery := was.named()
	isNamed, isEvery := is.named()
	wasLines, wasKnown := was.kept()
	isLines, isKnown := is.kept()
	switch {
	case wasEvery != isEvery || wasKnown != isKnown || !slices.Equal(wasNamed, isNamed):
		maps.DeleteFunc(d.judged, func(judged string, _ judgement) bool { return judged >= date })
	case wasLines != isLines:
		maps.DeleteFunc(d.judged, func(_ string, j judgement) bool { return j.on[date] })
	}
}

// build builds the digest of the local day with the given date, the last of
// dates, those from where the ledger begins, as digestOf does, view finding
// the days, and writes it, as writeDigest does, judging it not due from then
// on. It writes none where ctx ended first, or the day's summary or its
// files look otherwise once it's built than as view found them, as they
// changed while it was built, so it's due again. Where it can't be built, or
// written, that's warned of, as warn says, and it's built again at the next
// pass.
func (d *digesting) build(ctx context.Context, dates []string, view writerLook, now time.Time) {
	date := dates[len(dates)-1]
	looked := view.day(date).look
	digest, err := d.digestOf(ctx, dates, view, now)
	switch {
	case ctx.Err() != nil || !d.days.lookAt(date).same(looked):
		return
	case err == nil:
		err = d.days.writeDigest(digest)
	}
	if err != nil {
		d.warn(date, looked, "can't digest a day of the request ledger", "error", err)
		return
	}
	delete(d.warned, date)
	if file := d.days.digestInfo(date); file != nil {
		d.judged[date] = judgementOf(digest, file)
	}
	d.days.logger.Info("digested a day of the request ledger", "day", date, "sessions", len(digest.Sessions))
}

// digestOf returns the digest of the local day with the given date, the last
// of dates, those from where the ledger begins, at now, as view finds the
// days: its head of the day's bounds, its summary, and its lines as its files
// hold them now, as arrivals counts them; and an entry of each session its
// summary names, as of the day's end, as sessionsAsOf finds it, whose state
// can be kept, resting on what that says: its state the Digester's, folded
// from the session as DigestDay hands it, or its state in the day's own
// digest, where it rests on that still. A session handed resting on a state
// the Digester gave none of, as one it can't restore, is handed it again read
// whole, so a spoiled state is folded past, not rested on day after day; one
// it gives none of then has no entry. It fails where the day's files can't be
// read, or the Digester fails.
func (d *digesting) digestOf(ctx context.Context, dates []string, view writerLook, now time.Time) (Digest, error) {
	date := dates[len(dates)-1]
	day := view.day(date)
	start, end, _ := dayfile.Day(date)
	head := DigestHead{Body: view.body, Day: date, Start: start, End: end, SummaryLines: day.lines}
	var err error
	if head.Lines, head.Early, head.Late, err = d.days.arrivals(date, start, end); err != nil {
		return Digest{}, err
	}
	named, _ := day.named()
	lookup := newSessionsLookup(d.days, view)
	sessions := lookup.foldable(dates, named, nil, now)
	states, err := d.fold(ctx, head, sessions)
	if err != nil {
		return Digest{}, err
	}
	if spoiled := unfolded(head.Day, sessions, states); len(spoiled) > 0 {
		whole := lookup.foldable(dates, spoiled, func(string) bool { return true }, now)
		again, err := d.fold(ctx, head, whole)
		if err != nil {
			return Digest{}, err
		}
		for id, state := range again {
			sessions[id], states[id] = whole[id], state
		}
	}
	return Digest{Head: head, Sessions: entriesOf(date, sessions, states)}, nil
}

// foldable returns the sessions with the given ids as of the end of the last
// of the local days with the given dates, those from where the ledger begins,
// at now, as asOf finds them, those whole reports true of read whole: each
// whose state can be kept, as restsOn says.
func (l *sessionsLookup) foldable(dates, ids []string, whole func(id string) bool, now time.Time) map[string]SessionFrom {
	sessions := l.asOf(dates, nil, wanting(make(map[string]bool), ids), whole, newSessionsRead(now, nil))
	maps.DeleteFunc(sessions, func(_ string, s SessionFrom) bool { return s.restsOn == nil })
	return sessions
}

// fold returns the states the Digester gives of sessions as of the end of
// the day head is of, by their ids, never nil: of each but those resting on
// the day's own digest still.
func (d *digesting) fold(ctx context.Context, head DigestHead, sessions map[string]SessionFrom) (map[string]json.RawMessage, error) {
	folding := maps.Clone(sessions)
	maps.DeleteFunc(folding, func(_ string, s SessionFrom) bool { return s.restingOn(head.Day) })
	states, err := d.digester.Digest(ctx, DigestDay{Date: head.Day, Start: head.Start, End: head.End, Sessions: folding})
	if states == nil {
		states = make(map[string]json.RawMessage)
	}
	return states, err
}

// unfolded returns the ids of those of sessions resting on the state of a
// day before the one with the given date that states holds none of, in
// order.
func unfolded(date string, sessions map[string]SessionFrom, states map[string]json.RawMessage) []string {
	var ids []string
	for id, s := range sessions {
		if _, folded := states[id]; !folded && s.Base != nil && !s.restingOn(date) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// restingOn reports whether s rests on the state the digest of the local day
// with the given date holds.
func (s SessionFrom) restingOn(date string) bool {
	return s.Base != nil && s.Base.Date == date
}

// entriesOf returns the entries of the digest of the local day with the
// given date of sessions, by their ids: each resting on what its SessionFrom
// says, of its state among states, or, resting on the day's own digest
// still, its state there; none of one with neither.
func entriesOf(date string, sessions map[string]SessionFrom, states map[string]json.RawMessage) map[string]DigestEntry {
	entries := make(map[string]DigestEntry, len(sessions))
	for id, s := range sessions {
		state, folded := states[id]
		if s.restingOn(date) {
			state, folded = s.Base.State, true
		}
		if folded {
			entries[id] = DigestEntry{RestsOn: s.restsOn, State: state}
		}
	}
	return entries
}

// warn warns of the local day with the given date as msg and args, its
// attributes after the day's, say, once until its digest is built, or
// pruned, or the day looks otherwise than l does, as look.same says.
func (d *digesting) warn(date string, l look, msg string, args ...any) {
	if was, warned := d.warned[date]; !warned || !was.same(l) {
		d.days.logger.Warn(msg, append([]any{"day", date}, args...)...)
	}
	d.warned[date] = l
}

// pause waits as long as given, or until ctx ends, whichever comes first.
func pause(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// arrivals returns how many lines the files of the local day with the given
// date hold, as ReadDay counts them, and how many of those that read as
// lines arrived before start, and how many from end on, as a change of time
// zone files a line under a date beside its own. It fails as ReadDay does.
func (d days) arrivals(date string, start, end time.Time) (lines, early, late int, err error) {
	lines, _, err = dayfile.ReadDay(d.files, date, lineIn, func(line Line) bool {
		switch at := line.At; {
		case at.Before(start):
			early++
		case !at.Before(end):
			late++
		}
		return true
	})
	return lines, early, late, err
}
