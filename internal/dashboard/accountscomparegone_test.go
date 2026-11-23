package dashboard

import (
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
)

// Compare's last line, wide or turned, a blank row under VALUE, says how
// many accounts no longer here all accounts' totals include, as the
// period's page counts them, one or many, in text.faint, two cells in; and
// where they include none, there's no line and no blank row: VALUE ends
// Compare.
func TestCompareSaysHowManyAccountsNoLongerHereAllAccountsTotalsInclude(t *testing.T) {
	forms := []struct {
		name      string
		width     int
		valueLast string
		drawn     func(f Frame, doc status.Document) *canvas
	}{
		{name: "wide", width: 160, valueLast: "  against its price", drawn: func(f Frame, doc status.Document) *canvas {
			return f.comparing(doc, routedAt).rows()
		}},
		{name: "turned", width: 73, valueLast: " all ", drawn: func(f Frame, doc status.Document) *canvas {
			return f.turned(doc, routedAt).turnedWhole().c
		}},
	}
	tests := []struct {
		name string
		gone int
		want string
	}{
		{name: "none", gone: 0},
		{name: "one", gone: 1, want: "  all accounts' totals include 1 account no longer here"},
		{name: "two", gone: 2, want: "  all accounts' totals include 2 accounts no longer here"},
	}
	for _, form := range forms {
		for _, tt := range tests {
			t.Run(form.name+", "+tt.name, func(t *testing.T) {
				f, doc := threeCompared(t, form.width, 61)
				f.Accounts.Page.NoLongerHere = tt.gone
				c := form.drawn(f, doc)
				rows := c.rows(Look{})
				last := len(rows) - 1
				if tt.want == "" {
					if !strings.HasPrefix(rows[last], form.valueLast) {
						t.Errorf("Compare's last row is %q, want VALUE's last, starting %q", rows[last], form.valueLast)
					}
					return
				}
				if got := strings.TrimRight(rows[last], " "); got != tt.want {
					t.Errorf("Compare's last row is %q, want %q", got, tt.want)
				}
				if blank := strings.TrimSpace(rows[last-1]); blank != "" || !strings.HasPrefix(rows[last-2], form.valueLast) {
					t.Errorf("the rows over the last are %q and %q, want VALUE's last, starting %q, then a blank row", rows[last-2], rows[last-1], form.valueLast)
				}
				if got := c.at(labelIndent, last).ink; got != faintInk {
					t.Errorf("the line is in %+v, want text.faint", got)
				}
			})
		}
	}
}
