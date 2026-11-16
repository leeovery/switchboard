package dashboard

import "testing"

// A turn of the wheel over the list chooses the project before or after the
// one chosen, the selection let go, and brings ↑ and ↓ back to the list from
// SESSIONS; at the list's ends it moves no choice, changing nothing from the
// list, and only where ↑ and ↓ act from SESSIONS.
func TestWheeledChoosesThroughTheListAndComesBackFromSessions(t *testing.T) {
	rows := projectRows(5)
	page := chosenPage(chosenSessions())
	tests := []struct {
		name       string
		chosen, by int
		inSessions bool
		want       int
		changed    bool
	}{
		{name: "down in the middle, from the list", chosen: 2, by: 1, want: 3, changed: true},
		{name: "up in the middle, from the list", chosen: 2, by: -1, want: 1, changed: true},
		{name: "up at the top, from the list", chosen: 0, by: -1, want: 0},
		{name: "down at the bottom, from the list", chosen: 4, by: 1, want: 4},
		{name: "down in the middle, from SESSIONS", chosen: 2, by: 1, inSessions: true, want: 3, changed: true},
		{name: "up in the middle, from SESSIONS", chosen: 2, by: -1, inSessions: true, want: 1, changed: true},
		{name: "up at the top, from SESSIONS", chosen: 0, by: -1, inSessions: true, want: 0, changed: true},
		{name: "down at the bottom, from SESSIONS", chosen: 4, by: 1, inSessions: true, want: 4, changed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := projectsShownOf(rows, tt.chosen, &page)
			s.InSessions, s.Selected = tt.inSessions, "5b19d3e7"
			got, changed := s.Wheeled(tt.by)
			want := s
			if tt.changed {
				want.InSessions = false
			}
			if tt.want != tt.chosen {
				want.Chosen, want.Selected = rows[tt.want].Path, ""
			}
			same := got.Chosen == want.Chosen && got.InSessions == want.InSessions && got.Selected == want.Selected &&
				got.ListScroll == want.ListScroll && got.Page == want.Page && got.Project == want.Project
			if changed != tt.changed || !same {
				t.Errorf("Wheeled(%d) is %s chosen, in sessions %v, %q selected, changed %v; want %s, in sessions %v, %q, changed %v",
					tt.by, got.Chosen, got.InSessions, got.Selected, changed, want.Chosen, want.InSessions, want.Selected, tt.changed)
			}
		})
	}
}
