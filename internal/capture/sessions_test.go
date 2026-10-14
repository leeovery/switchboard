package capture

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// runningSaid is a session the router lists, as Sessions says of it: the
// account its conversation goes to, its model, what it's doing, and how long
// since the router last saw it, to the second.
type runningSaid struct {
	account, model, state string
	seen                  time.Duration
}

// runningOf is what the List says of the sessions running at now, the router
// listing those given, the ledger holding the lines given.
func runningOf(sessions []status.Session, lines []ledger.Line, now time.Time) map[string]runningSaid {
	l, _, _ := (&records{lines: filedOf(heldAsWritten(lines))}).readers(func() time.Time { return now })
	list := views.ListSessions(views.SessionSources{Running: sessions, Ledger: l, Prices: drawnPrices, Now: now})
	said := make(map[string]runningSaid)
	for _, s := range list.Sessions {
		if s.Running {
			said[short(s.Session)] = runningSaid{account: s.Account, model: ledger.ShortModelID(s.Model), state: s.State,
				seen: now.Sub(s.LastSeen).Truncate(time.Second)}
		}
	}
	return said
}

// todaysPlanned are the lines of today's sessions whose pages the frames
// draw, d28c and 9c2e.
func todaysPlanned(now time.Time) []ledger.Line {
	return append(d28cPlan(now).lines(), quietPlan(now).lines()...)
}

func TestTheRoutersSessionsAtTheLimitAreTheSessionsFrames(t *testing.T) {
	limit := clockAt(localMoment(), 0, 16, 5, 41)
	tests := []struct {
		name     string
		sessions []status.Session
		now      time.Time
		want     map[string]runningSaid
	}{
		{
			name: "16:05:41, personal reaching its limit as 41e0 asks", sessions: routedAtTheLimit(limit), now: limit,
			want: map[string]runningSaid{
				"41e0": {"personal", "opus-5-5", status.Asking, 0}, "9c2e": {"personal", "opus-5-5", "", 13*time.Minute + 11*time.Second},
				"c61b": {"side", "opus-5-5", status.Answering, 0}, "d28c": {"side", "opus-5-5", "", 5 * time.Second},
				"7f3a": {"side", "opus-5-5", "", 2*time.Minute + 29*time.Second}, "db8a": {"side", "opus-5-5", "", 46 * time.Second},
			},
		},
		{
			name: "16:05:42, 41e0's request moved to side", sessions: routedAfterTheLimit(limit.Add(time.Second)), now: limit.Add(time.Second),
			want: map[string]runningSaid{
				"41e0": {"side", "opus-5-5", status.Asking, 0}, "9c2e": {"personal", "opus-5-5", "", 13*time.Minute + 12*time.Second},
				"c61b": {"side", "opus-5-5", status.Answering, 0}, "d28c": {"side", "opus-5-5", "", 6 * time.Second},
				"7f3a": {"side", "opus-5-5", "", 2*time.Minute + 30*time.Second}, "db8a": {"side", "opus-5-5", "", 47 * time.Second},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runningOf(tt.sessions, todaysPlanned(tt.now), tt.now)
			if len(got) != len(tt.want) {
				t.Errorf("the List says of %d sessions running, want %d: %+v", len(got), len(tt.want), got)
			}
			for id, want := range tt.want {
				if got[id] != want {
					t.Errorf("the List says %s is %+v, want %+v", id, got[id], want)
				}
			}
		})
	}
}

// f41a's world's router lists six sessions, as its status line counts them:
// the Overview's, f41a, answering on personal, in 9c2e's place; and its
// ledger holds f41a's lines, of yesterday and today, among the sessions'
// world's.
func TestF41AsWorldListsItInPlaceOf9C2EAndHoldsItsLines(t *testing.T) {
	worlds := worldsNow()
	w := worlds.resumed()
	listed := w.routed(worlds.now)
	if doc := w.document(worlds.now); len(listed) != doc.Sessions {
		t.Errorf("the router lists %d sessions, where its status line counts %d", len(listed), doc.Sessions)
	}
	ids := make([]string, len(listed))
	for i, s := range listed {
		ids[i] = short(s.ID)
	}
	if want := []string{"d28c", "c61b", "41e0", "7f3a", "db8a", "f41a"}; !slices.Equal(ids, want) {
		t.Errorf("the router lists %v, want %v", ids, want)
	}
	held := 0
	for _, f := range w.records().lines {
		if f.Session == idF41A {
			held++
		}
	}
	if want := len(resumedPlan(worlds.now).lines()); held != want {
		t.Errorf("the ledger holds %d of f41a's lines, want its %d", held, want)
	}
}
