package events

import (
	"testing"
	"time"
)

func TestAMergeHoldsNoMoreThanTheDaysAnEventCanChangeOver(t *testing.T) {
	const days = 60
	first := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	run := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	var handed int
	m := merge{from: first, to: first.AddDate(0, 0, days), held: make(map[key]Line), yield: func(Line) bool {
		handed++
		return true
	}}

	for day := range days {
		at := first.AddDate(0, 0, day)
		// Each day's file holds its own event, and a version of each event
		// of the 8 days before, as each changes.
		for back := range ChangesFor + 1 {
			if day >= back {
				m.hold(Line{ID: day - back + 1, At: at.AddDate(0, 0, -back), Kind: "limit", Run: run})
			}
		}
		m.dayRead(at.Format(time.DateOnly))
		if len(m.held) > ChangesFor+1 {
			t.Fatalf("after day %d, %d events held, want no more than the %d of that day and the 8 before", day, len(m.held), ChangesFor+1)
		}
	}
	m.handOn(func(Line) bool { return true })
	if handed != days {
		t.Errorf("%d events handed on, want each of the %d once", handed, days)
	}
}
