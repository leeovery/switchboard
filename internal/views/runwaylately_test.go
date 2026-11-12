package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayLately are doc's events as Runway's LATELY tells them at now.
func runwayLately(doc status.Document, now time.Time) []views.Sentence {
	return views.RunwayLately(doc.At(now, claude.Policy()))
}

// fullOf are the sentences given in full.
func fullOf(told []views.Sentence) []string {
	var full []string
	for _, s := range told {
		full = append(full, s.Full.String())
	}
	return full
}

func TestRunwaysLATELYTellsWhatBefellAnAccountAlone(t *testing.T) {
	now := at(16, 0)
	doc := latelyOf(
		status.Event{At: at(15, 50), Kind: status.EventHealth, Reason: "most requests failing"},
		status.Event{At: at(15, 40), Kind: status.EventRoom, Account: "work", Windows: []string{"5h"}},
		status.Event{At: at(15, 30), Kind: status.EventPin, Account: "personal", Accounts: []string{"personal"}},
		status.Event{At: at(15, 20), Kind: status.EventPressure, Account: "personal", Windows: []string{"7d"}, Until: at(19, 0)},
		status.Event{At: at(15, 10), Kind: status.EventStarted, Account: "side", Session: "c61b2f8d", Reason: status.ReasonNew},
		status.Event{At: at(15, 0), Kind: status.EventRefused, Account: "work", Status: 401},
		status.Event{At: at(14, 50), Kind: status.EventMoved, Session: "db8a71e3", From: "personal", To: "work", Reason: status.ReasonMovedByPin},
		status.Event{At: at(14, 40), Kind: status.EventLimit, Account: "side", Windows: []string{"5h"}},
		status.Event{At: at(14, 30), Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}},
		status.Event{At: at(14, 20), Kind: status.EventForeign, Account: "work"},
		status.Event{At: at(14, 10), Kind: status.EventCap, Account: "personal", Windows: []string{"7d"}},
		status.Event{At: at(14, 0), Kind: status.EventRestart, Reason: status.RestartForConfig},
		status.Event{At: at(13, 50), Kind: status.EventAuto, Accounts: []string{"personal"}},
	)
	want := []string{
		"work open again: its 5-hour window reset",
		"personal came under pressure: its week runs out ~19:00",
		"work refused (401)",
		"side reached its 5-hour limit",
		"personal reached its cap",
	}
	if got := fullOf(runwayLately(doc, now)); !slices.Equal(got, want) {
		t.Errorf("Runway's LATELY tells %q, want %q: a cap, a limit, a refusal, pressure and room again alone, newest first", got, want)
	}
}

func TestRunwaysLATELYReachesBackAWeekFromNow(t *testing.T) {
	now := at(16, 0)
	week := 7 * 24 * time.Hour
	doc := latelyOf(
		status.Event{At: now.Add(-time.Hour), Kind: status.EventRoom, Account: "work"},
		status.Event{At: now.Add(-week), Kind: status.EventCap, Account: "side"},
		status.Event{At: now.Add(-week - time.Minute), Kind: status.EventLimit, Account: "personal"},
	)
	want := []string{"work open again", "side reached its cap"}
	if got := fullOf(runwayLately(doc, now)); !slices.Equal(got, want) {
		t.Errorf("Runway's LATELY tells %q, want %q: a week back from now, and no further", got, want)
	}
}

func TestRunwaysLATELYFoldsTheMovesALimitACapOrARefusalForcedIntoItsLine(t *testing.T) {
	tests := []struct {
		kind, full, short string
	}{
		{kind: status.EventCap, full: "work reached its cap: 3 sessions moved to side", short: "work at its cap: 3 moved to side"},
		{kind: status.EventLimit, full: "work reached its 5-hour limit: 3 sessions moved to side", short: "work at its limit: 3 moved to side"},
		{kind: status.EventRefused, full: "work refused (401): 3 sessions moved to side", short: "work refused (401): 3 moved to side"},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := runwayLately(heldBackAt(tt.kind, false), at(16, 0))
			if len(got) != 1 || got[0].ID != heldLine {
				t.Fatalf("Runway's LATELY tells %q, want the line its moves are folded into alone", fullOf(got))
			}
			if full, short := got[0].Full.String(), got[0].Short.String(); full != tt.full || short != tt.short {
				t.Errorf("the line reads %q, short %q; want %q, %q: its moves counted in it", full, short, tt.full, tt.short)
			}
		})
	}
}
