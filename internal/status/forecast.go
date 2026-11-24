package status

import (
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/forecast"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// playWeeks plays a pool's use forward, as forecast.Forecast does, and
// marginOf plays it for its margin, as forecast.MarginOf does: seams, so a
// test can count how often a document's weeks and margin are played.
var (
	playWeeks = forecast.Forecast
	marginOf  = forecast.MarginOf
)

// weeksAhead is where each account's shared week is heading, by the
// account's id, as a document's forecast has it, played as policy judges the
// pool; and how near the pool is to running out, as poolMargin plays it.
type weeksAhead struct {
	policy score.Policy
	outs   map[string]forecast.Out
	margin *marginAhead
}

// marginAhead is how near a document's pool is to running out, played the
// first time it's asked for, as it costs a dozen plays. It holds no pool and
// no func, which a pool's hold curve is, so a document worked out stays
// equal to itself worked out again while neither's margin has been read:
// the pool is built afresh from the document that asks, whose forecastPool
// is the one its weeks were played from, as nothing it reads changes after
// forecasted.
type marginAhead struct {
	once   sync.Once
	margin forecast.Margin
}

// forecasted is the document with its weeks played afresh, as policy judges
// the pool, as forecastPool has it, for every window and moment of it to
// read, and its margin to be: none where it has nothing to play.
func (d Document) forecasted(policy score.Policy) Document {
	d.weeks = nil
	if pool, ok := d.forecastPool(policy); ok {
		d.weeks = &weeksAhead{policy: policy, outs: playWeeks(pool), margin: &marginAhead{}}
	}
	return d
}

// poolMargin is how near the pool is to running out within the week ahead,
// as forecast.MarginOf plays the pool the document's weeks were played from,
// played once for every moment of it: none where its weeks aren't played.
func (d Document) poolMargin() forecast.Margin {
	if d.weeks == nil {
		return forecast.Margin{}
	}
	ahead := d.weeks.margin
	ahead.once.Do(func() {
		if pool, ok := d.forecastPool(d.weeks.policy); ok {
			ahead.margin = marginOf(pool)
		}
	})
	return ahead.margin
}

// withWeeks is the document with its weeks played, as forecasted has them,
// where they haven't been, as policy judges the pool: as it is where they
// have, or where it has nothing to play.
func (d Document) withWeeks(policy score.Policy) Document {
	if d.weeks != nil && d.weeks.policy.Equal(policy) {
		return d
	}
	return d.forecasted(policy)
}

// weekAhead is where the account with the given id's window w is heading, as
// the document's forecast has it, reporting false where the forecast doesn't
// say: of a window other than the shared week, as policy names it, of a
// document whose weeks aren't played, and of an account whose week's reset
// isn't known.
func (d Document) weekAhead(id string, w quota.Window, policy score.Policy) (forecast.Out, bool) {
	if d.weeks == nil || w.Key != policy.Perishable {
		return forecast.Out{}, false
	}
	out, ok := d.weeks.outs[id]
	return out, ok
}

// forecastPool is the pool the document's forecast plays, as policy judges
// it, from when the document was built, reporting false where it has nothing
// to play: no Demand, as a probed document has none, or no time it was
// built. Each of its accounts, in their order, has its shared week as it
// stands then, as WindowAt has it, so a week reset since it was read stands
// empty, with its next reset; its room ending at its floor, as floor has it;
// its size by its plan, as the price table's Sizes has every account's; the
// router's fast rate of its week; and its taking requests, new sessions and
// the use it holds alike, while it takes them as the document was built, as
// Takes says, held so for the whole play.
// The pool's use goes to the pool's pin's accounts while one holds, at the
// Demand's day rate, held on each account as its hold curve has it.
func (d Document) forecastPool(policy score.Policy) (forecast.Pool, bool) {
	if d.Demand == nil || d.GeneratedAt.IsZero() {
		return forecast.Pool{}, false
	}
	at := d.GeneratedAt
	plans := make([]string, len(d.Accounts))
	for i, a := range d.Accounts {
		plans[i] = a.Plan
	}
	sizes, _ := ledger.Pricing.Sizes(plans)
	accounts := make([]forecast.Account, len(d.Accounts))
	for i, a := range d.Accounts {
		week, _ := a.WindowAt(policy.Perishable, at, policy)
		accounts[i] = forecast.Account{
			ID: a.ID, Week: week, Reserve: a.Reserve, Floor: d.floor(a), Size: float64(sizes[i]),
			Fast: a.fast(policy.Perishable), Takes: a.Takes(at),
		}
	}
	return forecast.Pool{
		Now: at, Policy: policy, Accounts: accounts, Pinned: d.Pin.Accounts, Day: d.Demand.Day, Held: d.Demand.curve(),
	}, true
}

// curve is the hold curve the forecast plays by, as forecast.Curve draws it
// through the Demand's points.
func (d Demand) curve() func(after time.Duration) float64 {
	points := make([]forecast.Point, len(d.Hold))
	for i, p := range d.Hold {
		points[i] = forecast.Point{After: time.Duration(p.Hours * float64(time.Hour)), Share: p.Share}
	}
	return forecast.Curve(points)
}

// headingOf is where a week heads as the forecast has it, out, and when it
// reaches where its account's room ends: running out then, where it does
// before it resets, as the forecast plays it to there already; else on pace
// for its use at its reset.
func headingOf(out forecast.Out) (Heading, time.Time) {
	if out.RunsOut.IsZero() {
		return Heading{Kind: score.OnPace, AtReset: out.AtReset}, time.Time{}
	}
	return Heading{Kind: score.RunsOut, At: out.RunsOut}, out.RunsOut
}
