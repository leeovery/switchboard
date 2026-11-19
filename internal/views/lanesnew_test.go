package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// newWeek is the week work and spare read: a fifth used, resetting on Monday
// 5 October at 10:00.
var newWeek = quota.Window{Key: claude.WeekWindow, Label: claude.WindowLabel(claude.WeekWindow), Utilization: 0.2, ResetsAt: time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)}

// newDoc is the router's document at beyondNow, of work and spare, each
// reading newWeek.
func newDoc() status.Document {
	doc := status.Document{GeneratedAt: beyondNow, Source: status.SourceRouter, Router: status.Health{Healthy: true}, Best: "work"}
	for _, id := range []string{"work", "spare"} {
		a := status.Account{ID: id, Label: id, TokenSet: true, FetchedAt: beyondNow, Windows: []quota.Window{newWeek}}
		doc.Accounts = append(doc.Accounts, a.AsOf(claude.Policy(), beyondNow))
	}
	return doc.WorkedOut(claude.Policy())
}

// readAt are a reading of newWeek of the account with the given id at each
// of the times given, oldest first.
func readAt(id string, at ...time.Time) []readings.Reading {
	var read []readings.Reading
	for _, t := range at {
		read = append(read, readings.Of(id, []quota.Window{newWeek}, t, readings.FromAnswer)...)
	}
	return read
}

// A lane is new in the view where its account's first reading falls inside
// it and the readings reach back before it, another account read by then:
// its stretch runs from the lanes' first column to the last whose moment is
// before that reading, since it. On the live week at Thursday 13:42, from
// Wednesday 13:40, 136 columns of about 74 minutes, spare first read at 09:00
// has 16 columns, the 16th's moment 08:48 and the 17th's 10:02. On a fresh
// install, where every account's readings start inside the view, no lane is
// new; nor is one read before the view; nor one removed and added again, its
// history restored, its first reading old though nothing was read of it for
// days; nor one not read yet. The lanes draw a new one, as the legend keys
// it, only where one is.
func TestALaneIsNewWhereItsAccountsFirstReadingFallsInsideTheView(t *testing.T) {
	monday, wednesday, thursday := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC), time.Date(2026, 9, 30, 20, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		read []readings.Reading
		want views.LaneNew
	}{
		{name: "first read mid-view", read: slices.Concat(readAt("work", monday, beyondNow), readAt("spare", thursday)), want: views.LaneNew{Since: thursday, Columns: 16}},
		{name: "a fresh install", read: slices.Concat(readAt("work", wednesday), readAt("spare", thursday))},
		{name: "read before the view", read: slices.Concat(readAt("work", monday), readAt("spare", monday, thursday))},
		{name: "removed and added again, its history restored", read: slices.Concat(readAt("work", monday, wednesday), readAt("spare", monday.Add(-24*time.Hour), thursday))},
		{name: "not read yet", read: readAt("work", monday, beyondNow)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			read := slices.Clone(tt.read)
			slices.SortStableFunc(read, func(a, b readings.Reading) int { return a.At.Compare(b.At) })
			from := views.LanesFrom{Moment: newDoc().At(beyondNow, claude.Policy()), Readings: read, Timeline: views.LiveTimeline(beyondNow, 136)}
			lanes := views.Lanes{}.Look(from)
			if got := lanes.Accounts[1].New; !got.Since.Equal(tt.want.Since) || got.Columns != tt.want.Columns {
				t.Errorf("spare's lane is new %+v, want %+v", got, tt.want)
			}
			if got := lanes.Accounts[0].New; got != (views.LaneNew{}) {
				t.Errorf("work's lane is new %+v, want not", got)
			}
			if got := lanes.Drawn.New; got != (tt.want.Columns > 0) {
				t.Errorf("the lanes draw a new one %t, want %t", got, tt.want.Columns > 0)
			}
			if n := tt.want.Columns; n > 0 && (lanes.Accounts[1].Cells[n-1] != views.CellUnknown || lanes.Accounts[1].Cells[n] == views.CellUnknown) {
				t.Errorf("spare's cells %v, want nothing known through column %d and known from the next", lanes.Accounts[1].Cells[:n+1], n-1)
			}
		})
	}
}

// The readings needn't come oldest first across accounts, as a tail carries
// each window's run in before the rest: the first of them all and each
// account's first are found wherever they fall.
func TestALaneIsNewWhateverOrderTheReadingsCarriedInCome(t *testing.T) {
	thursday := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	read := slices.Concat(readAt("spare", thursday), readAt("work", time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC), beyondNow))
	from := views.LanesFrom{Moment: newDoc().At(beyondNow, claude.Policy()), Readings: read, Timeline: views.LiveTimeline(beyondNow, 136)}
	if got := (views.Lanes{}).Look(from).Accounts[1].New; !got.Since.Equal(thursday) {
		t.Errorf("spare's lane is new %+v, want since %s", got, thursday)
	}
}
