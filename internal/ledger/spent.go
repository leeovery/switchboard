package ledger

import (
	"cmp"
	"slices"
	"strings"
)

// Spent is what requests spent, summed by the model each asked for and the
// geo it asked its inference to run in, as Add takes each in: counts alone,
// never a price, so a sum of requests is kept as tokens and priced only as
// it's shown, by the table and at the day it's shown at, as SummedSpent
// prices it. Priced, it's exactly what its requests add to a sum of
// requests' worth, each priced on its own as SummedSpend prices it. So of
// each model and geo it keeps what pricing reads: each count a price charges
// for, summed by its charge; the paths of the counts none charges for, each
// once; whether any request held a count that costs anything, and whether
// any went upstream without usage, as wholly names them; and that it holds
// any of their requests at all, as a sum holding one the table prices is
// priced, at nothing where it spent nothing. A copy of a Spent is its own:
// adding to one never changes another. Its zero value holds no request.
type Spent struct {
	// groups are what the requests of each model and geo spent, ordered by
	// model, then geo, as compareGroups orders them.
	groups []spentGroup
}

// spentGroup is what requests of the model with the given id that asked for
// the geo given spent, as pricing reads it: the bill of their usage, each
// charge's counts summed and each path it leaves unpriced once, in order;
// whether any held a count that costs anything; and whether any went
// upstream without usage. A request priced on its own is a group of one.
type spentGroup struct {
	model, geo        string
	billed            bill
	costly, unmetered bool
}

// spentOf is what a request that spent s, of the model with the given id
// that asked for the geo given, adds to a Spent: none of a quota check's
// usage, as worthy has it, but the request all the same.
func spentOf(model, geo string, s Spend) spentGroup {
	s = s.worthy()
	return spentGroup{model: model, geo: geo, billed: s.all, costly: s.costly, unmetered: s.unmetered}
}

// Add takes in what a request spent, s, of the model with the given id that
// asked for the geo given, as SummedSpend would price it.
func (sp *Spent) Add(model, geo string, s Spend) {
	sp.with(spentOf(model, geo, s))
}

// with takes in what the requests g sums spent, among those of its model and
// geo, into groups of its own, never writing where a copy of sp may read.
func (sp *Spent) with(g spentGroup) {
	i, found := slices.BinarySearchFunc(sp.groups, g, compareGroups)
	var groups []spentGroup
	if found {
		groups = slices.Clone(sp.groups)
	} else {
		groups = slices.Insert(slices.Clip(sp.groups), i, spentGroup{model: g.model, geo: g.geo})
	}
	groups[i] = groups[i].and(g)
	sp.groups = groups
}

// and is what g's requests and o's spent together, of the same model and
// geo: each charge's counts summed, each path left unpriced once, in order,
// and each flag set where either's is.
func (g spentGroup) and(o spentGroup) spentGroup {
	for c, n := range o.billed.charged {
		g.billed.charged[c] += n
	}
	g.billed.unpriced = withPaths(g.billed.unpriced, o.billed.unpriced)
	g.costly, g.unmetered = g.costly || o.costly, g.unmetered || o.unmetered
	return g
}

// compareGroups orders a Spent's groups: by model, then geo.
func compareGroups(a, b spentGroup) int {
	return cmp.Or(strings.Compare(a.model, b.model), strings.Compare(a.geo, b.geo))
}

// withPaths is paths, each once and in order, with those of more among
// them: paths itself where it holds each already, else all of them afresh,
// each once and in order, so what shares paths never sees it change.
func withPaths(paths, more []string) []string {
	if !slices.ContainsFunc(more, func(path string) bool { _, found := slices.BinarySearch(paths, path); return !found }) {
		return paths
	}
	all := slices.Concat(paths, more)
	slices.Sort(all)
	return slices.Compact(all)
}

// SummedSpent returns what the requests sp holds add to a sum of requests'
// worth at the prices in effect on the local day with the given date,
// reporting whether the table priced any of them: exactly what they add
// each priced on its own, as SummedSpend gives it, summed, its cost that of
// those priced and each name they leave unpriced once, in order. Summing a
// group's counts before pricing them gives the same cost as pricing each
// request's and summing those, even past what int64 holds, as both wrap
// alike.
func (t Table) SummedSpent(sp Spent, date string) (Worth, bool) {
	var sum Worth
	priced := false
	for _, g := range sp.groups {
		w, ok := t.summedOf(g, date)
		sum.Cost += w.Cost
		sum.Unpriced = withPaths(sum.Unpriced, w.Unpriced)
		priced = priced || ok
	}
	return sum, priced
}

// summedOf returns what the requests g sums add to a sum of requests' worth
// at the prices in effect on the local day with the given date, reporting
// whether the table priced them: their worth, where it did; or none, naming
// what they leave unpriced as wholly says, where it didn't.
func (t Table) summedOf(g spentGroup, date string) (Worth, bool) {
	if w, ok := t.priced(g.model, g.geo, g.billed, g.unmetered, date); ok {
		return w, true
	}
	return Worth{Unpriced: t.wholly(g.model, g.geo, g.costly, g.unmetered, date)}, false
}
