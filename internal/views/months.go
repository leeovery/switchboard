package views

import (
	"strings"
	"time"
)

// monthLayout lays out a month as a Month names it, as 2026-10.
const monthLayout = "2006-01"

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

// monthsOf are BY MONTH's rows over days, asked for from the day with the
// date from, a month each. A month costs its plans' monthly prices whole, as
// of its first day, the month the ledger began part-way through among them,
// but for one the days asked for begin part-way through, as cutShort says;
// the month so far, that of the day with the date today, has no ratio, as
// it's still running.
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
		first, _ := civil(s.key + "-01")
		priced, at := cutShort(s.key+"-01", first.AddDate(0, 1, -1).Format(time.DateOnly), from, aMonth)
		m.Against = p.against(accountWorths(s.days), at, priced)
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
	day, ok := civil(d.Day)
	return day.Format(monthLayout), ok
}
