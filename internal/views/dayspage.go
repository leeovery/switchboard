package views

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// DaysPage is MODELS BY DAY as Trends draws it: the eight weeks to this
// one, a day at a time, a strip a family, on one scale.
type DaysPage struct {
	// Weeks are the eight weeks' first days' dates, oldest first, this week
	// last, and ToCome how many of this week's days are still to come.
	Weeks  []string
	ToCome int
	// ByModel are the families used in the eight weeks, a strip each, in the
	// version table's order.
	ByModel []DaysStrip
}

// DaysStrip is a family's strip: Of, the family's name, as the version table
// gives it; its days, the eight weeks' from the first, those still to come
// among them; and Share, its share of the eight weeks' worth.
type DaysStrip struct {
	Of    string
	Days  []StripDay
	Share *float64
}

// StripDay is a day of a strip: its level, 0 to 4, on the strips' scale; and
// of an account's, as Accounts' BY DAY draws it, whether it hit a 5-hour
// limit that day.
type StripDay struct {
	Level int
	Limit bool
}

// Recorded reports whether anything is recorded in the eight weeks: a
// family's use.
func (p DaysPage) Recorded() bool {
	return len(p.ByModel) > 0
}

// weekDays are the days of some weeks, as Days and Accounts' BY DAY show
// them, from the first day of the first week to the last of the last, those
// to come among them: each day's summary, nil where there's none, and the
// index of the last that's come, today's, or the weeks' last where they end
// before today.
type weekDays struct {
	first   time.Time
	days    []*Day
	through int
}

// weekDaysOf are the days of the n weeks to the one beginning on last, as
// dayfile.Civil gives a day, now's or one before it, from the day weeks
// start on, of days, which run to today: none to come where they end before
// today's week.
func weekDaysOf(days []Day, now time.Time, starts time.Weekday, n int, last time.Time) weekDays {
	today := dayfile.CivilOf(now)
	w := weekDays{first: weeksTo(last, starts, n), days: make([]*Day, 7*n)}
	w.through = min(w.index(today), len(w.days)-1)
	for i := range days {
		if day, ok := dayfile.Civil(days[i].Day); ok && !day.Before(w.first) && w.index(day) <= w.through {
			w.days[w.index(day)] = &days[i]
		}
	}
	return w
}

// toCome is how many of the weeks' days are still to come, after today.
func (w weekDays) toCome() int {
	return len(w.days) - 1 - w.through
}

// index is the place among the weeks' days of the day given, as
// dayfile.Civil gives it.
func (w weekDays) index(day time.Time) int {
	return int(day.Sub(w.first) / (24 * time.Hour))
}

// daysPageOf is Days' page of days, which run to now's day, priced: the eight
// weeks to now's, from the day weeks start on.
func daysPageOf(in HistoryInput, days []Day) DaysPage {
	w := weekDaysOf(days, in.Now, in.WeekStarts, weeksShown, weekOf(dayfile.CivilOf(in.Now), in.WeekStarts))
	return DaysPage{
		Weeks:   weekDates(w.first, weeksShown),
		ToCome:  w.toCome(),
		ByModel: w.byFamily(versions{table: in.Prices, family: in.Family}),
	}
}

// familyDays are a family's eight weeks: what its requests were worth each
// day, and in all; and whether any went out.
type familyDays struct {
	worth []Worth
	sum   Worth
	used  bool
}

// amounts are what its requests were worth each day, as amount has it.
func (f *familyDays) amounts() []ledger.Picodollars {
	amounts := make([]ledger.Picodollars, len(f.worth))
	for i, w := range f.worth {
		amounts[i] = w.amount()
	}
	return amounts
}

// byFamily are BY MODEL's strips: a family each that had requests in the
// eight weeks, in the version table's order, each day its worth, every
// account's, the strips on one scale; and each its share of the eight weeks'
// worth, none where none of it is priced.
func (w weekDays) byFamily(v versions) []DaysStrip {
	families := make(map[string]*familyDays)
	for i, d := range w.days {
		if d == nil {
			continue
		}
		for _, a := range d.Accounts {
			for _, m := range a.Models {
				w.addModel(families, v, i, m)
			}
		}
	}
	names := slices.SortedFunc(maps.Keys(families), func(a, b string) int {
		return cmp.Or(cmp.Compare(v.familyRank(a), v.familyRank(b)), cmp.Compare(a, b))
	})
	names = slices.DeleteFunc(names, func(name string) bool { return !families[name].used })
	var total Worth
	var active []ledger.Picodollars
	amounts := make([][]ledger.Picodollars, len(names))
	for i, name := range names {
		total.add(families[name].sum)
		amounts[i] = families[name].amounts()
		active = appendActive(active, amounts[i])
	}
	cuts := quarterCuts(active)
	strips := make([]DaysStrip, len(names))
	for i, name := range names {
		strips[i] = DaysStrip{Of: name, Days: levelled(amounts[i], cuts), Share: shareOf(families[name].sum, total.amount())}
	}
	return strips
}

// addModel adds a model's day, the i-th of the eight weeks, to its
// family's: none of the requests whose bodies couldn't be read, which name
// no model.
func (w weekDays) addModel(families map[string]*familyDays, v versions, i int, m ledger.PricedModel) {
	if m.Model == "" {
		return
	}
	name := v.familyOf(m.Model)
	f := families[name]
	if f == nil {
		f = &familyDays{worth: make([]Worth, len(w.days))}
		families[name] = f
	}
	f.worth[i].addModel(m)
	f.sum.addModel(m)
	f.used = f.used || used(m.ModelDay)
}

// risesOf are, of the account with the given id, each day's rise of the
// window with the key week, the points of its week it used that day, and the
// 5-hour limits it hit that day.
func (w weekDays) risesOf(id, week string) (rises []float64, limits []int) {
	rises, limits = make([]float64, len(w.days)), make([]int, len(w.days))
	for d, day := range w.days {
		own := accountOn(day, id)
		rises[d], limits[d] = own.Rise[week], fiveHourLimits(own.Limits)
	}
	return rises, limits
}

// accountOn is the day of the account with the given id on the day given:
// the zero day where it has none, or there's no day.
func accountOn(day *Day, id string) ledger.AccountDay {
	if day == nil {
		return ledger.AccountDay{}
	}
	i := slices.IndexFunc(day.Accounts, func(a ledger.PricedAccount) bool { return a.Account == id })
	if i < 0 {
		return ledger.AccountDay{}
	}
	return day.Accounts[i].AccountDay
}

// fiveHourLimits counts the limits of an account's 5-hour window among
// limits.
func fiveHourLimits(limits []ledger.Limit) int {
	n := 0
	for _, l := range limits {
		if l.Window == claude.FiveHourWindow {
			n++
		}
	}
	return n
}

// appendActive appends to active the values of days that are active, those
// shade draws above level 0.
func appendActive[V cmp.Ordered](active, values []V) []V {
	for _, value := range values {
		if shade(value, nil) > 0 {
			active = append(active, value)
		}
	}
	return active
}

// levelled are a strip's days, each at the level its value is shaded at on
// the scale the cuts give.
func levelled[V cmp.Ordered](values []V, cuts []V) []StripDay {
	days := make([]StripDay, len(values))
	for i, value := range values {
		days[i].Level = shade(value, cuts)
	}
	return days
}
