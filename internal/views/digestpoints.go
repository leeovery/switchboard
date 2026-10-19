package views

import (
	"fmt"
	"maps"
	"slices"
	"unique"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// storedPointing is a page's points as a day's digest stores them, sealed at
// the end of its day, as pointing's sealedAt seals them: the dates of the
// days the session made a request on, oldest first; its shares of the runs
// before the last, done, by account, then by window key; its last run, as
// storedRun holds it, none once it's done; the answers it holds of each of
// those days, in the order it took them in, as storedAnswer holds each; and
// how many of each day's answers it let go of. Not the answers it holds of a
// day the session made no request on, every account's of the day: they'd
// count only once a line of the session filed on the day made it one of its
// days, and that line stops the state standing. Nor its points, worked out
// again as it's restored. Its keys are short and its zeros left out.
type storedPointing struct {
	Days   []string                      `json:"d,omitempty"`
	Done   map[string]map[string]float64 `json:"dn,omitempty"`
	Run    *storedRun                    `json:"r,omitempty"`
	Held   map[string][]storedAnswer     `json:"h,omitempty"`
	Folded map[string]int                `json:"f,omitempty"`
}

// storedRun is the last run of a page's points' days as their stored form
// holds it: its first and last days; how far its answers are taken in; and
// its accounts' windows' shares, by account, as storedAccount holds each.
type storedRun struct {
	First    string                   `json:"f,omitempty"`
	Last     string                   `json:"l,omitempty"`
	Through  storedInstant            `json:"t,omitzero"`
	Accounts map[string]storedAccount `json:"a,omitempty"`
}

// storedAccount is an account's windows over a run as their shares' stored
// form holds them: each by its key, as storedShare holds it; and the models
// of the session's own requests among its requests, in order.
type storedAccount struct {
	Windows map[string]storedShare `json:"w,omitempty"`
	Ours    []string               `json:"o,omitempty"`
}

// storedShare is a window as its share's stored form holds it: how its use
// climbs, in ledger.Climb's own stored form; whether every model shares it,
// else the models whose answers read it, in order; the tokens of the
// requests ended since its last reading took a climb; and the session's
// share of its rises so far.
type storedShare struct {
	Climb   *ledger.Climb `json:"c,omitempty"`
	Shared  bool          `json:"s,omitzero"`
	Models  []string      `json:"m,omitempty"`
	Pending storedEnded   `json:"p,omitzero"`
	Share   float64       `json:"sh,omitzero"`
}

// storedEnded are the tokens of requests ended as a window's stored share
// holds them: every one's, and the session's own.
type storedEnded struct {
	All  int64 `json:"a,omitzero"`
	Ours int64 `json:"o,omitzero"`
}

// storedAnswer is an answer a page's points hold, as their stored form holds
// it: its flight; when its answer's headers came; its account and model; its
// tokens; whether it's the session's own; and the windows its answer read,
// in claude.KeptWindow's own stored form. Not its sum, worked out again as
// it's restored.
type storedAnswer struct {
	storedFlight
	Read    storedInstant       `json:"rd,omitzero"`
	Account string              `json:"ac,omitempty"`
	Model   string              `json:"m,omitempty"`
	Tokens  int64               `json:"tk,omitzero"`
	Ours    bool                `json:"o,omitzero"`
	Windows []claude.KeptWindow `json:"w,omitempty"`
}

// stored is p as a day's digest stores it, as storedPointing has it. A share
// that isn't a finite number, which JSON can't carry, has it refused as it's
// written, for the session's page to be read whole.
func (p *pointing) stored() storedPointing {
	st := storedPointing{Days: p.days, Done: p.done, Held: make(map[string][]storedAnswer), Folded: p.folded}
	if p.run != nil {
		run := p.run.stored()
		st.Run = &run
	}
	for date, held := range p.held {
		if !p.isDay(date) {
			continue
		}
		for _, a := range held.all() {
			st.Held[date] = append(st.Held[date], a.stored())
		}
	}
	return st
}

// restored is the points st stores, as they were sealed: its held answers
// taken in afresh, each summed of what it holds, as answered's summed has
// it, and its points worked out again, as worked has them. It takes in what
// it's given after, as took and sealedAt have it, as the points stored
// would: what they held of a day that isn't the session's, left out, would
// have counted toward none of it. It's refused where its days aren't dates,
// oldest first, each once, as a day is known by them, or a window of its run
// has no climb.
func (st *storedPointing) restored() (*pointing, error) {
	if err := datesInOrder(st.Days); err != nil {
		return nil, fmt.Errorf("restore a page's points: %w", err)
	}
	p := &pointing{days: st.Days, done: st.Done, held: make(map[string]answersHeld, len(st.Held)), folded: make(map[string]int, len(st.Folded))}
	maps.Copy(p.folded, st.Folded)
	if st.Run != nil {
		run, err := st.Run.restored()
		if err != nil {
			return nil, fmt.Errorf("restore a page's points: %w", err)
		}
		p.run = run
	}
	for date, stored := range st.Held {
		answers := make([]answered, len(stored))
		for i := range stored {
			answers[i] = stored[i].restored()
		}
		p.held[date] = heldOf(answers)
	}
	p.points = p.worked()
	return p, nil
}

// datesInOrder fails where dates aren't dates, oldest first, each once.
func datesInOrder(dates []string) error {
	for i, date := range dates {
		if _, _, ok := dayfile.Day(date); !ok {
			return fmt.Errorf("%q: not a date", date)
		}
		if i > 0 && dates[i-1] >= date {
			return fmt.Errorf("%s after %s: not in order, each once", date, dates[i-1])
		}
	}
	return nil
}

// stored is r as its stored form holds it.
func (r *runShares) stored() storedRun {
	st := storedRun{First: r.first, Last: r.last, Through: r.through.stored(), Accounts: make(map[string]storedAccount, len(r.accounts))}
	for account, a := range r.accounts {
		st.Accounts[account] = a.stored()
	}
	return st
}

// restored is the run st stores, refused where a window of it has no climb.
func (st *storedRun) restored() (*runShares, error) {
	r := &runShares{first: st.First, last: st.Last, through: st.Through.instant(), accounts: make(map[string]*accountShares, len(st.Accounts))}
	for account, stored := range st.Accounts {
		a, err := stored.restored()
		if err != nil {
			return nil, fmt.Errorf("the run of %s to %s, account %s: %w", st.First, st.Last, account, err)
		}
		r.accounts[account] = a
	}
	return r, nil
}

// stored is a as its stored form holds it.
func (a *accountShares) stored() storedAccount {
	st := storedAccount{Windows: make(map[string]storedShare, len(a.windows)), Ours: namesOf(a.ours)}
	for key, w := range a.windows {
		st.Windows[key] = storedShare{
			Climb: w.climb, Shared: w.shared, Models: namesOf(w.models), Pending: storedEnded{All: w.pending.all, Ours: w.pending.ours}, Share: w.share,
		}
	}
	return st
}

// restored is the account's windows st stores, refused where one has no
// climb.
func (st *storedAccount) restored() (*accountShares, error) {
	a := &accountShares{windows: make(map[string]*windowShare, len(st.Windows)), ours: modelsNamed(st.Ours)}
	for key, w := range st.Windows {
		if w.Climb == nil {
			return nil, fmt.Errorf("window %s: no climb", key)
		}
		a.windows[key] = &windowShare{
			climb: w.Climb, shared: w.Shared, models: modelsNamed(w.Models), pending: tokensEnded{all: w.Pending.All, ours: w.Pending.Ours}, share: w.Share,
		}
	}
	return a, nil
}

// namesOf are the names of models, in order.
func namesOf(models map[unique.Handle[string]]bool) []string {
	names := make([]string, 0, len(models))
	for m := range models {
		names = append(names, m.Value())
	}
	slices.Sort(names)
	return names
}

// modelsNamed are the models with the names given, interned.
func modelsNamed(names []string) map[unique.Handle[string]]bool {
	models := make(map[unique.Handle[string]]bool, len(names))
	for _, name := range names {
		models[unique.Make(name)] = true
	}
	return models
}

// stored is a as its stored form holds it.
func (a *answered) stored() storedAnswer {
	return storedAnswer{
		storedFlight: a.flight.stored(), Read: a.read.stored(), Account: a.account.Value(), Model: a.model.Value(), Tokens: a.tokens, Ours: a.ours,
		Windows: a.windows,
	}
}

// restored is the answer st stores, summed of what it holds, as summed has
// it, as the one taken of its line was.
func (st *storedAnswer) restored() answered {
	a := answered{
		flight: st.flight(), read: st.Read.instant(), account: unique.Make(st.Account), model: unique.Make(st.Model), tokens: st.Tokens, ours: st.Ours,
		windows: st.Windows,
	}
	a.sum = a.summed()
	return a
}
