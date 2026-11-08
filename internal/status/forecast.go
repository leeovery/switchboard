package status

import (
	"time"

	"github.com/leeovery/switchboard/internal/forecast"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// playWeeks plays a pool's use forward, as forecast.Forecast does: a seam, so
// a test can count how often a document's weeks are played.
var playWeeks = forecast.Forecast

// weeksAhead is where each account's shared week is heading, by the
// account's id, as a document's forecast has it, played as policy judges the
// pool.
type weeksAhead struct {
	policy score.Policy
	outs   map[string]forecast.Out
}

// forecasted is the document with its weeks played afresh, as policy judges
// the pool, as forecastPool has it, for every window and moment of it to
// read: none where it has nothing to play.
func (d Document) forecasted(policy score.Policy) Document {
	d.weeks = nil
	if pool, ok := d.forecastPool(policy); ok {
		d.weeks = &weeksAhead{policy: policy, outs: playWeeks(pool)}
	}
	return d
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
// its size by its plan, as forecast.Sizes has every account's; the router's
// fast rate of its week; and its taking requests, new sessions and the use it
// holds alike, while its token is usable and not refused as the document was
// built, held so for the whole play. The pool's use goes to the global pin's
// accounts while one holds, at the Demand's day rate, held on each account as
// its hold curve has it.
func (d Document) forecastPool(policy score.Policy) (forecast.Pool, bool) {
	if d.Demand == nil || d.GeneratedAt.IsZero() {
		return forecast.Pool{}, false
	}
	at := d.GeneratedAt
	plans := make([]string, len(d.Accounts))
	for i, a := range d.Accounts {
		plans[i] = a.Plan
	}
	sizes := forecast.Sizes(plans)
	accounts := make([]forecast.Account, len(d.Accounts))
	for i, a := range d.Accounts {
		week, _ := a.WindowAt(policy.Perishable, at, policy)
		accounts[i] = forecast.Account{
			ID: a.ID, Week: week, Reserve: a.Reserve, Floor: d.floor(a), Size: sizes[i],
			Fast: a.fast(policy.Perishable), Takes: a.TokenSet && !a.tokenRefused(at),
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
