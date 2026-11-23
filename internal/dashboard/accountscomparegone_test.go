package dashboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/status"
)

// wideFoot and turnedFoot draw Compare's rows, wide and turned, as a test
// reads them, with what starts VALUE's last row in each form.
var (
	wideFoot = compareFoot{valueLast: "  against its price", drawn: func(f Frame, doc status.Document) *canvas {
		return f.comparing(doc, routedAt).rows()
	}}
	turnedFoot = compareFoot{valueLast: " all ", drawn: func(f Frame, doc status.Document) *canvas {
		return f.turned(doc, routedAt).turnedWhole().c
	}}
)

// compareFoot is a form of Compare's rows, as drawn draws them, and what
// starts VALUE's last row in it.
type compareFoot struct {
	valueLast string
	drawn     func(f Frame, doc status.Document) *canvas
}

// Compare's last line, wide or turned, a blank row under VALUE, says how
// many accounts no longer here all accounts' totals include, as the
// period's page counts them, one or many, in text.faint, two cells in: in
// full where it fits the frame's edge, then without "all accounts'", and
// only where neither fits, its shorter form cut with …. Where they include
// none, there's no line and no blank row: VALUE ends Compare.
func TestCompareSaysHowManyAccountsNoLongerHereAllAccountsTotalsInclude(t *testing.T) {
	tests := []struct {
		name  string
		form  compareFoot
		width int
		gone  int
		want  string
	}{
		{name: "wide, none", form: wideFoot, width: 160},
		{name: "wide, one", form: wideFoot, width: 160, gone: 1, want: "  all accounts' totals include 1 account no longer here"},
		{name: "wide, two", form: wideFoot, width: 160, gone: 2, want: "  all accounts' totals include 2 accounts no longer here"},
		{name: "turned, none", form: turnedFoot, width: 73},
		{name: "turned, one", form: turnedFoot, width: 73, gone: 1, want: "  all accounts' totals include 1 account no longer here"},
		{name: "turned, two", form: turnedFoot, width: 73, gone: 2, want: "  all accounts' totals include 2 accounts no longer here"},
		{name: "the full form's least", form: turnedFoot, width: 56, gone: 1, want: "  all accounts' totals include 1 account no longer here"},
		{name: "a cell short of the full form", form: turnedFoot, width: 55, gone: 1, want: "  totals include 1 account no longer here"},
		{name: "a phone, two", form: turnedFoot, width: 52, gone: 2, want: "  totals include 2 accounts no longer here"},
		{name: "the shorter form's least", form: turnedFoot, width: 42, gone: 1, want: "  totals include 1 account no longer here"},
		{name: "neither fits", form: turnedFoot, width: 41, gone: 1, want: "  totals include 1 account no longer he…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, doc := threeCompared(t, tt.width, 61)
			f.Accounts.Page.NoLongerHere = tt.gone
			c := tt.form.drawn(f, doc)
			rows := c.rows(Look{})
			last := len(rows) - 1
			if tt.want == "" {
				if !strings.HasPrefix(rows[last], tt.form.valueLast) {
					t.Errorf("Compare's last row is %q, want VALUE's last, starting %q", rows[last], tt.form.valueLast)
				}
				return
			}
			got := strings.TrimRight(rows[last], " ")
			if got != tt.want {
				t.Errorf("Compare's last row is %q, want %q", got, tt.want)
			}
			if ansi.StringWidth(got) > f.edge() {
				t.Errorf("the line runs %d cells, past the frame's edge at %d", ansi.StringWidth(got), f.edge())
			}
			if blank := strings.TrimSpace(rows[last-1]); blank != "" || !strings.HasPrefix(rows[last-2], tt.form.valueLast) {
				t.Errorf("the rows over the last are %q and %q, want VALUE's last, starting %q, then a blank row", rows[last-2], rows[last-1], tt.form.valueLast)
			}
			if k := c.at(labelIndent, last).ink; k != faintInk {
				t.Errorf("the line is in %+v, want text.faint", k)
			}
		})
	}
}
