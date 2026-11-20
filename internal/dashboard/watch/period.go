package watch

import (
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// todayHour is the local hour now falls in, where the period is today at
// now, as today's charts mark it now's, else -1, as of a day gone, whose
// hours are its own: what a page of today is built of besides, so it's
// built afresh as the hour turns, though nothing arrives.
func todayHour(period views.Period, now time.Time, starts time.Weekday) int {
	if _, gone := period.Until(now, starts); gone || period.Length() != views.LengthToday {
		return -1
	}
	return now.Local().Hour()
}

// dial is the dial's period as it's said at now, as views.DialOf says it,
// its floor where the pool shown's work of the ledger begins, as begun holds
// it.
func (m Model) dial(now time.Time) views.Dial {
	return views.DialOf(m.period, now, m.cfg.WeekStarts, m.begun.began)
}

// dialShown reports whether the tab shown has the dial: Accounts, Projects,
// and Sessions' List.
func (m Model) dialShown() bool {
	switch m.shownView() {
	case dashboard.Accounts, dashboard.Projects:
		return true
	}
	return m.listShown()
}

// lengthKeyed are the dial's keys that change its period's length, each as
// it does: - a shorter, + a longer, and = as +, so + needs no shift on a Mac.
var lengthKeyed = map[string]func(views.Period, time.Time, time.Weekday) (views.Period, bool){
	"-": views.Period.Shorter,
	"+": views.Period.Longer,
	"=": views.Period.Longer,
}

// periodStepped is m with the dial's period stepped by the key given at now,
// reporting false for a key that isn't the dial's: < back in time, as Back
// has it, never past the ledger's first day; > on, as On has it, never past
// now; - and + to a shorter or a longer period, as lengthKeyed has them,
// the length kept in the preferences for the next watch to open with; and n
// straight back to now, keeping the length. A step that can't go further,
// at either end, leaves m as it is; < > and n keep nothing, as the
// dashboard always opens on now. A step shows the period as periodShown has
// it.
func (m Model) periodStepped(key string) (Model, bool) {
	now, starts := m.now(), m.cfg.WeekStarts
	var next views.Period
	var stepped bool
	switch key {
	case "<":
		next, stepped = m.period.Back(now, starts, m.begun.began)
	case ">":
		next, stepped = m.period.On(now, starts)
	case "n":
		next, stepped = m.period.Length().Now(), m.dial(now).Gone()
	default:
		lengthened, ok := lengthKeyed[key]
		if !ok {
			return m, false
		}
		if next, stepped = lengthened(m.period, now, starts); stepped {
			length := next.Length()
			m = m.keepPrefs("the period", func(prefs *theme.Prefs) { dashboard.KeepLength(prefs, length) }, "period", string(length))
		}
	}
	if stepped {
		m = m.periodShown(next)
	}
	return m, true
}

// periodShown is m showing the period given, the page built afresh as
// accountsBuilt has it. The view's shown from its top but on Projects, whose
// SESSIONS keeps its place for projectsBuilt to judge and keep the session
// selected in view by; on Sessions' List, which keeps its place for
// sessionsLooked to judge alike; and on Detail with its SESSIONS held for
// the focus, which keeps its place for sessionsKept to judge.
func (m Model) periodShown(period views.Period) Model {
	m.period = period
	if m.shownView() != dashboard.Projects && !m.listShown() && !m.sessionsHeld() {
		m.scroll = 0
	}
	return m
}

// The dial's keys as the key line and help name them: - and + its length,
// and < and > its steps back and on.
const (
	lengthKeys = "- +"
	stepKeys   = "< >"
)

// dialListings are the dial's keys, as the key line and help list them on a
// tab the dial shows on, where works says, the dial's period said as d says
// it, each in a group of its own: - + and the period's name; < > back · on;
// and while a past period shows, n now. At now, a phone's line names < >
// alone, and only where phoned says, as on Projects' list and the List,
// never Accounts; while a past period shows, it names - +, its name left
// off, which gives way first, and keeps < > and n to the last, as
// dashboard.KeyLine's PhoneKept keeps them.
func dialListings(d views.Dial, works, phoned bool) []keyListing {
	gone := d.Gone()
	return []keyListing{
		{Key: dashboard.Key{Key: lengthKeys, Does: "period", State: d.Name()}, works: works, apart: true, wide: !gone, phoneDoes: "period"},
		{Key: dashboard.Key{Key: stepKeys, Does: "back · on"}, works: works, apart: true, wide: !gone && !phoned, kept: gone},
		{Key: dashboard.Key{Key: "n", Does: "now"}, works: works && gone, apart: true, kept: true},
	}
}

// dialNarrower are the narrower forms of a wide key line of the groups
// given, the dial's keys among them, in the order it gives way where the
// one before doesn't fit, before a phone's line takes its place: the
// period's name after - + period left off; then p pin; then - + period
// whole.
func dialNarrower(groups [][]dashboard.Key) [][][]dashboard.Key {
	unnamed := keysKept(groups, func(k dashboard.Key) (dashboard.Key, bool) {
		if k.Key == lengthKeys {
			k.State = ""
		}
		return k, true
	})
	unpinned := keysKept(unnamed, func(k dashboard.Key) (dashboard.Key, bool) { return k, k.Key != pinWord.Key })
	return [][][]dashboard.Key{unnamed, unpinned, keysKept(unpinned, func(k dashboard.Key) (dashboard.Key, bool) { return k, k.Key != lengthKeys })}
}

// keysKept are the groups given, each key as keep has it, but those it
// reports false of, a group left empty where it keeps none of it.
func keysKept(groups [][]dashboard.Key, keep func(dashboard.Key) (dashboard.Key, bool)) [][]dashboard.Key {
	kept := make([][]dashboard.Key, len(groups))
	for i, group := range groups {
		for _, k := range group {
			if k, ok := keep(k); ok {
				kept[i] = append(kept[i], k)
			}
		}
	}
	return kept
}
