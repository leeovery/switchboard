package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
)

// StagedDay is a day of the ledger's staged to import, by its date: where the
// file of its lines is staged, as export writes a day's, and where its
// summary is, each "" where none is.
type StagedDay struct {
	Date           string
	Lines, Summary string
}

// ImportedDay is what taking a staged day in did: how its lines were taken
// in, as dayfile.Imported says, and whether its summary was copied in.
type ImportedDay struct {
	Lines   dayfile.Imported
	Summary bool
}

// Import takes the day staged in, as one piece of work on Run's goroutine,
// between the writer's writes and its rounds: its lines, as the writer's
// ImportHolding takes a day in, by the ledger's keep and clock, then its
// summary, as copySummary decides, whatever the keep, as summaries are kept
// for good. So the ledger's writer does nothing between the two, and
// whatever it ran before, a round, a stop or an import cut short, the
// summary is decided on the day's lines as the take left them. Summarising
// is held across both, as RenameDay holds it, before the day's files, so the
// readings history's writer, whose summarising before it prunes, as
// SummariseBeforePrune's, holds it too, summarises the day before the take
// or after the summary is decided, never between them; and the import waits
// on it, as it holds summarising for every day it summarises whole, those
// whose readings the prune takes, as well as its part of the rest. The
// ledger's own rounds run on Run's goroutine, as the import does, so never
// wait on it. Work taken up runs to its end, which leaves the day with both
// or neither, though a caller whose context ends first returns before it
// does, as Do says. It fails as ImportHolding does, and where the ledger's
// files can't be looked at, the staged summary read, or the summary written.
func (l *Ledger) Import(ctx context.Context, day StagedDay) (ImportedDay, error) {
	var copied bool
	copySummary := func(took dayfile.Imported) (err error) {
		copied, err = l.copySummary(day, took)
		return err
	}
	held := importHold{l}
	var lines dayfile.Imported
	var err error
	if day.Lines == "" {
		if done := l.writer.Do(ctx, func() {
			held.Lock()
			defer held.Unlock()
			err = copySummary(dayfile.Imported{})
		}); done != nil {
			return ImportedDay{}, done
		}
	} else {
		lines, err = l.writer.ImportHolding(ctx, day.Date, day.Lines, held, copySummary)
	}
	if err != nil {
		return ImportedDay{}, err
	}
	return ImportedDay{Lines: lines, Summary: copied}, nil
}

// importHold is the ledger's summarising, as an import holds it: calling
// importing, where it's set, as it takes it.
type importHold struct {
	l *Ledger
}

func (h importHold) Lock() {
	if h.l.importing != nil {
		h.l.importing()
	}
	h.l.summarising.Lock()
}

func (h importHold) Unlock() {
	h.l.summarising.Unlock()
}

// RoundNow makes the ledger's round at once, on Run's goroutine, as it makes
// one every hour, summarising the days that have ended, as round does, the
// rest of those to summarise again in the rounds it makes at once after, and
// kicking its digesting once none is left, and returns as dayfile.Writer's
// RoundNow does.
func (l *Ledger) RoundNow(ctx context.Context) error {
	return l.writer.RoundNow(ctx)
}

// RoundAgain has the ledger make its round at once, on Run's goroutine,
// as dayfile.Writer's RoundAgain does. It never waits.
func (l *Ledger) RoundAgain() {
	l.writer.RoundAgain()
}

// TendNow has the ledger compress and prune its files of lines at once where
// that's due, as dayfile.Writer's TendNow does: where it makes the round, it
// summarises the days that have ended first, as every round does.
func (l *Ledger) TendNow(ctx context.Context) error {
	return l.writer.TendNow(ctx)
}

// copySummary takes the summary staged of the day in, its lines taken in as
// took says, as copyOver decides, summarising held, so no round writes a
// summary between its look and its copy, and reports whether it copied it.
func (l *Ledger) copySummary(day StagedDay, took dayfile.Imported) (bool, error) {
	if day.Summary == "" {
		return false, nil
	}
	copied, err := l.days.copyOver(day, took)
	if err != nil {
		return false, fmt.Errorf("import the request ledger's summary of %s: %w", day.Date, err)
	}
	return copied, nil
}

// copyOver takes the summary staged of the day in, as stagedSummary reads it,
// its lines taken in as took says, so the day's summary knows every pool of
// a day of no work either Mac recorded: it copies it in, as writeStaged
// writes it, where copies says, carrying in the ledger's summary's days of
// no work that name a pool, as carriedIdle carries them; else it carries the
// summary staged's into the ledger's, as carryIdle does. Of an account
// neither named a pool for, the day names none. A day whose summary staged
// can be neither copied in nor carried from, as one the ledger holds lines
// of but no summary that reads as the day's, has it left unread. It reports
// whether it copied it.
func (d days) copyOver(day StagedDay, took dayfile.Imported) (bool, error) {
	none, err := d.holdsNoLine(day.Date, took)
	if err != nil {
		return false, err
	}
	held, readable, err := d.readableHeld(day.Date)
	if err != nil || !none && !readable {
		return false, err
	}
	staged, ok, err := d.stagedSummary(day)
	if err != nil || !ok {
		return false, err
	}
	copies, err := d.copies(day.Date, none, staged.Lines)
	switch {
	case err != nil || !copies && !readable:
		return false, err
	case !copies:
		return false, d.carryIdle(day.Date, held, staged)
	}
	if readable {
		if staged.written, err = withIdleOf(staged.written, held.written); err != nil {
			return false, err
		}
	}
	return true, d.writeStaged(day.Date, staged)
}

// copies reports whether the summary staged, counting the given number of
// lines, is copied in over the ledger's: where the ledger holds no line of
// the day, as none says, as holdsNoLine reports it, and its summary gives way
// to the one staged, as givesWay says, so what the ledger knows of the day's
// requests is nothing, and the summary staged knows more.
func (d days) copies(date string, none bool, staged int) (bool, error) {
	if !none {
		return false, nil
	}
	return d.givesWay(date, staged)
}

// carryIdle writes held, the ledger's summary of the local day with the given
// date, with the days of no work of staged, the summary staged, as the import
// renamed it, that name a pool, as carriedIdle carries them: as the other
// Mac alone had them. A day summarised again knows the summary before, whose
// pools are fixed, as IdlePools says, so without them, those accounts' days
// of no work would name none. It's marked as stampMarked marks a summary
// renamed: one whose lines an import merged is summarised again at the next
// round, and one whose files stand stands, with them.
func (d days) carryIdle(date string, held, staged stamped) error {
	written, took, err := carriedIdle(held.written, staged.written)
	if err != nil || !took {
		return err
	}
	if err := atomicfile.Write(SummaryFile(d.files.Dir, date), append(written, '\n'), 0o600); err != nil {
		return err
	}
	d.stampMarked(date, written)
	return nil
}

// withIdleOf returns staged, a summary staged as it's written, with the days
// of no work of held, the ledger's summary of its day as it's written, that
// name a pool carried in, as carriedIdle carries them: staged itself where
// that carries none.
func withIdleOf(staged, held []byte) ([]byte, error) {
	written, took, err := carriedIdle(staged, held)
	if err != nil || !took {
		return staged, err
	}
	return append(written, '\n'), nil
}

// readableHeld returns the summary the ledger holds of the local day with the
// given date, reporting false where it holds none, or one that doesn't read
// as the day's, which is left as it is. It fails where it can't be read.
func (d days) readableHeld(date string) (stamped, bool, error) {
	held, err := d.held(date)
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, errNotTheDaysSummary):
		return stamped{}, false, nil
	case err != nil:
		return stamped{}, false, err
	}
	return held, true, nil
}

// carriedIdle returns into, a summary as it's written, with the days of no
// work of from, another summary of its day as it's written, that name a
// pool, as IdlePool reads them, taken in as takeIdle takes each, in the
// order of their ids; and every other field of into, and each of its
// accounts' days, as it's written, so none a later release added is lost;
// and reports whether it took any. Of a field written twice, the last is
// read, as Go reads one. It fails where either doesn't read as a summary.
func carriedIdle(into, from []byte) ([]byte, bool, error) {
	fields, err := fieldsOf(into)
	if err != nil {
		return nil, false, err
	}
	i := lastField(fields, "accounts")
	if i < 0 {
		fields = append(fields, field{name: "accounts", value: json.RawMessage("[]")})
		i = len(fields) - 1
	}
	held, err := accountDaysIn(fields[i].value)
	if err != nil {
		return nil, false, err
	}
	var other struct {
		Accounts json.RawMessage `json:"accounts"`
	}
	if err := json.Unmarshal(from, &other); err != nil {
		return nil, false, err
	}
	carried, err := accountDaysIn(other.Accounts)
	if err != nil {
		return nil, false, err
	}
	took := false
	for _, c := range carried {
		var taken bool
		if held, taken, err = takeIdle(held, c); err != nil {
			return nil, false, err
		}
		took = took || taken
	}
	if !took {
		return into, false, nil
	}
	slices.SortStableFunc(held, func(a, b accountWritten) int { return strings.Compare(a.day.Account, b.day.Account) })
	written := make([][]byte, len(held))
	for j, h := range held {
		written[j] = h.written
	}
	fields[i].value = slices.Concat([]byte{'['}, bytes.Join(written, []byte{','}), []byte{']'})
	return objectOf(fields), true, nil
}

// takeIdle returns held, a summary's accounts' days, with c, another's day of
// an account, taken in where it's a day of no work that names a pool: whole,
// where held holds no day of its account; or its pool alone, where held's is
// a day of no work that names none. A worked day, or one that names a pool
// already, stands. It reports whether it took it.
func takeIdle(held []accountWritten, c accountWritten) ([]accountWritten, bool, error) {
	pool := c.day.IdlePool()
	if pool == "" {
		return held, false, nil
	}
	j := slices.IndexFunc(held, func(h accountWritten) bool { return h.day.Account == c.day.Account })
	switch {
	case j < 0:
		return append(held, c), true, nil
	case !held[j].day.Idle() || held[j].day.IdlePool() != "":
		return held, false, nil
	}
	pooled, err := held[j].pooled(pool)
	if err != nil {
		return nil, false, err
	}
	held[j] = pooled
	return held, true, nil
}

// pooled returns a with its day naming pool, in its "pool" field, written
// after its "account" field where it has none, every other field as written.
func (a accountWritten) pooled(pool string) (accountWritten, error) {
	fields, err := fieldsOf(a.written)
	if err != nil {
		return accountWritten{}, err
	}
	// A string always marshals.
	value, _ := json.Marshal(pool)
	if i := lastField(fields, "pool"); i >= 0 {
		fields[i].value = value
	} else {
		at := lastField(fields, "account") + 1
		fields = slices.Insert(fields, at, field{name: "pool", value: value})
	}
	a.day.Pool, a.written = pool, objectOf(fields)
	return a, nil
}

// lastField returns the index of the last of fields with the given name,
// as Go reads a field written twice, -1 where there's none.
func lastField(fields []field, name string) int {
	for i, f := range slices.Backward(fields) {
		if f.name == name {
			return i
		}
	}
	return -1
}

// accountWritten is an account's day as a summary writes it, and as it reads.
type accountWritten struct {
	day     AccountDay
	written json.RawMessage
}

// accountDaysIn returns the accounts' days of accounts, a summary's as it
// writes them, none where it holds none, each as it's written and as it
// reads, failing where they don't read.
func accountDaysIn(accounts json.RawMessage) ([]accountWritten, error) {
	if len(accounts) == 0 {
		return nil, nil
	}
	var written []json.RawMessage
	if err := json.Unmarshal(accounts, &written); err != nil {
		return nil, err
	}
	days := make([]accountWritten, len(written))
	for i, w := range written {
		days[i].written = w
		if err := json.Unmarshal(w, &days[i].day); err != nil {
			return nil, err
		}
	}
	return days, nil
}

// holdsNoLine reports whether the ledger holds no line of the local day with
// the given date, its lines just taken in as took says: it held none before
// where they were added, and some where they were merged; else, as of a day
// past keeping or one staged of no line, it's as its files are now. It fails
// where it can't look.
func (d days) holdsNoLine(date string, took dayfile.Imported) (bool, error) {
	switch took.Taken {
	case dayfile.Added:
		return true, nil
	case dayfile.Merged:
		return false, nil
	}
	_, err := d.files.Stat(date)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	return false, err
}

// givesWay reports whether the ledger's summary of the local day with the
// given date gives way to one staged counting the given number of lines:
// where it holds none; or one of no line, as a round summarises a day from
// the readings alone, where the one staged counts some. So one of no line
// never replaces another, and an import repeated, or of this Mac's own
// folder, copies none again. One that doesn't read as the day's may have
// counted some, which can't be had again, so it stands, as a round leaves
// it. It fails where the summary can't be read.
func (d days) givesWay(date string, staged int) (bool, error) {
	held, err := d.held(date)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, nil
	case errors.Is(err, errNotTheDaysSummary):
		return false, nil
	case err != nil:
		return false, err
	}
	return held.Lines == 0 && staged > 0, nil
}

// stagedSummary returns the summary staged of the day, as it was written,
// with its stamp, reporting false where it doesn't read as the day's, as one
// cut short as it synced, which is warned of: copied in, it would stand in
// the way of the whole one a later import brings. It fails where it can't be
// read.
func (d days) stagedSummary(day StagedDay) (stamped, bool, error) {
	data, stamp, err := readStamped(day.Summary)
	if err != nil {
		return stamped{}, false, err
	}
	summary, err := summaryIn(data, day.Date)
	if err != nil {
		d.logger.Warn("can't read the request ledger's summary of a day staged to import; it's left out", "day", day.Date, "file", day.Summary, "error", err)
		return stamped{}, false, nil
	}
	return stamped{Summary: summary, written: data, stamp: stamp}, true, nil
}

// writeStaged writes the summary staged as the ledger's summary of the local
// day with the given date, as it was written, with its stamp, its
// modification time, beside where it goes and renamed into place.
func (d days) writeStaged(date string, staged stamped) error {
	if err := os.MkdirAll(d.files.Dir, 0o700); err != nil {
		return err
	}
	return atomicfile.WriteModified(SummaryFile(d.files.Dir, date), 0o600, staged.stamp, func(w io.Writer) error {
		_, err := w.Write(staged.written)
		return err
	})
}
