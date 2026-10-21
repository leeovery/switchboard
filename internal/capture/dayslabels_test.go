package capture

import (
	"slices"
	"strings"
	"testing"
)

// Days' accounts read whole beside their strips at every one of the owner's
// sizes, as the sweep can't see a cut beside strips that fill the row: the
// label column as wide as the widest, ◆ with it, up to its most; or on a
// phone, each on a row of its own. Every row an account's name starts is
// that name whole, then its strip or nothing; and where the frame is tall
// enough to show them all, each heads one.
func TestDaysLabelsReadWholeAtTheOwnersSizes(t *testing.T) {
	tests := []struct {
		name  string
		heads []string
	}{
		{name: "history-days-160", heads: []string{"work", "personal ◆", "side"}},
		{name: "long-names-history-days", heads: []string{"work-team-plan", "personal-research ◆", "side-hustle-consulting"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := runwayFixture(t, tt.name)
			m := withNothingOver(t, f.settle(f.Size), f.Size)
			for _, size := range ownersSizes {
				var heads []string
				for _, row := range fluidAt(m, size).body() {
					i := slices.IndexFunc(tt.heads, func(head string) bool { return strings.HasPrefix(row, head[:3]) })
					if i < 0 {
						continue
					}
					head := tt.heads[i]
					if rest, whole := strings.CutPrefix(row, head); !whole || rest != "" && !strings.HasPrefix(rest, "  ") {
						t.Errorf("at %s, a row reads %q, want it to start %q whole, then two blank cells or nothing", sizeName(size), row, head)
					}
					heads = append(heads, head)
				}
				if size.Height >= daysWholeHeight && !slices.Equal(heads, tt.heads) {
					t.Errorf("at %s, the accounts are named %q, want every one, %q", sizeName(size), heads, tt.heads)
				}
			}
		})
	}
}

// daysWholeHeight is the least of the owner's heights at which Days shows
// every account of three.
const daysWholeHeight = 30
