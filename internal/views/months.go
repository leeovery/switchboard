package views

import (
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// Month is a row of BY MONTH: a calendar month's requests, what they were
// worth, what the plans cost that month and the worth against it, the
// limits reached in it, by window, and its worth by family.
type Month struct {
	Month    string `json:"month"`
	Requests int    `json:"requests"`
	Worth
	Against
	Limits   map[string]int `json:"limits,omitempty"`
	ByFamily []FamilyWorth  `json:"by_family,omitempty"`
}

// LimitCount is how many limits were reached in the month, every account's
// and every window's together, as AccountTotals.LimitCount counts them.
func (m Month) LimitCount() int {
	return limitCount(m.Limits)
}

// monthsOf are BY MONTH's rows over days, counted from the day with the
// date from, a month each. A month costs its plans' monthly prices whole, as
// of its first day, but for the one the days counted begin part-way through,
// as the month a young ledger began in, as pricedOver prices it; the month
// so far, that of the day with the date today, has no ratio, as it's still
// running.
func monthsOf(days []Day, p plans, v versions, from, today string) []Month {
	months := []Month{}
	for _, s := range spansOf(days, monthKey) {
		m := Month{Month: s.key}
		sums := familySums{}
		for _, d := range s.days {
			m.addDay(d)
			for _, f := range d.ByFamily {
				sums.add(f.Family, f.Worth)
			}
		}
		m.ByFamily = v.byFamily(sums)
		first, _ := dayfile.MonthStart(s.key)
		m.Against = p.against(accountWorths(s.days), pricedOver(s.key+"-01", first.AddDate(0, 1, -1).Format(time.DateOnly), from, aMonth))
		if strings.HasPrefix(today, s.key+"-") {
			m.Ratio = nil
		}
		months = append(months, m)
	}
	return months
}

// addDay adds the day's requests, worth and limits to the month's.
func (m *Month) addDay(d Day) {
	for _, a := range d.Accounts {
		for _, l := range a.Limits {
			if m.Limits == nil {
				m.Limits = make(map[string]int)
			}
			m.Limits[l.Window]++
		}
		for _, model := range a.Models {
			m.Requests += model.Requests()
			m.addModel(model)
		}
	}
}

// monthKey is the month the day falls in, as Month names it.
func monthKey(d Day) (string, bool) {
	return monthOf(d.Day)
}

// monthOf is the month the day with the date given falls in, as Month names
// it, as dayfile.MonthOf gives one, reporting false for a date that isn't
// one.
func monthOf(date string) (string, bool) {
	day, ok := dayfile.Civil(date)
	return dayfile.MonthOf(day), ok
}
