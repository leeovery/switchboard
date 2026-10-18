package ledger

import (
	"reflect"
	"slices"
	"testing"
	"time"
)

// The lines filed under the days asked are given afresh, the lines held left
// as they are, in their order, as a read goes on holding them to read more
// days into once a session's base falls.
func TestTheLinesFiledOnTheDaysAskedAreGivenAfresh(t *testing.T) {
	at := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	held := []filed{
		{Line: &Line{At: at, Request: "1"}, date: "2026-10-05", place: 0},
		{Line: &Line{At: at, Request: "2"}, date: "2026-10-04", place: 1},
		{Line: &Line{At: at, Request: "3"}, date: "2026-10-05", place: 2},
	}
	tests := []struct {
		name  string
		dates []string
		want  []int
	}{
		{name: "every day held", dates: []string{"2026-10-04", "2026-10-05"}, want: []int{0, 1, 2}},
		{name: "one of them", dates: []string{"2026-10-05"}, want: []int{0, 2}},
		{name: "none of them", dates: []string{"2026-10-06"}, want: []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			was := slices.Clone(held)
			got := filedOn(held, tt.dates)
			places := []int{}
			for _, l := range got {
				places = append(places, l.place)
			}
			if !slices.Equal(places, tt.want) {
				t.Errorf("filed on %q gives the lines in places %v, want %v", tt.dates, places, tt.want)
			}
			if len(got) > 0 {
				got[0] = filed{}
			}
			if !reflect.DeepEqual(held, was) {
				t.Errorf("filed on %q leaves the lines held as %+v, want them as they were, %+v", tt.dates, held, was)
			}
		})
	}
}

// The sessions given each have their lines oldest first, copied from where
// the read held them, and the read lets go of each session's lines as it's
// given, holding none once every one is.
func TestTheSessionsGivenAreLetGoOfByTheRead(t *testing.T) {
	at := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	near := []string{"2026-10-05"}
	read := newSessionsRead(at.Add(time.Hour), nil)
	read.held["work"] = []filed{
		{Line: &Line{At: at.Add(time.Minute), Request: "2", Session: "work"}, date: near[0], place: 1},
		{Line: &Line{At: at, Request: "1", Session: "work"}, date: near[0], place: 0},
	}
	read.held["personal"] = []filed{{Line: &Line{At: at, Request: "3", Session: "personal"}, date: near[0], place: 2}}
	held := map[string][]*Line{"work": {read.held["work"][1].Line, read.held["work"][0].Line}, "personal": {read.held["personal"][0].Line}}
	asked := map[string]*sessionAsked{"work": {near: near}, "personal": {near: near}}
	got := (&sessionsLookup{}).from(asked, read)
	for id, lines := range held {
		var want []Line
		for _, l := range lines {
			want = append(want, *l)
		}
		if !reflect.DeepEqual(got[id].Lines, want) {
			t.Errorf("%s is given %+v, want its lines oldest first, %+v", id, got[id].Lines, want)
		}
		lines[0].Request = "changed"
		if got[id].Lines[0].Request == "changed" {
			t.Errorf("%s is given the lines the read held, want them copied", id)
		}
	}
	if len(read.held) > 0 {
		t.Errorf("the read holds the lines of %d sessions once they're given, want none", len(read.held))
	}
}
