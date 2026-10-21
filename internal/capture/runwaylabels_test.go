package capture

import (
	"regexp"
	"slices"
	"testing"
)

// laneHead is the label heading a row of Runway's lanes: its number, then
// its name and marks, each a space on, up to the lane's first glyph.
var laneHead = regexp.MustCompile(`^ ?[0-9]+( [^ █▒─│]+)*`)

// Runway's labels read whole beside its lanes at every one of the owner's
// sizes, as the sweep can't see a cut beside lanes that fill the row: the
// label column as wide as the widest, up to its most, where the longest
// name alone is cut; and collapsed, each lane's number and marks. Every lane
// row drawn is headed by one of the labels, and each label heads one.
func TestRunwaysLabelsReadWholeAtTheOwnersSizes(t *testing.T) {
	frames := []string{"1 work", "2 personal ◆", "3 side ▲"}
	tests := []struct {
		name  string
		heads []string
	}{
		{name: "runway-week-160", heads: frames},
		{name: "runway-5-hour-160", heads: frames},
		{name: "runway-stepped-back-160", heads: frames},
		{
			name: "runway-ten-accounts-160",
			heads: []string{
				" 1 work", " 2 personal ◆", " 3 side ▲", " 4 client", " 5 spare", " 6 lab", " 7 team", " 8 extra", " 9 ops", "10 studio",
			},
		},
		{name: "long-names-runway", heads: []string{"1 work-team-plan", "2 personal-research ◆", "3 side-hustle-consu… ▲"}},
		{name: "runway-collapsed-160", heads: []string{"1", "2 ◆", "3 ▲"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := runwayFixture(t, tt.name)
			m := withNothingOver(t, f.settle(f.Size), f.Size)
			for _, size := range ownersSizes {
				heads := lanesHeads(fluidAt(m, size).body())
				for _, head := range heads {
					if !slices.Contains(tt.heads, head) {
						t.Errorf("at %s, a lane is headed %q, want one of %q", sizeName(size), head, tt.heads)
					}
				}
				if size.Height >= ownersSizes[0].Height && !slices.Equal(heads, tt.heads) {
					t.Errorf("at %s, the lanes are headed %q, want every one, %q", sizeName(size), heads, tt.heads)
				}
			}
		})
	}
}

// lanesHeads are the labels heading the rows of Runway's lanes among the
// rows given, as laneHead reads them.
func lanesHeads(rows []string) []string {
	var heads []string
	for _, row := range rows {
		if laneShown.MatchString(row) {
			heads = append(heads, laneHead.FindString(row))
		}
	}
	return heads
}

// runwayFixture is the fixture named, or the long-names world's view so
// named.
func runwayFixture(t *testing.T, name string) Fixture {
	t.Helper()
	if i := slices.IndexFunc(longNamesViews, func(p page) bool { return p.name == name }); i >= 0 {
		p := longNamesViews[i]
		p.size = wide(37)
		return p.fixture(worldsNow())
	}
	return fixture(t, name)
}
