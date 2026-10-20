package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
)

const (
	// logdayVersion is the version of the day heads' envelope the ledger
	// writes: every field of a head but its side and its chunks' meta, whose
	// version is their builder's, where its rows lie in the day's rows file
	// among them. A release that keeps a day head otherwise writes another,
	// so those written before read as outdated, and their heads are built
	// again.
	logdayVersion = 3
	// logdayPrefix and logdaySuffix begin and end the name of a day's head:
	// logday-<date>.json.gz.
	logdayPrefix, logdaySuffix = "logday-", ".json.gz"
	// logdaySide and logdayChunks are the names a day head's file keeps its
	// side and its rows' chunks under, after every other field, as
	// storedDayHead declares them.
	logdaySide, logdayChunks = "side", "chunks"
)

// logdayFile is where the ledger in dir keeps the head of the local day with
// the given date.
func logdayFile(dir, date string) string {
	return filepath.Join(dir, logdayPrefix+date+logdaySuffix)
}

// dayHeadInfo returns a look at the file of the head the ledger holds of the
// local day with the given date: nil where it holds none, or the file can't
// be looked at.
func (d days) dayHeadInfo(date string) fs.FileInfo {
	info, err := os.Stat(logdayFile(d.files.Dir, date))
	if err != nil {
		return nil
	}
	return info
}

// DayHead is a finished day's head, beside its lines and its summary: the
// day's side, each of its sessions as the Log's find reads them, and where
// its rows lie in the day's rows file, each built of exactly the lines
// Follower.Day gives of the day, in its order, and what it was built from.
// It's derived and disposable, as the day's lines stay the truth: it stands
// only while what it was built from does, as dayHeadStands says, its rows
// as rowsStand says, and a reader of one that doesn't, or of none, reads the
// day's lines instead, slower, but alike. Body is the version of its side
// and rows, its builder's, as the ledger never reads them; Day, the local
// day's date; Start and End, when the day started and ended, as dayfile.Day
// gave them in the time zone it was built in; SummaryLines, the Lines of the
// day's summary it was built against; Lines, how many lines Day gave of the
// day; RestsOn, each day beside it whose files gave it a line, as a change
// of time zone files one, by its date, with the lines the ledger kept of it
// as the head was built, as restingOn records them, none where none did;
// Rows, where its rows lie, as DayRows has them; and Side, the side, a
// JSON value of its builder's, carried as it's given, its insignificant
// spaces aside.
type DayHead struct {
	Body         int
	Day          string
	Start, End   time.Time
	SummaryLines int
	Lines        int
	RestsOn      map[string]int
	Rows         DayRows
	Side         json.RawMessage
}

// against is what h was built against.
func (h DayHead) against() builtAgainst {
	return builtAgainst{day: h.Day, start: h.Start, end: h.End, summaryLines: h.SummaryLines}
}

// dayHeadStands reports whether head stands, as g says, and its side with it:
// so a reader resting on it reads what a read of its day's lines reads. It
// stands as a digest's head does: it's current, as current says; and its day
// is alone, as alone says, as the day's read takes in the lines of its day
// that the files of the days either side hold, which its day's summary
// doesn't count, but the digests of those days tell of. And it stands as a
// digest's entry does: each day beside it that it rests on, whose files gave
// it a line, keeps the lines it did as it was built, as restsOn says, as one
// whose lines were pruned since, which alone passes, gives the day's read
// none of them, and one summarised again may give it others. A line filed
// under a day beside it since it was built, but arrived on it, as two
// writers in different time zones may file one, is told only once that
// day's digest is written again: until then, within a day, the head stands
// without it.
func (g digestGrounds) dayHeadStands(head DayHead) bool {
	return g.current(head.against()) && g.restsOn(head.RestsOn) && g.alone(head.Day)
}

// storedDayHead is a day head as its file holds it, gzipped: the version of
// the envelope it was written in, its fields, its day's bounds as
// nanoseconds since the Unix epoch, the days it rests on, its rows file's
// size and how many rows its chunks hold in all, zeros left out; then its
// side, and its rows' chunks last, as dayHeadIn reads the rest alone by
// stopping at whichever of the two comes first.
type storedDayHead struct {
	Version      int             `json:"version"`
	Body         int             `json:"body"`
	Day          string          `json:"day"`
	From         int64           `json:"from"`
	To           int64           `json:"to"`
	SummaryLines int             `json:"summary_lines,omitzero"`
	Lines        int             `json:"lines,omitzero"`
	RestsOn      map[string]int  `json:"rests_on,omitzero"`
	RowsSize     int64           `json:"rows_size,omitzero"`
	Rows         int             `json:"rows,omitzero"`
	Side         json.RawMessage `json:"side,omitempty"`
	Chunks       []storedChunk   `json:"chunks,omitempty"`
}

// stored is h as its file holds it, of the envelope's version the ledger
// writes.
func (h DayHead) stored() storedDayHead {
	return storedDayHead{
		Version: logdayVersion, Body: h.Body, Day: h.Day, From: h.Start.UnixNano(), To: h.End.UnixNano(),
		SummaryLines: h.SummaryLines, Lines: h.Lines, RestsOn: h.RestsOn, RowsSize: h.Rows.Size, Rows: h.Rows.Count, Side: h.Side,
		Chunks: listAs(h.Rows.Chunks, RowsChunk.stored),
	}
}

// dayHead is the day head s holds, its day's bounds in the local time zone.
func (s storedDayHead) dayHead() DayHead {
	return DayHead{
		Body: s.Body, Day: s.Day, Start: time.Unix(0, s.From), End: time.Unix(0, s.To),
		SummaryLines: s.SummaryLines, Lines: s.Lines, RestsOn: s.RestsOn,
		Rows: DayRows{Size: s.RowsSize, Count: s.Rows, Chunks: listAs(s.Chunks, storedChunk.chunk)}, Side: s.Side,
	}
}

// writeDay writes head as the head of its day, whole, the user's alone, over
// any there, as file makes it; and before it chunks, the day's rows in
// chunks, as the day's rows file, as rowsFileOf makes it and writeRows
// writes it, the head pointing at each chunk there, and recording the file's
// size. So a head is never written before the rows it points at: one left
// beside a rows file written since, as by a build that stopped between the
// two, is held beside a rows file of another size than it records, so its
// rows don't stand, and its day is due; or, the file as long, its chunks
// don't read, by their CRC, and its day isn't due, as headDue judges the
// file by its size, so each reader reads the day's lines till the rows file
// is deleted. It returns the head as written. It fails as file fails, writing
// nothing; where the rows can't be written, writing no head; and where the
// head can't be, leaving the rows written, pointed at by no head, as a build
// that stopped between the two leaves them, to be written again as the day
// is built again.
func (d days) writeDay(head DayHead, chunks []BuiltChunk) (DayHead, error) {
	file, rows := rowsFileOf(chunks)
	head.Rows = rows
	data, err := head.file()
	if err != nil {
		return DayHead{}, err
	}
	if err := d.writeRows(head.Day, file); err != nil {
		return DayHead{}, fmt.Errorf("rows of %s: %w", head.Day, err)
	}
	return head, atomicfile.Write(logdayFile(d.files.Dir, head.Day), data, 0o600)
}

// file is h's file, as stored keeps it, gzipped. It fails where its day
// isn't a date, or its side, or a chunk's meta, isn't a JSON value.
func (h DayHead) file() ([]byte, error) {
	if _, _, ok := dayfile.Day(h.Day); !ok {
		return nil, fmt.Errorf("head of %q: not a date", h.Day)
	}
	data, err := gzippedJSON(h.stored())
	if err != nil {
		return nil, fmt.Errorf("head of %s: %w", h.Day, err)
	}
	return data, nil
}

// errNotTheDaysHead is the error of a day head held of a day that doesn't
// read as the day's head, as one cut short or damaged, or of another day.
var errNotTheDaysHead = errors.New("not the day's head")

// errDayHeadOutdated is the error of a day head held of a day that's of
// another version of the envelope than the ledger writes, or of the side
// than its reader wants, as one another release wrote.
var errDayHeadOutdated = errors.New("a day head of another version")

// dayHead returns the head the ledger holds of the local day with the given
// date, its side and rows of the version body: failing with fs.ErrNotExist
// where it holds none; with errDayHeadOutdated where it's of another
// version, of the envelope or the side; and with errNotTheDaysHead where it
// doesn't read as the day's head. Its fields but its side and its rows'
// chunks are judged first, as sidelessHead judges them, so the side and
// chunks of one of another version, or of another day, are never read.
func (d days) dayHead(date string, body int) (DayHead, error) {
	return d.dayHeadRead(date, body, dayHeadIn, storedIn[storedDayHead])
}

// sidelessHead returns the head the ledger holds of the local day with the
// given date, as dayHead judges it, its side none, and its rows' chunks, as
// dayHeadIn reads it, so none of either is inflated: failing as dayHead
// fails, but for a file damaged after the fields before them, as the rest of
// it isn't read.
func (d days) sidelessHead(date string, body int) (DayHead, error) {
	return d.dayHeadRead(date, body, dayHeadIn)
}

// dayHeadRead returns the head the ledger holds of the local day with the
// given date, as the last of reads reads its file, as storedRead has them
// read, each judged as judged judges it, so one judged otherwise than as the
// day's of the version asked for is read no further.
func (d days) dayHeadRead(date string, body int, reads ...func(io.Reader) (storedDayHead, error)) (DayHead, error) {
	stored, err := storedRead(logdayFile(d.files.Dir, date), func(s storedDayHead, err error) error { return s.judged(date, body, err) }, reads...)
	if err != nil {
		return DayHead{}, err
	}
	return stored.dayHead(), nil
}

// dayHeadIn returns the day head r holds, as its file holds it, its side
// none, nor its rows' chunks, as storedHeadIn reads it, stopping at
// whichever of the two comes first.
func dayHeadIn(r io.Reader) (storedDayHead, error) {
	return storedHeadIn[storedDayHead](r, logdaySide, logdayChunks)
}

// judged returns what's wrong with s, a day head as read of the local day
// with the given date, its read failing with err, where its side is to be of
// the version body: errNotTheDaysHead where it didn't read, or is of another
// day; errDayHeadOutdated where it's of another version, of the envelope or
// the side; nil where nothing is.
func (s storedDayHead) judged(date string, body int, err error) error {
	switch {
	case err != nil:
		return fmt.Errorf("%w: %w", errNotTheDaysHead, err)
	case s.Version != logdayVersion || s.Body != body:
		return fmt.Errorf("%w: version %d, its side's %d", errDayHeadOutdated, s.Version, s.Body)
	case s.Day != date:
		return fmt.Errorf("%w: it's of %q", errNotTheDaysHead, s.Day)
	}
	return nil
}

// DaySide returns the side of the head the ledger holds of the local day
// with the given date, of the version body, its builder's, as it was given:
// reporting false where it holds none that stands, as dayHeadStands says by
// the follower's own look at the day, so its reader reads the day's lines
// instead.
func (f *Follower) DaySide(date string, body int) (json.RawMessage, bool) {
	head, err := f.days.dayHead(date, body)
	if err != nil || !f.grounds().dayHeadStands(head) {
		return nil, false
	}
	return head.Side, true
}

// grounds are what a day head is judged against by the follower's own look
// at the ledger's days, as followerLook has it.
func (f *Follower) grounds() digestGrounds {
	look := followerLook{f: f}
	return digestGrounds{kept: look.kept, heads: look.head}
}

// day hands take each line of the requests that arrived on the local day
// with the given date, by now, oldest first, as Follower.Day gives them, with
// the date of the day whose files hold it, as filedHeld has it, until take
// reports false: read whole from the day's files and those of the days
// either side of it, as a change of time zone files a line under a date
// beside its own, and a request that ends after midnight is its arrival
// day's. It notes in faults, where they're given, the days whose files it
// couldn't read for now, as dayfile.Faults says, and hands none of a date
// that isn't one. How many lines couldn't be read is warned of. It's the one
// read of a day's lines that both Day and a day's head take, so the head is
// built of exactly the lines Day gives, in its order.
func (d days) day(date string, now time.Time, take func(filedHeld) bool, faults *dayfile.Faults) {
	day, ok := dayfile.DayBounds(date)
	if !ok {
		return
	}
	within := func(h filedHeld) bool { return day.Holds(h.At) && !h.At.After(now) }
	d.lines(dayfile.Dates(day.Start, day.Start), lineHeldIn, within, take, faults, "day", date)
}

// errDayUnread is the error of a day whose lines, or those of a day either
// side of it, couldn't all be read for now.
var errDayUnread = errors.New("its files, or those of the days either side of it, can't all be read for now")

// seenHead is the head of a day as the digesting last read it, or wrote it:
// its file, as it looked then; the head, its side none, nor its rows'
// chunks, as current judges it afresh at each pass, and rowsHeld its rows
// file's size; and settled, the files of the digests beside its day as they
// looked as it was last built, or found to stand, nil where it's neither, as
// headDue holds them.
type seenHead struct {
	file    fs.FileInfo
	head    DayHead
	settled *digestsBeside
}

// digestsBeside is a look at the files of the digests the ledger holds of
// the local days either side of a day, which read none of them: nil of one
// it holds none of.
type digestsBeside struct {
	before, after fs.FileInfo
}

// digestsBeside returns a look at the files of the digests the ledger holds
// of the local days either side of the one with the given date.
func (d days) digestsBeside(date string) digestsBeside {
	before, after := beside(date)
	return digestsBeside{before: d.digestInfo(before), after: d.digestInfo(after)}
}

// same reports whether b looks as other does, each file as dayfile.SameAs
// says.
func (b digestsBeside) same(other digestsBeside) bool {
	return dayfile.SameAs(b.before, other.before) && dayfile.SameAs(b.after, other.after)
}

// settledAmong reports whether the head was built, or found to stand, as the
// digests beside its day look as around does.
func (s seenHead) settledAmong(around digestsBeside) bool {
	return s.settled != nil && s.settled.same(around)
}

// logdayDue builds the head of each of the days with the given dates, those
// from where the ledger begins to yesterday, at now, newest first, as none
// rests on another, so yesterday's stands soon after its summary does: each
// whose head is due, as headDue says. Where paced, it pauses after each build
// as long as the build took. It builds none once ctx has ended.
func (d *digesting) logdayDue(ctx context.Context, dates []string, now time.Time, paced bool) {
	view := d.view()
	for _, date := range slices.Backward(dates) {
		if ctx.Err() != nil {
			return
		}
		if d.headDue(date, view) {
			builtPaced(ctx, paced, func() { d.buildHead(ctx, date, view, now) })
		}
	}
}

// headDue reports whether the head of the local day with the given date is
// due, as view finds the day: it has lines, kept as its summary counts them,
// as kept tells, and no head is held of it whose rows stand, as rowsStand
// says, of the version its Digester gives. One that's current, as current
// says, its rows file as long as it records, but whose rows don't stand, as
// the digests beside its day tell of a line of it their days' files hold,
// or a day beside it that it rests on keeps other lines now, as restsOn
// says, is due once each time one of those digests changes, as building it
// again takes the lines in, but may not have it stand: not at every pass. A
// day's digest is written again once its lines change and it's summarised
// again, and pruned with its lines, so a head resting on a day pruned is
// built again, of none of its lines. A head is read only where its file
// looks otherwise than it did as the digesting last read it, as seenHead
// holds it, and the digests beside it only where one of their files looks
// otherwise than it did as the head was last built, or found to stand; its
// rows file is looked at, never read, so one damaged but as long leaves it
// not due; and a day whose lines can't be told isn't due, as no head built
// of it would stand.
func (d *digesting) headDue(date string, view writerLook) bool {
	day := view.day(date)
	if _, known := day.kept(); day.look.none || !known {
		return false
	}
	seen, held := d.seen(date)
	grounds := digestGrounds{kept: view.kept, heads: view.head}
	around := d.days.digestsBeside(date)
	switch {
	case !held || !grounds.current(seen.head.against()):
		return true
	case seen.settledAmong(around) && d.days.rowsHeld(date, seen.head.Rows.Size):
		return false
	case !d.days.rowsStand(grounds, seen.head):
		return true
	}
	d.settle(date, around)
	return false
}

// seen returns the head the ledger holds of the local day with the given
// date, of the version its Digester gives, its side none, nor its rows'
// chunks, as it was last read, or written, while its file looks as it did
// then; else read afresh, neither read, as sidelessHead reads it, and held.
// It reports false where the ledger holds none that reads.
func (d *digesting) seen(date string) (seenHead, bool) {
	file := d.days.dayHeadInfo(date)
	if s, ok := d.heads[date]; ok && file != nil && dayfile.SameAs(s.file, file) {
		return s, true
	}
	delete(d.heads, date)
	head, err := d.days.sidelessHead(date, d.digester.RowsVersion())
	if err != nil {
		return seenHead{}, false
	}
	d.see(date, file, head, nil)
	return d.heads[date], true
}

// see holds head, the head of the local day with the given date, as its file
// looks as given, its side and its rows' chunks let go of, settled as the
// digests beside its day look as given, nil for none.
func (d *digesting) see(date string, file fs.FileInfo, head DayHead, settled *digestsBeside) {
	head.Side, head.Rows.Chunks = nil, nil
	d.heads[date] = seenHead{file: file, head: head, settled: settled}
}

// settle holds the head of the local day with the given date, as seen holds
// it, settled as the digests beside its day look as around does.
func (d *digesting) settle(date string, around digestsBeside) {
	s := d.heads[date]
	s.settled = &around
	d.heads[date] = s
}

// buildHead builds the head of the local day with the given date and its
// rows, at now, as headOf does, view finding the day, and writes them, as
// writeDay does, holding the head as seen from then on, settled as the
// digests beside its day look as it's written. It writes none where ctx
// ended first, or the day's summary or its files look otherwise once it's
// built than as view found them, as they changed while it was built, so it's
// due again. Where it can't be built, or written, that's warned of, as
// warnings.warn says, and it's built again at the next pass.
func (d *digesting) buildHead(ctx context.Context, date string, view writerLook, now time.Time) {
	day := view.day(date)
	head, chunks, err := d.headOf(ctx, date, view, now)
	switch {
	case ctx.Err() != nil || !d.days.lookAt(date).same(day.look):
		return
	case err == nil:
		head, err = d.days.writeDay(head, chunks)
	}
	if err != nil {
		d.headsWarned.warn(d.days.logger, date, day.look, "can't build the request ledger's head of a day", "error", err)
		return
	}
	delete(d.headsWarned, date)
	if file := d.days.dayHeadInfo(date); file != nil {
		around := d.days.digestsBeside(date)
		d.see(date, file, head, &around)
	}
	d.days.logger.Info("built the head of a day of the request ledger", "day", date, "lines", head.Lines, "chunks", len(head.Rows.Chunks))
}

// headOf returns the head of the local day with the given date, at now, view
// finding the days, and its rows in chunks: of the day's bounds, its
// summary's lines, and its lines as Follower.Day gives them, as days.day
// reads them, each handed to a builder of its Digester's, whose side the
// head holds, and whose chunks it gives, as DayBuilt has them, the head
// pointing at none of them yet; resting on each day beside it whose files
// gave it a line, of the lines view found the ledger kept of it before any
// were read, as restingOn records them, as a digest's entry rests on the days
// it was folded from. It stops reading them once ctx ends, failing; and
// fails where the files of the day, or of a day either side of it, can't
// all be read for now, the builder fails, or its chunks count other than a
// row of each line it was handed, as no head of them would stand.
func (d *digesting) headOf(ctx context.Context, date string, view writerLook, now time.Time) (DayHead, []BuiltChunk, error) {
	start, end, _ := dayfile.Day(date)
	summaryLines, _ := view.kept(date)
	head := DayHead{Body: d.digester.RowsVersion(), Day: date, Start: start, End: end, SummaryLines: summaryLines}
	kept := keptBeside(date, view.kept)
	gave := make(map[string]bool)
	build := d.digester.Day()
	faults := dayfile.NewFaults(nil)
	d.days.day(date, now, func(h filedHeld) bool {
		build.Take(h.Line)
		head.Lines++
		gave[h.under] = true
		return ctx.Err() == nil
	}, faults)
	switch {
	case ctx.Err() != nil:
		return DayHead{}, nil, fmt.Errorf("the lines of %s: %w", date, ctx.Err())
	case faults.Any(dayfile.Dates(start, start)):
		return DayHead{}, nil, fmt.Errorf("the lines of %s: %w", date, errDayUnread)
	}
	built, err := build.Built()
	switch {
	case err != nil:
		return DayHead{}, nil, fmt.Errorf("the side and rows of %s: %w", date, err)
	case rowsIn(built.Chunks) != head.Lines:
		return DayHead{}, nil, fmt.Errorf("the rows of %s: %d built of %d lines, want one a line", date, rowsIn(built.Chunks), head.Lines)
	}
	head.Side, head.RestsOn = built.Side, restingOn(kept, gave)
	return head, built.Chunks, nil
}

// untold is what a day head records of the lines of a day beside its own
// whose files gave it a line, where they couldn't be told as it was built:
// of the day before, as kept gave none; of either, as kept gave none at all,
// as of a day of no files, or of a summary of none, which gave no line, so
// its files were written after it was looked at. It's a count no day keeps,
// so the head doesn't stand, as restsOn judges it, until it's built again.
// Nor would it while that day can't be told, as alone says; but alone has it
// stand once that day's lines are pruned, the line with them, where it would
// otherwise be none, as kept gives a day pruned.
const untold = -1

// keptBeside returns the lines the ledger keeps of each of the local days
// either side of the one with the given date, by their dates, as kept gives
// them, as a head of that day rests on them where their files give it a
// line: of the day before, untold where it can't tell them; of the day
// after, none, as while it's today, so the head rests on it only once it's
// told, and stands meanwhile, as alone has a head stand while that day's
// digest isn't current.
func keptBeside(date string, kept func(date string) (int, bool)) map[string]int {
	before, after := beside(date)
	lines := map[string]int{before: untold}
	if n, known := kept(before); known {
		lines[before] = n
	}
	if n, known := kept(after); known {
		lines[after] = n
	}
	return lines
}

// restingOn returns what a day head rests on, of kept, the lines the ledger
// keeps of each day beside its own, by its date, as keptBeside gives them:
// those of the days whose files gave it a line, as gave reports of each
// date, untold of one kept gives none, as no day of none gave a line; none
// where none did.
func restingOn(kept map[string]int, gave map[string]bool) map[string]int {
	on := make(map[string]int, len(kept))
	for date, lines := range kept {
		switch {
		case !gave[date]:
		case lines > 0:
			on[date] = lines
		default:
			on[date] = untold
		}
	}
	if len(on) == 0 {
		return nil
	}
	return on
}
