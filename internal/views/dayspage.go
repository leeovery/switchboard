package views

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
)

// DaysPage is Days as the dashboard draws it: the eight weeks to this one, a
// day at a time; BY MODEL's strips, a family each, on one scale; and BY
// ACCOUNT's, an account each, on another.
type DaysPage struct {
	// Weeks are the eight weeks' first days' dates, oldest first, this week
	// last, and ToCome how many of this week's days are still to come.
	Weeks  []string
	ToCome int
	// ByModel are the families used in the eight weeks, a strip each, in the
	// version table's order; ByAccount the configured accounts, in the
	// config's.
	ByModel   []DaysStrip
	ByAccount []DaysStrip
}

// DaysStrip is a strip of Days, a family's or an account's: Of, the family's
// name, as the version table gives it, or the account's id; its days, the
// eight weeks' from the first, those still to come among them; of a family,
// Share, its share of the eight weeks' worth; and of an account, Limits, the
// 5-hour limits it hit.
type DaysStrip struct {
	Of     string
	Days   []StripDay
	Share  *float64
	Limits int
}

// StripDay is a day of a strip: its level, 0 to 4, on the strips' scale; and
// of an account's, whether it hit a 5-hour limit that day.
type StripDay struct {
	Level int
	Limit bool
}

// Recorded reports whether anything is recorded in the eight weeks: a
// family's use, or an account's week rising or its 5-hour limit.
func (p DaysPage) Recorded() bool {
	return len(p.ByModel) > 0 || slices.ContainsFunc(p.ByAccount, func(s DaysStrip) bool {
		return s.Limits > 0 || slices.ContainsFunc(s.Days, func(d StripDay) bool { return d.Level > 0 })
	})
}

// daysWeeks are the weeks Days shows: the week so far and the seven before
// it.
const daysWeeks = 8

// eightWeeks are Days' days, from the first day of the week seven before
// this one to the last of this one, those to come among them: each day's
// summary, nil where there's none, and the index of today's.
type eightWeeks struct {
	first time.Time
	days  []*Day
	today int
}

// eightWeeksOf are the eight weeks to now's, from the day weeks start on, of
// days, which run to today.
func eightWeeksOf(days []Day, now time.Time, starts time.Weekday) eightWeeks {
	today := dateOf(now)
	w := eightWeeks{first: weekOf(today, starts).AddDate(0, 0, -7*(daysWeeks-1)), days: make([]*Day, 7*daysWeeks)}
	w.today = w.index(today)
	for i := range days {
		if day, ok := civil(days[i].Day); ok && !day.Before(w.first) && !day.After(today) {
			w.days[w.index(day)] = &days[i]
		}
	}
	return w
}

// index is the place among the eight weeks' days of the day given, as civil
// gives it.
func (w eightWeeks) index(day time.Time) int {
	return int(day.Sub(w.first) / (24 * time.Hour))
}

// weeks are the eight weeks' first days' dates, oldest first.
func (w eightWeeks) weeks() []string {
	weeks := make([]string, daysWeeks)
	for i := range weeks {
		weeks[i] = w.first.AddDate(0, 0, 7*i).Format(time.DateOnly)
	}
	return weeks
}

// daysPageOf is Days' page of days, which run to now's day, priced: the eight
// weeks to now's, from the day weeks start on.
func daysPageOf(in HistoryInput, days []Day) DaysPage {
	w := eightWeeksOf(days, in.Now, in.WeekStarts)
	return DaysPage{
		Weeks:     w.weeks(),
		ToCome:    len(w.days) - 1 - w.today,
		ByModel:   w.byFamily(versions{table: in.Prices, family: in.Family}),
		ByAccount: w.byAccount(in.Accounts, in.WeekWindow),
	}
}

// familyDays are a family's eight weeks: what its requests were worth each
// day, and in all; and whether any went out.
type familyDays struct {
	worth []ledger.Picodollars
	sum   Worth
	used  bool
}

// byFamily are BY MODEL's strips: a family each that had requests in the
// eight weeks, in the version table's order, each day its worth, every
// account's, the strips on one scale; and each its share of the eight weeks'
// worth, none where none of it is priced.
func (w eightWeeks) byFamily(v versions) []DaysStrip {
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
	var total ledger.Picodollars
	var active []ledger.Picodollars
	for _, name := range names {
		total += families[name].sum.amount()
		active = appendActive(active, families[name].worth)
	}
	cuts := quarterCuts(active)
	strips := make([]DaysStrip, len(names))
	for i, name := range names {
		f := families[name]
		strips[i] = DaysStrip{Of: name, Days: levelled(f.worth, cuts), Share: shareOf(f.sum, total)}
	}
	return strips
}

// addModel adds a model's day, the i-th of the eight weeks, to its
// family's: none of the requests whose bodies couldn't be read, which name
// no model.
func (w eightWeeks) addModel(families map[string]*familyDays, v versions, i int, m ledger.PricedModel) {
	if m.Model == "" {
		return
	}
	name := v.familyOf(m.Model)
	f := families[name]
	if f == nil {
		f = &familyDays{worth: make([]ledger.Picodollars, len(w.days))}
		families[name] = f
	}
	if m.Worth != nil {
		f.worth[i] += *m.Worth
	}
	f.sum.addModel(m)
	f.used = f.used || m.Requests() > 0
}

// shareOf is a family's worth, sum, as a share of total, every family's:
// none where none of either is priced.
func shareOf(sum Worth, total ledger.Picodollars) *float64 {
	if sum.Amount == nil || total <= 0 {
		return nil
	}
	share := float64(*sum.Amount) / float64(total)
	return &share
}

// byAccount are BY ACCOUNT's strips: a configured account each, in the
// config's order, each day the points of its week, the window with the key
// week, it used that day, every account on one scale; and the 5-hour limits
// it hit, each day and in all.
func (w eightWeeks) byAccount(accounts config.Accounts, week string) []DaysStrip {
	rises := make([][]float64, len(accounts))
	limits := make([][]int, len(accounts))
	var active []float64
	for i, a := range accounts {
		rises[i], limits[i] = make([]float64, len(w.days)), make([]int, len(w.days))
		for d, day := range w.days {
			own := accountOn(day, a.ID)
			rises[i][d], limits[i][d] = own.Rise[week], fiveHourLimits(own.Limits)
		}
		active = appendActive(active, rises[i])
	}
	cuts := quarterCuts(active)
	strips := make([]DaysStrip, len(accounts))
	for i, a := range accounts {
		strips[i] = DaysStrip{Of: a.ID, Days: levelled(rises[i], cuts)}
		for d, n := range limits[i] {
			strips[i].Days[d].Limit = n > 0
			strips[i].Limits += n
		}
	}
	return strips
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

// appendActive appends to active the values of days that are active, above
// nothing.
func appendActive[V cmp.Ordered](active, values []V) []V {
	var none V
	for _, value := range values {
		if value > none {
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
