package views

import (
	"context"
	"fmt"

	"github.com/leeovery/switchboard/internal/ledger"
)

// Digests are the views' ledger.Digester, which the router's ledger digests
// each finished day with: each session the day names held as of its end, as
// the store would hold it, its page tallied, and stored, as a day's digest
// and its parts file store it. They open nothing: the ledger hands them
// every line they fold.
type Digests struct{}

// Version is the version of the states and parts the Digests give,
// DigestVersion.
func (Digests) Version() int {
	return DigestVersion
}

// Builder gives a builder of a day's digest, as dayDigest has it, holding
// none of a read's answers yet.
func (Digests) Builder() ledger.DigestBuilder {
	return &dayDigest{answers: make(takenAnswers)}
}

// RowsVersion is the version of what a day's head holds of the day, and its
// rows file: its side, each of its sessions as the Log's find reads them, as
// storedDaySession stores them; and its rows, the record of each line Day
// gives of it, in its order, as storedRow stores one, in chunks, each with
// its meta, as storedChunkMeta stores it. A release that keeps either
// otherwise writes another, and a head of another version doesn't stand,
// nor its rows: the day's lines are read instead. Every head of this
// version holds a side, and a row of each of its lines: none was released
// without them.
const RowsVersion = 2

// RowsVersion is the version of the sides and rows of the days' heads the
// Digests give, RowsVersion.
func (Digests) RowsVersion() int {
	return RowsVersion
}

// Day gives a builder of a finished day's side and rows, as dayBuild has it,
// none of the day's lines taken in yet.
func (Digests) Day() ledger.DayBuilder {
	return &dayBuild{sessions: make(daySessions)}
}

// dayBuild is a builder of a finished day's side and rows by the views'
// Digests, of the record of each line Day gives, in its order: the day's
// sessions, folded as DaySessions folds them, so DaySessionsOn reads from
// its side what it would read of the lines; and its rows, as rowsBuild
// builds them, so a reader of them restores the records of those lines, in
// order.
type dayBuild struct {
	sessions daySessions
	rows     rowsBuild
}

// Take takes in l, the day's next line, its record, as daySessions and
// rowsBuild take it.
func (b *dayBuild) Take(l ledger.Line) {
	rec := lineRecordOf(&l)
	b.sessions.take(&rec)
	b.rows.take(&rec)
}

// Built gives the side of the lines taken in, as daySideOf has it, and
// their rows, as rowsBuild builds them: failing where either can't be
// encoded.
func (b *dayBuild) Built() (ledger.DayBuilt, error) {
	chunks, err := b.rows.built()
	if err != nil {
		return ledger.DayBuilt{}, err
	}
	side, err := daySideOf(b.sessions.sessions())
	if err != nil {
		return ledger.DayBuilt{}, err
	}
	return ledger.DayBuilt{Side: side, Chunks: chunks}, nil
}

// dayDigest is a builder of a day's digest by the views' Digests, for one
// read of the days its sessions are read from: every account's answers that
// read took, for its sessions' pages' points, as takenAnswers holds them.
type dayDigest struct {
	answers takenAnswers
}

// Take takes in l, a line of the day with the given date, where it's an
// answer, as takenAnswers takes it.
func (b *dayDigest) Take(date string, l ledger.Line) {
	b.answers.take(date, l)
}

// Digest gives each session day hands it folded at the day's end, by its id,
// as foldedAsOf has it, of the answers taken. It gives none of one whose
// base won't decode or restore, nor whose page's points can't take the days
// after its base in, nor of one with a request that arrived after the day,
// as a change of time zone files one, which stored refuses: the ledger hands
// one resting on a base again, read whole, and its readers read the rest
// whole, as they read a session of no state. It fails once ctx ends, looked
// at between sessions.
func (b *dayDigest) Digest(ctx context.Context, day ledger.DigestDay) (map[string]ledger.Folded, error) {
	d := LedgerDay{Start: day.Start, Date: day.Date}
	folded := make(map[string]ledger.Folded, len(day.Sessions))
	for id, from := range day.Sessions {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("digest the sessions of %s: %w", day.Date, err)
		}
		// A session left out is no worse than no digest: the session is read
		// whole, never told otherwise than its lines tell it.
		if f, err := foldedAsOf(id, from, b.answers, d); err == nil {
			folded[id] = f
		}
	}
	return folded, nil
}

// foldedAsOf is the session with the given id from gives folded at the end
// of the day d, as a day's digest keeps it, as folded has it: held as
// heldResting holds it on d, its page tallied, read to fold, its times
// counted unheld, as pageToFold has it, its points of the answers taken,
// every account's on the days read, sealed at the day's end; and its part
// over the tally its page rested on. Nothing priced is stored, so the
// built-in table prices it, whatever the config's: what's kept is the same
// whichever does. It fails as heldResting does, and where stored refuses it,
// as of a line of it arrived after the day.
func foldedAsOf(id string, from ledger.SessionFrom, answers takenAnswers, d LedgerDay) (ledger.Folded, error) {
	s, base, err := heldResting(id, from, answers, ledger.Pricing, d, endOfDay(d), pageToFold)
	if err != nil {
		return ledger.Folded{}, err
	}
	return s.folded(base)
}
