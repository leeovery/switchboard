package views

import (
	"maps"
	"slices"
	"testing"
	"time"
)

// countedOnce tells a read's lines given again as a request is counted: one
// with an id once, by its id and when it arrived, given again where one is
// held or was given before in the read; one without as often as given, by
// its sum too, given again only as often as one is held alike.
func TestCountedOnceTellsALineGivenAgainAsARequestIsCounted(t *testing.T) {
	at := instantOf(time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local))
	id := turnKey{request: "r1", at: at}
	later := turnKey{request: "r1", at: instant{sec: at.sec + 1, nsec: at.nsec}}
	idless := turnKey{at: at, sum: 1}
	other := turnKey{at: at, sum: 2}
	tests := []struct {
		name  string
		held  map[turnKey]int
		given []turnKey
		again []bool
	}{
		{name: "one with an id, held none, given twice", given: []turnKey{id, id}, again: []bool{false, true}},
		{name: "one with an id, held, given twice", held: map[turnKey]int{id: 1}, given: []turnKey{id, id}, again: []bool{true, true}},
		{name: "one id arrived at two moments", given: []turnKey{id, later}, again: []bool{false, false}},
		{name: "one without an id, held none, given twice", given: []turnKey{idless, idless}, again: []bool{false, false}},
		{name: "one without an id, held once, given twice", held: map[turnKey]int{idless: 1}, given: []turnKey{idless, idless}, again: []bool{true, false}},
		{
			name: "one without an id, held twice, given three times", held: map[turnKey]int{idless: 2},
			given: []turnKey{idless, idless, idless}, again: []bool{true, true, false},
		},
		{
			name: "one without an id, held once, given after another alike but for its sum", held: map[turnKey]int{idless: 1},
			given: []turnKey{other, idless}, again: []bool{false, true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			read := countedOnceOf(len(tt.given))
			var got []bool
			for _, k := range tt.given {
				got = append(got, read.again(k, tt.held[k]))
			}

			if !slices.Equal(got, tt.again) {
				t.Errorf("given again %v, want %v", got, tt.again)
			}
		})
	}
}

// A page's points holding a day's line without an id once, given the day
// again holding it twice, as the ledger's files came to, take the one copy
// more in, as a request without an id is counted as often as given: of the
// second day of givenTwiceRun's.
func TestAPagesPointsTakeALineWithoutAnIDGivenMoreOftenThanHeld(t *testing.T) {
	second := dayOf(dateOn(1))
	answers := answersOf(linesByDate(givenTwiceRun(5)), "s")
	day := answers[second.Date]
	i := slices.IndexFunc(day, func(a answered) bool { return a.request == "" })
	once := maps.Clone(answers)
	once[second.Date] = slices.Delete(slices.Clone(day), i, i+1)
	points, k := pointingOf(once, second), day[i].key()

	got, placed := points.took(second.Date, day, true, second)

	if held, took := points.held[second.Date].count(k), got.held[second.Date].count(k); !placed || held != 1 || took != 2 {
		t.Errorf("holding the line %d times, given it twice, the points hold it %d times, placed %t; want it held once, then twice", held, took, placed)
	}
}
