package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// trioConfigured are trio's accounts as the config has them, in its order:
// work, keeping 5%; personal, the primary; and side.
func trioConfigured() config.Accounts {
	return config.Accounts{{ID: "work", Label: "work", Reserve: 0.05}, {ID: "personal", Label: "personal", Primary: true}, {ID: "side", Label: "side"}}
}

// trioWeekStart is when trio's weeks started: a week before they reset.
var trioWeekStart = on(4, 21, 0).AddDate(0, 0, -7)

// trioTrails are trio's windows' trails: each week read at its start and two
// days on, work at 10% then 25%, personal at 20% then 45%, side at 30% then
// 60%; work's 5-hour at 50% as it started and 90% at 13:40, personal's at
// 10% then 30%; side's 5-hour not read since it started.
func trioTrails() Trails {
	week := func(first, then float64) Trail {
		return Trail{Readings: []score.Reading{{At: trioWeekStart, Utilization: first}, {At: trioWeekStart.Add(48 * time.Hour), Utilization: then}}}
	}
	session := func(first, then float64) Trail {
		return Trail{Readings: []score.Reading{{At: on(1, 12, 10), Utilization: first}, {At: on(1, 13, 40), Utilization: then}}}
	}
	return Trails{
		{Account: "work", Window: "7d"}: week(0.1, 0.25), {Account: "personal", Window: "7d"}: week(0.2, 0.45), {Account: "side", Window: "7d"}: week(0.3, 0.6),
		{Account: "work", Window: "5h"}: session(0.5, 0.9), {Account: "personal", Window: "5h"}: session(0.1, 0.3),
	}
}

// glanceOf is GLANCE of doc at printNow, its trails trioTrails, the pool's
// room over each window's span built from no readings history, so known
// from now's column, drawn in look.
func glanceOf(doc status.Document, look Look) glance {
	m := doc.At(printNow, printPolicy)
	pools := map[views.RunwayWindow]views.PoolSpan{}
	for _, w := range runwayWindows {
		pools[w] = views.PoolSpan{}.Look(views.PoolRoomFrom{Moment: m, Key: w.KeyIn(m.Policy()), Columns: sparkCells})
	}
	return glance{m: m, trails: trioTrails(), pools: pools, look: look}
}

// glanceIn is g drawn as laid out width cells wide, of the accounts given,
// numbered as its moment numbers them, from the top left of a canvas of its
// own extra rows taller than it takes.
func glanceIn(g glance, accounts config.Accounts, width, extra int) (*canvas, glanceLaid) {
	l := layGlance(g.m.Held().Numbering(), accounts, width)
	c := newCanvas(width, l.rows()+extra)
	g.draw(c, l, 0, 0)
	return c, l
}

// glanceRows are the rows drawn on c, as text.
func glanceRows(c *canvas) []string {
	rows := make([]string, c.height())
	for y := range rows {
		rows[y] = rowOf(c, y)
	}
	return rows
}

func TestGlanceIsAsWideAsItsPartsWholeAndNeverGrows(t *testing.T) {
	// The names, 2 personal ◆ with ▲'s cells kept, and their gap, 16; each
	// window's sparkline, a cell, its figure and two cells, 23, and its
	// words at their widest in full, 24; 3 between the two windows.
	// Stacked, a window a row, at every part's least: the names 16, as
	// personal is no longer than a label's least, 23 and the short words 14.
	if got, want := glancePart(status.Numbering{}, trioConfigured()), (flex.Part{Least: 53, Natural: 113, Most: 113}); got != want {
		t.Errorf("GLANCE's part is %+v, want %+v", got, want)
	}
	if fullWidest() != 24 {
		t.Fatalf("the widest words in full are %d cells, want 24, as the figures above count them", fullWidest())
	}
	for _, width := range []int{113, 160, 220} {
		if l := layGlance(status.Numbering{}, trioConfigured(), width); l.width != 113 || l.stacked {
			t.Errorf("at %d columns GLANCE takes %d, stacked %v, want 113 side by side: it doesn't grow", width, l.width, l.stacked)
		}
	}
}

// Narrowing, GLANCE's words take usage's short forms first, then its names
// narrow toward their least; where that doesn't fit, its windows stack, a
// window a row, its parts narrowing likewise, and where even that doesn't
// fit, its words take what's left.
func TestGlanceNarrowsItsWordsThenItsNamesThenStacks(t *testing.T) {
	// personal-research, the primary, is 17 cells: its name 25 with its
	// number, marks and gap, 16 at its least.
	accounts := config.Accounts{{ID: "work", Label: "work"}, {ID: "personal", Label: "personal-research", Primary: true}, {ID: "side", Label: "side"}}
	tests := []struct {
		name                  string
		width                 int
		column, words, across int
		stacked               bool
	}{
		{name: "whole", width: 122, column: 25, words: 24, across: 122},
		{name: "its words narrowing", width: 112, column: 25, words: 19, across: 112},
		{name: "its words short", width: 102, column: 25, words: 14, across: 102},
		{name: "its names narrowing", width: 99, column: 22, words: 14, across: 99},
		{name: "its names at their least", width: 93, column: 16, words: 14, across: 93},
		{name: "stacked whole", width: 92, column: 25, words: 24, across: 72, stacked: true},
		{name: "stacked, its words short and its names narrowing", width: 60, column: 23, words: 14, across: 60, stacked: true},
		{name: "stacked, its words cut", width: 52, column: 16, words: 13, across: 52, stacked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := layGlance(status.Numbering{}, accounts, tt.width)
			if l.column != tt.column || l.words != tt.words || l.width != tt.across || l.stacked != tt.stacked {
				t.Errorf("at %d columns: names %d, words %d, across %d, stacked %v; want %d, %d, %d, %v",
					tt.width, l.column, l.words, l.width, l.stacked, tt.column, tt.words, tt.across, tt.stacked)
			}
		})
	}
}

func TestGlanceDrawsEveryAccountAndThePoolAsItsWidthLaysThemOut(t *testing.T) {
	tests := []struct {
		name  string
		width int
		want  []string
	}{
		{name: "whole", width: 113, want: []string{
			"                WEEK                                              5-HOUR",
			"",
			"1 work          ▇▇▇▇▇▆▆▆▆▅▅▄▄▄▃▃  66%  19%/day till Sun 21:00     ▄▄▄▄▄▁▁▁▁▁▁▁▁▁▁▁   5%  at cap · back 17:10",
			"",
			"2 personal ◆    ▆▆▆▆▆▄▄▄▄▂▂▁✕     34%  out ~Sat 12:54             ▇▇▇▇▇▆▆▆▅▄▄▄▃▃▂▂  61%  25%/h till 17:10",
			"",
			"3 side ▲        ▆▆▆▆▆▃▃▃▃▁✕       22%  out ~Fri 16:00                     ▇▇▇▇▆▆▆▆  87%  35%/h till 17:10",
			strings.Repeat("─", 113),
			"Σ pool                  ▃▃▂▂▁▁▁▁ 1.2 of 3 left                            ▄▄▄▃▃▃▃▃ 1.5 of 3 left",
			"",
			"▆ room left, start to reset    ▆ heading    ✕ runs out    66% room left now",
		}},
		{name: "its words short", width: 100, want: []string{
			"                WEEK                                       5-HOUR",
			"",
			"1 work          ▇▇▇▇▇▆▆▆▆▅▅▄▄▄▃▃  66%  19%/day · Sun       ▄▄▄▄▄▁▁▁▁▁▁▁▁▁▁▁   5%  back 17:10",
			"",
			"2 personal ◆    ▆▆▆▆▆▄▄▄▄▂▂▁✕     34%  out ~Sat 12:54      ▇▇▇▇▇▆▆▆▅▄▄▄▃▃▂▂  61%  25%/h till 17:10",
			"",
			"3 side ▲        ▆▆▆▆▆▃▃▃▃▁✕       22%  out ~Fri 16:00              ▇▇▇▇▆▆▆▆  87%  35%/h till 17:10",
			strings.Repeat("─", 100),
			"Σ pool                  ▃▃▂▂▁▁▁▁ 1.2 of 3 left                     ▄▄▄▃▃▃▃▃ 1.5 of 3 left",
			"",
			"▆ room left, start to reset    ▆ heading    ✕ runs out    66% room left now",
		}},
		{name: "stacked on a phone", width: 73, want: []string{
			"                WEEK",
			"",
			"1 work          ▇▇▇▇▇▆▆▆▆▅▅▄▄▄▃▃  66%  19%/day till Sun 21:00",
			"",
			"2 personal ◆    ▆▆▆▆▆▄▄▄▄▂▂▁✕     34%  out ~Sat 12:54",
			"",
			"3 side ▲        ▆▆▆▆▆▃▃▃▃▁✕       22%  out ~Fri 16:00",
			strings.Repeat("─", 63),
			"Σ pool                  ▃▃▂▂▁▁▁▁ 1.2 of 3 left",
			"",
			"                5-HOUR",
			"",
			"1 work          ▄▄▄▄▄▁▁▁▁▁▁▁▁▁▁▁   5%  at cap · back 17:10",
			"",
			"2 personal ◆    ▇▇▇▇▇▆▆▆▅▄▄▄▃▃▂▂  61%  25%/h till 17:10",
			"",
			"3 side ▲                ▇▇▇▇▆▆▆▆  87%  35%/h till 17:10",
			strings.Repeat("─", 63),
			"Σ pool                  ▄▄▄▃▃▃▃▃ 1.5 of 3 left",
			"",
			"▆ room left, start to reset    ▆ heading    ✕ runs out",
			"66% room left now",
		}},
		{name: "stacked on the narrowest phone", width: 52, want: []string{
			"                WEEK",
			"",
			"1 work          ▇▇▇▇▇▆▆▆▆▅▅▄▄▄▃▃  66%  19%/day · Sun",
			"",
			"2 personal ◆    ▆▆▆▆▆▄▄▄▄▂▂▁✕     34%  out ~Sat",
			"",
			"3 side ▲        ▆▆▆▆▆▃▃▃▃▁✕       22%  out ~Fri",
			strings.Repeat("─", 52),
			"Σ pool                  ▃▃▂▂▁▁▁▁ 1.2 of 3 left",
			"",
			"                5-HOUR",
			"",
			"1 work          ▄▄▄▄▄▁▁▁▁▁▁▁▁▁▁▁   5%  back 17:10",
			"",
			"2 personal ◆    ▇▇▇▇▇▆▆▆▅▄▄▄▃▃▂▂  61%  25%/h · 17:10",
			"",
			"3 side ▲                ▇▇▇▇▆▆▆▆  87%  35%/h · 17:10",
			strings.Repeat("─", 52),
			"Σ pool                  ▄▄▄▃▃▃▃▃ 1.5 of 3 left",
			"",
			"▆ room left, start to reset    ▆ heading",
			"✕ runs out    66% room left now",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := glanceIn(glanceOf(trio(), Look{}), trioConfigured(), tt.width, 0)
			if got := glanceRows(c); !slices.Equal(got, tt.want) {
				t.Errorf("GLANCE at %d columns is\n%s\nwant\n%s", tt.width, strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

// GLANCE numbers its accounts as Runway's lanes do, as the cards do: by the
// moment's place where the moment holds the account, else after the
// moment's last, in the config's order among those it lacks.
func TestGlanceNumbersItsAccountsAsRunwayDoes(t *testing.T) {
	for _, tt := range numberings() {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := glanceIn(glanceOf(tt.doc, Look{}), tt.config(), 113, 0)
			for i, id := range tt.ids {
				row := rowOf(c, 2+2*i)
				if want, _ := tt.want(id); !labelledAs(row, want) {
					t.Errorf("%s's row is %q, want it to start %q", id, row, want)
				}
			}
		})
	}
}

// Once its words are short, GLANCE's names narrow toward their least, a
// label cut short with an ellipsis, its number and marks kept.
func TestGlancesNamesNarrowOnceItsWordsAreShort(t *testing.T) {
	long := func(a *status.Account) { a.Label = "personal-research" }
	accounts := trioConfigured()
	accounts[1].Label = "personal-research"
	c, l := glanceIn(glanceOf(trio(account("personal", long)), Look{}), accounts, 99, 0)
	if l.column != 22 || l.words != shortCells {
		t.Fatalf("at 99 columns the names take %d and the words %d, want 22 and %d", l.column, l.words, shortCells)
	}
	want := "2 personal-rese… ◆    ▆▆▆▆▆▄▄▄▄▂▂▁✕     34%  out ~Sat 12:54"
	if got := rowOf(c, 4); !strings.HasPrefix(got, want) {
		t.Errorf("personal's row is %q, want it to start %q", got, want)
	}
}

// GLANCE's inks: each window's name over its sparkline in text.subtle; an
// account's number in text.subtle, its label bold text.primary, its marks
// as usage's; its room's figure as usage colours a use; its words as usage
// inks them; the rule in border; Σ in text.subtle and pool in
// text.secondary; and the pool's room bold text.secondary.
func TestGlanceInksItsPartsAsUsageDoes(t *testing.T) {
	c, l := glanceIn(glanceOf(trio(), Look{}), trioConfigured(), 113, 0)
	group := l.group()
	tests := []struct {
		name string
		x, y int
		want ink
	}{
		{name: "WEEK", x: 16, y: 0, want: dimInk},
		{name: "5-HOUR", x: 16 + group, y: 0, want: dimInk},
		{name: "work's number", x: 0, y: 2, want: dimInk},
		{name: "work's label", x: 2, y: 2, want: titleInk},
		{name: "personal's ◆", x: 11, y: 4, want: primaryInk},
		{name: "side's ▲", x: 7, y: 6, want: accentInk},
		{name: "work's week's room, 66%", x: 34, y: 2, want: ink{token: theme.VizRamp1, bold: true}},
		{name: "work's 5-hour's room, 5%", x: 16 + group + 19, y: 2, want: ink{token: theme.VizRamp4, bold: true}},
		{name: "work's week's words", x: 39, y: 2, want: mutedInk},
		{name: "work's 5-hour's words", x: 16 + group + 23, y: 2, want: warningInk},
		{name: "the rule", x: 0, y: 7, want: borderInk},
		{name: "Σ", x: 0, y: 8, want: dimInk},
		{name: "pool", x: 2, y: 8, want: noteInk},
		{name: "the pool's room", x: 33, y: 8, want: strongInk},
		{name: "of 3 left", x: 37, y: 8, want: mutedInk},
	}
	for _, tt := range tests {
		if got := c.at(tt.x, tt.y).ink; got != tt.want {
			t.Errorf("%s, %q at %d along row %d, is in %+v, want %+v", tt.name, c.at(tt.x, tt.y).glyph, tt.x, tt.y, got, tt.want)
		}
	}
}

// A sparkline is the card's model in a row of eighths: before now's column
// the room each column's middle had, in the window's ink; from now's, the
// room it has ahead, in that ink mixed with the canvas so 50% of it shows,
// faint where the look can't fade it; and ✕, bold state.destructive, in the
// column it runs out in, over what it falls on.
func TestASparklineIsTheCardsRoomInARowOfEighths(t *testing.T) {
	// Ten columns of an hour from 08:00; now, 12:40, in column 5; half used
	// all along, so 4 eighths before now; a quarter ahead, 2 eighths, till
	// it runs out at 15:30, column 7, then none.
	cd := burnCardAt(clockOn(12, 40), readOn(8, 0, 0.5))
	cd.ink, cd.out = warningInk, clockOn(15, 30)
	cd.ahead = func(at time.Time) (float64, bool) {
		if at.Before(cd.out) {
			return 0.25, true
		}
		return 0, true
	}
	c := newCanvas(10, 1)
	cd.spark(c, Look{}, 0, 0, 10)
	if got, want := rowOf(c, 0), "▄▄▄▄▄▂▂✕"; got != want {
		t.Errorf("the sparkline is %q, want %q", got, want)
	}
	before, ahead := ink{token: theme.AccentAttention}, ink{token: theme.AccentAttention, fade: aheadFade, dim: true}
	for x, want := range []ink{before, before, before, before, before, ahead, ahead, outInk} {
		if got := c.at(x, 0).ink; got != want {
			t.Errorf("column %d is in %+v, want %+v", x, got, want)
		}
	}
}

// A sparkline's ✕ is where the moment has the window run out, its column of
// the window's span across sparkCells; none where it doesn't.
func TestGlancesSparklinesAgreeWithTheMoment(t *testing.T) {
	g := glanceOf(trio(), Look{})
	for _, a := range g.m.Accounts {
		for _, key := range []string{"7d", "5h"} {
			w, _ := a.Window(key)
			c := newCanvas(sparkCells, 1)
			burnCardOf(a, key, g.trails[Ref{Account: a.ID, Window: key}], printNow).spark(c, Look{}, 0, 0, sparkCells)
			x, _, crossed := crossOn(c, sparkCells, 1)
			if crossed != w.RunsOut {
				t.Errorf("%s's %s has ✕ %v, want %v", a.ID, key, crossed, w.RunsOut)
				continue
			}
			tl := views.Timeline{Start: w.Start, End: w.Start.Add(w.Length), Columns: sparkCells}
			if want := tl.Column(w.Out.At); crossed && x != want {
				t.Errorf("%s's %s has ✕ in column %d, want %d, where Out.At falls", a.ID, key, x, want)
			}
		}
	}
}

// Each account's row says how it stands as its card does: not read, or
// without a usable token, its sparkline empty, its figure —, and usage's
// words for it; stale, its sparkline faded and its figure in text.subtle;
// out of the pool, faint, its name, figure and words in text.faint, its
// sparkline faded; and one the config has that the document doesn't hold
// yet, as one not read.
func TestGlanceSaysEachAccountsStateAsItsCardDoes(t *testing.T) {
	unread := trio(account("work", func(a *status.Account) { a.FetchedAt = time.Time{} }))
	tokenless := trio(account("work", func(a *status.Account) { a.TokenSet = false }))
	stale := trio(account("work", func(a *status.Account) { a.Error = "usage endpoint answered 500" }))
	out := trio(pinnedTo("personal", "side"))
	added := append(trioConfigured(), config.Account{ID: "spare", Label: "spare"})
	faded := ink{token: theme.VizRamp1, fade: staleFade, dim: true}
	tests := []struct {
		name     string
		doc      status.Document
		accounts config.Accounts
		row      int
		want     string
		spark    *ink
		figure   ink
		words    ink
		label    ink
	}{
		{name: "not read", doc: unread, row: 2, want: "1 work                              —  not read yet", figure: faintInk, words: dimInk, label: titleInk},
		{name: "without a usable token", doc: tokenless, row: 2, want: "1 work                              —  no token", figure: faintInk, words: errorInk, label: titleInk},
		{name: "stale", doc: stale, row: 2, want: "1 work          ▇▇▇▇▇▆▆▆▆         66%  not read since 14:42", spark: &faded, figure: dimInk, words: dimInk, label: titleInk},
		{name: "out of the pool", doc: out, row: 2, want: "1 work          ▇▇▇▇▇▆▆▆▆▅▅▄▄▄▃▃  66%  19%/day till Sun 21:00", spark: &faded, figure: ink{token: theme.TextFaint, bold: true}, words: faintInk, label: ink{token: theme.TextFaint, bold: true}},
		{name: "not in the document yet", doc: trio(), accounts: added, row: 8, want: "4 spare                             —  not read yet", figure: faintInk, words: dimInk, label: titleInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts := tt.accounts
			if accounts == nil {
				accounts = trioConfigured()
			}
			c, _ := glanceIn(glanceOf(tt.doc, Look{}), accounts, 113, 0)
			if row := rowOf(c, tt.row); !strings.HasPrefix(row, tt.want) {
				t.Fatalf("the row is %q, want it to start %q", row, tt.want)
			}
			if tt.spark != nil {
				if got := c.at(16, tt.row).ink; !sameFade(got, *tt.spark) {
					t.Errorf("its sparkline is in %+v, want %+v", got, *tt.spark)
				}
			}
			for _, at := range []struct {
				what string
				x    int
				want ink
			}{{"its label", 2, tt.label}, {"its figure", 36, tt.figure}, {"its words", 39, tt.words}} {
				if got := c.at(at.x, tt.row).ink; got != at.want {
					t.Errorf("%s, %q, is in %+v, want %+v", at.what, c.at(at.x, tt.row).glyph, got, at.want)
				}
			}
		})
	}
}

// The pool's row, under usage's rule, has each window's sparkline of the
// pool's room over its span, each column its room of all its accounts' in
// eighths, before now's column in a lasting window's ink, from now's as it
// heads; and its room, as usage words it, pinned after the first. With one
// account there's no pool, nor a row for it; with two configured that the
// document doesn't hold yet, its rows are kept, blank.
func TestGlancesPoolRowIsThePoolsRoomOverEachWindowsSpan(t *testing.T) {
	start := printNow.Add(-8 * time.Hour)
	used := []float64{1, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.75, 0.75, 1, 1, 1, 1, 1, 1}
	span := views.PoolSpan{Timeline: views.Timeline{Start: start, End: start.Add(16 * time.Hour), Columns: sparkCells, Now: printNow}, Of: 3}
	for i, u := range used {
		span.Columns = append(span.Columns, views.PoolRoomColumn{Room: 3 * (1 - u), Used: u, Known: i > 0})
	}
	c := newCanvas(sparkCells, 1)
	poolSpark{span: span}.spark(c, Look{}, 0, 0, sparkCells)
	if got, want := rowOf(c, 0), " ▄▄▄▄▄▄▄▂▂"; got != want {
		t.Errorf("the pool's sparkline is %q, want %q: none where not known, round(room × 8) eighths of all its accounts'", got, want)
	}
	if got, want := c.at(1, 0).ink, lastingInk; got != want {
		t.Errorf("before now it's in %+v, want %+v", got, want)
	}
	if got, want := c.at(8, 0).ink, aheadInk(lastingInk); got != want {
		t.Errorf("from now it's in %+v, want %+v", got, want)
	}

	pinned, _ := glanceIn(glanceOf(trio(pinnedTo("personal", "side")), Look{}), trioConfigured(), 113, 0)
	if got, want := rowOf(pinned, 8), "Σ pool                  ▂▂▁▁▁    0.6 of 2 left · pinned"; !strings.HasPrefix(got, want) || strings.Count(got, "pinned") != 1 {
		t.Errorf("pinned, the pool's row is %q, want it to start %q, pinned once", got, want)
	}

	tests := []struct {
		name     string
		doc      status.Document
		accounts config.Accounts
		rows     int
		pooled   bool
	}{
		{name: "a pool", doc: trio(), accounts: trioConfigured(), rows: 11, pooled: true},
		{name: "one account", doc: trio(func(d *status.Document) { d.Accounts = d.Accounts[:1] }), accounts: trioConfigured()[:1], rows: 5},
		{name: "a pool not read yet", doc: status.Document{GeneratedAt: printNow, Source: status.SourceRouter}, accounts: trioConfigured(), rows: 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, l := glanceIn(glanceOf(tt.doc, Look{}), tt.accounts, 113, 0)
			rows := glanceRows(c)
			if l.rows() != tt.rows {
				t.Errorf("GLANCE takes %d rows, want %d", l.rows(), tt.rows)
			}
			ruled := slices.ContainsFunc(rows, func(r string) bool { return strings.HasPrefix(r, "───") })
			named := slices.ContainsFunc(rows, func(r string) bool { return strings.HasPrefix(r, "Σ pool") })
			if ruled != tt.pooled || named != tt.pooled {
				t.Errorf("the pool's rule is drawn %v and its row %v, want %v:\n%s", ruled, named, tt.pooled, strings.Join(rows, "\n"))
			}
		})
	}
}

// GLANCE's key, under a blank row: a sparkline's cell for the room left, in
// a lasting window's ink; the cell as it heads; ✕; and a figure, bold
// text.secondary, each with what it means in text.muted, four spaces apart,
// flowed onto as many rows as it takes, an item never broken.
func TestGlancesKeySaysWhatItsMarksMean(t *testing.T) {
	want := line{
		{sparkTile, lastingInk}, {" room left, start to reset", mutedInk}, {"    ", ink{}},
		{sparkTile, aheadInk(lastingInk)}, {" heading", mutedInk}, {"    ", ink{}},
		{outMark, outInk}, {" runs out", mutedInk}, {"    ", ink{}},
		{"66%", strongInk}, {" room left now", mutedInk},
	}
	if got := layGlance(status.Numbering{}, trioConfigured(), 113).key; len(got) != 1 || got[0].width() != want.width() || got[0].plain() != want.plain() {
		t.Fatalf("the key is %v, want one row, %q", got, want.plain())
	}
	c, l := glanceIn(glanceOf(trio(), Look{}), trioConfigured(), 113, 0)
	x := 0
	for _, s := range want {
		for range []rune(s.text) {
			if s.ink != (ink{}) {
				if got := c.at(x, l.rows()-1).ink; got != s.ink {
					t.Errorf("%q is in %+v, want %+v", s.text, got, s.ink)
					break
				}
			}
			x++
		}
	}
	if got := len(layGlance(status.Numbering{}, trioConfigured(), 52).key); got != 2 {
		t.Errorf("at 52 columns the key takes %d rows, want 2", got)
	}
}

// The rows the planner counts GLANCE taking are what it draws at every
// width: its key's last row on the last of them, with every row of its key
// whole, and nothing under them; of a pool, one account, and four with a
// name at a label's most.
func TestGlanceTakesTheRowsThePlannerCountsAtEveryWidth(t *testing.T) {
	four := append(trioConfigured(), config.Account{ID: "lab", Label: "research-lab-and-consult"})
	for _, accounts := range []config.Accounts{trioConfigured(), trioConfigured()[:1], four} {
		for width := 28; width <= 220; width++ {
			counted := graphsGiven{width: width, rows: 200, shown: GraphsSectionsNamed([]string{"glance"}), accounts: accounts}.shape().glance.rows()
			c, l := glanceIn(glanceOf(trio(), Look{}), accounts, width, 3)
			rows := glanceRows(c)
			key := rows[counted-len(l.key) : counted]
			for i, k := range l.key {
				if key[i] != strings.TrimRight(k.plain(), " ") {
					t.Fatalf("of %d accounts at %d columns, the key's row %d is %q, want %q, on the last of the %d rows counted", len(accounts), width, i, key[i], k.plain(), counted)
				}
			}
			if rows[counted-len(l.key)-1] != "" || slices.ContainsFunc(rows[counted:], func(r string) bool { return r != "" }) {
				t.Fatalf("of %d accounts at %d columns, GLANCE is\n%s\nwant its key on the last of %d rows, under a blank row", len(accounts), width, strings.Join(rows, "\n"), counted)
			}
		}
	}
}
