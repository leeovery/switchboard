package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// beyondNow is when the lanes past their view's end are built: Thursday 1
// October 2026, 13:42 UTC, so the live week view ends on Wednesday 7th at
// 13:40.
var beyondNow = time.Date(2026, 10, 1, 13, 42, 0, 0, time.UTC)

// beyondPolicy shares a second week, "7d_x", among every model, after the
// week the week view is named for.
var beyondPolicy = score.Policy{Shared: []string{"5h", "7d", "7d_x"}, Perishable: "7d", Tiebreak: "5h", Started: "5h", Pressure: "5h"}

// beyondDoc is a document of side alone, read as beyondNow, keeping the
// reserve given, its 5-hour window 10% used, resetting in two hours, and its
// weeks as given.
func beyondDoc(reserve float64, weeks ...quota.Window) status.Document {
	windows := append([]quota.Window{{Key: "5h", Label: "Session", Utilization: 0.1, ResetsAt: beyondNow.Add(2 * time.Hour)}}, weeks...)
	side := status.Account{ID: "side", Label: "side", TokenSet: true, FetchedAt: beyondNow, Reserve: reserve, Windows: windows}
	return status.Document{GeneratedAt: beyondNow, Source: status.SourceRouter, Router: status.Health{Healthy: true}, Accounts: []status.Account{side}}.WorkedOut(beyondPolicy)
}

// week is the week with the given key, used as given, resetting in the
// hours given.
func week(key string, used float64, hours int) quota.Window {
	return quota.Window{Key: key, Label: key, Utilization: used, ResetsAt: beyondNow.Add(time.Duration(hours) * time.Hour)}
}

// A lane has, past its live view's end, the stretches it would have were it
// drawn on till the view's window resets, as the moment has its account
// stand: a run-out to its limit, or to its cap where its reserve holds,
// beginning after the view's last column and before its window resets, from
// when the moment has it run out, projected, back as its window resets, and
// said from the column after the last; none where the run-out falls inside
// the view, which its own stretches have, nor in the 5-hour view, its window
// resetting inside it, nor of another week running out after the view's
// resets. Its lane over the view is as it was: with room all along, or
// running out inside it.
func TestALaneHasWhatItWouldPastItsViewsEndBeforeItsWindowResets(t *testing.T) {
	tests := []struct {
		name   string
		doc    status.Document
		window views.RunwayWindow
		day    time.Time
		key    string
		want   views.Cell
		resets bool
	}{
		{name: "running out past the view's end", doc: beyondDoc(0, week("7d", 0.12, 148)), key: "7d", want: views.CellLimited, resets: true},
		{name: "running out in the last column, after its moment", doc: beyondDoc(0, week("7d", 0.1221, 148)), key: "7d", want: views.CellLimited, resets: true},
		{name: "a week stepped back to", doc: beyondDoc(0, week("7d", 0.12, 148)), day: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)},
		{name: "reaching its cap past the view's end", doc: beyondDoc(0.01, week("7d", 0.12, 148)), key: "7d", want: views.CellCapped, resets: true},
		{name: "another week running out past the view's end", doc: beyondDoc(0, week("7d", 0.1, 160), week("7d_x", 0.12, 148)), key: "7d_x", want: views.CellLimited},
		{name: "running out inside the view", doc: beyondDoc(0, week("7d", 0.3, 148))},
		{name: "in the 5-hour view", doc: beyondDoc(0, week("7d", 0.12, 148)), window: views.RunwayFiveHour},
		{name: "another week running out after the view's resets", doc: beyondDoc(0, week("7d", 0.1, 140), week("7d_x", 0.12, 148))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.doc.At(beyondNow, beyondPolicy)
			tl := views.Stepped(tt.window, tt.day, beyondNow, time.Monday, 136)
			lane := views.Lanes{}.Look(views.LanesFrom{Moment: m, Timeline: tl, Window: tt.window}).Accounts[0]
			if tt.key == "" {
				if len(lane.Beyond) > 0 {
					t.Errorf("the lane has %+v past its view's end, want nothing", lane.Beyond)
				}
				return
			}
			a, _ := m.Account("side")
			w, _ := a.Window(tt.key)
			want := views.LaneStretch{
				Cell: tt.want, Start: w.Out.At, Projected: true, Back: w.Next, AsItResets: tt.resets, Column: tl.Columns, First: tl.Columns, Last: tl.Columns,
			}
			if len(lane.Beyond) != 1 || lane.Beyond[0] != want {
				t.Errorf("past its view's end the lane has %+v, want %+v", lane.Beyond, want)
			}
			if len(lane.Stretches) > 0 || !lane.Room {
				t.Errorf("over its view the lane has %+v, room all along %v; want room all along", lane.Stretches, lane.Room)
			}
		})
	}
}
