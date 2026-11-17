package ledger

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
)

// Files are the ledger's files of lines, which the ledger's writer appends
// to, holding them as dayfile.Files says: one who rewrites them as it runs
// holds them alike.
func (l *Ledger) Files() *dayfile.Files {
	return l.days.files
}

// Renaming is how RenameDay and RenameSummary rename a day's summary: each
// account's id as Account gives it, nil leaving every one as it is; each
// pool's id as Pool gives it, nil leaving every one as it is, as an import
// gives another Mac's pool the pool here holding its accounts; and the work
// of the requests that named no pool given the pool Unpooled, by its id, as
// the pool they were in after all, "" leaving it in none.
type Renaming struct {
	Account  func(id string) string
	Pool     func(id string) string
	Unpooled string
}

// pooling is how r gives each pool of an account's day its id: work in no
// pool the pool Unpooled names, and each pool's as Pool gives it; nil where
// it gives none another.
func (r Renaming) pooling() func(id string) string {
	if r.Unpooled == "" && r.Pool == nil {
		return nil
	}
	return func(id string) string {
		switch {
		case id == "":
			return r.Unpooled
		case r.Pool == nil:
			return id
		}
		return r.Pool(id)
	}
}

// Moving has the ledger's rounds know the move of the accounts onto new ids,
// as moving gives the id the move renames each to, the id itself where it
// renames none, asked once a round, none where it gives nil. The move
// renames a day's lines, and the readings, before the day's summary: a round
// summarising the day between the two, where it failed, or the router
// stopped, finds its lines and readings under the new ids, and its summary
// under the old, which, knowing it as it stands, would count the day's work
// under both, and the move's rename of the summary would then add the two
// together. So the summary's day of an account the day's lines and
// readings no longer name is taken under its new id, as movedOn has it.
func Moving(moving func() func(id string) string) Option {
	return func(o *options) { o.moving = moving }
}

// renamedByMove gives the id a move renames each to, as Moving has the
// ledger ask: nil where none does.
func (l *Ledger) renamedByMove() func(id string) string {
	if l.moving == nil {
		return nil
	}
	return l.moving()
}

// movedOn returns s, a day's summary held, as made, the day summarised
// afresh from its lines and readings, is to know it, where a move renames
// ids as renamed gives them: each account's day s holds under an id the move
// renames that made has no day of, its lines and readings renamed, under the
// new id, two days of one account then merged into one, as mergeRenamed
// merges them. A day made still has, its lines not yet renamed, stays as it
// is, for the move to rename with them. It's s as it is where renamed is nil.
func (s Summary) movedOn(made Summary, renamed func(id string) string) Summary {
	if renamed == nil {
		return s
	}
	var stay, moving []AccountDay
	for _, a := range s.Accounts {
		if to := renamed(a.Account); a.Account != "" && to != a.Account && !made.hasDayOf(a.Account) {
			a.Account = to
			moving = append(moving, a)
			continue
		}
		stay = append(stay, a)
	}
	if moving != nil {
		s.Accounts = mergeRenamed(stay, moving)
	}
	return s
}

// mergeRenamed returns an account's days of a summary, those of stay as
// they are and those of moving renamed onto new ids, in the order of their
// accounts' ids, two of one account merged into one: two of moving as
// mergedDays merges them, as of two ids the account had in two cases, each
// read of readings of its own; and one of moving onto one of stay as
// mergedSameReadings merges them, the move having renamed the readings the
// day under the new id read.
func mergeRenamed(stay, moving []AccountDay) []AccountDay {
	byAccount := func(a AccountDay) string { return a.Account }
	moving = mergedBy(moving, byAccount, mergedDays, strings.Compare)
	return mergedBy(slices.Concat(stay, moving), byAccount, mergedSameReadings, strings.Compare)
}

// hasDayOf reports whether the summary has a day of the account with the
// given id, in any case.
func (s Summary) hasDayOf(id string) bool {
	return slices.ContainsFunc(s.Accounts, func(a AccountDay) bool { return strings.EqualFold(a.Account, id) })
}

// RenameDay renames the local day with the given date: each of its lines as
// line makes it, its files rewritten as dayfile's Rewrite rewrites them,
// then its summary, as rename has it, as renameSummary renames it. It holds
// summarising throughout, so no round summarises the day from lines of one
// form, knowing no less than a summary of the other, which would count its
// work twice, as Moving has a round know where it stops between the two;
// and the day's files only as Rewrite holds them. A summary that doesn't read
// as one is left as it is, for a round to summarise again, as it would.
func (l *Ledger) RenameDay(date string, line func([]byte) []byte, rename Renaming) error {
	l.summarising.Lock()
	defer l.summarising.Unlock()
	rewritten, err := l.days.files.Rewrite(date, line)
	if err != nil {
		return err
	}
	return l.days.renameSummary(date, rename, rewritten)
}

// RenameSummary renames the summary of the local day with the given date
// alone, as RenameDay renames it, its day's files left as they are, as of a
// day whose files read none of their lines. It holds summarising
// throughout, as RenameDay does.
func (l *Ledger) RenameSummary(date string, rename Renaming) error {
	l.summarising.Lock()
	defer l.summarising.Unlock()
	return l.days.renameSummary(date, rename, dayfile.Rewritten{})
}

// Rebuild deletes what the ledger built of its days' lines, their digests,
// parts files, heads and rows files, so each is built afresh from the lines
// as they stand, as once they're rewritten: they'd stand on lines whose count
// hasn't changed, holding what the lines held before. Where the ledger
// digests its days, it holds its digesting's passes off meanwhile, so none
// builds one from lines read before, lets go of what they held of the days,
// and kicks it to build them again.
func (l *Ledger) Rebuild() error {
	if l.digesting == nil {
		return removeBuilt(l.days.files.Dir)
	}
	return l.digesting.rebuild()
}

// rebuild deletes what the digesting built of the days' lines, holding its
// passes off, lets go of what they held of the days, and kicks it.
func (d *digesting) rebuild() error {
	d.passing.Lock()
	err := removeBuilt(d.days.files.Dir)
	d.held, d.judged, d.heads = make(map[string]writtenDay), make(map[string]judgement), make(map[string]seenHead)
	d.warned, d.headsWarned = make(warnings), make(warnings)
	d.passing.Unlock()
	d.kick()
	return err
}

// removeBuilt deletes every file in dir built of a day's lines, as builtName
// names them: none where dir isn't there.
func removeBuilt(dir string) error {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("remove what the request ledger built of its days: %w", err)
	}
	var errs []error
	for _, e := range entries {
		if _, built := builtName(e.Name()); built {
			errs = append(errs, removed(filepath.Join(dir, e.Name())))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("remove what the request ledger built of its days: %w", err)
	}
	return nil
}

// renameSummary renames the summary of the local day with the given date,
// where there's one, as rename has it: each account's id, its accounts' days
// kept in the order of their ids, and each one's work in no pool given the
// pool rename names, as unpooledIn gives it, every other field as it's
// written; and, where its day's files were rewritten as rewritten says, and
// it was marked with them as they were, marks it with them as they are, so
// its lines aren't counted again to tell it stands. It's written only where
// that changed it, stamped as mark stamps one where it's marked with its
// day's files as they now stand. One that doesn't read as a summary is left
// as it is, which is logged. Where two of its accounts' days, renamed, are
// of one account, as of two ids an account had in two cases, or of a round
// that made it from the day's lines under the old ids and its readings under
// the new while they were renamed, they're merged into one, as mergeRenamed
// merges them, so its totals stand whether or not its lines are still there.
func (d days) renameSummary(date string, rename Renaming, rewritten dayfile.Rewritten) error {
	path := SummaryFile(d.files.Dir, date)
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("rename the request ledger's summary of %s: %w", date, err)
	}
	renamed, changed, err := renamedSummary(data, rename, rewritten)
	if err != nil {
		d.logger.Warn("can't rename the request ledger's summary of a day; it's left for a round to summarise again", "day", date, "error", err)
		return nil
	}
	if !changed {
		return nil
	}
	if err := atomicfile.Write(path, append(renamed, '\n'), 0o600); err != nil {
		return fmt.Errorf("rename the request ledger's summary of %s: %w", date, err)
	}
	d.stampMarked(date, renamed)
	return nil
}

// stampMarked stamps the summary of the local day with the given date, as
// written, as mark stamps one, where it's marked with its day's files as
// they now stand, none of them there among it: as of its own version, so a
// look still tells one of an older version than the ledger writes, which is
// summarised again from its lines while they're kept.
func (d days) stampMarked(date string, written []byte) {
	var summary Summary
	if json.Unmarshal(written, &summary) != nil {
		return
	}
	stat, err := d.files.Stat(date)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return
	}
	if marked(summary, stat) {
		d.stamp(date, stat, summary.Version)
	}
}

// renamedSummary returns summary, as it's written, with its accounts' days
// renamed and its bytes marked as renameSummary says, and reports whether
// that changed it, failing where it doesn't read as a summary.
func renamedSummary(summary []byte, rename Renaming, rewritten dayfile.Rewritten) ([]byte, bool, error) {
	fields, err := fieldsOf(summary)
	if err != nil {
		return nil, false, err
	}
	changed := false
	if i := slices.IndexFunc(fields, func(f field) bool { return f.name == "accounts" }); i >= 0 {
		if fields[i].value, changed, err = renamedAccounts(fields[i].value, rename); err != nil {
			return nil, false, err
		}
	}
	renamed := objectOf(fields)
	var marks struct {
		Bytes Bytes `json:"bytes"`
	}
	if err := json.Unmarshal(summary, &marks); err != nil {
		return nil, false, err
	}
	if !rewritten.Changed || marks.Bytes != bytesOf(rewritten.Before) {
		return renamed, changed, nil
	}
	remarked, err := rebytes(renamed, bytesOf(rewritten.After))
	return remarked, true, err
}

// accountDay is an account's day as a summary writes it, the id of its
// account, renamed, and whether the rename gave it that id.
type accountDay struct {
	account string
	written json.RawMessage
	moved   bool
}

// renamedAccounts returns accounts, a summary's accounts' days as they're
// written, each renamed as renamedDay renames it, in the order of their ids,
// as a summary keeps them, two of one account merged into one, as
// mergeRenamed merges them; and reports whether that changed them.
func renamedAccounts(accounts json.RawMessage, rename Renaming) (json.RawMessage, bool, error) {
	var written []json.RawMessage
	if err := json.Unmarshal(accounts, &written); err != nil {
		return nil, false, err
	}
	var stay, moving []accountDay
	changed := false
	for _, w := range written {
		day, renamed, err := renamedDay(w, rename)
		if err != nil {
			return nil, false, err
		}
		changed = changed || renamed
		if day.moved {
			moving = append(moving, day)
		} else {
			stay = append(stay, day)
		}
	}
	if !changed {
		return accounts, false, nil
	}
	moving, err := mergedWrittenBy(moving, mergedDays)
	if err != nil {
		return nil, false, err
	}
	days, err := mergedWrittenBy(slices.Concat(stay, moving), mergedSameReadings)
	if err != nil {
		return nil, false, err
	}
	renamed := make([]json.RawMessage, len(days))
	for i, day := range days {
		renamed[i] = day.written
	}
	out, err := json.Marshal(renamed)
	return out, true, err
}

// mergedWrittenBy returns days in the order of their accounts' ids, two of
// one account merged into one, as mergedWritten merges them by merge.
func mergedWrittenBy(days []accountDay, merge func(a, b AccountDay) AccountDay) ([]accountDay, error) {
	slices.SortStableFunc(days, func(a, b accountDay) int { return cmp.Compare(a.account, b.account) })
	var merged []accountDay
	for _, day := range days {
		last := len(merged) - 1
		if last < 0 || merged[last].account != day.account {
			merged = append(merged, day)
			continue
		}
		written, err := mergedWritten(merged[last].written, day.written, merge)
		if err != nil {
			return nil, err
		}
		merged[last].written = written
	}
	return merged, nil
}

// renamedDay returns an account's day as a summary writes it, renamed as
// rename has it: its account, as renamedAccount renames it, then its pools,
// as pooledDay gives them theirs; and reports whether that changed it.
func renamedDay(written json.RawMessage, rename Renaming) (accountDay, bool, error) {
	id := rename.Account
	if id == nil {
		id = func(id string) string { return id }
	}
	day, renamed, err := renamedAccount(written, id)
	pool := rename.pooling()
	if err != nil || pool == nil {
		return day, renamed, err
	}
	day, pooled, err := pooledDay(day, pool)
	return day, renamed || pooled, err
}

// pooledDay returns an account's day as a summary writes it, each pool's
// work in the pool pool gives its id, as repooled gives it, and reports
// whether that changed it: written anew, where it did, as an AccountDay
// marshals, so a field it doesn't know of, as a later version wrote it,
// isn't kept.
func pooledDay(day accountDay, pool func(id string) string) (accountDay, bool, error) {
	var a AccountDay
	if err := json.Unmarshal(day.written, &a); err != nil {
		return accountDay{}, false, err
	}
	pooled, changed := a.repooled(pool)
	if !changed {
		return day, false, nil
	}
	written, err := json.Marshal(pooled)
	day.written = written
	return day, true, err
}

// renamedAccount returns an account's day as a summary writes it, its
// account renamed as id gives it, and reports whether that changed it. One
// of no account, the requests the router answered itself, has none to
// rename.
func renamedAccount(written json.RawMessage, id func(string) string) (accountDay, bool, error) {
	fields, err := fieldsOf(written)
	if err != nil {
		return accountDay{}, false, err
	}
	i := slices.IndexFunc(fields, func(f field) bool { return f.name == "account" })
	if i < 0 {
		return accountDay{written: written}, false, nil
	}
	var was string
	if err := json.Unmarshal(fields[i].value, &was); err != nil {
		return accountDay{}, false, err
	}
	now := id(was)
	if now == was {
		return accountDay{account: was, written: written}, false, nil
	}
	// A string always marshals.
	fields[i].value, _ = json.Marshal(now)
	return accountDay{account: now, written: objectOf(fields), moved: true}, true, nil
}
