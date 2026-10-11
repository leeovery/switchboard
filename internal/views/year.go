package views

import (
	"cmp"
	"slices"
)

// Year is Year's grid over the days: each day's requests and the level its
// shade is drawn at, and the cuts between the levels, which the key gives in
// requests a day.
type Year struct {
	Cuts []int     `json:"cuts"`
	Days []YearDay `json:"days"`
}

// YearDay is a day of Year's grid: its requests, and its level, 0 to 4.
type YearDay struct {
	Day      string `json:"day"`
	Requests int    `json:"requests"`
	Level    int    `json:"level"`
}

// yearOf is Year's grid over days, shaded by their requests, as quarterCuts
// and shade say.
func yearOf(days []Day) Year {
	year := Year{Days: make([]YearDay, len(days))}
	var active []int
	for i, d := range days {
		n := d.requests()
		year.Days[i] = YearDay{Day: d.Day, Requests: n}
		if n > 0 {
			active = append(active, n)
		}
	}
	year.Cuts = quarterCuts(active)
	for i := range year.Days {
		year.Days[i].Level = shade(year.Days[i].Requests, year.Cuts)
	}
	return year
}

// quarterCuts are the cuts that split the active days into quarters by
// their values: of n, in order, those at n × q // 4, q from 1 to 3. None
// where no day is active.
func quarterCuts[V cmp.Ordered](active []V) []V {
	if len(active) == 0 {
		return []V{}
	}
	sorted := slices.Sorted(slices.Values(active))
	cuts := make([]V, 3)
	for q := range cuts {
		cuts[q] = sorted[len(sorted)*(q+1)/4]
	}
	return cuts
}

// shade is the level a day of the value given is drawn at: 0 for one with
// nothing on it, else 1 more than the cuts it reaches.
func shade[V cmp.Ordered](value V, cuts []V) int {
	var none V
	if value <= none {
		return 0
	}
	level := 1
	for _, cut := range cuts {
		if value >= cut {
			level++
		}
	}
	return level
}
