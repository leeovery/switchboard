package claude_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
)

func TestTheSharedWindowsAreTheFiveHourWindowAndTheWeek(t *testing.T) {
	if want := []string{"5h", "7d"}; !slices.Equal(claude.SharedWindows, want) {
		t.Errorf("SharedWindows = %q, want %q", claude.SharedWindows, want)
	}
	if got := claude.WindowLabel(claude.FiveHourWindow); got != "Session" {
		t.Errorf("FiveHourWindow is labelled %q, want Session", got)
	}
	if got := claude.WindowLabel(claude.WeekWindow); got != "Week" {
		t.Errorf("WeekWindow is labelled %q, want Week", got)
	}
}
