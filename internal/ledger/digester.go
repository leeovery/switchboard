package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// Digester folds the sessions of a finished local day into their states as
// of its end, for the ledger to keep in the day's digest, and their parts of
// the day, for it to keep in the day's parts file; and builds the day's side,
// for it to keep in the day's head, and its rows, for it to keep in the
// day's rows file. Version is the version of the states and parts it gives,
// as a digest of another doesn't stand; Builder gives a builder of a day's
// digest, as DigestBuilder has it, for one read of the days its sessions are
// read from; RowsVersion is the version of the sides and rows it gives, as a
// day head of another doesn't stand, nor its rows; and Day gives a builder
// of a day's side and rows, as DayBuilder has it, for one read of the day's
// lines.
type Digester interface {
	Version() int
	Builder() DigestBuilder
	RowsVersion() int
	Day() DayBuilder
}

// DayBuilder builds a finished local day's side and rows, its Digester's,
// from the day's lines: Take is handed each line Follower.Day gives of the
// day, in its order, as days.day reads them for both, every one before Built
// is called; and Built gives what it built of them, as DayBuilt has it,
// failing where it can't.
type DayBuilder interface {
	Take(l Line)
	Built() (DayBuilt, error)
}

// DayBuilt is what a DayBuilder builds of a day's lines: Side, the day's
// sessions as the Log's find reads them, a JSON value of its Digester's, as
// the day's head keeps it; and Chunks, the day's rows, in chunks, in the
// order of the lines they're built of, as BuiltChunk has each, each kept as
// a gzip member of its own in the day's rows file, the day's head pointing
// at it there: none of a day it builds no rows of.
type DayBuilt struct {
	Side   json.RawMessage
	Chunks []BuiltChunk
}

// BuiltChunk is a chunk of a day's rows as a DayBuilder builds it: Rows, its
// rows, in its Digester's encoding, carried as they're given and never read,
// as the day's rows file keeps them; Count, how many rows they are; and
// Meta, what the chunk holds, as its Digester tells it, a JSON value of its
// own, carried as it's given, its insignificant spaces aside, as the day's
// head keeps it beside where the chunk lies, so a reader can tell what a
// chunk holds without reading it: none where it's nil.
type BuiltChunk struct {
	Rows  []byte
	Count int
	Meta  json.RawMessage
}

// DigestBuilder builds a finished local day's digest, its Digester's, from one
// read of the days its sessions are read from: Take is handed each line the
// read reads, of whichever session, with the date of the day whose files
// hold it, as sessionsLookup.asOf hands a reader's take them, every one
// before Digest is called; and Digest gives each session day hands it as it
// folds it, by its id, as Folded has it, failing where it can't fold them,
// or ctx ends first. A session handed resting on a state it gives none of is
// handed again read whole, to a builder of its own, handed each line of
// that read; one it gives no state of then has no entry in the digest,
// which leaves the day due: it's built again whenever it's judged afresh,
// as at each start of the router.
type DigestBuilder interface {
	Take(date string, l Line)
	Digest(ctx context.Context, day DigestDay) (map[string]Folded, error)
}

// Folded is a session as a Digester folds it as of a day's end: State, its
// state, as the day's digest keeps it in its entry; Part, what the day's
// fold added to the state it was folded on, as the day's parts file keeps
// it, nil for none; and Times, what of its part a fold resting on its state
// needs none of, kept in a member of its own after the part's, so such a
// fold reads none of it, nil for none: each a JSON value of the Digester's,
// as RestToFold and RestOnPage hand them back. Its part is the ledger's to
// place: it names the day of the state the session was handed resting on,
// none where it rested on none, as PartRef's On has it.
type Folded struct {
	State json.RawMessage
	Part  json.RawMessage
	Times json.RawMessage
}

// DigestDay is a finished local day as the ledger hands it to its Digester:
// Date, its date; Start and End, when it started and ended, in this time
// zone; and Sessions, the sessions its summary names, as of the day's end,
// by their ids, as the ledger finds them: each to fold, as RestToFold rests
// it, on the newest state the digest of an earlier day holds that stands for
// it whose chain of parts reads, with that chain, none of its parts' times,
// or on none, and its lines after that, the day's own among them, read by
// the days' files. A session whose state can't be kept, as a day it rests on
// can't be told, isn't among them; nor is one resting on the day's own
// digest still, whose state and part there are as of the day's end.
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

// digesting is the ledger's digesting of its days with its Digester, and the
// building of their heads: a pass over them at each kick, on a goroutine of
// Run's, as run makes it, or at once, on DigestEnded's caller's, as pass
// says. It holds, from pass to pass, each day as it last found it, the
// digests it judged not due, the heads it read, and the days whose digests
// or heads couldn't be built, so a pass that finds nothing changed reads no
// digest, nor head, and a failure is warned of once.
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
	// each day's digest last judged not due; heads, each day's head as a
	// pass last read it, as seen holds it; warned, each day whose digest
	// couldn't be built, or whose files built of its lines couldn't be
	// pruned, and headsWarned, each day whose head couldn't be built, as it
	// looked then; and unlisted, how the passes' listings of the ledger's
	// directory went.
	passing     sync.Mutex
	held        map[string]writtenDay
	judged      map[string]judgement
	heads       map[string]seenHead
	warned      warnings
	headsWarned warnings
	unlisted    failing
}

// newDigesting returns the digesting of the ledger's days d with digester,
// by now's clock, having made no pass yet.
func newDigesting(digester Digester, d days, now func() time.Time) *digesting {
	return &digesting{digester: digester, days: d, now: now, kicks: make(chan struct{}, 1),
		held: make(map[string]writtenDay), judged: make(map[string]judgement), heads: make(map[string]seenHead),
		warned: make(warnings), headsWarned: make(warnings)}
}

// run makes a pass, paced, at the clock's now, at each kick, until ctx ends:
// none before the first, which the first round's summarising gives as it
// ends, so no pass finds a day whose summary that round is to write again,
// as under a new version, which it can't tell the day's lines against.
func (d *digesting) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.kicks:
		}
		d.pass(ctx, d.now(), true)
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
// ended: it deletes the files built of the lines of days without lines, as
// prune does, even where the ledger holds no day; lets go of what it holds
// of the days before where the ledger begins, as letGoBefore does; then
// digests each finished day that's due, as digestDue does, and builds the
// head of each that's due, as logdayDue does, pausing after each build,
// where paced, as long as it took. Where the ledger's directory can't be
// listed, it does none of these, as listedNoting says.
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
	dates := d.days.datesBefore(dayfile.DateOf(now), first, nil, now)
	d.digestDue(ctx, dates, now, paced)
	d.logdayDue(ctx, dates, now, paced)
}

// letGoBefore lets go of the days before the one with the given date, where
// the ledger begins, its earliest summary or file of lines, their heads, and
// the judgements of the digests resting on one of them. A prune leaves a
// day's summary, so where the ledger begins moves only where a day's summary
// and files are removed alike; forget lets go of the judgements resting on a
// pruned day's lines. No other judgement is on a day before where the ledger
// begins, as holds judges none that rests on one due.
func (d *digesting) letGoBefore(first string) {
	maps.DeleteFunc(d.held, func(date string, _ writtenDay) bool { return date < first })
	maps.DeleteFunc(d.heads, func(date string, _ seenHead) bool { return date < first })
	maps.DeleteFunc(d.judged, func(_ string, j judgement) bool {
		for date := range j.on {
			if date < first {
				return true
			}
		}
		return false
	})
}

// prune deletes the digests, parts files, heads and rows files listing finds
// of days it finds no lines of, their lines pruned, or never written, as
// each is kept only beside its day's lines. A day's it can't delete are
// warned of, as warnings.warn says.
func (d *digesting) prune(listing dayListing) {
	dir := d.days.files.Dir
	for _, date := range listing.built {
		if _, lined := slices.BinarySearch(listing.lines, date); lined {
			continue
		}
		if err := errors.Join(removed(digestFile(dir, date)), removed(partsFile(dir, date)), removed(logdayFile(dir, date)), removed(rowsFile(dir, date))); err != nil {
			d.warned.warn(d.days.logger, date, d.days.lookAt(date), "can't prune what the request ledger built of a day's lines", "error", err)
			continue
		}
		delete(d.judged, date)
		delete(d.heads, date)
		delete(d.warned, date)
		delete(d.headsWarned, date)
	}
}

// digestDue digests each of the days with the given dates, those from where
// the ledger begins to yesterday, at now, oldest first, so a day built again
// has those resting on it built after it: each with files whose digest is
// due, as due says, so long as it's told, as kept tells, its summary standing
// for exactly the lines its files hold. It stops at a day that can't be
// told, as a day that might name any session leaves no state after it one
// to keep; that's warned of where days after it go undigested, as
// warnings.warn says, as it may last, as a day whose files can't be looked
// at. It looks at each day once, as find finds it, so it judges and builds
// each on the one look at the days before it. Where paced, it pauses after
// each build as long as the build took. It digests none once ctx has ended.
func (d *digesting) digestDue(ctx context.Context, dates []string, now time.Time, paced bool) {
	view := d.view()
	for i, date := range dates {
		if ctx.Err() != nil {
			return
		}
		day := view.day(date)
		if _, known := day.kept(); !known {
			if i < len(dates)-1 {
				d.warned.warn(d.days.logger, date, day.look, "can't digest the request ledger's days after one whose lines can't be told against its summary")
			}
			return
		}
		if !day.look.none && d.due(dates[:i+1], view) {
			builtPaced(ctx, paced, func() { d.build(ctx, dates[:i+1], view, now) })
		}
	}
}

// builtPaced builds as build does, then, where paced, pauses as long as the
// build took, or until ctx ends, so the builds never take more than half a
// core.
func builtPaced(ctx context.Context, paced bool, build func()) {
	began := time.Now()
	build()
	if paced {
		pause(ctx, time.Since(began))
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
// gives, with its parts file as long as its head records, as view's digest
// gives it, or the one held holds no longer, as holds says. One judged not
// due before isn't, without its digest read, while its file looks as it did
// then, and its parts file is as long, as partsHeld looks at it, as nothing
// it was judged on changed since, as forget and letGoBefore let go of those
// something did.
func (d *digesting) due(dates []string, view writerLook) bool {
	date := dates[len(dates)-1]
	file := d.days.digestInfo(date)
	if file == nil {
		return true
	}
	if j, judged := d.judged[date]; judged && dayfile.SameAs(j.digest, file) && d.days.partsHeld(date, j.parts) {
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
	if !grounds.current(digest.Head.against()) || !digest.entering(named) {
		return false
	}
	asked := newSessionsLookup(view.days, view).namingEach(dates, nil, wanting(make(map[string]bool), slices.Collect(maps.Keys(digest.Sessions))))
	for id, entry := range digest.Sessions {
		if !grounds.restsOn(entry.RestsOn) || !restsOnEach(entry, asked[id].naming) {
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
// file, as it looked then; parts, the size of its parts file its head
// records; and on, the dates of the days whose lines it was judged on, its
// own and those its entries rest on.
type judgement struct {
	digest fs.FileInfo
	parts  int64
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
	return judgement{digest: file, parts: digest.Head.PartsSize, on: on}
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
// dates, those from where the ledger begins, and its sessions' parts, as
// digestOf does, view finding the days, and writes them, as writeDigest
// does, judging the digest not due from then on. It writes none where ctx
// ended first, or the day's summary or its files look otherwise once it's
// built than as view found them, as they changed while it was built, so it's
// due again. Where it can't be built, or written, that's warned of, as
// warnings.warn says, and it's built again at the next pass.
func (d *digesting) build(ctx context.Context, dates []string, view writerLook, now time.Time) {
	date := dates[len(dates)-1]
	looked := view.day(date).look
	digest, parts, err := d.digestOf(ctx, dates, view, now)
	switch {
	case ctx.Err() != nil || !d.days.lookAt(date).same(looked):
		return
	case err == nil:
		digest, err = d.days.writeDigest(digest, parts)
	}
	if err != nil {
		d.warned.warn(d.days.logger, date, looked, "can't digest a day of the request ledger", "error", err)
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
// days, and the parts of its sessions, by their ids, as writeDigest takes
// them: its head of the day's bounds, its summary, and its lines as its files
// hold them now, as arrivals counts them; and an entry of each session its
// summary names, as of the day's end, as sessionsLookup.asOf finds it, whose
// state can be kept, resting to fold, as RestToFold has it, on the state a
// reader of its page rests it on, its state and its part as readFolded gives
// them. A
// session resting on a state fold gives none of, as one the Digester can't
// restore, or whose page's points can't take the days since in, is handed
// again read whole, so a spoiled state is folded past, not rested on day
// after day; one it gives none of then has no entry. It fails where the
// day's files can't be read, or the Digester fails.
func (d *digesting) digestOf(ctx context.Context, dates []string, view writerLook, now time.Time) (Digest, map[string]builtPart, error) {
	date := dates[len(dates)-1]
	day := view.day(date)
	start, end, _ := dayfile.Day(date)
	head := DigestHead{Body: view.body, Day: date, Start: start, End: end, SummaryLines: day.lines}
	var err error
	if head.Lines, head.Early, head.Late, err = d.days.arrivals(date, start, end); err != nil {
		return Digest{}, nil, err
	}
	named, _ := day.named()
	lookup := newSessionsLookup(d.days, view)
	sessions, folded, err := d.readFolded(ctx, lookup, head, dates, named, func(string) Rest { return RestToFold }, now)
	if err != nil {
		return Digest{}, nil, err
	}
	if spoiled := unfolded(sessions, folded); len(spoiled) > 0 {
		whole, again, err := d.readFolded(ctx, lookup, head, dates, spoiled, func(string) Rest { return RestWhole }, now)
		if err != nil {
			return Digest{}, nil, err
		}
		for id, f := range again {
			sessions[id], folded[id] = whole[id], f
		}
	}
	return Digest{Head: head, Sessions: entriesOf(sessions, folded)}, partsOf(folded), nil
}

// readFolded returns the sessions with the given ids as of the end of the
// day head is of, the last of the local days with the given dates, those
// from where the ledger begins, at now, as lookup's foldable finds them,
// resting as rest says of each; and each folded, as fold folds it, by a
// builder of the day's digest of their own, as the Digester gives one,
// handed each line their read reads first, so a second read of the day's
// sessions hands its lines afresh. It fails as fold fails.
func (d *digesting) readFolded(ctx context.Context, lookup *sessionsLookup, head DigestHead, dates, ids []string, rest func(id string) Rest, now time.Time) (map[string]SessionFrom, map[string]sessionFolded, error) {
	build := d.digester.Builder()
	sessions := lookup.foldable(dates, ids, rest, now, build.Take)
	folded, err := d.fold(ctx, build, head, sessions)
	return sessions, folded, err
}

// foldable returns the sessions with the given ids as of the end of the last
// of the local days with the given dates, those from where the ledger begins,
// at now, as asOf finds them, resting as rest says of each, take handed each
// line read: each whose state can be kept, as restsOn says.
func (l *sessionsLookup) foldable(dates, ids []string, rest func(id string) Rest, now time.Time, take func(date string, l Line)) map[string]SessionFrom {
	sessions := l.asOf(dates, nil, wanting(make(map[string]bool), ids), rest, newSessionsRead(now, take))
	maps.DeleteFunc(sessions, func(_ string, s SessionFrom) bool { return s.restsOn == nil })
	return sessions
}

// sessionFolded is a session folded as of a day's end, as the day's digest
// and parts file keep it: its state, and its part, none where it has no
// member.
type sessionFolded struct {
	state json.RawMessage
	part  builtPart
}

// fold returns sessions folded as of the end of the day head is of, by their
// ids: each build gives of those handed it, all but those resting on the
// day's own digest still, its part on the date of the state it was handed
// resting on, none where it rested on none, as builtPartOf makes it; and
// each of those as it's folded there, as keptFolded keeps it. It gives none
// of another, nor of one build gives none of. It fails where build fails, or
// gives a part that isn't JSON.
func (d *digesting) fold(ctx context.Context, build DigestBuilder, head DigestHead, sessions map[string]SessionFrom) (map[string]sessionFolded, error) {
	handing := maps.Clone(sessions)
	maps.DeleteFunc(handing, func(_ string, s SessionFrom) bool { return s.restingOn(head.Day) })
	given, err := build.Digest(ctx, DigestDay{Date: head.Day, Start: head.Start, End: head.End, Sessions: handing})
	if err != nil {
		return nil, err
	}
	folded := make(map[string]sessionFolded, len(sessions))
	for id, s := range sessions {
		if s.restingOn(head.Day) {
			if f, ok := d.keptFolded(head.Day, s); ok {
				folded[id] = f
			}
			continue
		}
		f, ok := given[id]
		if !ok {
			continue
		}
		part, err := builtPartOf(s.BaseDate(), f.Part, f.Times)
		if err != nil {
			return nil, fmt.Errorf("fold %s of %s: %w", id, head.Day, err)
		}
		folded[id] = sessionFolded{state: f.State, part: part}
	}
	return folded, nil
}

// keptFolded returns the session s, resting on the state the digest of the
// local day with the given date holds, as it's folded there, as of that
// day's end: its state, and its part, on the date its entry names, copied
// as keptPart copies it, none of it inflated. It reports false where s rests
// on no part of the day's, as a state whose page was left out, or its part
// doesn't read as written, by its CRC, for it to be folded afresh.
func (d *digesting) keptFolded(date string, s SessionFrom) (sessionFolded, bool) {
	if s.Base.Part == nil {
		return sessionFolded{}, false
	}
	part, err := d.days.keptPart(date, *s.Base.Part)
	if err != nil {
		return sessionFolded{}, false
	}
	return sessionFolded{state: s.Base.State, part: part}, true
}

// unfolded returns the ids of those of sessions resting on a state that
// folded holds none of, in order.
func unfolded(sessions map[string]SessionFrom, folded map[string]sessionFolded) []string {
	var ids []string
	for id, s := range sessions {
		if _, ok := folded[id]; !ok && s.Base != nil {
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

// BaseDate returns the date of the day whose digest holds the state s rests
// on: none where it rests on none.
func (s SessionFrom) BaseDate() string {
	if s.Base == nil {
		return ""
	}
	return s.Base.Date
}

// entriesOf returns the entries of a digest of sessions, by their ids: each
// resting on what its SessionFrom says, of its state as folded holds it;
// none of one it holds none of.
func entriesOf(sessions map[string]SessionFrom, folded map[string]sessionFolded) map[string]DigestEntry {
	entries := make(map[string]DigestEntry, len(sessions))
	for id, s := range sessions {
		if f, ok := folded[id]; ok {
			entries[id] = DigestEntry{RestsOn: s.restsOn, State: f.state}
		}
	}
	return entries
}

// partsOf returns the parts folded holds, by their sessions' ids: none of a
// session whose part has no member, as one given empty.
func partsOf(folded map[string]sessionFolded) map[string]builtPart {
	parts := make(map[string]builtPart)
	for id, f := range folded {
		if f.part.part != nil {
			parts[id] = f.part
		}
	}
	return parts
}

// warnings are the local days the digesting warned of, by their dates, each
// as it looked as it was warned of: held so each is warned of once while it
// looks so, and let go of once what was warned of is built, or pruned.
type warnings map[string]look

// warn warns of the local day with the given date to logger as msg and
// args, its attributes after the day's, say, once until it's let go of, or
// the day looks otherwise than l does, as look.same says.
func (w warnings) warn(logger *slog.Logger, date string, l look, msg string, args ...any) {
	if was, warned := w[date]; !warned || !was.same(l) {
		logger.Warn(msg, append([]any{"day", date}, args...)...)
	}
	w[date] = l
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
