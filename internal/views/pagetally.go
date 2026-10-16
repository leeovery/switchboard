package views

import (
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// pageTally is what a session's page tallies of its requests as its turns
// take them in, as sentOf has them, folded a request at a time, so the store
// folds the requests it seals once, and a page is built of it with only the
// open requests and the stream's rows folded on: its turns, without their
// requests, each prompt's turn and each agent's prompt, as turn has them;
// whether a request in none is in flight; its first and last request sent,
// its idle gaps folded and when it last came back after one; its stretches,
// both of every request and of its own conversation's, for whichever it's
// told by; where its models went last, in the order they first came, as
// goesOf has them; and of its lines alone, the accounts they went to, what
// they tell of why each model goes where it does, as keptLines has it, its
// totals, when each that went upstream arrived, oldest first, each request
// once, as the traces count them, with those that may still be in flight as
// the next arrives, and the models they asked for. What's taken in tells on
// a copy of it, as told has it, so a page built of it never changes it.
type pageTally struct {
	turns   []*turnTally
	prompts map[string]int
	agents  map[string]string
	inNone  bool

	begun       bool
	first, last time.Time
	folds       []Fold
	resumed     time.Time
	// byMain is set once a request of its own conversation is taken in:
	// its stretches are then its own conversation's, stretched[1], else
	// every request's, stretched[0].
	byMain    bool
	stretched [2]stretching
	goes      []SessionModel

	accounts []SessionAccount
	kept     map[string]keptLines
	totals   sessionSums
	upstream []instant
	flying   []flight
	models   map[string]bool

	// own are the turns a copy telling on has made its own, and whether its
	// maps are, so it changes nothing it shares with what it was copied of.
	own     map[*turnTally]bool
	ownMaps bool
}

// stretching is a session's stretches as they're taken in, as stretchesOf
// takes them, and whether the last is still to be placed on an account.
type stretching struct {
	stretches []Stretch
	open      bool
}

// sessionSums are what a session's totals sum of its lines, as totalsOf sums
// them: how many there are, their worth, and the tokens they read from the
// cache, and sent.
type sessionSums struct {
	requests   int
	worth      Worth
	read, sent int
}

// newPageTally is the tally of a session before any of its requests is
// taken in.
func newPageTally() *pageTally {
	return &pageTally{prompts: make(map[string]int), agents: make(map[string]string), kept: make(map[string]keptLines), models: make(map[string]bool),
		ownMaps: true}
}

// told is p told on afresh, the requests sent, its session's next as its
// turns take them in, taken in, in order.
func (p *pageTally) told(sent []sent) *pageTally {
	c := p.cloned()
	for i := range sent {
		c.add(&sent[i])
	}
	c.own, c.ownMaps = nil, false
	return c
}

// took is p told on afresh of requests, its session's next lines, in order,
// as inOrder has it.
func (p *pageTally) took(requests []heldRequest) *pageTally {
	sent := make([]sent, len(requests))
	for i := range requests {
		sent[i] = lineSent(&requests[i])
	}
	return p.told(sent)
}

// cloned is a copy of p for what tells it on, sharing with p what it
// copies only once it changes it, as own has it: its turns, each, and its
// maps; its few accounts, models' goes and stretches its own at once.
func (p *pageTally) cloned() *pageTally {
	c := *p
	c.turns = slices.Clone(p.turns)
	c.accounts, c.goes = slices.Clone(p.accounts), slices.Clone(p.goes)
	for i := range c.stretched {
		c.stretched[i].stretches = slices.Clone(p.stretched[i].stretches)
	}
	c.folds, c.upstream, c.flying = slices.Clip(p.folds), slices.Clip(p.upstream), slices.Clone(p.flying)
	c.own, c.ownMaps = make(map[*turnTally]bool), false
	return &c
}

// owningMaps makes p's maps its own, where they aren't yet.
func (p *pageTally) owningMaps() {
	if p.ownMaps {
		return
	}
	p.prompts, p.agents, p.kept, p.models = maps.Clone(p.prompts), maps.Clone(p.agents), maps.Clone(p.kept), maps.Clone(p.models)
	p.ownMaps = true
}

// turnOwned is p's turn at index i, its own, as what tells p on may change
// it.
func (p *pageTally) turnOwned(i int) *turnTally {
	if t := p.turns[i]; p.own[t] {
		return t
	}
	t := p.turns[i].cloned()
	p.turns[i], p.own[t] = t, true
	return t
}

// add takes in r, the session's next request as its turns take them in.
func (p *pageTally) add(r *sent) {
	p.sent(r)
	p.turn(r)
	if r.line != nil {
		p.lined(r.line)
	}
}

// sent takes in r as the session's story and its models' goes follow every
// request, whether its line has come or not.
func (p *pageTally) sent(r *sent) {
	starts := !p.begun || idled(p.last, r.at)
	if p.begun && starts {
		p.folds = append(p.folds, Fold{From: p.last, To: r.at})
		p.resumed = r.at.UTC()
	}
	if !p.begun {
		p.begun, p.first = true, r.at
	}
	p.byMain = p.byMain || r.class == ledger.ClassMain
	p.stretched[0].take(r, starts, false)
	p.stretched[1].take(r, starts, true)
	p.last = r.at
	if r.account == "" {
		return
	}
	i := slices.IndexFunc(p.goes, func(g SessionModel) bool { return g.Model == r.model })
	if i < 0 {
		i, p.goes = len(p.goes), append(p.goes, SessionModel{Model: r.model})
	}
	p.goes[i].Account = r.account
}

// take takes r into the stretches s, as stretchesOf does: one begun where
// starts says, as the session starts or comes back after a fold; and, of
// its own conversation's where byMain says, r another account's.
func (s *stretching) take(r *sent, starts, byMain bool) {
	if starts {
		var on string
		if n := len(s.stretches); n > 0 {
			on = s.stretches[n-1].Account
		}
		s.stretches, s.open = append(s.stretches, Stretch{Account: on, From: r.at}), true
	}
	if r.account == "" || byMain && r.class != ledger.ClassMain {
		return
	}
	last := &s.stretches[len(s.stretches)-1]
	switch {
	case s.open:
		last.Account, s.open = r.account, false
	case last.Account != r.account:
		s.stretches = append(s.stretches, Stretch{Account: r.account, From: r.at})
	}
}

// turn counts r in its turn, as talliesOf does: a subagent's in the turn of
// the prompt its first request named, noted as it comes.
func (p *pageTally) turn(r *sent) {
	prompt := r.prompt
	if r.agentID != "" {
		first, ok := p.agents[r.agentID]
		if !ok {
			p.owningMaps()
			first, p.agents[r.agentID] = r.prompt, r.prompt
		}
		prompt = first
	}
	if prompt == "" {
		p.inNone = p.inNone || r.pending && r.end.IsZero()
		return
	}
	i, ok := p.prompts[prompt]
	if !ok {
		p.owningMaps()
		i, p.prompts[prompt] = len(p.turns), len(p.turns)
		t := &turnTally{prompt: prompt, started: r.at, tools: make(map[string]int), accounts: []string{}, mainAccounts: []string{}}
		p.turns, p.own[t] = append(p.turns, t), true
	}
	p.turnOwned(i).add(r)
}

// lined takes in r, a line of the session's, as its accounts, what keeps its
// models where they go, its totals and its traces count it.
func (p *pageTally) lined(r *heldRequest) {
	worth := r.worthSaid()
	model, account, reason := r.model.Value(), r.account.Value(), r.reason.Value()
	if account != "" {
		at := r.arrived()
		i := slices.IndexFunc(p.accounts, func(a SessionAccount) bool { return a.Account == account })
		if i < 0 {
			i, p.accounts = len(p.accounts), append(p.accounts, SessionAccount{Account: account, From: at})
		}
		a := &p.accounts[i]
		a.To, a.Requests = at, a.Requests+1
		a.add(worth)
	}
	k := keptLines{account: account, came: p.kept[model].came}
	if status.Kept(reason) {
		k.kept = reason
	} else {
		k.came = reason
	}
	if p.kept[model] != k || !p.models[model] {
		p.owningMaps()
		p.kept[model], p.models[model] = k, true
	}
	tokens, _ := r.spend.Tokens()
	p.totals.requests++
	p.totals.worth.add(worth)
	p.totals.read += tokens.CacheRead
	p.totals.sent += tokens.Input + tokens.CacheRead + tokens.CacheWrite
	if r.upstream {
		p.wentUp(r)
	}
}

// wentUp counts r, a line of a request that went upstream, among those the
// traces count, as counting counts them: once, but where it arrived while
// one of the same id counted before it was in flight, as one request. Those
// counted that ended before r arrived are let go of, as no later line
// arrives while they're in flight.
func (p *pageTally) wentUp(r *heldRequest) {
	at := r.arrived()
	p.flying = slices.DeleteFunc(p.flying, func(f flight) bool { return !inFlight(at, f.arrived(), f.ended()) })
	if slices.ContainsFunc(p.flying, func(f flight) bool { return f.request == r.request }) {
		return
	}
	p.flying = append(p.flying, r.flight)
	p.upstream = append(p.upstream, instantOf(at))
}

// pricedAlike reports whether every model p's lines asked for costs alike
// on the days with the dates a and b, as ledger.Table.SamePrices has it, so
// what it tallied priced on one is worth as much on the other.
func (p *pageTally) pricedAlike(prices ledger.Table, a, b string) bool {
	for model := range p.models {
		if !prices.SamePrices(model, a, b) {
			return false
		}
	}
	return true
}

// stretches are the session's stretches, as stretchesOf has them: its own
// conversation's where a request of it was taken in, else every request's.
func (p *pageTally) stretches() []Stretch {
	s := p.stretched[0]
	if p.byMain {
		s = p.stretched[1]
	}
	return slices.DeleteFunc(slices.Clone(s.stretches), func(s Stretch) bool { return s.Account == "" })
}

// accountsSaid are the accounts the session ran on, as accountsOf has them,
// each with the points given it took, by account.
func (p *pageTally) accountsSaid(points map[string]map[string]int) []SessionAccount {
	accounts := make([]SessionAccount, len(p.accounts))
	for i, a := range p.accounts {
		a.Points = points[a.Account]
		accounts[i] = a
	}
	return accounts
}

// totalsOf sums the session, of which it told turns turns, as totalsOf sums
// its requests.
func (p *pageTally) totalsOf(turns int) SessionTotals {
	t := p.totals
	totals := SessionTotals{Turns: turns, Requests: t.requests, Worth: t.worth}
	if t.sent > 0 {
		totals.FromCache = float64(t.read) / float64(t.sent)
	}
	return totals
}

// accountSet are the accounts the session's lines went to.
func (p *pageTally) accountSet() map[string]bool {
	set := make(map[string]bool, len(p.accounts))
	for _, a := range p.accounts {
		set[a.Account] = true
	}
	return set
}

// member reports whether the request of the agent with the given id, naming
// the prompt given, counts in the turn of the prompt given, turn, as
// promptOf has it, of the agents p knows.
func (p *pageTally) member(agentID, prompt, turn string) bool {
	if first, ok := p.agents[agentID]; agentID != "" && ok {
		prompt = first
	}
	return prompt != "" && prompt == turn
}
