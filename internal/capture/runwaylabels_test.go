package capture

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// laneHead is the label heading a row of Runway's lanes: its name, then its
// marks at the label column's end, up to the lane's first glyph.
var laneHead = regexp.MustCompile(`^[^ █▒─│]+( [^ █▒─│]+)*( +[◆▲])*`)

// Runway's labels read whole beside its lanes at every one of the owner's
// sizes, as the sweep can't see a cut beside lanes that fill the row: the
// label column as wide as the widest, ▲'s cells kept, each label whole up to
// 24 cells, its marks at the column's end, no number before it, so the long
// names' 17 and 22 read whole; and collapsed, nothing: each lane starts the
// row, with no name or marks. Every lane row drawn is headed by one of the labels, and
// each label heads one.
func TestRunwaysLabelsReadWholeAtTheOwnersSizes(t *testing.T) {
	frames := []string{"work", "personal ◆", "side ▲"}
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
				"work", "personal ◆", "side ▲", "client", "spare", "lab", "team", "extra", "ops", "studio",
			},
		},
		{name: "long-names-runway", heads: []string{"work-team-plan", "personal-research ◆", "side-hustle-consulting ▲"}},
		{name: "runway-collapsed-160", heads: []string{"", "", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := fixtureOrLongNamed(t, tt.name)
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
// rows given, as laneHead reads them, a space between the name and its marks.
func lanesHeads(rows []string) []string {
	var heads []string
	for _, row := range rows {
		if laneShown.MatchString(row) {
			heads = append(heads, strings.Join(strings.Fields(laneHead.FindString(row)), " "))
		}
	}
	return heads
}

// fixtureOrLongNamed is the fixture named, or the long-names world's view so
// named.
func fixtureOrLongNamed(t *testing.T, name string) Fixture {
	t.Helper()
	if i := slices.IndexFunc(longNamesViews, func(p page) bool { return p.name == name }); i >= 0 {
		p := longNamesViews[i]
		p.size = wide(37)
		return p.fixture(worldsNow())
	}
	return fixture(t, name)
}
