package views

import (
	"cmp"
	"maps"
	"math"
	"slices"
	"strings"
	"time"
	"unique"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
)

// pointing is what a session's page tallies of the points it took of the
// windows of the accounts its requests went to, sealed as the store seals
// its requests. A window counts its account's whole use, every session's,
// so each rise in it is shared out by tokens, as windowShare takes it, among
// the requests of the account on the days the session made requests on,
// run by run of those days one after another: the rises between two days
// apart, with a day between them the session made no request on, are none
// of its. Held: the dates of those days, oldest first; its shares of the
// runs before the last, done, by account, then by window key; the last run,
// sealed as runShares has it, none once it's done too, as sealedAt has it;
// the answers it holds, by the date of the day
// whose files hold them: those of the last run it hasn't sealed, and every
// one of today's and of yesterday's while the day isn't yet one of the
// session's, for a run to take them in as it becomes one; how many answers
// of each day it let go of, sealed or done, for a day read again whole to be
// known; and its points, in whole points, as prose.WholePercent rounds a
// share, of it all, by account, then by window key. A window the account's
// answers never read has no points. What changes it returns afresh, sharing with it what it
// doesn't change.
type pointing struct {
	days   []string
	done   map[string]map[string]float64
	run    *runShares
	held   map[string]answersHeld
	folded map[string]int
	points map[string]map[string]int
}

// runShares are the session's shares of the rises of its accounts' windows
// over a run of its days one after another, from first to last, by
// account, as accountShares has them: every reading read and every request
// ended before through taken in, as no answer yet to come reads or ends
// before it, nor one of a day after last.
type runShares struct {
	first, last string
	through     instant
	accounts    map[string]*accountShares
}

// accountShares are an account's windows over a run of days as their rises
// are shared out, each by its key, and the models of the session's own
// requests among its requests.
type accountShares struct {
	windows map[string]*windowShare
	ours    map[unique.Handle[string]]bool
}

// tokensEnded are the tokens of requests ended: every one's, and the
// session's own.
type tokensEnded struct {
	all, ours int64
}

// windowShare is a window of an account as its rises are shared out: how
// its use climbs; whether every model shares it, as claude.SharedWindows
// says, else the models whose answers read it over the run; the tokens of
// the requests it counts that ended since its last reading took a climb;
// and the session's share of its rises so far, as a fraction of the window.
type windowShare struct {
	climb   *ledger.Climb
	shared  bool
	models  map[unique.Handle[string]]bool
	pending tokensEnded
	share   float64
}

// answered is a request of an account, as the rises in its windows' use are
// shared out: its flight; when its answer's headers came, reading its
// windows; its account and model, interned; its tokens, every kind counted
// alike; whether it's of the session whose share is taken; the windows its
// answer's limits read, kept small; and the sum of what its line says, as
// sumOf has it, so a line given again without an id is known.
type answered struct {
	flight
	read           instant
	account, model unique.Handle[string]
	tokens         int64
	ours           bool
	windows        []claude.KeptWindow
	sum            uint64
}

// endless is an instant after every moment, through which every answer is
// done.
var endless = instant{sec: math.MaxInt64}

// isAnswer reports whether the line l is an answer the points share rises
// among: a request of an account's.
func isAnswer(l *ledger.Line) bool {
	return l.Counted() && l.Account != ""
}

// answeredOf returns the request the line l holds, ours where it's of the
// session with the given id: its windows read as its answer's first byte
// passed on, where one did, else as it ended.
func answeredOf(l *ledger.Line, id string) answered {
	t, _ := l.Tokens()
	a := answered{
		flight: flightOf(l), account: unique.Make(l.Account), model: unique.Make(l.Model), tokens: int64(t.Input + t.Output + t.CacheRead + t.CacheWrite),
		ours: l.Session == id, windows: claude.KeptWindows(l.Limits), sum: sumOf(l),
	}
	a.read = a.end
	if l.FirstMS != nil {
		a.read = instantOf(l.At.Add(time.Duration(*l.FirstMS) * time.Millisecond))
	}
	return a
}

// answersOf returns the answers byDate holds, by the date of the day whose
// files hold them, each answered as answeredOf has it, those of the session
// with the given id its own.
func answersOf(byDate map[string][]ledger.Line, id string) map[string][]answered {
	answers := make(map[string][]answered, len(byDate))
	for date, lines := range byDate {
		for i := range lines {
			if isAnswer(&lines[i]) {
				answers[date] = append(answers[date], answeredOf(&lines[i], id))
			}
		}
	}
	return answers
}

// answersIn returns the answers among lines, as answersOf has them, of the
// day they're all filed on.
func answersIn(lines []ledger.Held, id string) []answered {
	var answers []answered
	for i := range lines {
		if l := &lines[i].Line; isAnswer(l) {
			answers = append(answers, answeredOf(l, id))
		}
	}
	return answers
}

// key is a as a held page's points know it from another answer, as turnKey
// knows a line: by its request's id and when it arrived, or of one without
// an id, by the sum of what its line says too.
func (a *answered) key() turnKey {
	k, ok := a.flight.key()
	if ok {
		return turnKey{lineKey: k}
	}
	return turnKey{lineKey: k, sum: a.sum}
}

// sealedBy reports whether both a's reading and its end came before through,
// and whether either did.
func (a *answered) sealedBy(through instant) (both, either bool) {
	read, ended := a.read.compare(through) < 0, a.end.compare(through) < 0
	return read && ended, read || ended
}

// readFirst orders answers as their readings are taken: by when each was
// read, then when it ended, then by its id, then by the sum of what its line
// says, so readings read at once are taken alike however the lines were
// read.
func readFirst(a, b *answered) int {
	return cmp.Or(a.read.compare(b.read), a.end.compare(b.end), strings.Compare(a.request, b.request), cmp.Compare(a.sum, b.sum))
}

// pointingOf is what a session's page tallies of its points, read whole of
// answers, every account's, by the date of the day whose files hold them,
// at the day given: every run of its days but the last done; the last's
// answers, and today's and yesterday's, held, none sealed yet, as sealedAt
// seals them.
func pointingOf(answers map[string][]answered, day LedgerDay) *pointing {
	p := &pointing{days: daysOf(answers), held: make(map[string]answersHeld), folded: make(map[string]int)}
	runs := runsOf(p.days)
	for i, dates := range runs {
		r := &runShares{first: dates[0], last: dates[len(dates)-1], through: unsealed, accounts: make(map[string]*accountShares)}
		if i == len(runs)-1 {
			p.run = r
			break
		}
		var all []answered
		for _, date := range dates {
			all = append(all, answers[date]...)
			p.folded[date] = len(answers[date])
		}
		r.fold(all, unsealed, endless)
		p.done = withShares(p.done, r.shares())
	}
	yesterday := yesterdayOf(day)
	for date, given := range answers {
		if len(given) > 0 && (p.run.holds(date) || date >= yesterday && !p.isDay(date)) {
			p.held[date] = heldOf(given)
		}
	}
	return p
}

// daysOf returns the dates of the days answers holds a request of the
// session's own on, oldest first.
func daysOf(answers map[string][]answered) []string {
	var dates []string
	for date, held := range answers {
		if slices.ContainsFunc(held, func(a answered) bool { return a.ours }) {
			dates = append(dates, date)
		}
	}
	slices.Sort(dates)
	return dates
}

func runsOf(dates []string) [][]string {
	var runs [][]string
	for i, date := range dates {
		if i == 0 || date != nextDate(dates[i-1]) {
			runs = append(runs, nil)
		}
		runs[len(runs)-1] = append(runs[len(runs)-1], date)
	}
	return runs
}

func nextDate(date string) string {
	_, end, _ := dayfile.Day(date)
	return Today(end).Date
}

// yesterdayOf is the date of the day before the day given.
func yesterdayOf(day LedgerDay) string {
	return dayfile.DateOf(dayfile.DayStart(day.Start, -1))
}

// startOf is when the day with the given date starts.
func startOf(date string) instant {
	start, _, _ := dayfile.Day(date)
	return instantOf(start)
}

// isDay reports whether the session made a request on the day with the
// given date, as p holds its days.
func (p *pointing) isDay(date string) bool {
	_, found := slices.BinarySearch(p.days, date)
	return found
}

// holds reports whether the day with the given date is one of r's, false
// of none.
func (r *runShares) holds(date string) bool {
	return r != nil && r.first <= date && date <= r.last
}

// sealedTo is how far the answers of the day with the given date are taken
// in: through r's through, of a day of its last run; through every moment,
// of a run done; and none, of a day it made no request on.
func (p *pointing) sealedTo(date string) instant {
	switch {
	case !p.isDay(date):
		return unsealed
	case p.run.holds(date):
		return p.run.through
	}
	return endless
}

// took is p with given, the answers of a read of the day with the given
// date, at the day given, taken in, whole where the read is of the day
// whole, reporting false where it can't place them, so the session's page is
// read whole again: each one it holds, of the same request, or without an
// id, as often as it holds it alike, and each it sealed, given again, as
// answersHeld's given has them, passed over, but where a read of the day
// whole gives more or fewer of those than it let go of; one read before
// what it sealed and ended after, which would change what's sealed, can't
// be placed; of one of its own on a day not yet the session's, the day
// joining its days, as joining has it; and of one of a model that first
// reads a window only some models share, of its last run, which changes
// what it sealed, as takes has it. Of a day before yesterday it made no
// request on, only one of its own changes anything, and can't be placed.
// It's p itself where it takes nothing in.
func (p *pointing) took(date string, given []answered, whole bool, day LedgerDay) (*pointing, bool) {
	yesterday := yesterdayOf(day)
	if !p.isDay(date) && date < yesterday {
		return p, !slices.ContainsFunc(given, func(a answered) bool { return a.ours })
	}
	added, sealed, ok := p.held[date].given(given, p.sealedTo(date), whole)
	switch {
	case !ok, whole && p.isDay(date) && sealed != p.folded[date]:
		return nil, false
	case len(added) == 0:
		return p, true
	}
	q := p.cloned()
	q.held[date] = q.held[date].with(added)
	if !q.isDay(date) && slices.ContainsFunc(added, func(a answered) bool { return a.ours }) {
		return q.joining(date)
	}
	return q, !q.run.holds(date) || q.run.takes(added)
}

// joining is p, its own, with the day with the given date, of yesterday's or
// later, one of the session's, reporting false where it can't place it: one
// after its last run, the day after it, extending it, the day's answers
// taken in as takes has it; else after another, done, beginning a new one.
// One on or before its last day, or the day after it, its last run done,
// would change what's sealed or done.
func (p *pointing) joining(date string) (*pointing, bool) {
	r := p.run
	switch {
	case r != nil && date == nextDate(r.last):
		extended := *r
		extended.last = date
		p.run, p.days = &extended, append(slices.Clip(p.days), date)
		return p, p.run.takes(p.held[date].all())
	case len(p.days) > 0 && date <= nextDate(p.days[len(p.days)-1]):
		return nil, false
	case r != nil:
		p.finish()
	}
	p.run = &runShares{first: date, last: date, through: unsealed, accounts: make(map[string]*accountShares)}
	p.days = append(slices.Clip(p.days), date)
	return p, true
}

// finish is p, its own, with its last run done: its every answer taken in,
// its shares among those done, and each of its days' answers let go of.
func (p *pointing) finish() {
	r := p.run.cloned()
	r.fold(p.runAnswers(), r.through, endless)
	p.done = withShares(p.done, r.shares())
	for date, held := range p.held {
		if r.holds(date) {
			p.folded[date] += len(held.all())
			delete(p.held, date)
		}
	}
	p.run = nil
}

// takes reports whether r can take answers in on what it sealed: none reads
// a window only some models share, sealed, with a model none of whose
// answers read it before, which would have counted otherwise the requests
// it sealed, as countsToward has it.
func (r *runShares) takes(answers []answered) bool {
	for i := range answers {
		a := &answers[i]
		sealed := r.accounts[a.account.Value()]
		if sealed == nil {
			continue
		}
		for _, k := range a.windows {
			if w := sealed.windows[k.Window().Key]; w != nil && !w.shared && !w.models[a.model] {
				return false
			}
		}
	}
	return true
}

// sealedAt is p sealed behind the horizon given, at the day given: the
// answers it held of a day before yesterday the session made no request on
// let go of; its last run done, as finish has it, once its last day is
// before yesterday, so it holds no answer of a day before yesterday, a day
// joining it then read whole, as joining has it; else its readings and ends
// before the horizon, or the start of the day after its last, as one of that
// day joining it comes after them, taken in, and the answers both of whose
// came before let go of; and its points worked out afresh, its last run's
// held answers taken in on what it sealed.
func (p *pointing) sealedAt(horizon instant, day LedgerDay) *pointing {
	q := p.cloned()
	yesterday := yesterdayOf(day)
	maps.DeleteFunc(q.held, func(date string, _ answersHeld) bool { return !q.isDay(date) && date < yesterday })
	switch r := q.run; {
	case r == nil:
	case r.last < yesterday:
		q.finish()
	default:
		through := later(r.through, earlier(horizon, startOf(nextDate(r.last))))
		if through.compare(r.through) > 0 {
			sealed := r.cloned()
			sealed.fold(q.runAnswers(), r.through, through)
			sealed.through = through
			q.run = sealed
			q.letGo(through)
		}
	}
	q.points = q.worked()
	return q
}

// letGo lets go of the answers of p's last run, its own, read and ended
// before through, counting them.
func (p *pointing) letGo(through instant) {
	for date, held := range p.held {
		if !p.run.holds(date) {
			continue
		}
		all := held.all()
		kept := slices.DeleteFunc(slices.Clone(all), func(a answered) bool { both, _ := a.sealedBy(through); return both })
		switch p.folded[date] += len(all) - len(kept); {
		case len(kept) == 0:
			delete(p.held, date)
		case len(kept) < len(all):
			p.held[date] = heldOf(kept)
		}
	}
}

// worked is p's points: its shares done, and its last run's, its held
// answers taken in on what it sealed, together, in whole points.
func (p *pointing) worked() map[string]map[string]int {
	taken := withShares(p.done, nil)
	if p.run != nil {
		open := p.run.cloned()
		open.fold(p.runAnswers(), p.run.through, endless)
		taken = withShares(taken, open.shares())
	}
	points := make(map[string]map[string]int, len(taken))
	for account, shares := range taken {
		points[account] = make(map[string]int, len(shares))
		for key, share := range shares {
			points[account][key] = prose.WholePercent(share)
		}
	}
	return points
}

// of are the points p took of the windows of the given account, by key:
// none of a page untallied.
func (p *pointing) of(account string) map[string]int {
	if p == nil {
		return nil
	}
	return p.points[account]
}

// runAnswers are the answers p holds of the days of its last run.
func (p *pointing) runAnswers() []answered {
	var answers []answered
	for date, held := range p.held {
		if p.run.holds(date) {
			answers = append(answers, held.all()...)
		}
	}
	return answers
}

// cloned is a copy of p to change, sharing what it holds of each day, its
// last run and its shares done, which nothing changes in place.
func (p *pointing) cloned() *pointing {
	c := *p
	c.held, c.folded, c.days = maps.Clone(p.held), maps.Clone(p.folded), slices.Clip(p.days)
	return &c
}

// withShares is taken, shares by account, then by window key, with shares
// added to each, afresh: an account given a share of none of its windows
// isn't among them.
func withShares(taken, shares map[string]map[string]float64) map[string]map[string]float64 {
	added := make(map[string]map[string]float64, len(taken))
	for account, s := range taken {
		added[account] = maps.Clone(s)
	}
	for account, s := range shares {
		for key, share := range s {
			if added[account] == nil {
				added[account] = make(map[string]float64)
			}
			added[account][key] += share
		}
	}
	return added
}

// cloned is a copy of r to fold on, every account's windows its own.
func (r *runShares) cloned() *runShares {
	c := *r
	c.accounts = make(map[string]*accountShares, len(r.accounts))
	for account, a := range r.accounts {
		c.accounts[account] = a.cloned()
	}
	return &c
}

// fold takes answers, every one r holds of its days, in on r, its own: the
// readings read and the requests ended from from until until, by account,
// as accountShares' fold has them.
func (r *runShares) fold(answers []answered, from, until instant) {
	byAccount := make(map[string][]*answered)
	for i := range answers {
		account := answers[i].account.Value()
		byAccount[account] = append(byAccount[account], &answers[i])
	}
	for account, given := range byAccount {
		a := r.accounts[account]
		if a == nil {
			a = &accountShares{windows: make(map[string]*windowShare), ours: make(map[unique.Handle[string]]bool)}
			r.accounts[account] = a
		}
		a.fold(given, from, until)
	}
}

// shares are the session's shares of the rises of r's accounts' windows so
// far, by account, then by window key, as accountShares' shares has them.
func (r *runShares) shares() map[string]map[string]float64 {
	shares := make(map[string]map[string]float64, len(r.accounts))
	for account, a := range r.accounts {
		if s := a.shares(); len(s) > 0 {
			shares[account] = s
		}
	}
	return shares
}

// cloned is a copy of a to fold on, each window's climb its own.
func (a *accountShares) cloned() *accountShares {
	c := &accountShares{windows: make(map[string]*windowShare, len(a.windows)), ours: maps.Clone(a.ours)}
	for key, w := range a.windows {
		cw := *w
		cw.climb = w.climb.Clone()
		c.windows[key] = &cw
	}
	return c
}

// fold takes in the readings and the ends of answers, the account's every
// one held over the run, that came from from until until: its readings in
// the order they were read, as readFirst has it, each climbing the window
// as ledger.Climb says, as a day's summary counts its rise; and each climb
// shared out, by their tokens, among the requests it counts that ended by
// then since the reading before that took one, as windowShare's take has
// it. A request's own reading comes with its answer's first byte, before its
// output, which a later one counts. A window every model shares counts
// every request; a model's own, as Fable's week, only those of the models
// whose answers read it over the run.
func (a *accountShares) fold(answers []*answered, from, until instant) {
	models := modelsRead(answers)
	var reading, ending []*answered
	for _, r := range answers {
		if r.read.compare(from) >= 0 && r.read.compare(until) < 0 {
			reading = append(reading, r)
		}
		if r.end.compare(from) >= 0 && r.end.compare(until) < 0 {
			ending = append(ending, r)
		}
	}
	slices.SortFunc(reading, readFirst)
	slices.SortFunc(ending, func(x, y *answered) int { return x.end.compare(y.end) })
	next := 0
	for _, r := range reading {
		for ; next < len(ending) && ending[next].end.compare(r.read) <= 0; next++ {
			a.ended(ending[next])
		}
		at := r.read.time()
		for _, k := range r.windows {
			w := k.Window()
			a.window(w.Key, models).take(w, at)
		}
	}
	for _, r := range ending[next:] {
		a.ended(r)
	}
}

// modelsRead are the models whose answers among answers read each window,
// by its key.
func modelsRead(answers []*answered) map[string]map[unique.Handle[string]]bool {
	models := make(map[string]map[unique.Handle[string]]bool)
	for _, r := range answers {
		for _, k := range r.windows {
			key := k.Window().Key
			if models[key] == nil {
				models[key] = make(map[unique.Handle[string]]bool)
			}
			models[key][r.model] = true
		}
	}
	return models
}

// ended takes in r, a request that ended, among those each window it
// counts toward shares its next climb among.
func (a *accountShares) ended(r *answered) {
	for _, w := range a.windows {
		if w.countsToward(r.model) {
			w.pending.add(r)
		}
	}
	if r.ours {
		a.ours[r.model] = true
	}
}

// window is a's window with the given key, made where it has none yet, as
// it's first read, with the models given reading it, as modelsRead has
// them. The requests that ended before go uncounted, a window's first
// reading taken climbing it none, as ledger.Climb begins its run at it.
func (a *accountShares) window(key string, models map[string]map[unique.Handle[string]]bool) *windowShare {
	if w := a.windows[key]; w != nil {
		return w
	}
	w := &windowShare{climb: ledger.NewClimb(key, nil), shared: slices.Contains(claude.SharedWindows(), key), models: models[key]}
	a.windows[key] = w
	return w
}

// shares are the session's shares of each of a's windows' rises, by key: of
// each one the session's requests count toward.
func (a *accountShares) shares() map[string]float64 {
	shares := make(map[string]float64)
	for key, w := range a.windows {
		for model := range a.ours {
			if w.countsToward(model) {
				shares[key] = w.share
				break
			}
		}
	}
	return shares
}

func (e *tokensEnded) add(r *answered) {
	e.all += r.tokens
	if r.ours {
		e.ours += r.tokens
	}
}

func (w *windowShare) countsToward(model unique.Handle[string]) bool {
	return w.shared || w.models[model]
}

// take takes reading, the window's, read at at: its climb is shared out
// among the requests it counts that ended since the reading before that
// took one, by their tokens. A reading that says nothing of the window, as
// ledger.Climb says, counts none of them, leaving them to the next.
func (w *windowShare) take(reading quota.Window, at time.Time) {
	climbed, ok := w.climb.Take(reading, at)
	if !ok {
		return
	}
	all, ours := w.pending.all, w.pending.ours
	w.pending = tokensEnded{}
	if climbed > 0 && all > 0 {
		w.share += climbed * float64(ours) / float64(all)
	}
}

// earlier is the earlier of two instants.
func earlier(a, b instant) instant {
	if a.compare(b) <= 0 {
		return a
	}
	return b
}
