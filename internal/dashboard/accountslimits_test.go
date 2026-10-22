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

// An account's limits hit, said as how many, take the column's ink, as all
// accounts' count does, never the windows' state.destructive: in turned
// Compare's forms and the wide form's alike.
func TestLimitsHitCountedTakeTheColumnsInkAsAllAccountsDoes(t *testing.T) {
	name := func(key string) string { return key }
	hit := views.AccountTotals{Limits: map[string]int{"5h": 2, "7d": 1, "7d_oi": 1}}
	all := views.AccountTotals{All: true, Limits: map[string]int{"5h": 3}}
	tests := []struct {
		name  string
		parts []part
		want  line
	}{
		{name: "turned", parts: limitsShortForms(hit, name, secondaryInk).shortest(), want: line{{"4", secondaryInk}}},
		{name: "wide", parts: limitsForms(hit, name, secondaryInk).shortest(), want: line{{"4", secondaryInk}}},
		{name: "all accounts'", parts: limitsForms(all, name, strongInk).shortest(), want: line{{"3", strongInk}}},
	}
	for _, tt := range tests {
		if got := fitParts(200, tt.parts...).inked(); !slices.Equal(got, tt.want) {
			t.Errorf("%s, the count said %+v, want %+v", tt.name, got, tt.want)
		}
	}
}
