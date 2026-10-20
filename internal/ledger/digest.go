package ledger

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
)

const (
	// digestVersion is the version of the digests' envelope the ledger
	// writes: their head, and how each session's entry is kept, its part's
	// place in the day's parts file among it, but for the sessions' states
	// and parts, whose version is their builder's. A release that keeps a
	// digest otherwise writes another, so the digests written before it read
	// as outdated, and their days are digested again.
	digestVersion = 2
	// digestPrefix and digestSuffix begin and end the name of a day's digest:
	// digest-<date>.json.gz.
	digestPrefix, digestSuffix = "digest-", ".json.gz"
	// storedMax is the most a digest's file, or a day head's, is inflated to
	// as it's read: a busy day's digest runs to a few megabytes, and a file
	// made by hand to inflate past it mustn't be read whole into memory.
	storedMax = 64 << 20
)

// digestFile is where the ledger in dir keeps the digest of the local day
// with the given date.
func digestFile(dir, date string) string {
	return filepath.Join(dir, digestPrefix+date+digestSuffix)
}

// digestInfo returns a look at the file of the digest the ledger holds of the
// local day with the given date: nil where it holds none, or the file can't
// be looked at.
func (d days) digestInfo(date string) fs.FileInfo {
	info, err := os.Stat(digestFile(d.files.Dir, date))
	if err != nil {
		return nil
	}
	return info
}

// Digest is a finished day's digest, beside the day's lines and its summary:
// each session's state at the day's end, as its builder folded it from the
// day's lines onto the session's state before them, where its part of the
// day lies in the day's parts file, and what it was built from, as its head
// says. It's derived and disposable, as the day's lines stay the truth: it
// stands only while what it was built from does, as digestGrounds.stands
// says, and a reader of one that doesn't, or of none, reads the day's lines
// instead, slower, but alike.
type Digest struct {
	Head DigestHead
	// Sessions are the sessions' entries, by their ids.
	Sessions map[string]DigestEntry
}

// DigestHead is what a digest says of its day, as it was built: Body, the
// version of its sessions' states and parts, their builder's, as the ledger
// never reads them; Day, the local day's date; Start and End, when the day
// started and ended, as dayfile.Day gave them in the time zone it was built
// in; SummaryLines, the Lines of the day's summary it was built against;
// Lines, how many of the day's lines it read; Early and Late, how many of
// those arrived before the day started, or after it ended, as a change of
// time zone files a line under a date beside its own; and PartsSize, how
// long the day's parts file was written beside it, in bytes, none where
// none was.
type DigestHead struct {
	Body         int
	Day          string
	Start, End   time.Time
	SummaryLines int
	Lines        int
	Early, Late  int
	PartsSize    int64
}

// DigestEntry is a session's entry in a digest: RestsOn, each day its state
// was folded from, by its date, the digest's own day among them, with the
// Lines of that day's summary as the state was built; State, the session's
// state at the day's end, a JSON value of its builder's, carried as it's
// given, its insignificant spaces aside, and never read: so a reader of a
// few sessions of a busy day decodes theirs alone; and Part, where its part
// of the day lies in the day's parts file, nil where it has none.
type DigestEntry struct {
	RestsOn map[string]int
	State   json.RawMessage
	Part    *PartRef
}

// PartRef is where a session's part of a day lies in the day's parts file,
// as the day's digest points at it: On, the date of the state the part was
// folded on, "" where it was folded on none, as a session's parts chain from
// one folded on none through the states each was folded on; the Member, the
// part's gzip member there; and Times, the member of its times, after it,
// as Folded has them, nil where it has none.
type PartRef struct {
	On string
	Member
	Times *Member
}

// storedDigest is a digest as its file holds it, gzipped: the version of the
// envelope it was written in, its head, its day's bounds as nanoseconds
// since the Unix epoch, and its sessions' entries, by their ids, zeros left
// out. The entries come last, as digestHeadIn reads the head alone by
// stopping at them.
type storedDigest struct {
	Version      int                    `json:"version"`
	Body         int                    `json:"body"`
	Day          string                 `json:"day"`
	From         int64                  `json:"from"`
	To           int64                  `json:"to"`
	SummaryLines int                    `json:"summary_lines,omitzero"`
	Lines        int                    `json:"lines,omitzero"`
	Early        int                    `json:"early,omitzero"`
	Late         int                    `json:"late,omitzero"`
	PartsSize    int64                  `json:"parts_size,omitzero"`
	Sessions     map[string]storedEntry `json:"sessions,omitzero"`
}

// storedEntry is a session's entry as a digest's file holds it.
type storedEntry struct {
	RestsOn map[string]int  `json:"rests_on,omitzero"`
	State   json.RawMessage `json:"state,omitempty"`
	Part    *storedPart     `json:"part,omitzero"`
}

// storedPart is where a session's part lies, and its times, as a digest's
// file holds it.
type storedPart struct {
	On string `json:"on,omitzero"`
	storedMember
	Times *storedMember `json:"times,omitempty"`
}

// storedOf is digest as its file holds it, of the envelope's version the
// ledger writes.
func storedOf(digest Digest) storedDigest {
	h := digest.Head
	return storedDigest{
		Version: digestVersion, Body: h.Body, Day: h.Day, From: h.Start.UnixNano(), To: h.End.UnixNano(),
		SummaryLines: h.SummaryLines, Lines: h.Lines, Early: h.Early, Late: h.Late, PartsSize: h.PartsSize,
		Sessions: entriesAs(digest.Sessions, DigestEntry.stored),
	}
}

// digest is the digest s holds, its day's bounds in the local time zone.
func (s storedDigest) digest() Digest {
	return Digest{
		Head: DigestHead{
			Body: s.Body, Day: s.Day, Start: time.Unix(0, s.From), End: time.Unix(0, s.To),
			SummaryLines: s.SummaryLines, Lines: s.Lines, Early: s.Early, Late: s.Late, PartsSize: s.PartsSize,
		},
		Sessions: entriesAs(s.Sessions, storedEntry.entry),
	}
}

// stored is e as a digest's file holds it.
func (e DigestEntry) stored() storedEntry {
	return storedEntry{RestsOn: e.RestsOn, State: e.State, Part: e.Part.stored()}
}

// entry is the entry e holds.
func (e storedEntry) entry() DigestEntry {
	return DigestEntry{RestsOn: e.RestsOn, State: e.State, Part: e.Part.ref()}
}

// stored is r as a digest's file holds it: none of none.
func (r *PartRef) stored() *storedPart {
	if r == nil {
		return nil
	}
	p := &storedPart{On: r.On, storedMember: r.Member.stored()}
	if r.Times != nil {
		times := r.Times.stored()
		p.Times = &times
	}
	return p
}

// ref is where p says a part lies: none of none.
func (p *storedPart) ref() *PartRef {
	if p == nil {
		return nil
	}
	r := &PartRef{On: p.On, Member: p.member()}
	if p.Times != nil {
		times := p.Times.member()
		r.Times = &times
	}
	return r
}

// entriesAs is entries, each as as gives it, by the same ids: none of none,
// so a digest reads back as it was written.
func entriesAs[From, To any](entries map[string]From, as func(From) To) map[string]To {
	if entries == nil {
		return nil
	}
	converted := make(map[string]To, len(entries))
	for id, e := range entries {
		converted[id] = as(e)
	}
	return converted
}

// listAs is list, each as as gives it, in the same order: none of none, so a
// day head reads back as it was written.
func listAs[From, To any](list []From, as func(From) To) []To {
	if list == nil {
		return nil
	}
	converted := make([]To, len(list))
	for i, e := range list {
		converted[i] = as(e)
	}
	return converted
}

// writeDigest writes digest as the digest of its day, whole, the user's
// alone, over any there, as storedOf keeps it, gzipped; and before it parts,
// the parts of sessions it holds entries of, by their ids, as the day's parts
// file, as partsFileOf makes it and writeParts writes it: each entry pointing
// at its session's part there, none at any other, and its head recording the
// file's size. So a digest is never written before the parts it points at:
// one left beside a parts file written since, as by a build that stopped
// between the two, is held beside a parts file of another size than its head
// records, so its day is due, or its parts don't read, by their CRC. It
// returns the digest as written. It fails where its day isn't a date, or a
// session's state isn't a JSON value, writing nothing.
func (d days) writeDigest(digest Digest, parts map[string]builtPart) (Digest, error) {
	date := digest.Head.Day
	if _, _, ok := dayfile.Day(date); !ok {
		return Digest{}, fmt.Errorf("digest of %q: not a date", date)
	}
	file, refs := partsFileOf(parts)
	digest = digest.pointedAt(refs, int64(len(file)))
	data, err := gzippedJSON(storedOf(digest))
	if err != nil {
		return Digest{}, fmt.Errorf("digest of %s: %w", date, err)
	}
	if err := d.writeParts(date, file); err != nil {
		return Digest{}, fmt.Errorf("parts of %s: %w", date, err)
	}
	return digest, atomicfile.Write(digestFile(d.files.Dir, date), data, 0o600)
}

// pointedAt returns d with each session's entry pointing at its part where
// refs has it, by its id, none where it doesn't, and its head recording the
// parts file's size as given, d left as it is.
func (d Digest) pointedAt(refs map[string]PartRef, size int64) Digest {
	d.Head.PartsSize = size
	d.Sessions = maps.Clone(d.Sessions)
	for id, e := range d.Sessions {
		e.Part = nil
		if ref, ok := refs[id]; ok {
			e.Part = &ref
		}
		d.Sessions[id] = e
	}
	return d
}

// gzippedJSON is v as JSON, a line of its own, its strings as they're given,
// none of their characters escaped for HTML, as one gzip member.
func gzippedJSON(v any) ([]byte, error) {
	var data bytes.Buffer
	w := gzip.NewWriter(&data)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := errors.Join(enc.Encode(v), w.Close()); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

// errNotTheDaysDigest is the error of a digest held of a day that doesn't
// read as the day's digest, as one cut short or damaged, or of another day.
var errNotTheDaysDigest = errors.New("not the day's digest")

// errDigestOutdated is the error of a digest held of a day that's of another
// version of the envelope than the ledger writes, or of the sessions' states
// than its reader wants, as one another release wrote.
var errDigestOutdated = errors.New("a digest of another version")

// digest returns the digest the ledger holds of the local day with the
// given date, its sessions' states of the version body: failing with
// fs.ErrNotExist where it holds none; with errDigestOutdated where it's of
// another version, of the envelope or the states; and with
// errNotTheDaysDigest where it doesn't read as the day's digest. Its head is
// judged first, as digestHead judges it, so the sessions' entries of one of
// another version, as every one is after a release raising either, or of
// another day, are never read; and it reads none of the sessions' states.
func (d days) digest(date string, body int) (Digest, error) {
	return d.digestRead(date, body, digestHeadIn, storedIn[storedDigest])
}

// digestHead returns the head of the digest the ledger holds of the local
// day with the given date, as digest judges it, reading none of its
// sessions' entries, as digestHeadIn reads it: failing as digest fails,
// but for a file damaged after its head, as the rest of it isn't read.
func (d days) digestHead(date string, body int) (DigestHead, error) {
	digest, err := d.digestRead(date, body, digestHeadIn)
	return digest.Head, err
}

// digestRead returns the digest the ledger holds of the local day with the
// given date, as the last of reads reads its file, as storedRead has them
// read, each judged as judged judges it, so one judged otherwise than as the
// day's of the version asked for is read no further.
func (d days) digestRead(date string, body int, reads ...func(io.Reader) (storedDigest, error)) (Digest, error) {
	stored, err := storedRead(digestFile(d.files.Dir, date), func(s storedDigest, err error) error { return s.judged(date, body, err) }, reads...)
	if err != nil {
		return Digest{}, err
	}
	return stored.digest(), nil
}

// storedRead returns what the file at path holds, as the last of reads reads
// it: each reading it from its start in turn, what it read judged, as judged
// judges it with the read's failure, before the next reads, so one judged
// wanting is read no further. It fails as os.Open fails where the file can't
// be opened, with fs.ErrNotExist where there's none, and as judged fails.
func storedRead[S any](path string, judged func(S, error) error, reads ...func(io.Reader) (S, error)) (S, error) {
	var stored S
	file, err := os.Open(path)
	if err != nil {
		return stored, err
	}
	defer func() { _ = file.Close() }()
	for _, read := range reads {
		_, err := file.Seek(0, io.SeekStart)
		if err == nil {
			stored, err = read(file)
		}
		if err := judged(stored, err); err != nil {
			return stored, err
		}
	}
	return stored, nil
}

// judged returns what's wrong with s, a digest as read of the local day with
// the given date, its read failing with err, where it's to be of the
// version body of the sessions' states: errNotTheDaysDigest where it didn't
// read, or is of another day; errDigestOutdated where it's of another
// version, of the envelope or the states; nil where nothing is.
func (s storedDigest) judged(date string, body int, err error) error {
	switch {
	case err != nil:
		return fmt.Errorf("%w: %w", errNotTheDaysDigest, err)
	case s.Version != digestVersion || s.Body != body:
		return fmt.Errorf("%w: version %d, its states' %d", errDigestOutdated, s.Version, s.Body)
	case s.Day != date:
		return fmt.Errorf("%w: it's of %q", errNotTheDaysDigest, s.Day)
	}
	return nil
}

// errStoredTooLarge is the error of a digest's file, or a day head's, that
// inflates past storedMax.
var errStoredTooLarge = fmt.Errorf("inflates past %d bytes", storedMax)

// storedIn returns the S r holds, gzipped JSON, as its file holds it: read to
// its end, so a file cut short, or damaged, fails as its gzip trailer is
// checked, whatever its JSON reads as; and failing with errStoredTooLarge,
// having inflated no more than storedMax and a byte, where it inflates past
// that.
func storedIn[S any](r io.Reader) (S, error) {
	var stored S
	members, err := gzip.NewReader(r)
	if err != nil {
		return stored, err
	}
	data, err := io.ReadAll(io.LimitReader(members, storedMax+1))
	switch {
	case err != nil:
		return stored, err
	case len(data) > storedMax:
		return stored, errStoredTooLarge
	}
	err = json.Unmarshal(data, &stored)
	return stored, err
}

// digestSessions is the name a digest's file keeps its sessions' entries
// under, after every field of its head, as storedDigest declares them.
const digestSessions = "sessions"

// digestHeadIn returns the head of the digest r holds, as its file holds it,
// its sessions none, as storedHeadIn reads it, stopping at its sessions'
// entries.
func digestHeadIn(r io.Reader) (storedDigest, error) {
	return storedHeadIn[storedDigest](r, digestSessions)
}

// storedHeadIn returns the head of the S r holds, gzipped JSON, as its file
// holds it, none of what follows it: reading its fields as far as the first
// of those named rest, which follow every field of the head, so none of
// what follows is inflated or read. It fails where the head doesn't read
// whole, inflating no more than storedMax.
func storedHeadIn[S any](r io.Reader, rest ...string) (S, error) {
	var stored S
	members, err := gzip.NewReader(r)
	if err != nil {
		return stored, err
	}
	var head bytes.Buffer
	if err := headCopied(jsontext.NewDecoder(io.LimitReader(members, storedMax)), jsontext.NewEncoder(&head), rest); err != nil {
		return stored, err
	}
	err = json.Unmarshal(head.Bytes(), &stored)
	return stored, err
}

// headCopied writes to head, as an object of its own, the members of the
// object from reads, as far as the first of those named rest, or its end.
func headCopied(from *jsontext.Decoder, head *jsontext.Encoder, rest []string) error {
	begin, err := from.ReadToken()
	switch {
	case err != nil:
		return err
	case begin.Kind() != jsontext.BeginObject.Kind():
		return fmt.Errorf("begins with %v, not an object", begin.Kind())
	}
	if err := head.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	for {
		token, err := from.ReadToken()
		switch {
		case err != nil:
			return err
		case token.Kind() == jsontext.EndObject.Kind(), slices.Contains(rest, token.String()):
			return head.WriteToken(jsontext.EndObject)
		}
		name := jsontext.String(token.String())
		value, err := from.ReadValue()
		if err != nil {
			return err
		}
		if err := errors.Join(head.WriteToken(name), head.WriteValue(value)); err != nil {
			return err
		}
	}
}

// digestGrounds are what a digest is judged against, as stands judges it,
// and a day head, as dayHeadStands does, each from its caller's own look at
// the ledger, the writer's round's and a reader's alike. kept gives how many
// lines the ledger keeps of a local day, by its date, as the summary of them
// that stands as held counts them, its Lines; none of a day it keeps no
// lines of, its lines pruned, or none ever written; and reports false of a
// day whose lines it keeps without a summary that stands, or with one
// counting more lines than its files hold. heads gives the head of the
// digest the ledger holds of a day, by its date, as digest reads it,
// reporting false where it holds none that reads.
type digestGrounds struct {
	kept  func(date string) (lines int, known bool)
	heads func(date string) (DigestHead, bool)
}

// stands reports whether the digest whose head is head stands, and with it
// entry, one of its sessions' entries, where it's given, as g says: so a
// reader resting on them reads what a read of the raw lines reads. The head
// is of its day as the ledger keeps it now, as current says; none of the
// lines it read arrived on another day, and none of the lines of the days
// beside it arrived on its own, as alone says; and each day the entry rests
// on keeps the lines it did as the entry was built, as one whose summary was
// made again since, or whose lines were pruned, gives a raw read otherwise.
// The zero entry judges the head alone.
func (g digestGrounds) stands(head DigestHead, entry DigestEntry) bool {
	return g.current(head.against()) && head.Early == 0 && head.Late == 0 && g.alone(head.Day) && g.restsOn(entry.RestsOn)
}

// builtAgainst is what a head built of a local day, a digest's or the day's
// own, was built against, as current judges it: day, the day's date; start
// and end, when it started and ended, as dayfile.Day gave them in the time
// zone it was built in; and summaryLines, the Lines of the day's summary.
type builtAgainst struct {
	day          string
	start, end   time.Time
	summaryLines int
}

// against is what h was built against.
func (h DigestHead) against() builtAgainst {
	return builtAgainst{day: h.Day, start: h.Start, end: h.End, summaryLines: h.SummaryLines}
}

// current reports whether a head built against built is of its day as the
// ledger keeps it now: its bounds the day's in this time zone, as one built
// in another counted its lines into other days, and the lines it was built
// against those the day keeps.
func (g digestGrounds) current(built builtAgainst) bool {
	start, end, ok := dayfile.Day(built.day)
	lines, known := g.kept(built.day)
	return ok && built.start.Equal(start) && built.end.Equal(end) && known && lines == built.summaryLines
}

// alone reports whether no line the days either side of the local day with
// the given date hold arrived on it, as far as their digests tell. Of the day
// after's, it's told only where its digest is current: one without is the
// lookup's to catch, as sessionsLookup.asOf rests a session on none where
// one of the lines it hands on after its base, as that day's, read raw after
// a state resting on this one, arrived before the base's day ended. The
// store's gained doesn't catch it: once the store's day is two or more after
// this one, it only counts such a line among its own day's requests, and may
// take it in.
// Of the day before's, it can't be told without one, as no reader resting on
// the day reads that day's lines: its digest must be current and hold none
// that arrived after it ended, unless it keeps no lines at all.
func (g digestGrounds) alone(date string) bool {
	before, after := beside(date)
	if head, current := g.currentHead(after); current && head.Early > 0 {
		return false
	}
	if lines, known := g.kept(before); known && lines == 0 {
		return true
	}
	head, current := g.currentHead(before)
	return current && head.Late == 0
}

// beside returns the dates of the local days either side of the one with the
// given date.
func beside(date string) (before, after string) {
	start, end, _ := dayfile.Day(date)
	return dayfile.DateOf(dayfile.DayStart(start, -1)), dayfile.DateOf(end)
}

// currentHead returns the head of the digest held of the local day with the
// given date, reporting false where none is held, or it isn't current, as
// current says.
func (g digestGrounds) currentHead(date string) (DigestHead, bool) {
	head, held := g.heads(date)
	return head, held && g.current(head.against())
}

// restsOn reports whether each day of on, those a digest's entry or a day
// head rests on, keeps the lines on gives of it, by its date, as it did as
// what rests on it was built.
func (g digestGrounds) restsOn(on map[string]int) bool {
	for date, built := range on {
		if lines, known := g.kept(date); !known || lines != built {
			return false
		}
	}
	return true
}
