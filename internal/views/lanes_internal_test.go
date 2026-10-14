package views

import (
	"reflect"
	"testing"
	"time"
)

// The parts of a hold, a hold at now and run-outs to it under way, are one:
// since the first began, projected where it is, or from before anything
// known, where one is; until the last lifts, or at no known time, where one
// does.
func TestAHoldsPartsAreOne(t *testing.T) {
	at := func(minutes int) time.Time { return time.Date(2026, 10, 1, 16, minutes, 0, 0, time.UTC) }
	fiveHour := stood{cell: CellLimited, since: at(4), until: at(55), projected: true}
	week := stood{cell: CellLimited, since: at(2), until: at(59).Add(96 * time.Hour), projected: true}
	held := stood{cell: CellLimited, until: at(30)}
	tests := []struct {
		name  string
		parts []stood
		want  stood
	}{
		{name: "one", parts: []stood{fiveHour}, want: fiveHour},
		{name: "the later begun first", parts: []stood{fiveHour, week}, want: week},
		{name: "from before anything known", parts: []stood{fiveHour, held}, want: stood{cell: CellLimited, until: at(55)}},
		{name: "one lifting at no known time", parts: []stood{fiveHour, {cell: CellLimited, since: at(1)}}, want: stood{cell: CellLimited, since: at(1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := asOne(tt.parts); !ok || got != tt.want {
				t.Errorf("asOne = %+v, %v; want %+v", got, ok, tt.want)
			}
		})
	}
	if _, ok := asOne(nil); ok {
		t.Errorf("asOne of no parts reports a hold")
	}
}

// Where the pool's accounts lose their room between the same two columns'
// moments, its count reaches the dip's when the last does; where they get it
// back so, the dip ends when the first does. A change whose time isn't known
// tells neither.
func TestADipsCountChangesWhenItsAccountsDo(t *testing.T) {
	start := time.Date(2026, 10, 1, 14, 0, 0, 0, time.UTC)
	at := func(minute int) time.Time { return start.Add(time.Duration(minute) * time.Minute) }
	tl := Timeline{Start: start, End: at(60), Columns: 6}
	cells := []Cell{CellRoom, CellLimited, CellLimited, CellLimited, CellRoom, CellRoom}
	held := func(since, back time.Time) Lane {
		return Lane{Cells: cells, Stretches: []LaneStretch{{Cell: CellLimited, Start: since, Back: back, First: 1, Last: 3}}}
	}
	members := []Lane{held(at(12), at(43)), held(at(14), at(41)), held(time.Time{}, time.Time{})}
	pool := Pool{Of: 3, Columns: []PoolColumn{{3, true}, {0, true}, {0, true}, {0, true}, {3, true}, {3, true}}}
	want := []Dip{{Steps: []Step{{With: 0, From: Edge{At: at(14)}, Until: Edge{At: at(41)}, First: 1, Last: 3}}, Column: 1, First: 1, Last: 3}}
	if got := pool.dipsOver(members, tl); !reflect.DeepEqual(got, want) {
		t.Errorf("the dips are %+v, want %+v", got, want)
	}
}

// A stretch without room is a run of columns held back alike: a hold whose
// until is still to come at a later column, room between, as where a window
// reset by hand lifts a limit early and is held again, is two stretches.
func TestAStretchIsARunOfColumnsHeldAlike(t *testing.T) {
	start := time.Date(2026, 10, 1, 14, 0, 0, 0, time.UTC)
	tl := Timeline{Start: start, End: start.Add(30 * time.Minute), Columns: 3}
	held := stood{cell: CellLimited, since: start, until: start.Add(time.Hour)}
	got := laneStretches([]stood{held, {cell: CellRoom}, held}, tl, nil)
	if len(got) != 2 || got[0].Last != 0 || got[1].First != 2 {
		t.Errorf("the stretches are %+v, want one of column 0 and one of column 2", got)
	}
}
