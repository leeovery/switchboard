package views

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// storedPage is a page's tally as a day's digest carries it in its session's
// state, sealed at the end of its day as the store seals it: all of it but
// its turns, its agents first seen and when its requests went upstream,
// which its parts hold, as storedPart has them; of its turns, how many
// there are, for its parts to fill. Each of the rest as the tally holds it:
// whether a request in none is in flight; whether it's taken one in, its
// first and last; its idle gaps folded and when it last came back after
// one; whether its own conversation tells its stretches, and its stretches
// of every request and of its own conversation's; where its models went
// last; its accounts, why each model goes where it does, and its totals,
// each with what it spent in ledger.Spent's own stored form; the requests
// that may still be in flight as the next arrives; and its points, as
// storedPointing has them. Not its prompts' turns, worked out of its turns
// as it's restored, nor what a copy of it takes in. Its keys are short and
// its zeros left out.
type storedPage struct {
	Turns     int                   `json:"tn,omitzero"`
	InNone    bool                  `json:"nn,omitzero"`
	Begun     bool                  `json:"b,omitzero"`
	First     storedInstant         `json:"f,omitzero"`
	Last      storedInstant         `json:"l,omitzero"`
	Folds     []storedFold          `json:"fd,omitempty"`
	Resumed   storedInstant         `json:"rs,omitzero"`
	ByMain    bool                  `json:"bm,omitzero"`
	Stretched [2]storedStretching   `json:"st,omitzero"`
	Goes      []storedGo            `json:"g,omitempty"`
	Accounts  []storedAccountSums   `json:"a,omitempty"`
	Kept      map[string]storedKept `json:"k,omitempty"`
	Totals    storedSums            `json:"tt,omitzero"`
	Flying    []storedFlight        `json:"fl,omitempty"`
	Points    storedPointing        `json:"pt,omitzero"`
}

// storedFold is an idle gap a page's tally folded, as its stored form holds
// it: from its last request before it to its first after.
type storedFold struct {
	From storedInstant `json:"f,omitzero"`
	To   storedInstant `json:"t,omitzero"`
}

// storedStretching is a page's stretches as its stored form holds them, and
// whether the last is still to be placed on an account.
type storedStretching struct {
	Stretches []storedStretch `json:"s,omitempty"`
	Open      bool            `json:"o,omitzero"`
}

// storedStretch is a stretch as a page's stored form holds it: its account
// and when it began.
type storedStretch struct {
	Account string        `json:"a,omitempty"`
	From    storedInstant `json:"f,omitzero"`
}

// storedGo is where a model of a page's went last, as its stored form holds
// it: the model and the account. Why, and since when, are worked out as the
// page is built.
type storedGo struct {
	Model   string `json:"m,omitempty"`
	Account string `json:"a,omitempty"`
}

// storedAccountSums are what a page's lines that went to an account sum, as
// its stored form holds them: the account, when the first and the last of
// them arrived, how many there are, and what they spent.
type storedAccountSums struct {
	Account  string        `json:"ac,omitempty"`
	From     storedInstant `json:"f,omitzero"`
	To       storedInstant `json:"t,omitzero"`
	Requests int           `json:"r,omitzero"`
	Spent    ledger.Spent  `json:"sp,omitzero"`
}

// storedKept is what a model's lines tell of why it goes where it does, as
// a page's stored form holds it, as keptLines has it.
type storedKept struct {
	Account string `json:"a,omitempty"`
	Came    string `json:"c,omitempty"`
	Kept    string `json:"k,omitempty"`
}

// storedSums are a page's totals as its stored form holds them: how many
// lines it has, what they spent, and the tokens they read from the cache,
// and sent.
type storedSums struct {
	Requests int          `json:"r,omitzero"`
	Spent    ledger.Spent `json:"sp,omitzero"`
	Read     int          `json:"rd,omitzero"`
	Sent     int          `json:"s,omitzero"`
}

// storedPart is what a fold of a page's day added to the tally it was
// folded on, as the day's parts file keeps it: each turn it took a request
// into, or began, as it is after, by its index; each agent first seen, with
// the prompt of its first request; and when the requests it sealed went
// upstream, oldest first. A fold on no tally adds all of it. Its turns come
// first and its times last, so a reader wanting its turns alone can stop
// before its times. Not the date of the state it was folded on, which the
// digest pointing at it names. Its keys are short and its zeros left out.
type storedPart struct {
	Turns  []storedTurn      `json:"t,omitempty"`
	Agents map[string]string `json:"a,omitempty"`
	Times  storedTimes       `json:"s,omitzero"`
}

// storedTurn is a turn as a page's part holds it: its index among the
// page's turns, from 0 at its first, and all the turn holds, as turnTally
// has it, what it spent in ledger.Spent's own stored form.
type storedTurn struct {
	Index        int            `json:"i,omitzero"`
	Prompt       string         `json:"p,omitempty"`
	Started      storedInstant  `json:"s,omitzero"`
	Last         *storedEnd     `json:"l,omitempty"`
	LastMain     *storedEnd     `json:"lm,omitempty"`
	Requests     int            `json:"r,omitzero"`
	Tools        map[string]int `json:"tl,omitempty"`
	Read         int            `json:"rd,omitzero"`
	Written      int            `json:"w,omitzero"`
	Out          int            `json:"o,omitzero"`
	Spent        ledger.Spent   `json:"sp,omitzero"`
	Accounts     []string       `json:"a,omitempty"`
	MainAccounts []string       `json:"ma,omitempty"`
	Pending      bool           `json:"pn,omitzero"`
	MainPending  bool           `json:"mp,omitzero"`
}

// storedEnd is how a turn's request ended as a stored turn holds it, as
// turnEnd has it: none of a turn none of whose requests set it.
type storedEnd struct {
	End   storedInstant `json:"e,omitzero"`
	Lined bool          `json:"l,omitzero"`
	Stop  string        `json:"s,omitempty"`
}

// storedTimes are when requests went upstream, oldest first, as a page's
// part holds them: as nanoseconds since the epoch, the first whole and each
// after as how many after the one before, while its tally holds them as
// nanoseconds, as upstreamTimes has it; else each as an instant.
type storedTimes struct {
	Nanos []int64         `json:"n,omitempty"`
	Far   []storedInstant `json:"f,omitempty"`
}

// folded is s as a day's digest keeps it, folded at the end of its day:
// its state, as stored has it, put as JSON; and, where that holds its page,
// the part of its page the fold added to base, the tally of the state it
// was folded on, as partOver has it, whole where base is nil, as of a fold
// on no state. It's refused as stored refuses it.
func (s *heldSession) folded(base *pageTally) (ledger.Folded, error) {
	st, err := s.stored()
	if err != nil {
		return ledger.Folded{}, err
	}
	state, err := st.put()
	if err != nil {
		return ledger.Folded{}, err
	}
	f := ledger.Folded{State: state}
	if st.Page == nil {
		return f, nil
	}
	if f.Part, err = json.Marshal(s.page.partOver(base)); err != nil {
		return ledger.Folded{}, fmt.Errorf("put the part of the page of a session of %s: %w", st.Date, err)
	}
	return f, nil
}

// put is what p carries, as carried has it, put as JSON: none of a page
// untallied, nor of one JSON can't carry, as one whose points hold a share
// that isn't a finite number, for the page to be read whole.
func (p *pageTally) put() json.RawMessage {
	if p == nil {
		return nil
	}
	data, err := json.Marshal(p.carried())
	if err != nil {
		return nil
	}
	return data
}

// carried is p as a day's digest carries it in its session's state, as
// storedPage has it. It shares its points' days and shares done, so it's
// put as JSON at once.
func (p *pageTally) carried() storedPage {
	st := storedPage{
		Turns: len(p.turns), InNone: p.inNone, Begun: p.begun, First: instantOf(p.first).stored(), Last: instantOf(p.last).stored(),
		Resumed: instantOf(p.resumed).stored(), ByMain: p.byMain, Totals: p.totals.stored(), Points: p.points.stored(),
	}
	for _, f := range p.folds {
		st.Folds = append(st.Folds, storedFold{From: instantOf(f.From).stored(), To: instantOf(f.To).stored()})
	}
	for i, s := range p.stretched {
		st.Stretched[i] = s.stored()
	}
	for _, g := range p.goes {
		st.Goes = append(st.Goes, storedGo{Model: g.Model, Account: g.Account})
	}
	for _, a := range p.accounts {
		st.Accounts = append(st.Accounts, storedAccountSums{
			Account: a.account, From: instantOf(a.from).stored(), To: instantOf(a.to).stored(), Requests: a.requests, Spent: a.spent,
		})
	}
	st.Kept = make(map[string]storedKept, len(p.kept))
	for model, k := range p.kept {
		st.Kept[model] = storedKept{Account: k.account, Came: k.came, Kept: k.kept}
	}
	for _, f := range p.flying {
		st.Flying = append(st.Flying, f.stored())
	}
	return st
}

// partOver is what p's fold added to base, the tally it was folded on, as
// storedPart has it: each turn of p's base doesn't hold, or holds otherwise,
// as a fold makes the turns it takes a request into its own, as turnOwned
// has it, so base shares the rest with p; each agent of p's base doesn't
// know; and p's times after base's. Of a base of nil, all of p. It shares
// its turns' tools and accounts, so it's put as JSON at once.
func (p *pageTally) partOver(base *pageTally) storedPart {
	if base == nil {
		base = &pageTally{}
	}
	part := storedPart{Agents: make(map[string]string)}
	for i, t := range p.turns {
		if i < len(base.turns) && base.turns[i] == t {
			continue
		}
		part.Turns = append(part.Turns, t.stored(i))
	}
	for agent, prompt := range p.agents {
		if _, known := base.agents[agent]; !known {
			part.Agents[agent] = prompt
		}
	}
	part.Times = p.upstream.storedFrom(base.upstream.len())
	return part
}

// pageOf is the page st carries, as its stored form put it, restored with
// parts, the parts of the folds of the states it was folded on, oldest
// first, from one folded on none, each as partOver put it, as storedPage's
// restored has it. It's refused where st carries no page, as a session's
// state whose page was left out, or its page or a part won't read, or won't
// restore.
func (st *storedSession) pageOf(parts []json.RawMessage) (*pageTally, error) {
	if st.Page == nil {
		return nil, fmt.Errorf("restore the page of a session of %s: none stored", st.Date)
	}
	var carried storedPage
	if err := json.Unmarshal(st.Page, &carried); err != nil {
		return nil, fmt.Errorf("read the page of a session of %s: %w", st.Date, err)
	}
	chain := make([]storedPart, len(parts))
	for i, part := range parts {
		if err := json.Unmarshal(part, &chain[i]); err != nil {
			return nil, fmt.Errorf("read part %d of %d of the page of a session of %s: %w", i+1, len(parts), st.Date, err)
		}
	}
	p, err := carried.restored(chain)
	if err != nil {
		return nil, fmt.Errorf("restore the page of a session of %s: %w", st.Date, err)
	}
	return p, nil
}

// restoredPaged is the session st stores restored onto the day given, as
// restored has it, its page tallied as st carries it with parts laid over
// it, as pageOf has it. It's refused as either refuses it.
func (st *storedSession) restoredPaged(prices ledger.Table, day LedgerDay, parts []json.RawMessage) (*heldSession, error) {
	page, err := st.pageOf(parts)
	if err != nil {
		return nil, err
	}
	s, err := st.restored(prices, day)
	if err != nil {
		return nil, err
	}
	s.page = page
	return s, nil
}

// restored is the tally st carries with parts laid over it, the parts of
// the folds of the states it was folded on, oldest first, from one folded
// on none: each turn as the newest part holding it has it; each agent first
// seen as a part gives it; and the times of one part after another's, as
// upstreamTimes' with takes them in, so they're held as a fold of them all
// at once holds them. Its prompts' turns are worked out of its turns, and
// it owns nothing a copy of it shares, as told leaves a tally. It's refused
// where its points won't restore, as storedPointing's restored has it, a
// part's times won't read, a part holds a turn beyond the count st carries
// or leaves one below it in none, or two turns serve one prompt.
func (st *storedPage) restored(parts []storedPart) (*pageTally, error) {
	points, err := st.Points.restored()
	if err != nil {
		return nil, err
	}
	p := st.tally()
	p.points = points
	if st.Turns > 0 {
		p.turns = make([]*turnTally, st.Turns)
	}
	for i := range parts {
		if err := p.overlaid(&parts[i]); err != nil {
			return nil, fmt.Errorf("part %d of %d: %w", i+1, len(parts), err)
		}
	}
	if err := p.prompted(); err != nil {
		return nil, err
	}
	return p, nil
}

// tally is the tally st carries, without its turns, its points, or its
// agents and times, which its parts hold: its maps of its own, as a tally
// told on writes them.
func (st *storedPage) tally() *pageTally {
	p := &pageTally{
		prompts: make(map[string]int, st.Turns), agents: make(map[string]string), kept: make(map[string]keptLines, len(st.Kept)),
		inNone: st.InNone, begun: st.Begun, first: st.First.instant().time(), last: st.Last.instant().time(), resumed: st.Resumed.instant().time(),
		byMain: st.ByMain, totals: st.Totals.sums(),
	}
	for _, f := range st.Folds {
		p.folds = append(p.folds, Fold{From: f.From.instant().time(), To: f.To.instant().time()})
	}
	for i := range st.Stretched {
		p.stretched[i] = st.Stretched[i].stretching()
	}
	for _, g := range st.Goes {
		p.goes = append(p.goes, SessionModel{Model: g.Model, Account: g.Account})
	}
	for _, a := range st.Accounts {
		p.accounts = append(p.accounts, accountSums{
			account: a.Account, from: a.From.instant().time(), to: a.To.instant().time(), requests: a.Requests, spent: a.Spent,
		})
	}
	for model, k := range st.Kept {
		p.kept[model] = keptLines{account: k.Account, came: k.Came, kept: k.Kept}
	}
	for _, f := range st.Flying {
		p.flying = append(p.flying, f.flight())
	}
	return p
}

// overlaid lays part over p, the newer: its turns in place of those p holds
// at their indices, its agents among p's, and its times after p's. It fails
// where a turn's index is beyond p's turns, or part's times won't read.
func (p *pageTally) overlaid(part *storedPart) error {
	for i := range part.Turns {
		t := &part.Turns[i]
		if t.Index < 0 || t.Index >= len(p.turns) {
			return fmt.Errorf("turn %d of %d turns", t.Index, len(p.turns))
		}
		p.turns[t.Index] = t.restored()
	}
	maps.Copy(p.agents, part.Agents)
	times, err := part.Times.times()
	if err != nil {
		return err
	}
	p.upstream = p.upstream.with(times)
	return nil
}

// prompted works out p's prompts' turns of its turns, failing where one is
// in no part, or two serve one prompt.
func (p *pageTally) prompted() error {
	for i, t := range p.turns {
		if t == nil {
			return fmt.Errorf("turn %d of %d in no part", i, len(p.turns))
		}
		if j, served := p.prompts[t.prompt]; served {
			return fmt.Errorf("turns %d and %d serve one prompt", j, i)
		}
		p.prompts[t.prompt] = i
	}
	return nil
}

// stored is s as a page's stored form holds it.
func (s stretching) stored() storedStretching {
	st := storedStretching{Open: s.open}
	for _, r := range s.stretches {
		st.Stretches = append(st.Stretches, storedStretch{Account: r.Account, From: instantOf(r.From).stored()})
	}
	return st
}

// stretching is the stretches st stores.
func (st *storedStretching) stretching() stretching {
	s := stretching{open: st.Open}
	for _, r := range st.Stretches {
		s.stretches = append(s.stretches, Stretch{Account: r.Account, From: r.From.instant().time()})
	}
	return s
}

// stored is s as a page's stored form holds it.
func (s sessionSums) stored() storedSums {
	return storedSums{Requests: s.requests, Spent: s.spent, Read: s.read, Sent: s.sent}
}

// sums are the totals st stores.
func (st storedSums) sums() sessionSums {
	return sessionSums{requests: st.Requests, spent: st.Spent, read: st.Read, sent: st.Sent}
}

// stored is t as a page's part holds it, at the index given.
func (t *turnTally) stored(index int) storedTurn {
	return storedTurn{
		Index: index, Prompt: t.prompt, Started: instantOf(t.started).stored(), Last: t.last.stored(), LastMain: t.lastMain.stored(), Requests: t.requests,
		Tools: t.tools, Read: t.read, Written: t.written, Out: t.out, Spent: t.spent, Accounts: t.accounts, MainAccounts: t.mainAccounts,
		Pending: t.pending, MainPending: t.mainPending,
	}
}

// restored is the turn st stores, its maps and lists its own, as a turn
// begun has them, if empty.
func (st *storedTurn) restored() *turnTally {
	t := &turnTally{
		prompt: st.Prompt, started: st.Started.instant().time(), last: st.Last.turnEnd(), lastMain: st.LastMain.turnEnd(), requests: st.Requests,
		tools: make(map[string]int, len(st.Tools)), read: st.Read, written: st.Written, out: st.Out, spent: st.Spent,
		accounts: append([]string{}, st.Accounts...), mainAccounts: append([]string{}, st.MainAccounts...), pending: st.Pending, mainPending: st.MainPending,
	}
	maps.Copy(t.tools, st.Tools)
	return t
}

// stored is e as a stored turn holds it: none where nothing set it.
func (e turnEnd) stored() *storedEnd {
	if !e.set {
		return nil
	}
	return &storedEnd{End: instantOf(e.end).stored(), Lined: e.lined, Stop: e.stop}
}

// turnEnd is the end st stores: none set, of none.
func (st *storedEnd) turnEnd() turnEnd {
	if st == nil {
		return turnEnd{}
	}
	return turnEnd{end: st.End.instant().time(), lined: st.Lined, stop: st.Stop, set: true}
}

// storedFrom are u's times from its ith on as a page's part holds them, as
// storedTimes has them.
func (u upstreamTimes) storedFrom(i int) storedTimes {
	if u.far != nil {
		far := make([]storedInstant, 0, len(u.far)-i)
		for _, t := range u.far[i:] {
			far = append(far, t.stored())
		}
		return storedTimes{Far: far}
	}
	nanos := slices.Clone(u.nanos[i:])
	for k := len(nanos) - 1; k > 0; k-- {
		nanos[k] -= nanos[k-1]
	}
	return storedTimes{Nanos: nanos}
}

// times are the times st stores, oldest first, in UTC. They're refused
// where st holds both nanoseconds and instants, as no part is put so.
func (st *storedTimes) times() ([]time.Time, error) {
	if len(st.Nanos) > 0 && len(st.Far) > 0 {
		return nil, fmt.Errorf("times as %d nanoseconds and %d instants at once", len(st.Nanos), len(st.Far))
	}
	times := make([]time.Time, 0, len(st.Nanos)+len(st.Far))
	for _, t := range st.Far {
		times = append(times, t.instant().time())
	}
	var n int64
	for _, d := range st.Nanos {
		n += d
		times = append(times, time.Unix(0, n).UTC())
	}
	return times, nil
}
