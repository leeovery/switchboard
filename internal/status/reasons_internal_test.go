package status

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// The forms of the reasons and the whys in which a placeholder stands for
// what varies, as the reference writes them.
const (
	placedID     = "<id>"
	placedWhy    = "<why>"
	placedIdle   = "<idle>"
	placedShare  = "<n>%"
	placedReason = "<reason>"
)

// reasonForms are every reason the router gives, a reason with a tail in the
// form the reference writes it.
func reasonForms() []string {
	return []string{
		ReasonNew, ReasonPinned, ReasonGlobalPin, ReasonMovedByPin, ReasonSticky, ReasonBound,
		ReasonUnsessioned, ReasonNoRoom, ReasonClient, ReasonBack,
		ReasonMovedOff + placedID + " " + placedWhy,
		ReasonPinYields + placedID + " " + placedWhy,
		ReasonRescored + placedIdle + rescoredIdle,
		ReasonCompacted + placedID + compactedResets + placedShare + compactedLeft,
		PassingOver(placedReason, placedID),
	}
}

// whys are every why an account can't take a request.
var whys = []string{WhyLimit, WhyRefused, WhyNewToken, WhyReserve, WhyNoRoom}

// The reasons and whys are stored for years, in the ledger, the events,
// state.json and /sessions, and read back, so the reference lists every one
// as fixed: a reason reworded in the code, or in the reference, fails here.
func TestTheReasonsAreTheOnesTheReferenceFixes(t *testing.T) {
	reference := readReference(t)
	tests := []struct {
		heading string
		want    []string
	}{
		{heading: "### The reasons", want: reasonForms()},
		{heading: "### The whys", want: whys},
	}
	for _, tt := range tests {
		t.Run(tt.heading, func(t *testing.T) {
			listed := firstCells(t, reference, tt.heading)
			if !sameSet(listed, tt.want) {
				t.Errorf("docs/reference.md's %q lists\n%q\nwant the code's\n%q", tt.heading, listed, tt.want)
			}
		})
	}
}

// The forms <idle> takes in the reference are the ones idled gives.
func TestTheIdleSpansTheReferenceGivesAreTheOnesIdledFiles(t *testing.T) {
	reference := readReference(t)
	rescored := ReasonRescored + placedIdle + rescoredIdle
	for _, span := range []string{idled(132 * time.Hour), idled(144 * time.Hour), idled(4*time.Hour + 57*time.Minute), idled(7 * time.Minute), idled(0)} {
		if !strings.Contains(rowOf(reference, rescored), "`"+span+"`") {
			t.Errorf("docs/reference.md's row of %q doesn't give %q as an <idle>", rescored, span)
		}
	}
}

// readReference reads docs/reference.md, from the module's root.
func readReference(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(sourcecheck.ModuleRoot(t), "docs", "reference.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// firstCells are the code in the first cell of each row of the table under
// the heading given in doc, up to the next heading.
func firstCells(t *testing.T, doc, heading string) []string {
	t.Helper()
	_, under, ok := strings.Cut(doc, "\n"+heading+"\n")
	if !ok {
		t.Fatalf("docs/reference.md has no heading %q", heading)
	}
	var cells []string
	for line := range strings.Lines(under) {
		if strings.HasPrefix(line, "#") {
			break
		}
		if code, ok := strings.CutPrefix(line, "| `"); ok {
			cell, _, _ := strings.Cut(code, "` |")
			cells = append(cells, cell)
		}
	}
	return cells
}

// rowOf is the line of the table row in doc whose first cell is the code
// given: "" where there's none.
func rowOf(doc, code string) string {
	for line := range strings.Lines(doc) {
		if strings.HasPrefix(line, "| `"+code+"` |") {
			return line
		}
	}
	return ""
}

// sameSet reports whether a and b hold the same strings, in any order.
func sameSet(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}
