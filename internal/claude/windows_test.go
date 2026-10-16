package claude_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
)

func TestTheSharedWindowsAreTheFiveHourWindowAndTheWeek(t *testing.T) {
	if want := []string{"5h", "7d"}; !slices.Equal(claude.SharedWindows(), want) {
		t.Errorf("SharedWindows = %q, want %q", claude.SharedWindows(), want)
	}
	if got := claude.WindowLabel(claude.FiveHourWindow); got != "Session" {
		t.Errorf("FiveHourWindow is labelled %q, want Session", got)
	}
	if got := claude.WindowLabel(claude.WeekWindow); got != "Week" {
		t.Errorf("WeekWindow is labelled %q, want Week", got)
	}
}

// A caller changing what it was given changes nothing another is given.
func TestEachCallerIsGivenClaudesWindowsOfItsOwn(t *testing.T) {
	shared, policy := claude.SharedWindows(), claude.Policy()
	shared[0], policy.Shared[0], policy.Perishable = "changed", "changed", "changed"

	if got, want := claude.SharedWindows(), []string{"5h", "7d"}; !slices.Equal(got, want) {
		t.Errorf("having changed one caller's, SharedWindows() = %q, want %q", got, want)
	}
	if got := claude.Policy(); !slices.Equal(got.Shared, []string{"5h", "7d"}) || got.Perishable != claude.WeekWindow {
		t.Errorf("having changed one caller's, Policy() shares %q and perishes on %q, want 5h and 7d shared, perishing on the week", got.Shared, got.Perishable)
	}
}
