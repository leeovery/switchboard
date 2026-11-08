package forecast

import (
	"math"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

const (
	// playStep is how far each step of the forecast plays the pool's use
	// forward.
	playStep = 10 * time.Minute
	// playSpan is how far ahead of its Now the forecast plays it.
	playSpan = 7 * quota.Day
	// demandFade is the time constant with which the pool's use fades from
	// its fast rate to its day's rate.
	demandFade = 3 * time.Hour
)

// Account is an account as the forecast starts it.
type Account struct {
	ID string
	// Week is its shared week as it stands at the pool's Now: a week reset
	// since it was read already stands at 0, with its next reset.
	Week quota.Window
	// Reserve is its reserve as configured, which the choice of account
	// judges its room by, and a pin spends.
	Reserve float64
	// Floor is where its room ends: 1 − Reserve where its reserve holds it
	// back, else 1, as the caller works it out.
	Floor float64
	// Size is how big its week is, in Pro weeks: 1 where it isn't a positive
	// number.
	Size float64
	// Fast is its week's fast rate, a share of it an hour.
	Fast float64
	// Takes is set where requests can go to it: its token usable, and not
	// refused. One that can't holds none of its use, which goes where a new
	// session would, as the router sends its requests elsewhere.
	Takes bool
}

// Pool is what the forecast plays forward.
type Pool struct {
	Now    time.Time
	Policy score.Policy
	// Accounts are in the config's order.
	Accounts []Account
	// Pinned are the global pin's accounts while one holds, else none.
	Pinned []string
	// Day is the pool's day's rate, in Pro weeks an hour.
	Day float64
	// Held is the share of an account's fast rate still on it after so
	// long: the default hold curve where it's nil.
	Held func(after time.Duration) float64
}

// Out is where an account's current week is heading.
type Out struct {
	// RunsOut is when its use reaches its Floor before its week resets:
	// zero where it doesn't.
	RunsOut time.Time
	// AtReset is its use at its week's reset, as the forecast has it: its
	// Floor where it runs out.
	AtReset float64
}

// Forecast plays the pool's use forward from its Now, every 10 minutes for a
// week, and returns where each account's current week is heading, by its ID,
// but for an account whose week's reset isn't known. The pool's use runs at
// the day's rate, plus how far its fast rate, every account's together,
// stands from it now, fading with a time constant of 3 hours. What of it an
// account holds, its own fast rate as Held keeps it, stays on it while it
// takes requests, its week runs and it has room, each hold scaled down where
// they add up to more than the pool's use; the rest goes where a new session
// would go, as Policy.Choose chooses among the accounts that take requests,
// each judged by its week alone, until no account has room. A week that
// resets comes back empty, a week from resetting again, and holds nothing
// more.
func Forecast(p Pool) map[string]Out {
	run := newPlay(p)
	run.playTo(p.Now.Add(playSpan))
	return run.outs()
}

// play is a pool's use as it plays forward.
type play struct {
	Pool
	// fast is the pool's fast rate, in Pro weeks an hour.
	fast float64
	// weeks are the accounts' weeks as they stand, in the accounts' order,
	// which the candidates' windows are cut from.
	weeks  []quota.Window
	tracks []track
	// candidates are those a new session can go to, gathered afresh for each
	// choice into the one slice, and takers each one's place among the
	// accounts, as one ID can name more than one.
	candidates []score.Candidate
	takers     []int
	// applies counts the week alone of a candidate's windows: the window the
	// policy's perishability is measured on.
	applies func(key string) bool
	// at is when the step played last started, and next when the next does.
	at, next time.Time
}

// track is what the forecast keeps of an account as it plays.
type track struct {
	// holds is set until its week resets: use already on it stays there.
	holds bool
	// open is set while its Out is still to be told: its week's reset is
	// known, and its week hasn't run out or reset.
	open bool
	out  Out
	// from is its use as the step played last started, and offered the share
	// of its week offered it in that step, spread across it.
	from, offered float64
}

// newPlay starts the pool's use playing from its Now, its accounts its own.
func newPlay(pool Pool) *play {
	if pool.Held == nil {
		pool.Held = defaultHold
	}
	pool.Accounts = slices.Clone(pool.Accounts)
	week := pool.Policy.Perishable
	p := &play{
		Pool:       pool,
		weeks:      make([]quota.Window, len(pool.Accounts)),
		tracks:     make([]track, len(pool.Accounts)),
		candidates: make([]score.Candidate, 0, len(pool.Accounts)),
		takers:     make([]int, 0, len(pool.Accounts)),
		applies:    func(key string) bool { return key == week },
		at:         pool.Now,
		next:       pool.Now,
	}
	for i := range pool.Accounts {
		a := &pool.Accounts[i]
		if !countable(a.Size) {
			a.Size = 1
		}
		p.fast += a.Size * a.Fast
		p.weeks[i] = a.Week
		p.tracks[i] = track{holds: true, open: !a.Week.ResetsAt.IsZero(), from: a.Week.Utilization}
	}
	return p
}

// playTo plays the pool's use forward, a step at a time, to until, and brings
// every week up to where it stands then.
func (p *play) playTo(until time.Time) {
	for ; p.next.Before(until); p.next = p.next.Add(playStep) {
		p.step(p.next)
	}
	p.reset(p.next)
}

// step plays the step that starts at t: the weeks reset by then come back,
// then the pool's use in the step goes to the accounts that hold it, and the
// rest where a new session would go.
func (p *play) step(t time.Time) {
	p.reset(t)
	p.at = t
	for i := range p.tracks {
		p.tracks[i].from, p.tracks[i].offered = p.weeks[i].Utilization, 0
	}
	mid := t.Add(playStep / 2).Sub(p.Now)
	p.place(t, p.hold(t, mid, p.demand(mid)))
}

// demand is the pool's use, in Pro weeks, in a step whose middle comes so
// long after its Now: never less than none.
func (p *play) demand(after time.Duration) float64 {
	rate := p.Day + (p.fast-p.Day)*decay(after, demandFade)
	return max(rate, 0) * playStep.Hours()
}

// hold puts on each account that holds its use its part of demand, the
// pool's use in the step that starts at t, whose middle comes so long after
// its Now: its fast rate, as Held keeps it then, every hold scaled down where
// they add up to more than demand. It returns what's left of demand: what no
// account holds, and what one holds past its room.
func (p *play) hold(t time.Time, after time.Duration, demand float64) float64 {
	share := p.Held(after) * playStep.Hours()
	var held float64
	for i, a := range p.Accounts {
		if p.holding(i, t) {
			held += a.Size * a.Fast * share
		}
	}
	scale, rest := 1.0, demand-held
	if held > demand {
		scale, rest = demand/held, 0
	}
	for i, a := range p.Accounts {
		if p.holding(i, t) {
			rest += p.fill(i, a.Size*a.Fast*share*scale, t)
		}
	}
	return rest
}

// holding reports whether account i holds its use at t: it takes requests,
// its week hasn't reset, and it has room.
func (p *play) holding(i int, t time.Time) bool {
	return p.Accounts[i].Takes && p.tracks[i].holds && p.room(i, t) > score.Tolerance
}

// place sends rest, the pool's use in the step that starts at t that no
// account holds, where a new session would go, choosing again each time the
// account chosen runs out, until it's all placed or no account has room.
func (p *play) place(t time.Time, rest float64) {
	for rest > 0 {
		p.gather(t)
		c, ok := p.Policy.Choose(p.candidates, p.Pinned, p.applies, "", t)
		if !ok {
			return
		}
		chosen := slices.IndexFunc(p.candidates, func(candidate score.Candidate) bool { return candidate.ID == c.ID })
		rest = p.fill(p.takers[chosen], rest, t)
	}
}

// gather makes the candidates for a new session at t: the accounts that take
// requests and have room, each with its week alone, as it stands. One
// whose week's reset isn't known has no score: Choose sends it new sessions
// only where the global pin names it and none of the pin's accounts it can
// score has room, as the router does.
func (p *play) gather(t time.Time) {
	p.candidates, p.takers = p.candidates[:0], p.takers[:0]
	for i, a := range p.Accounts {
		if a.Takes && p.room(i, t) > score.Tolerance {
			p.candidates = append(p.candidates, score.Candidate{ID: a.ID, Windows: p.weeks[i : i+1 : i+1], Reserve: a.Reserve})
			p.takers = append(p.takers, i)
		}
	}
}

// room is how much of account i's week is left at t before its use reaches
// its floor.
func (p *play) room(i int, t time.Time) float64 {
	return score.Room(p.weeks[i], p.Accounts[i].Floor, t)
}

// fill puts amount, in Pro weeks, on account i in the step that starts at t,
// as far as its room goes, and returns what it had no room for, in Pro weeks.
// Where that runs its week out before it resets, its Out says when: as far
// into the step as the share offered it, spread across the step, reached its
// floor.
func (p *play) fill(i int, amount float64, t time.Time) float64 {
	a, w, tr := &p.Accounts[i], &p.weeks[i], &p.tracks[i]
	share, room := amount/a.Size, p.room(i, t)
	tr.offered += share
	if share < room-score.Tolerance {
		w.Utilization += share
		return 0
	}
	w.Utilization += room
	ran := t.Add(time.Duration(math.Round(min((a.Floor-tr.from)/tr.offered, 1) * float64(playStep))))
	if tr.open && ran.Before(w.ResetsAt) {
		tr.out, tr.open = Out{RunsOut: ran, AtReset: a.Floor}, false
	}
	return max(share-room, 0) * a.Size
}

// reset brings back each week whose reset has come by t: empty, its next
// reset a week on, and holding nothing more. Where it's the week an account
// started in, its Out says its use at its reset.
func (p *play) reset(t time.Time) {
	for i := range p.weeks {
		w, tr := &p.weeks[i], &p.tracks[i]
		if !w.ResetBy(t) {
			continue
		}
		if tr.open {
			tr.out, tr.open = Out{AtReset: p.useAt(i, w.ResetsAt)}, false
		}
		*w = score.Current(*w, t)
		tr.holds = false
	}
}

// useAt is account i's use at t, within the step played last, as the share
// offered it is spread across the step.
func (p *play) useAt(i int, t time.Time) float64 {
	tr := p.tracks[i]
	into := min(max(t.Sub(p.at), 0), playStep)
	return min(tr.from+tr.offered*float64(into)/float64(playStep), p.weeks[i].Utilization)
}

// outs are where each account's week is heading, as played so far, but for
// an account whose week's reset isn't known: a week that hasn't run out or
// reset yet at the use it stands at.
func (p *play) outs() map[string]Out {
	outs := make(map[string]Out, len(p.Accounts))
	for i, a := range p.Accounts {
		switch {
		case a.Week.ResetsAt.IsZero():
		case p.tracks[i].open:
			outs[a.ID] = Out{AtReset: p.weeks[i].Utilization}
		default:
			outs[a.ID] = p.tracks[i].out
		}
	}
	return outs
}
