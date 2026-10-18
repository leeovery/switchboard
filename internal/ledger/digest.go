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
	"os"
	"path/filepath"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
)

const (
	// digestVersion is the version of the digests' envelope the ledger
	// writes: their head, and how each session's entry is kept, but for the
	// sessions' states, whose version is their builder's. A release that
	// keeps a digest otherwise writes another, so the digests written before
	// it read as outdated, and their days are digested again.
	digestVersion = 1
	// digestPrefix and digestSuffix begin and end the name of a day's digest:
	// digest-<date>.json.gz.
	digestPrefix, digestSuffix = "digest-", ".json.gz"
	// digestMax is the most a digest's file is inflated to as it's read: a
	// busy day's runs to a few megabytes, and a file made by hand to inflate
	// past it mustn't be read whole into memory.
	digestMax = 64 << 20
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
// day's lines onto the session's state before them, and what it was built
// from, as its head says. It's derived and disposable, as the day's lines
// stay the truth: it stands only while what it was built from does, as
// digestGrounds.stands says, and a reader of one that doesn't, or of none,
// reads the day's lines instead, slower, but alike.
type Digest struct {
	Head DigestHead
	// Sessions are the sessions' entries, by their ids.
	Sessions map[string]DigestEntry
}

// DigestHead is what a digest says of its day, as it was built: Body, the
// version of its sessions' states, their builder's, as the ledger never
// reads them; Day, the local day's date; Start and End, when the day started
// and ended, as dayfile.Day gave them in the time zone it was built in;
// SummaryLines, the Lines of the day's summary it was built against; Lines,
// how many of the day's lines it read; and Early and Late, how many of those
// arrived before the day started, or after it ended, as a change of time
// zone files a line under a date beside its own.
type DigestHead struct {
	Body         int
	Day          string
	Start, End   time.Time
	SummaryLines int
	Lines        int
	Early, Late  int
}

// DigestEntry is a session's entry in a digest: RestsOn, each day its state
// was folded from, by its date, the digest's own day among them, with the
// Lines of that day's summary as the state was built; and State, the
// session's state at the day's end, a JSON value of its builder's, carried
// as it's given, its insignificant spaces aside, and never read: so a reader
// of a few sessions of a busy day decodes theirs alone.
type DigestEntry struct {
	RestsOn map[string]int
	State   json.RawMessage
}

// storedDigest is a digest as its file holds it, gzipped: the version of the
// envelope it was written in, its head, its day's bounds as nanoseconds
// since the Unix epoch, and its sessions' entries, by their ids, zeros left
// out. The entries come last, as storedHeadIn reads the head alone by
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
	Sessions     map[string]storedEntry `json:"sessions,omitzero"`
}

// storedEntry is a session's entry as a digest's file holds it.
type storedEntry struct {
	RestsOn map[string]int  `json:"rests_on,omitzero"`
	State   json.RawMessage `json:"state,omitempty"`
}

// storedOf is digest as its file holds it, of the envelope's version the
// ledger writes.
func storedOf(digest Digest) storedDigest {
	h := digest.Head
	return storedDigest{
		Version: digestVersion, Body: h.Body, Day: h.Day, From: h.Start.UnixNano(), To: h.End.UnixNano(),
		SummaryLines: h.SummaryLines, Lines: h.Lines, Early: h.Early, Late: h.Late,
		Sessions: entriesAs(digest.Sessions, DigestEntry.stored),
	}
}

// digest is the digest s holds, its day's bounds in the local time zone.
func (s storedDigest) digest() Digest {
	return Digest{
		Head: DigestHead{
			Body: s.Body, Day: s.Day, Start: time.Unix(0, s.From), End: time.Unix(0, s.To),
			SummaryLines: s.SummaryLines, Lines: s.Lines, Early: s.Early, Late: s.Late,
		},
		Sessions: entriesAs(s.Sessions, storedEntry.entry),
	}
}

// stored is e as a digest's file holds it.
func (e DigestEntry) stored() storedEntry {
	return storedEntry(e)
}

// entry is the entry e holds.
func (e storedEntry) entry() DigestEntry {
	return DigestEntry(e)
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

// writeDigest writes digest as the digest of its day, whole, the user's
// alone, over any there, as storedOf keeps it, gzipped. It fails where its
// day isn't a date, or a session's state isn't a JSON value, writing
// nothing.
func (d days) writeDigest(digest Digest) error {
	date := digest.Head.Day
	if _, _, ok := dayfile.Day(date); !ok {
		return fmt.Errorf("digest of %q: not a date", date)
	}
	data, err := gzippedJSON(storedOf(digest))
	if err != nil {
		return fmt.Errorf("digest of %s: %w", date, err)
	}
	return atomicfile.Write(digestFile(d.files.Dir, date), data, 0o600)
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
// errNotTheDaysDigest where it doesn't read as the day's digest. It reads
// none of the sessions' states.
func (d days) digest(date string, body int) (Digest, error) {
	return d.digestRead(date, body, storedIn)
}

// digestHead returns the head of the digest the ledger holds of the local
// day with the given date, as digest judges it, reading none of its
// sessions' entries, as storedHeadIn reads it: failing as digest fails,
// but for a file damaged after its head, as the rest of it isn't read.
func (d days) digestHead(date string, body int) (DigestHead, error) {
	digest, err := d.digestRead(date, body, storedHeadIn)
	return digest.Head, err
}

// digestRead returns the digest the ledger holds of the local day with the
// given date, as read reads its file, judged as digest judges it.
func (d days) digestRead(date string, body int, read func(io.Reader) (storedDigest, error)) (Digest, error) {
	file, err := os.Open(digestFile(d.files.Dir, date))
	if err != nil {
		return Digest{}, err
	}
	defer func() { _ = file.Close() }()
	stored, err := read(file)
	switch {
	case err != nil:
		return Digest{}, fmt.Errorf("%w: %w", errNotTheDaysDigest, err)
	case stored.Version != digestVersion || stored.Body != body:
		return Digest{}, fmt.Errorf("%w: version %d, its states' %d", errDigestOutdated, stored.Version, stored.Body)
	case stored.Day != date:
		return Digest{}, fmt.Errorf("%w: it's of %q", errNotTheDaysDigest, stored.Day)
	}
	return stored.digest(), nil
}

// errDigestTooLarge is the error of a digest's file that inflates past
// digestMax.
var errDigestTooLarge = fmt.Errorf("inflates past %d bytes", digestMax)

// storedIn returns the digest r holds, as its file holds it: read to its
// end, so a file cut short, or damaged, fails as its gzip trailer is
// checked, whatever its JSON reads as; and failing with errDigestTooLarge,
// having inflated no more than digestMax and a byte, where it inflates past
// that.
func storedIn(r io.Reader) (storedDigest, error) {
	members, err := gzip.NewReader(r)
	if err != nil {
		return storedDigest{}, err
	}
	data, err := io.ReadAll(io.LimitReader(members, digestMax+1))
	switch {
	case err != nil:
		return storedDigest{}, err
	case len(data) > digestMax:
		return storedDigest{}, errDigestTooLarge
	}
	var stored storedDigest
	err = json.Unmarshal(data, &stored)
	return stored, err
}

// digestSessions is the name a digest's file keeps its sessions' entries
// under, after every field of its head, as storedDigest declares them.
const digestSessions = "sessions"

// storedHeadIn returns the head of the digest r holds, as its file holds it,
// its sessions none: reading its fields as far as its sessions' entries,
// which follow every field of the head, so none of them is inflated or
// read. It fails where the head doesn't read whole, inflating no more than
// digestMax.
func storedHeadIn(r io.Reader) (storedDigest, error) {
	members, err := gzip.NewReader(r)
	if err != nil {
		return storedDigest{}, err
	}
	var head bytes.Buffer
	if err := headCopied(jsontext.NewDecoder(io.LimitReader(members, digestMax)), jsontext.NewEncoder(&head)); err != nil {
		return storedDigest{}, err
	}
	var stored storedDigest
	err = json.Unmarshal(head.Bytes(), &stored)
	return stored, err
}

// headCopied writes to head, as an object of its own, the members of the
// object from reads, as far as its sessions' entries, or its end.
func headCopied(from *jsontext.Decoder, head *jsontext.Encoder) error {
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
		case token.Kind() == jsontext.EndObject.Kind(), token.String() == digestSessions:
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
// each from its caller's own look at the ledger, the writer's round's and a
// reader's alike. kept gives how many lines the ledger keeps of a local day,
// by its date, as the summary of them that stands as held counts them, its
// Lines; none of a day it keeps no lines of, its lines pruned, or none ever
// written; and reports false of a day whose lines it keeps without a summary
// that stands, or with one counting more lines than its files hold. heads gives the head of the digest the ledger holds of a
// day, by its date, as digest reads it, reporting false where it holds none
// that reads.
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
	return g.current(head) && head.Early == 0 && head.Late == 0 && g.alone(head.Day) && g.restsOn(entry)
}

// current reports whether head is of its day as the ledger keeps it now: its
// bounds the day's in this time zone, as a digest built in another counted
// its lines into other days, and the lines it was built against those the
// day keeps.
func (g digestGrounds) current(head DigestHead) bool {
	start, end, ok := dayfile.Day(head.Day)
	lines, known := g.kept(head.Day)
	return ok && head.Start.Equal(start) && head.End.Equal(end) && known && lines == head.SummaryLines
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
	start, end, _ := dayfile.Day(date)
	if after, current := g.currentHead(dayfile.DateOf(end)); current && after.Early > 0 {
		return false
	}
	before := dayfile.DateOf(dayfile.DayStart(start, -1))
	if lines, known := g.kept(before); known && lines == 0 {
		return true
	}
	head, current := g.currentHead(before)
	return current && head.Late == 0
}

// currentHead returns the head of the digest held of the local day with the
// given date, reporting false where none is held, or it isn't current, as
// current says.
func (g digestGrounds) currentHead(date string) (DigestHead, bool) {
	head, held := g.heads(date)
	return head, held && g.current(head)
}

// restsOn reports whether each day entry rests on keeps the lines it did as
// entry was built.
func (g digestGrounds) restsOn(entry DigestEntry) bool {
	for date, built := range entry.RestsOn {
		if lines, known := g.kept(date); !known || lines != built {
			return false
		}
	}
	return true
}
