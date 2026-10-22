package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/views"
)

// The limits an account hit in three windows, a model's week among them,
// take the fullest of their forms the column holds, whole, turned or wide:
// how many, where neither the windows named in full nor short fit, as in
// every turned column; short where they fit, as at 160 columns; in full
// where those fit, as at 220. Turned Compare's forms are the wide form's
// but the full.
func TestLimitsHitInThreeWindowsTakeTheFullestFormThatFits(t *testing.T) {
	work := compareWork
	work.limits = map[string]int{"5h": 2, "7d": 1, "7d_oi": 1}
	tests := []struct {
		width int
		want  string
	}{
		{width: 52, want: "4"},
		{width: 82, want: "4"},
		{width: 83, want: "4"},
		{width: 160, want: "week ×1 · 5h ×2 · Fable wk ×1"},
		{width: 220, want: "week ×1 · 5-hour ×2 · Fable week ×1"},
	}
	for _, tt := range tests {
		page := comparedPage(views.AccountsThisWeek, comparedVersions(t), work, comparePersonal, compareSide, compareAll)
		f := phoneText(t, tt.width, 60, AccountsCompare, "", &page, nil)
		rows := f.Draw(phoneDoc().WorkedOut(claudeLike), routedAt)
		label := "  limits hit"
		if !f.CompareWide() {
			label = " work"
		}
		limits := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(row, "LIMITS") })
		at := slices.IndexFunc(rows[limits:], func(row string) bool { return strings.HasPrefix(row, label) })
		if limits < 0 || at < 0 {
			t.Fatalf("at %d columns, no LIMITS row of work's in\n%s", tt.width, strings.Join(rows, "\n"))
		}
		row := rows[limits+at]
		if got := strings.TrimLeft(strings.TrimPrefix(row, label), " "); !strings.HasPrefix(got, tt.want+" ") || strings.Contains(row, "…") {
			t.Errorf("at %d columns, work's limits hit read %q, want %q whole", tt.width, row, tt.want)
		}
	}
}
