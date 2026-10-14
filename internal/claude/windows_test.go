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
	if got := claude.WindowName(claude.FiveHourWindow); got != "5-hour" {
		t.Errorf("FiveHourWindow is named %q, want 5-hour", got)
	}
	if got := claude.WindowName(claude.WeekWindow); got != "Week" {
		t.Errorf("WeekWindow is named %q, want Week", got)
	}
}
