package dashboard

import (
	"testing"

	"github.com/leeovery/switchboard/internal/status"
)

// USAGE's rows are worked out from its layout, never by drawing it: in
// every form the Overview lays it out in, as it gives way and with its big
// bars, grouped either way, at every width, they're the rows it draws, its
// key line after a blank row among them; and the block ends where its
// drawing does.
func TestUSAGEsRowsWorkedOutAreTheRowsItDraws(t *testing.T) {
	t.Parallel()
	docs := map[string]status.Document{
		"three":                    usageWorld(),
		"one":                      usageWorld(workAlone),
		"work without a 5-hour":    trio(account("work", noSession)),
		"side alone with a 5-hour": trio(account("work", noSession), account("personal", noSession)),
		"four with long names":     latelyWorld(4, true),
		"ten":                      manyRead(10),
		"nothing yet":              {},
	}
	for name, doc := range docs {
		for grouped, by := range map[string]By{"by window": ByWindow, "by account": ByAccount} {
			t.Run(name+", "+grouped, func(t *testing.T) {
				t.Parallel()
				for width := range 221 {
					f := frameOf(width, 40)
					f.By = by
					v := f.overviewBlock(doc, now).views()
					for _, form := range append([]usageViews{v}, v.leaner...) {
						if got, want := form.all, usageDrawnRows(t, form.block); got != want {
							t.Errorf("at %d columns, given way %d, USAGE takes %d rows, want the %d it draws", width, form.block.given, got, want)
						}
						if !form.block.enlarges() {
							continue
						}
						if got, want := form.big, usageDrawnRows(t, form.block.enlarged()); got != want {
							t.Errorf("at %d columns, given way %d, USAGE takes %d rows at its most, want the %d it draws", width, form.block.given, got, want)
						}
					}
				}
			})
		}
	}
}

// usageDrawnRows are the rows b takes drawn whole on a canvas that grows,
// its key line after a blank row. The rows drawn before the key line are
// held to the row after the block's last, as it says: never past it, and
// reaching it wherever the block draws a row with a cell to draw it in. With
// no rows it ends on headings' blank rows, and a column wide its last row's
// words start past its edge, so there the drawing stops short.
func usageDrawnRows(t *testing.T, b usageBlock) int {
	t.Helper()
	c := growing(b.width)
	l := b.lay(c, 0)
	end := l.end
	if drawn, rowed := c.height(), len(l.placed) > 0 && b.width > 1; drawn > end || rowed && drawn != end {
		t.Errorf("at %d columns, given way %d, USAGE's block ends at row %d, but draws %d", b.width, b.given, end, drawn)
	}
	b.key(c, end+1)
	return max(c.height(), end)
}
