package claude_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
)

func TestModelWindowsAreTheWindowsOneFamilyAloneReports(t *testing.T) {
	want := []claude.ModelWindow{{Name: "Fable", Key: "7d_oi"}}
	if got := claude.ModelWindows(); !slices.Equal(got, want) {
		t.Errorf("ModelWindows() = %+v, want %+v: Fable's week, and none of the windows every model shares", got, want)
	}
}
