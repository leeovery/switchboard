// Package idmove moves switchboard's accounts and their history onto the ids
// it makes, once, as the router first starts on a build that makes them:
// each configured account takes the id its organization derives, its token
// file and its token's record that id's names, and the config its id, and its
// old id as its label where it had none; each account the history holds that
// the config doesn't goes into the accounts record, removed; and every file
// that names an account is written anew under the new ids, and what the
// router builds of them built again.
//
// What's decided goes into the accounts record before anything else
// changes, each account's old id its was, so a stop anywhere carries on to
// the same end the next time: and a new id is never an old one, so a file
// moved reads the same moved again.
package idmove

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/tokens"
)

var logger = logs.For("idmove")

const (
	// markName is the name, in the state directory, of the file that marks
	// the move done, holding when it was.
	markName = "accounts.moved"
	// stateName is the router's state file's name in the state directory.
	stateName = "state.json"
	// firstRetry is how long the move waits to ask the API again where it
	// didn't answer for a token, the wait doubling each time to mostRetry.
	firstRetry = 30 * time.Second
	mostRetry  = 10 * time.Minute
	// configTries is how many times the move edits the config afresh where
	// it changed as it was edited.
	configTries = 5
)

// ErrAwaitingRestart is what a move run beside a router on the accounts' old
// ids returns once the config holds their new ids: the rest of it, which
// would take the old ids' files from under that router, waits for it to
// restart onto the new ones, as a supervised router does once its config
// changes.
var ErrAwaitingRestart = errors.New("the config holds the accounts' new ids, so the rest of the move waits for the router to restart onto them")

// errUnanswered is what an attempt at the move fails with where the API
// didn't answer for a token whose organization decides an id.
var errUnanswered = errors.New("the API didn't answer for every token whose organization decides an id")

// Organizations tells which organization a token is of, as the API's answer
// to a request made with it names it: "" where it names none. A failure
// whose Refused method reports true is the API refusing the token, an answer
// that names none; any other is the API not answering, which is asked again.
type Organizations interface {
	Organization(ctx context.Context, token tokens.Token) (string, error)
}

// History is the history's files the move writes anew: the request ledger,
// as a router keeps it, and the readings history's and the router's events'
// files. A router hands its own, so each rewrite holds off its writers as
// they hold one another off.
type History struct {
	Ledger   *ledger.Ledger
	Readings *dayfile.Files
	Events   *dayfile.Files
}

// Mover moves the accounts and their history onto the ids switchboard makes,
// as Run does.
type Mover struct {
	// ConfigPath is the config file, StateDir the state directory, and Tokens
	// the token files.
	ConfigPath string
	StateDir   string
	Tokens     tokens.Store
	// Organizations tells the organization of each account's token.
	Organizations Organizations
	// Routing are the ids of the accounts the router running beside the move
	// routes under, which keeps the state file and appends to the history
	// meanwhile: nil where none does, as for a copy of a state directory.
	Routing []string
	// History is the history's files as the router running beside the move
	// keeps them: nil opens them afresh, where none does.
	History *History
	// Now reads the clock, and Random is what random ids are drawn from:
	// nil reads the system's, and draws from crypto/rand.
	Now    func() time.Time
	Random io.Reader
	// Retry is how long the move first waits to ask the API again where it
	// didn't answer: zero is firstRetry.
	Retry time.Duration

	// stop, where it's set, is called with each step's name once it's done:
	// a failure stops the move there, as a test of one stopped there has it.
	stop func(step string) error
	// seen is the accounts the history holds, as the first attempt of the
	// mover found them, kept for those after it.
	seen map[string]seenDays
}

// Moved reports whether the move is done in the state directory given, as
// its mark says.
func Moved(stateDir string) bool {
	_, err := os.Lstat(filepath.Join(stateDir, markName))
	return err == nil
}

// Pending reports whether the history in the state directory given waits on
// the move: it isn't done, and there's a history to move, the request
// ledger's directory, the readings history's or the router's state file.
func Pending(stateDir string) bool {
	if Moved(stateDir) {
		return false
	}
	for _, path := range []string{ledger.Dir(stateDir), readings.Dir(stateDir), filepath.Join(stateDir, stateName)} {
		if _, err := os.Lstat(path); err == nil {
			return true
		}
	}
	return false
}

// Run moves the accounts and their history onto the ids switchboard makes,
// as the package says, unless it's done, and marks it done: deciding each
// account's id and writing it to the accounts record; giving each moved
// account's token file its new id's name beside its old, as a link to it;
// writing the config's ids and labels; where a router routes under the old
// ids, returning ErrAwaitingRestart for it to restart onto the new; then
// moving the state file, where no router keeps it, moving the token files
// and their records onto the new ids' names, writing the history anew, and having the request ledger
// build what it builds of it again. Where the API doesn't answer for a token
// whose organization decides an id, it waits and asks again, from the
// start, until ctx ends, changing nothing meanwhile, so no passing outage
// gives an account a random id. It fails, changing no more, where a step
// does, which a Run after carries on from.
func (m *Mover) Run(ctx context.Context) error {
	wait := cmp.Or(m.Retry, firstRetry)
	for {
		err := m.attempt(ctx)
		if !errors.Is(err, errUnanswered) {
			return err
		}
		logger.Info("the move onto the accounts' ids waits to ask the API again for the organizations of their tokens", "in", wait)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		wait = min(wait*2, max(mostRetry, wait))
	}
}

// RunBeside runs the move as Run does, beside a router that keeps the
// history's files as history has them.
func (m *Mover) RunBeside(ctx context.Context, history History) error {
	m.History = &history
	return m.Run(ctx)
}

// step is a step of the move, by its name.
type step struct {
	name string
	run  func() error
}

// attempt makes the move once, step by step, as Run says.
func (m *Mover) attempt(ctx context.Context) error {
	if Moved(m.StateDir) {
		return nil
	}
	steps := []step{
		{"decide", func() error { return m.decide(ctx) }},
		{"give", m.give},
		{"config", m.configure},
		{"restart", m.awaitRestart},
		{"state", m.moveState},
		{"drop", m.drop},
		{"history", func() error { return m.moveHistory(ctx) }},
		{"rebuild", m.rebuild},
		{"mark", m.mark},
	}
	for _, s := range steps {
		if err := s.run(); err != nil {
			return err
		}
		if err := m.stopped(s.name); err != nil {
			return err
		}
	}
	return nil
}

// stopped fails where the move is to stop once the step with the given name
// is done, as stop says.
func (m *Mover) stopped(name string) error {
	if m.stop == nil {
		return nil
	}
	return m.stop(name)
}

// Settle moves the files a router takes up on its accounts' ids as it
// starts, before it does: the state file, and the readings history's newest
// two days, which it takes the recent rates up from, each account under the
// id the accounts record has for the one it had, once the config no longer
// configures that one. Nothing else runs on them then, so nothing's held. It
// does nothing once the move is done.
func (m *Mover) Settle() error {
	if Moved(m.StateDir) {
		return nil
	}
	rename, err := m.renames()
	if err != nil || rename.none() {
		return err
	}
	if err := m.renameState(rename); err != nil {
		return err
	}
	files := readings.Files(readings.Dir(m.StateDir), logger)
	for _, date := range files.Newest(m.now(), 2) {
		if _, err := files.Rewrite(date, renamingLines(readingNames, rename.of)); err != nil {
			return err
		}
	}
	return nil
}

// standing reads the config's accounts, as the config file stands, and the
// accounts record.
func (m *Mover) standing() (*config.Draft, identity.Record, error) {
	draft, err := config.Edit(m.ConfigPath)
	if err != nil {
		return nil, identity.Record{}, err
	}
	record, err := m.record().Read()
	return draft, record, err
}

// record is the accounts record's file.
func (m *Mover) record() identity.File {
	return identity.NewFile(m.StateDir)
}

// now reads the mover's clock, else the system's.
func (m *Mover) now() time.Time {
	if m.Now == nil {
		return time.Now()
	}
	return m.Now()
}

// random is what random ids are drawn from: the mover's, else crypto/rand's.
func (m *Mover) random() io.Reader {
	if m.Random == nil {
		return rand.Reader
	}
	return m.Random
}

// give gives each account moved its token file by its new id, as
// tokens.Store's Give does, beside its old: while the config still
// configures its old id, so a router on either finds its token; or where its
// new id names no token file, as of an account another Mac moved, whose
// config this one shares.
func (m *Mover) give() error {
	draft, record, err := m.standing()
	if err != nil {
		return err
	}
	for _, e := range moved(record) {
		if !configures(draft, e.Was) && named(m.Tokens.Path(e.ID)) {
			continue
		}
		if err := m.Tokens.Give(e.Was, e.ID); err != nil {
			return err
		}
	}
	return nil
}

// drop moves the token file and record of each account moved that the
// config configures by its new id from its old id's names onto its new id's,
// as tokens.Store's Drop does.
func (m *Mover) drop() error {
	draft, record, err := m.standing()
	if err != nil {
		return err
	}
	for _, e := range moved(record) {
		if !configures(draft, e.ID) {
			continue
		}
		if err := m.Tokens.Drop(e.Was, e.ID); err != nil {
			return err
		}
	}
	return nil
}

// configure writes each moved account's new id in the config, and its old id
// as its label, where it has none, as one edit, made afresh where the config
// changed as it was made, configTries times at most.
func (m *Mover) configure() error {
	for range configTries {
		draft, record, err := m.standing()
		if err != nil {
			return err
		}
		changed, err := renameConfig(draft, record)
		if err != nil || !changed {
			return err
		}
		err = draft.Save()
		if err == nil {
			logger.Info("wrote the accounts' new ids in the config", "path", m.ConfigPath)
		}
		if !errors.Is(err, config.ErrChanged) {
			return err
		}
	}
	return fmt.Errorf("write the accounts' new ids in the config: %w", config.ErrChanged)
}

// renameConfig gives each account draft configures that the record has moved
// its new id, and its old id as its label where draft writes none, reporting
// whether it gave any.
func renameConfig(draft *config.Draft, record identity.Record) (bool, error) {
	if draft.Config() == nil {
		return false, nil
	}
	changed := false
	for _, a := range draft.Config().Accounts {
		e, ok := record.ByWas(a.ID)
		if !ok || !e.Removed.IsZero() {
			continue
		}
		labelled := draft.Labelled(a.ID)
		if err := draft.SetID(a.ID, e.ID); err != nil {
			return false, err
		}
		if !labelled {
			if err := draft.SetLabel(e.ID, a.ID); err != nil {
				return false, err
			}
		}
		changed = true
	}
	return changed, nil
}

// awaitRestart fails with ErrAwaitingRestart where the router running beside
// the move routes under an old id the record has moved.
func (m *Mover) awaitRestart() error {
	if m.Routing == nil {
		return nil
	}
	record, err := m.record().Read()
	if err != nil {
		return err
	}
	for _, id := range m.Routing {
		if e, ok := record.ByWas(id); ok && e.Removed.IsZero() {
			return ErrAwaitingRestart
		}
	}
	return nil
}

// moveState moves the router's state file, where no router keeps it: one
// that does has its moved as it starts, as Settle moves it, and writes it
// whole after.
func (m *Mover) moveState() error {
	if m.Routing != nil {
		return nil
	}
	rename, err := m.renames()
	if err != nil {
		return err
	}
	return m.renameState(rename)
}

// renameState writes the router's state file anew, each account it names
// renamed as rename has it, where that changes it.
func (m *Mover) renameState(rename renames) error {
	path := filepath.Join(m.StateDir, stateName)
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("move the router's state file: %w", err)
	}
	renamed, changed := stateNames.renamed(data, rename.of)
	if !changed {
		return nil
	}
	if err := atomicfile.Write(path, renamed, 0o600); err != nil {
		return fmt.Errorf("move the router's state file: %w", err)
	}
	return nil
}

// moveHistory writes the history anew, each account it names renamed as the
// record has it: the readings history first, which the request ledger's
// summaries read, then the router's events, then the ledger's days, a day's
// lines with its summary. It stops, failing, once ctx ends.
func (m *Mover) moveHistory(ctx context.Context) error {
	rename, err := m.renames()
	if err != nil {
		return err
	}
	return m.renameHistory(ctx, m.history(), rename)
}

// renameHistory writes h anew, each account it names renamed as rename has
// it, as moveHistory says.
func (m *Mover) renameHistory(ctx context.Context, h History, rename renames) error {
	for _, kind := range []struct {
		files *dayfile.Files
		names names
	}{{h.Readings, readingNames}, {h.Events, eventNames}} {
		if err := m.rewriteAll(ctx, kind.files, kind.names, rename); err != nil {
			return err
		}
	}
	dates, err := ledgerDates(h.Ledger)
	if err != nil {
		return err
	}
	for _, date := range dates {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := h.Ledger.RenameDay(date, renamingLines(lineNames, rename.of), rename.of); err != nil {
			return err
		}
		if err := m.stopped(h.Ledger.Files().Name + " " + date); err != nil {
			return err
		}
	}
	return nil
}

// rewriteAll writes each day's files anew, each account named renamed, as n
// says where they name them. It stops, failing, once ctx ends.
func (m *Mover) rewriteAll(ctx context.Context, files *dayfile.Files, n names, rename renames) error {
	dates, err := files.All()
	if err != nil {
		return fmt.Errorf("move the %s: %w", files.Name, err)
	}
	for _, date := range dates {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := files.Rewrite(date, renamingLines(n, rename.of)); err != nil {
			return err
		}
		if err := m.stopped(files.Name + " " + date); err != nil {
			return err
		}
	}
	return nil
}

// ledgerDates are the dates of the days the ledger holds lines or a summary
// of, oldest first, each once.
func ledgerDates(l *ledger.Ledger) ([]string, error) {
	lined, err := l.Files().All()
	if err != nil {
		return nil, fmt.Errorf("move the request ledger: %w", err)
	}
	summarised, err := ledger.SummaryDates(l.Files().Dir)
	if err != nil {
		return nil, fmt.Errorf("move the request ledger: %w", err)
	}
	return sortedOnce(append(lined, summarised...)), nil
}

// renamingLines renames each account a line names, as n says where, as
// rename gives its id.
func renamingLines(n names, rename func(string) string) func([]byte) []byte {
	return func(line []byte) []byte {
		renamed, _ := n.renamed(line, rename)
		return renamed
	}
}

// rebuild has the request ledger build what it builds of its days again,
// from their lines as they now stand.
func (m *Mover) rebuild() error {
	return m.history().Ledger.Rebuild()
}

// mark marks the move done, as Moved reads it.
func (m *Mover) mark() error {
	at := m.now().UTC().Format(time.RFC3339)
	if err := atomicfile.Write(filepath.Join(m.StateDir, markName), []byte(at+"\n"), 0o600); err != nil {
		return fmt.Errorf("mark the move onto the accounts' ids done: %w", err)
	}
	logger.Info("moved the accounts and their history onto their ids")
	return nil
}

// history is the history's files the move writes anew: the router's, where
// it's running, else opened afresh.
func (m *Mover) history() History {
	if m.History != nil {
		return *m.History
	}
	files := readings.Files(readings.Dir(m.StateDir), logger)
	return History{
		Ledger:   ledger.Open(ledger.Dir(m.StateDir), dayfile.Forever, m.now, ledger.HistoryIn(files), ledger.Caps{}, logger),
		Readings: files,
		Events:   events.Files(ledger.Dir(m.StateDir), logger),
	}
}

// renames are each old id the record has moved, in lower case, and its new
// id.
type renames map[string]string

// of renames id as the record has it: as it is, where it hasn't moved it.
func (r renames) of(id string) string {
	if to, ok := r[strings.ToLower(id)]; ok {
		return to
	}
	return id
}

// none reports whether there's nothing to rename.
func (r renames) none() bool {
	return len(r) == 0
}

// renames are the ids the record has moved that the config no longer
// configures, so files named by them can be renamed: each account moved by
// the old id it had, and each removed before ids were made.
func (m *Mover) renames() (renames, error) {
	draft, record, err := m.standing()
	if err != nil {
		return nil, err
	}
	r := make(renames)
	for _, e := range record.Accounts {
		if e.Was != "" && !configures(draft, e.Was) {
			r[strings.ToLower(e.Was)] = e.ID
		}
	}
	return r, nil
}

// moved are the accounts the record has moved onto ids that are configured,
// as the record has them: each with the id it had before.
func moved(record identity.Record) []identity.Entry {
	var found []identity.Entry
	for _, e := range record.Accounts {
		if e.Was != "" && e.Removed.IsZero() {
			found = append(found, e)
		}
	}
	return found
}

// configures reports whether draft configures an account with the given id,
// in any case, as the token files' names ignore it.
func configures(draft *config.Draft, id string) bool {
	if draft.Config() == nil {
		return false
	}
	_, found := findFold(draft.Config().Accounts.IDs(), id)
	return found
}

// named reports whether anything's at path, not followed where it's a link.
func named(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
