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
	// writes: every field of a head but its side, whose version is its
	// builder's. A release that keeps a day head otherwise writes another, so
	// those written before read as outdated, and their heads are built again.
	logdayVersion = 1
	// logdayPrefix and logdaySuffix begin and end the name of a day's head:
	// logday-<date>.json.gz.
	logdayPrefix, logdaySuffix = "logday-", ".json.gz"
	// logdaySide is the name a day head's file keeps its side under, after
	// every other field, as storedDayHead declares them.
	logdaySide = "side"
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
// day's side, each of its sessions as the Log's find reads them, built of
// exactly the lines Follower.Day gives of the day, in its order, and what it
// was built from. It's derived and disposable, as the day's lines stay the
// truth: it stands only while what it was built from does, as dayHeadStands
// says, and a reader of one that doesn't, or of none, reads the day's lines
// instead, slower, but alike. Body is the version of its side, its
// builder's, as the ledger never reads it; Day, the local day's date; Start
// and End, when the day started and ended, as dayfile.Day gave them in the
// time zone it was built in; SummaryLines, the Lines of the day's summary it
// was built against; Lines, how many lines Day gave of the day; and Side,
// the side, a JSON value of its builder's, carried as it's given, its
// insignificant spaces aside.
type DayHead struct {
	Body         int
	Day          string
	Start, End   time.Time
	SummaryLines int
	Lines        int
	Side         json.RawMessage
}

// against is what h was built against.
func (h DayHead) against() builtAgainst {
	return builtAgainst{day: h.Day, start: h.Start, end: h.End, summaryLines: h.SummaryLines}
}

// dayHeadStands reports whether head stands, as g says, and its side with it:
// so a reader resting on it reads what a read of its day's lines reads. It
// stands where it's current, as current says, as a digest's head must be;
// unlike a digest, it rests on no state of another day's.
func (g digestGrounds) dayHeadStands(head DayHead) bool {
	return g.current(head.against())
}

// storedDayHead is a day head as its file holds it, gzipped: the version of
// the envelope it was written in, its fields, its day's bounds as
// nanoseconds since the Unix epoch, zeros left out, and its side last, as
// dayHeadIn reads the rest alone by stopping at it.
type storedDayHead struct {
	Version      int             `json:"version"`
	Body         int             `json:"body"`
	Day          string          `json:"day"`
	From         int64           `json:"from"`
	To           int64           `json:"to"`
	SummaryLines int             `json:"summary_lines,omitzero"`
	Lines        int             `json:"lines,omitzero"`
	Side         json.RawMessage `json:"side,omitempty"`
}

// stored is h as its file holds it, of the envelope's version the ledger
// writes.
func (h DayHead) stored() storedDayHead {
	return storedDayHead{
		Version: logdayVersion, Body: h.Body, Day: h.Day, From: h.Start.UnixNano(), To: h.End.UnixNano(),
		SummaryLines: h.SummaryLines, Lines: h.Lines, Side: h.Side,
	}
}

// dayHead is the day head s holds, its day's bounds in the local time zone.
func (s storedDayHead) dayHead() DayHead {
	return DayHead{
		Body: s.Body, Day: s.Day, Start: time.Unix(0, s.From), End: time.Unix(0, s.To),
		SummaryLines: s.SummaryLines, Lines: s.Lines, Side: s.Side,
	}
}

// writeDayHead writes head as the head of its day, whole, the user's alone,
// over any there, as stored keeps it, gzipped. It fails where its day isn't
// a date, or its side isn't a JSON value, writing nothing.
func (d days) writeDayHead(head DayHead) error {
	if _, _, ok := dayfile.Day(head.Day); !ok {
		return fmt.Errorf("head of %q: not a date", head.Day)
	}
	data, err := gzippedJSON(head.stored())
	if err != nil {
		return fmt.Errorf("head of %s: %w", head.Day, err)
	}
	return atomicfile.Write(logdayFile(d.files.Dir, head.Day), data, 0o600)
}

// errNotTheDaysHead is the error of a day head held of a day that doesn't
// read as the day's head, as one cut short or damaged, or of another day.
var errNotTheDaysHead = errors.New("not the day's head")

// errDayHeadOutdated is the error of a day head held of a day that's of
// another version of the envelope than the ledger writes, or of the side
// than its reader wants, as one another release wrote.
var errDayHeadOutdated = errors.New("a day head of another version")

// dayHead returns the head the ledger holds of the local day with the given
// date, its side of the version body: failing with fs.ErrNotExist where it
// holds none; with errDayHeadOutdated where it's of another version, of the
// envelope or the side; and with errNotTheDaysHead where it doesn't read as
// the day's head. Its fields but its side are judged first, as dayHeadIn
// reads them, so the side of one of another version, or of another day, is
// never read.
func (d days) dayHead(date string, body int) (DayHead, error) {
	stored, err := storedRead(logdayFile(d.files.Dir, date), func(s storedDayHead, err error) error { return s.judged(date, body, err) },
		dayHeadIn, storedIn[storedDayHead])
	if err != nil {
		return DayHead{}, err
	}
	return stored.dayHead(), nil
}

// dayHeadIn returns the day head r holds, as its file holds it, its side
// none, as storedHeadIn reads it, stopping at its side.
func dayHeadIn(r io.Reader) (storedDayHead, error) {
	return storedHeadIn[storedDayHead](r, logdaySide)
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
	if err != nil || !(digestGrounds{kept: followerLook{f: f}.kept}).dayHeadStands(head) {
		return nil, false
	}
	return head.Side, true
}

// day hands take each line of the requests that arrived on the local day
// with the given date, by now, oldest first, as Follower.Day gives them,
// until take reports false: read whole from the day's files and those of the
// days either side of it, as a change of time zone files a line under a date
// beside its own, and a request that ends after midnight is its arrival
// day's. It notes in faults, where they're given, the days whose files it
// couldn't read for now, as dayfile.Faults says, and hands none of a date
// that isn't one. How many lines couldn't be read is warned of. It's the one
// read of a day's lines that both Day and a day's head take, so the head is
// built of exactly the lines Day gives, in its order.
func (d days) day(date string, now time.Time, take func(Held) bool, faults *dayfile.Faults) {
	start, end, ok := dayfile.Day(date)
	if !ok {
		return
	}
	within := func(h Held) bool { return !h.At.Before(start) && h.At.Before(end) && !h.At.After(now) }
	d.lines(dayfile.Dates(start, start), lineHeldIn, within, take, faults, "day", date)
}

// errDayUnread is the error of a day whose lines, or those of a day either
// side of it, couldn't all be read for now.
var errDayUnread = errors.New("its files, or those of the days either side of it, can't all be read for now")

// seenHead is the head of a day as the digesting last read it: its file, as
// it looked then, and the head, its side none, as dayHeadStands judges it
// afresh at each pass.
type seenHead struct {
	file fs.FileInfo
	head DayHead
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
// as kept tells, and no head is held of it that stands, as dayHeadStands
// says, of the version its Digester gives. A head is read only where its file
// looks otherwise than it did as the digesting last read it, as seenHead
// holds it; and a day whose lines can't be told isn't due, as no head built
// of it would stand.
func (d *digesting) headDue(date string, view writerLook) bool {
	day := view.day(date)
	if _, known := day.kept(); day.look.none || !known {
		return false
	}
	head, held := d.seen(date)
	return !held || !(digestGrounds{kept: view.kept}).dayHeadStands(head)
}

// seen returns the head the ledger holds of the local day with the given
// date, of the version its Digester gives, its side none, as it was last
// read, while its file looks as it did then; else read afresh, and held. It
// reports false where the ledger holds none that reads.
func (d *digesting) seen(date string) (DayHead, bool) {
	file := d.days.dayHeadInfo(date)
	if s, ok := d.heads[date]; ok && file != nil && dayfile.SameAs(s.file, file) {
		return s.head, true
	}
	delete(d.heads, date)
	head, err := d.days.dayHead(date, d.digester.RowsVersion())
	if err != nil {
		return DayHead{}, false
	}
	d.see(date, file, head)
	return head, true
}

// see holds head, the head of the local day with the given date, as its file
// looks as given, its side let go of.
func (d *digesting) see(date string, file fs.FileInfo, head DayHead) {
	head.Side = nil
	d.heads[date] = seenHead{file: file, head: head}
}

// buildHead builds the head of the local day with the given date, at now, as
// headOf does, view finding the day, and writes it, as writeDayHead does,
// holding it as seen from then on. It writes none where ctx ended first, or
// the day's summary or its files look otherwise once it's built than as view
// found them, as they changed while it was built, so it's due again. Where
// it can't be built, or written, that's warned of, as warnings.warn says,
// and it's built again at the next pass.
func (d *digesting) buildHead(ctx context.Context, date string, view writerLook, now time.Time) {
	day := view.day(date)
	head, err := d.headOf(ctx, date, day, now)
	switch {
	case ctx.Err() != nil || !d.days.lookAt(date).same(day.look):
		return
	case err == nil:
		err = d.days.writeDayHead(head)
	}
	if err != nil {
		d.headsWarned.warn(d.days.logger, date, day.look, "can't build the request ledger's head of a day", "error", err)
		return
	}
	delete(d.headsWarned, date)
	if file := d.days.dayHeadInfo(date); file != nil {
		d.see(date, file, head)
	}
	d.days.logger.Info("built the head of a day of the request ledger", "day", date, "lines", head.Lines)
}

// headOf returns the head of the local day with the given date, at now, day
// as the look found it: of the day's bounds, its summary's lines, and its
// lines as Follower.Day gives them, as days.day reads them, each handed to a
// builder of its Digester's, whose side it holds. It stops reading them once
// ctx ends, failing; and fails where the files of the day, or of a day
// either side of it, can't all be read for now, or the builder fails.
func (d *digesting) headOf(ctx context.Context, date string, day writtenDay, now time.Time) (DayHead, error) {
	start, end, _ := dayfile.Day(date)
	summaryLines, _ := day.kept()
	head := DayHead{Body: d.digester.RowsVersion(), Day: date, Start: start, End: end, SummaryLines: summaryLines}
	build := d.digester.Day()
	faults := dayfile.NewFaults(nil)
	d.days.day(date, now, func(h Held) bool {
		build.Take(h.Line)
		head.Lines++
		return ctx.Err() == nil
	}, faults)
	switch {
	case ctx.Err() != nil:
		return DayHead{}, fmt.Errorf("the lines of %s: %w", date, ctx.Err())
	case faults.Any(dayfile.Dates(start, start)):
		return DayHead{}, fmt.Errorf("the lines of %s: %w", date, errDayUnread)
	}
	built, err := build.Built()
	if err != nil {
		return DayHead{}, fmt.Errorf("the side of %s: %w", date, err)
	}
	head.Side = built.Side
	return head, nil
}
