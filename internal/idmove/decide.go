package idmove

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/redact"
)

// takenSuffix ends the name of the record beside a token file of when its
// token was taken.
const takenSuffix = ".taken"

// decide decides the id of each account the record doesn't know yet, as
// deciding does, and writes it to the record: each configured account's, its
// token's organization asked first, and each the history holds that the
// config doesn't. Where the API doesn't answer for a token asked, it fails
// with errUnanswered, writing nothing. With nothing to decide, it writes
// nothing.
func (m *Mover) decide(ctx context.Context) error {
	draft, record, err := m.standing()
	if err != nil {
		return err
	}
	seen, err := m.historyAccounts()
	if err != nil {
		return err
	}
	held, err := m.tokenFiles()
	if err != nil {
		return err
	}
	d := deciding{configured: configuredIn(draft), seen: seen, tokened: held, now: m.now(), random: m.random()}
	d.record = &record
	if !d.undecided() {
		return nil
	}
	answers, err := m.ask(ctx, d.asks())
	if err != nil {
		return err
	}
	return m.record().Update(func(r *identity.Record) error {
		d.record, d.answers = r, answers
		return d.decide()
	})
}

// configuredIn are the ids of the accounts draft configures, in its order.
func configuredIn(draft *config.Draft) []string {
	if draft.Config() == nil {
		return nil
	}
	return draft.Config().Accounts.IDs()
}

// answer is what the API answered for a token: the organization it named, ""
// for none; or that it didn't answer.
type answer struct {
	organization string
	unanswered   bool
}

// refusal is a failure that says whether the API refused a token.
type refusal interface {
	error
	Refused() bool
}

// ask asks the organization of the token of each account with the given ids,
// by its id. It fails with errUnanswered where the API didn't answer for any
// of them, or a token file can't be read for now, and with ctx's error once
// ctx has ended.
func (m *Mover) ask(ctx context.Context, ids []string) (map[string]answer, error) {
	answers := make(map[string]answer, len(ids))
	unanswered := false
	for _, id := range ids {
		a, err := m.answerFor(ctx, id)
		if err != nil {
			return nil, err
		}
		answers[id] = a
		unanswered = unanswered || a.unanswered
	}
	if unanswered {
		return nil, errUnanswered
	}
	return answers, nil
}

// answerFor asks the organization of the token of the account with the given
// id, failing once ctx has ended. A token file that can't be read, as one
// others can read, is as the API not answering: it's put right, and asked
// again then.
func (m *Mover) answerFor(ctx context.Context, id string) (answer, error) {
	token, err := m.Tokens.Read(id)
	if err != nil {
		logger.Warn("can't read an account's token to move it onto its id; the move waits for it to be put right", "account", id, "error", err)
		return answer{unanswered: true}, nil
	}
	organization, err := m.Organizations.Organization(ctx, token)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return answer{}, ctxErr
	}
	if refused, ok := errors.AsType[refusal](err); err != nil && (!ok || !refused.Refused()) {
		logger.Info("the API didn't answer for an account's token, which the id it moves onto waits on", "account", id, "error", redact.Text(err.Error(), token.Reveal()))
		return answer{unanswered: true}, nil
	}
	return answer{organization: organization}, nil
}

// tokenFiles are the ids, in lower case, named by the token files in the
// tokens directory, files or links, but the records beside them and anything
// hidden: none where there's no directory.
func (m *Mover) tokenFiles() (map[string]string, error) {
	entries, err := os.ReadDir(m.Tokens.Dir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	held := make(map[string]string)
	for _, e := range entries {
		name := e.Name()
		fileOrLink := e.Type().IsRegular() || e.Type()&os.ModeSymlink != 0
		if fileOrLink && !strings.HasPrefix(name, ".") && !strings.HasSuffix(name, takenSuffix) && config.CheckID(name) == nil {
			held[strings.ToLower(name)] = name
		}
	}
	return held, nil
}

// seenDays are the dates of the first and last days the history tells of an
// account on, as its files' names give them: "" where none does, as the
// router's state file alone names it.
type seenDays struct {
	first, last string
}

// on is s, the account seen on the day with the given date too: none for
// "".
func (s seenDays) on(date string) seenDays {
	if date == "" {
		return s
	}
	if s.first == "" || date < s.first {
		s.first = date
	}
	s.last = max(s.last, date)
	return s
}

// historyAccounts are the accounts the history names, by their ids, and the
// days it tells of each on: the readings history's lines, the router's
// events', the request ledger's and its summaries, and the router's state
// file. They're read once, the first time the mover asks, and kept for the
// attempts after it.
func (m *Mover) historyAccounts() (map[string]seenDays, error) {
	if m.seen != nil {
		return m.seen, nil
	}
	h := m.history()
	seen := make(map[string]seenDays)
	note := func(date string) func(string) {
		return func(id string) { seen[id] = seen[id].on(date) }
	}
	for _, kind := range []struct {
		files *dayfile.Files
		names names
	}{{h.Readings, readingNames}, {h.Events, eventNames}, {h.Ledger.Files(), lineNames}} {
		if err := visitAll(kind.files, kind.names, note); err != nil {
			return nil, err
		}
	}
	if err := visitSummaries(h.Ledger.Files().Dir, note); err != nil {
		return nil, err
	}
	state, err := os.ReadFile(filepath.Join(m.StateDir, stateName))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	stateNames.visit(state, note(""))
	m.seen = seen
	return seen, nil
}

// visitAll hands note, by each day's date, each account each day's files
// name, as n says where.
func visitAll(files *dayfile.Files, n names, note func(date string) func(string)) error {
	dates, err := files.All()
	if err != nil {
		return err
	}
	for _, date := range dates {
		seen := note(date)
		_, _, err := dayfile.ReadDay(files, date, func(line []byte) (struct{}, bool) {
			n.visit(line, seen)
			return struct{}{}, true
		}, func(struct{}) bool { return true })
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// visitSummaries hands note, by each day's date, each account the request
// ledger's summary of the day in dir names.
func visitSummaries(dir string, note func(date string) func(string)) error {
	dates, err := ledger.SummaryDates(dir)
	if err != nil {
		return err
	}
	for _, date := range dates {
		data, err := os.ReadFile(ledger.SummaryFile(dir, date))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		summaryNames.visit(data, note(date))
	}
	return nil
}

// deciding decides the id of each account the record doesn't know: a
// configured account that isn't one of its accounts, by id or by the id it
// had before, and an account the history names that's neither configured
// nor known. A configured account with a token file takes the id its
// organization derives, or the record's for it, as idOf has it; one without
// takes the id of a token file of no configured account whose organization
// spells its id, as of an account another Mac moved, whose config this one
// shares, as join has it, else keeps its id where switchboard makes one like
// it and the history names none of it, as one never used, else a random id.
// Each the history names alone is an account removed before ids were made,
// under a random id. No id decided is held already, as an old id, a
// configured one, a token file's, or one decided before it.
type deciding struct {
	record *identity.Record
	// configured are the ids of the accounts configured, in the config's
	// order.
	configured []string
	// answers are the organizations of the tokens asked, by account id.
	answers map[string]answer
	// seen are the accounts the history names, by id.
	seen map[string]seenDays
	// tokened are the ids the token files are named by, in lower case.
	tokened map[string]string
	now     time.Time
	random  io.Reader
	// decided are the ids decided, in lower case.
	decided map[string]bool
}

// undecided reports whether there's an account to decide the id of.
func (d *deciding) undecided() bool {
	return slices.ContainsFunc(d.configured, func(id string) bool { return !d.known(id) }) || len(d.removedBefore()) > 0
}

// asks are the ids of the accounts whose tokens' organizations decide them:
// each configured account to decide with a token file; and, where a
// configured account to decide has none, each token file of an account
// that's neither configured nor known, which may be it, as join says.
func (d *deciding) asks() []string {
	var asks []string
	tokenless := false
	for _, id := range d.configured {
		switch {
		case d.known(id):
		case d.hasToken(id):
			asks = append(asks, id)
		default:
			tokenless = true
		}
	}
	if tokenless {
		asks = append(asks, d.strays()...)
	}
	return asks
}

// strays are the names of the token files of accounts that are neither
// configured nor known, in order.
func (d *deciding) strays() []string {
	var strays []string
	for _, name := range d.tokened {
		if !d.isConfigured(name) && !d.known(name) {
			strays = append(strays, name)
		}
	}
	slices.Sort(strays)
	return strays
}

// decide decides each account the record doesn't know, as deciding says, in
// the record: configured ones in the config's order, then those the history
// names alone, in the order of their ids. It fails with errUnanswered where
// an answer it needs is missing.
func (d *deciding) decide() error {
	d.decided = make(map[string]bool)
	var tokenless []string
	for _, id := range d.configured {
		switch {
		case d.known(id):
		case d.hasToken(id):
			if err := d.byToken(id); err != nil {
				return err
			}
		default:
			tokenless = append(tokenless, id)
		}
	}
	joined, err := d.join(tokenless)
	if err != nil {
		return err
	}
	for _, id := range tokenless {
		if !joined[id] {
			if err := d.withoutToken(id); err != nil {
				return err
			}
		}
	}
	for _, was := range d.removedBefore() {
		if err := d.removed(was); err != nil {
			return err
		}
	}
	return nil
}

// byToken decides the id of the configured account with the given id by its
// token's organization, as idOf does.
func (d *deciding) byToken(was string) error {
	a, ok := d.answers[was]
	if !ok {
		return errUnanswered
	}
	id, organization, returning, err := d.idOf(a.organization, was)
	if err != nil {
		return err
	}
	if returning {
		d.record.Return(id, organization)
		d.record.Former(id, was)
	} else {
		d.record.Move(id, organization, was, d.now)
	}
	d.decide1(id)
	logger.Info("an account moves onto its id", "was", was, "id", id, "organization_known", organization != "")
	return nil
}

// idOf decides the id of the account with the given old id, was, whose
// token's organization is organization, "" for none: the record's account
// of it, which comes back, where it has one that's neither configured nor
// decided, nor had another id before; else, where neither has it, the id it
// derives. Where the answer named none, or another account configured or
// decided has it, as two accounts of one subscription have, it's a random
// id, its organization unknown.
func (d *deciding) idOf(organization, was string) (id, known string, returning bool, err error) {
	if organization == "" {
		id, err := d.randomID()
		return id, "", false, err
	}
	e, ok := d.record.ByOrganization(organization)
	switch {
	case !ok:
		return d.record.Derive(organization, d.held()), organization, false, nil
	case !d.isConfigured(e.ID) && !d.decided[strings.ToLower(e.ID)] && (e.Was == "" || strings.EqualFold(e.Was, was)):
		return e.ID, organization, true, nil
	}
	logger.Warn("an account's token is of another account's subscription, so it moves onto a random id", "was", was, "other", e.ID)
	id, err = d.randomID()
	return id, "", false, err
}

// join decides the ids of those of the configured accounts given, which have
// no token file, that a token file of an account neither configured nor
// known is of: one whose organization spells the configured account's id,
// as Derive spells one, and that the record has no account of. The token
// file's account moves onto the configured account's id. It reports which it
// joined, by id, and fails with errUnanswered where an answer it needs is
// missing.
func (d *deciding) join(tokenless []string) (map[string]bool, error) {
	joined := make(map[string]bool)
	for _, stray := range d.strays() {
		a, ok := d.answers[stray]
		if !ok {
			return nil, errUnanswered
		}
		if _, held := d.record.ByOrganization(a.organization); a.organization == "" || held {
			continue
		}
		i := slices.IndexFunc(tokenless, func(id string) bool { return !joined[id] && identity.Spells(a.organization, id) })
		if i < 0 {
			continue
		}
		d.record.Move(tokenless[i], a.organization, stray, d.now)
		d.decide1(tokenless[i])
		joined[tokenless[i]] = true
		logger.Info("an account another Mac moved takes its token file here", "was", stray, "id", tokenless[i])
	}
	return joined, nil
}

// withoutToken decides the id of the configured account with the given id,
// which has no token file: its own, where switchboard makes one like it and
// the history names none of it, else a random one.
func (d *deciding) withoutToken(was string) error {
	if _, named := d.seen[was]; identity.WellFormed(was) && !named {
		d.record.Add(was, "", d.now)
		d.decide1(was)
		return nil
	}
	id, err := d.randomID()
	if err != nil {
		return err
	}
	d.record.Move(id, "", was, d.now)
	d.decide1(id)
	logger.Info("an account without a token file moves onto a random id", "was", was, "id", id)
	return nil
}

// removedBefore are the ids the history names that are neither configured
// nor known, nor ids no account can have, in order: accounts removed before
// ids were made.
func (d *deciding) removedBefore() []string {
	var removed []string
	for id := range d.seen {
		if config.CheckID(id) == nil && !d.isConfigured(id) && !d.known(id) {
			removed = append(removed, id)
		}
	}
	slices.Sort(removed)
	return removed
}

// removed adds the account removed before ids were made that had the id
// was to the record, under a random id, as removed when the history last
// tells of it, added when it first does: at the start of its first day, and
// the end of its last, or now where no day tells of it, never after now.
func (d *deciding) removed(was string) error {
	id, err := d.randomID()
	if err != nil {
		return err
	}
	added, removed := d.now, d.now
	seen := d.seen[was]
	if start, _, ok := dayfile.Day(seen.first); ok {
		added = start
	}
	if _, end, ok := dayfile.Day(seen.last); ok && end.Before(d.now) {
		removed = end
	}
	d.record.RemovedBefore(id, was, added, removed)
	d.decide1(id)
	logger.Info("an account removed before ids were made goes into the accounts record", "was", was, "id", id)
	return nil
}

// randomID draws a random id held by none, as held says.
func (d *deciding) randomID() (string, error) {
	return d.record.Random(d.random, d.held())
}

// decide1 notes id as decided.
func (d *deciding) decide1(id string) {
	d.decided[strings.ToLower(id)] = true
}

// held are the ids no account decided can have, but for those the record
// holds, which it knows: every configured id, every id the history names,
// every token file's, and every id decided.
func (d *deciding) held() []string {
	held := slices.Clone(d.configured)
	for id := range d.seen {
		held = append(held, id)
	}
	for _, name := range d.tokened {
		held = append(held, name)
	}
	for id := range d.decided {
		held = append(held, id)
	}
	return held
}

// known reports whether the record knows the account with the given id: as
// one of its ids, or one an account had before.
func (d *deciding) known(id string) bool {
	_, byID := d.record.ByID(id)
	_, byWas := d.record.ByWas(id)
	return byID || byWas
}

// isConfigured reports whether an account with the given id is configured,
// in any case.
func (d *deciding) isConfigured(id string) bool {
	_, found := findFold(d.configured, id)
	return found
}

// hasToken reports whether the account with the given id has a token file.
func (d *deciding) hasToken(id string) bool {
	_, ok := d.tokened[strings.ToLower(id)]
	return ok
}

// findFold returns the one of ids that's id, in any case, reporting false
// where none is.
func findFold(ids []string, id string) (string, bool) {
	i := slices.IndexFunc(ids, func(held string) bool { return strings.EqualFold(held, id) })
	if i < 0 {
		return "", false
	}
	return ids[i], true
}

// sortedOnce returns dates sorted, each once.
func sortedOnce(dates []string) []string {
	slices.Sort(dates)
	return slices.Compact(dates)
}
