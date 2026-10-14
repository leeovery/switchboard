package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// gridDay is the day with the given date of the Year's grid: its requests,
// and whether it's left out, as a day to come is; reporting false where the
// grid has no such day.
func gridDay(page views.YearPage, date string) (views.GridDay, bool) {
	for _, w := range page.Weeks {
		for _, d := range w.Days {
			if d.Day == date {
				return d, true
			}
		}
	}
	return views.GridDay{}, false
}

// Since takes the days given in place of those the Past held of them, from
// the first given on, as a look reads today's afresh: today's counts what it
// has grown to, and the days before it stand as they were.
func TestSinceTakesTheDaysGivenInPlaceOfThoseHeld(t *testing.T) {
	past := views.NewPast(input(overTheYear), overTheYear)
	today := day("2026-10-07", usedBy("work", sessionsOf(sent("claude-opus-5-5", 5, inputUsage), sessionE)))

	page := past.Since([]ledger.Summary{today}, now).Year(views.YearAsk{})
	if got, _ := gridDay(page, "2026-10-07"); got.Requests != 5 {
		t.Errorf("today has %d requests, want 5, as read again", got.Requests)
	}
	if got, _ := gridDay(page, "2025-10-08"); got.Requests != 10 {
		t.Errorf("8 October 2025 has %d requests, want its 10 standing", got.Requests)
	}
	if was, is := past.Year(views.YearAsk{}).Numbers.Requests, page.Numbers.Requests; is != was+2 {
		t.Errorf("the 12 months count %d requests, then %d, want today's 3 grown to 5", was, is)
	}
}

// The first look on a new date reads the day before it, finished, and the
// new day: the Past then runs to it, its grid's today the new day's.
func TestSinceMovesThePastToANewDay(t *testing.T) {
	past := views.NewPast(input(overTheYear), overTheYear)
	tomorrow := now.Add(24 * time.Hour)
	finished := day("2026-10-07", usedBy("work", sessionsOf(sent("claude-opus-5-5", 7, inputUsage), sessionE)))
	newDay := day("2026-10-08", usedBy("side", sessionsOf(sent("claude-sonnet-5-5", 4, inputUsage), sessionD)))

	page := past.Since([]ledger.Summary{finished, newDay}, tomorrow).Year(views.YearAsk{})
	for date, want := range map[string]int{"2026-10-07": 7, "2026-10-08": 4} {
		if got, ok := gridDay(page, date); !ok || got.LeftOut || got.Requests != want {
			t.Errorf("%s is %+v, want %d requests, drawn", date, got, want)
		}
	}
	if got, ok := gridDay(page, "2026-10-09"); !ok || !got.LeftOut {
		t.Errorf("9 October is %+v, want it left out, still to come", got)
	}
}

// Given no days, Since moves the Past to the time given alone.
func TestSinceWithNoDaysMovesThePastAlone(t *testing.T) {
	past := views.NewPast(input(overTheYear), overTheYear)

	page := past.Since(nil, now.Add(24*time.Hour)).Year(views.YearAsk{})
	if got, ok := gridDay(page, "2026-10-08"); !ok || got.LeftOut || got.Requests != 0 {
		t.Errorf("the next day is %+v, want it today, drawn with nothing on it", got)
	}
	if got, _ := gridDay(page, "2026-10-07"); got.Requests != 3 {
		t.Errorf("7 October has %d requests, want its 3 standing", got.Requests)
	}
}
