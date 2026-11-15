package watch

import (
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/views"
)

// trendsPast is what the watch holds of the pages Trends draws of the
// ledger's days: what v has YEAR show, the run's alone; YEAR's and BY
// MONTH's page, the last 12 months shown so, and what it was built of; and
// MODELS BY DAY's, the last eight weeks, and what it was built of.
type trendsPast struct {
	split  views.YearAsk
	year   *views.YearPage
	yearOf pastOf
	days   *views.DaysPage
	daysOf pastOf
}

// pastOf is what a page of Trends was built of: the last 12 months' days
// held, as historyTab counts them as they change, and the local day it was
// built on; and of YEAR's, what v had it show.
type pastOf struct {
	held  int
	day   string
	split views.YearAsk
}

// trendsDaysWanted reports whether Trends wants the ledger's days: while it
// shows, WHEN YOU WORK, YEAR, BY MONTH or MODELS BY DAY among its sections.
// History's reads read them, and Trends shares what they hold, as Accounts
// does.
func (m Model) trendsDaysWanted() bool {
	return m.graphsHoursWanted() || m.yearWanted() || m.modelsWanted()
}

// yearWanted reports whether Trends shows YEAR or BY MONTH, which draw the
// last 12 months as v has them show.
func (m Model) yearWanted() bool {
	return m.shownView() == dashboard.Graphs && (m.graphs.Has(dashboard.GraphsYear) || m.graphs.Has(dashboard.GraphsMonths))
}

// modelsWanted reports whether Trends shows MODELS BY DAY.
func (m Model) modelsWanted() bool {
	return m.shownView() == dashboard.Graphs && m.graphs.Has(dashboard.GraphsModels)
}

// trendsBuilt is m with Trends' pages of the ledger's days built afresh at
// now of the last 12 months' days held, brought up to now, as Accounts' are,
// each where what it's built of has changed since, while it shows: YEAR's
// and BY MONTH's, as views.Past.Year builds it shown as v has it, showing
// all use where v had it show a version the days don't use; and MODELS BY
// DAY's, as views.Past.Days builds it. Once for each, never a frame at a
// time: where nothing it's built of has changed, the days aren't touched.
func (m Model) trendsBuilt() Model {
	if !m.past.last.held() {
		return m
	}
	now := m.now()
	t, of := m.trends, pastOf{held: m.past.held, day: dayfile.DateOf(now)}
	yearOf := of
	yearOf.split = t.split
	year := m.yearWanted() && (t.year == nil || t.yearOf != yearOf)
	models := m.modelsWanted() && (t.days == nil || t.daysOf != of)
	if !year && !models {
		return m
	}
	past := m.past.last.past.Since(nil, now)
	if year {
		t.split = usedAsk(t.split, past)
		yearOf.split = t.split
		t.year, t.yearOf = new(past.Year(t.split)), yearOf
	}
	if models {
		t.days, t.daysOf = new(past.Days()), of
	}
	m.trends = t
	return m
}

// trendsKey acts on v and ← → on Trends, while YEAR or BY MONTH shows,
// reporting false for another key, or elsewhere: v cycles what YEAR shows,
// all use, each version, then each account, as dashboard.GraphsShown's
// Cycled has it; ← and →, while it shows one account, step to the account
// before and after, round, as its Moved has it.
func (m Model) trendsKey(key string) (Model, bool) {
	if !m.yearWanted() {
		return m, false
	}
	var next dashboard.GraphsShown
	var changed bool
	switch key {
	case "v":
		next, changed = m.graphsShown().Cycled()
	case "left", "right":
		next, changed = m.graphsShown().Moved(historySteps[key])
	default:
		return m, false
	}
	if changed {
		m.trends.split = next.Split
	}
	return m, true
}
