package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/quota"
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
// room over the week's span across sparkCells built from no readings
// history, so known from now's column, drawn in look.
func glanceOf(doc status.Document, look Look) glance {
	return glanceAcross(doc, look, sparkCells)
}

// glanceAcross is glanceOf, the pool's room over the week's span built
// across the columns given.
func glanceAcross(doc status.Document, look Look, columns int) glance {
	m := doc.At(printNow, printPolicy)
	pools := map[views.RunwayWindow]views.PoolSpan{
		GraphsWindow: views.PoolSpan{}.Look(views.PoolRoomFrom{Moment: m, Key: GraphsWindow.KeyIn(m), Columns: columns}),
	}
	return glance{m: m, trails: trioTrails(), pools: pools, look: look}
}

// glanceIn is g drawn as laid out width cells wide, of the accounts given,
// numbered as its moment numbers them, from the top left of a canvas of its
// own extra rows taller than it takes.
func glanceIn(g glance, accounts config.Accounts, width, extra int) (*canvas, glanceLaid) {
	l := layGlance(accounts, width)
	return glanceDrawnIn(g, l, width, extra), l
}

// glanceDrawnIn is g drawn as l lays it out, from the top left of a canvas
// width cells wide and extra rows taller than it takes.
func glanceDrawnIn(g glance, l glanceLaid, width, extra int) *canvas {
	c := newCanvas(width, l.rows()+extra)
	g.draw(c, l, 0, 0)
	return c
}

// glanceRows are the rows drawn on c, as text.
func glanceRows(c *canvas) []string {
	rows := make([]string, c.height())
	for y := range rows {
		rows[y] = rowOf(c, y)
	}
	return rows
}

// GLANCE takes the width: its sparklines grow with it from sparkCells, its
// names, figures and words keeping what they take whole.
func TestGlanceGrowsItsSparklinesToTheWidth(t *testing.T) {
	// The names, personal ◆ with ▲'s cells kept, and their gap, 14; the
	// sparkline 16 at its least, a cell, its figure and two cells, 7; and
	// the words at their widest in full, 24. At its least, the names 14, as
	// personal is no longer than a label's least, and the short words 14.
	if got, want := glancePart(trioConfigured()), (flex.Part{Least: 51, Natural: 61, Grows: true}); got != want {
		t.Errorf("GLANCE's part is %+v, want %+v", got, want)
	}
	if fullWidest() != 24 {
		t.Fatalf("the widest words in full are %d cells, want 24, as the figures above count them", fullWidest())
	}
	for _, width := range []int{61, 62, 113, 160, 220} {
		l := layGlance(trioConfigured(), width)
		if l.width != width || l.column != 14 || l.words != 24 || l.spark != width-45 {
			t.Errorf("at %d columns GLANCE takes %d, its names %d, its words %d, its sparklines %d; want %d, 14, 24 and %d",
				width, l.width, l.column, l.words, l.spark, width, width-45)
		}
	}
}

// Narrowing from its natural, GLANCE's words take usage's short forms first,
// then its names narrow toward their least, its sparklines at their least;
// and where even that doesn't fit, its words take what's left.
func TestGlanceNarrowsItsWordsThenItsNames(t *testing.T) {
	// personal-research, the primary, is 17 cells: its name 23 with its
	// marks and gap, 14 at its least.
	accounts := config.Accounts{{ID: "work", Label: "work"}, {ID: "personal", Label: "personal-research", Primary: true}, {ID: "side", Label: "side"}}
	tests := []struct {
		name                 string
		width                int
		column, spark, words int
	}{
		{name: "growing", width: 120, column: 23, spark: 66, words: 24},
		{name: "whole", width: 70, column: 23, spark: 16, words: 24},
		{name: "its words narrowing", width: 65, column: 23, spark: 16, words: 19},
		{name: "its words short", width: 60, column: 23, spark: 16, words: 14},
		{name: "its names narrowing", width: 58, column: 21, spark: 16, words: 14},
		{name: "its names at their least", width: 51, column: 14, spark: 16, words: 14},
		{name: "its words cut", width: 50, column: 14, spark: 16, words: 13},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := layGlance(accounts, tt.width)
			if l.column != tt.column || l.spark != tt.spark || l.words != tt.words || l.width != tt.width {
				t.Errorf("at %d columns: names %d, sparklines %d, words %d, across %d; want %d, %d, %d, %d",
					tt.width, l.column, l.spark, l.words, l.width, tt.column, tt.spark, tt.words, tt.width)
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
		{name: "at its natural", width: 61, want: []string{
			"              WEEK",
			"",
			"work          ▇▇▇▇▇▆▆▆▆▅▅▄▄▄▃▃  66%  19%/day till Sun 21:00",
			"",
			"personal ◆    ▆▆▆▆▆▄▄▄▄▂▂▁✕     34%  out ~Sat",
			"",
			"side ▲        ▆▆▆▆▆▃▃▃▃▁✕       22%  out ~Fri",
			strings.Repeat("─", 61),
			"Σ pool                ▃▃▂▂▁▁▁▁ 1.2 of 3 left",
			"",
			"▆ room left, start to reset    ▆ heading    ✕ runs out",
			"66% room left now",
		}},
		{name: "on the narrowest phone", width: 52, want: []string{
			"              WEEK",
			"",
			"work          ▇▇▇▇▇▆▆▆▆▅▅▄▄▄▃▃  66%  19%/day · Sun",
			"",
			"personal ◆    ▆▆▆▆▆▄▄▄▄▂▂▁✕     34%  out ~Sat",
			"",
			"side ▲        ▆▆▆▆▆▃▃▃▃▁✕       22%  out ~Fri",
			strings.Repeat("─", 52),
			"Σ pool                ▃▃▂▂▁▁▁▁ 1.2 of 3 left",
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

// Wider than its natural, every row's sparkline, the pool's among them, runs
// from the name column across the cells the layout gives it, as its gauge
// draws it that wide; and each row's figure and words follow it, lined up
// down the rows.
func TestGlancesSparklinesGrowWithTheWidthTheirWordsLinedUp(t *testing.T) {
	for _, width := range []int{61, 80, 113, 160, 220} {
		l := layGlance(trioConfigured(), width)
		g := glanceAcross(trio(), Look{}, l.spark)
		c := glanceDrawnIn(g, l, width, 0)
		accounts, pool := g.lines(l)
		rows := map[int]glanceLine{8: *pool}
		for i, a := range accounts {
			rows[2+2*i] = a
		}
		for y, r := range rows {
			want := newCanvas(l.spark, 1)
			r.gauge.spark.spark(want, Look{}, 0, 0, l.spark)
			cells := []rune(rowOf(c, y) + strings.Repeat(" ", width))
			if got := strings.TrimRight(string(cells[l.column:l.column+l.spark]), " "); got != rowOf(want, 0) {
				t.Errorf("at %d columns, row %d's sparkline reads %q, want %q, its gauge's %d cells wide", width, y, got, rowOf(want, 0), l.spark)
			}
		}
		figure, words := l.column+l.spark+1, l.column+l.spark+afterBar
		for y, want := range map[int][2]string{2: {"66%", "19%/day till Sun 21:00"}, 4: {"34%", "out ~Sat"}, 6: {"22%", "out ~Fri"}} {
			cells := []rune(rowOf(c, y) + strings.Repeat(" ", width))
			if got := [2]string{strings.TrimSpace(string(cells[figure : figure+percentCells])), strings.TrimSpace(string(cells[words:]))}; got != want {
				t.Errorf("at %d columns, row %d's figure and words read %q, want %q, from %d and %d", width, y, got, want, figure, words)
			}
		}
	}
}

// GLANCE labels its accounts as Runway's lanes do, as the cards do,
// unnumbered, in the config's order, those the moment lacks among them.
func TestGlanceLabelsItsAccountsAsRunwayDoes(t *testing.T) {
	for _, tt := range disagreements() {
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
// label cut short with an ellipsis, its marks kept.
func TestGlancesNamesNarrowOnceItsWordsAreShort(t *testing.T) {
	long := func(a *status.Account) { a.Label = "personal-research" }
	accounts := trioConfigured()
	accounts[1].Label = "personal-research"
	c, l := glanceIn(glanceOf(trio(account("personal", long)), Look{}), accounts, 57, 0)
	if l.column != 20 || l.words != shortCells {
		t.Fatalf("at 57 columns the names take %d and the words %d, want 20 and %d", l.column, l.words, shortCells)
	}
	want := "personal-rese… ◆    ▆▆▆▆▆▄▄▄▄▂▂▁✕     34%  out ~Sat"
	if got := rowOf(c, 4); !strings.HasPrefix(got, want) {
		t.Errorf("personal's row is %q, want it to start %q", got, want)
	}
}

// GLANCE's inks: the week's name over its sparklines in text.subtle; an
// account's label bold text.primary, its marks as usage's; its room's figure
// as usage colours a use; its words as usage inks them; the rule in border;
// Σ in text.subtle and pool in text.secondary; and the pool's room bold
// text.secondary.
func TestGlanceInksItsPartsAsUsageDoes(t *testing.T) {
	c, l := glanceIn(glanceOf(trio(), Look{}), trioConfigured(), 113, 0)
	figure := l.column + l.spark + 1
	tests := []struct {
		name string
		x, y int
		want ink
	}{
		{name: "WEEK", x: 14, y: 0, want: dimInk},
		{name: "work's label", x: 0, y: 2, want: titleInk},
		{name: "personal's ◆", x: 9, y: 4, want: primaryInk},
		{name: "side's ▲", x: 5, y: 6, want: accentInk},
		{name: "work's week's room, 66%", x: figure + 1, y: 2, want: ink{token: theme.VizRamp1, bold: true}},
		{name: "work's week's words", x: figure + 6, y: 2, want: mutedInk},
		{name: "the rule", x: 0, y: 7, want: borderInk},
		{name: "Σ", x: 0, y: 8, want: dimInk},
		{name: "pool", x: 2, y: 8, want: noteInk},
		{name: "the pool's room", x: figure, y: 8, want: strongInk},
		{name: "of 3 left", x: figure + 4, y: 8, want: mutedInk},
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

// A sparkline's ✕ is where the moment has the week run out, its column of
// the week's span across the sparkline's cells; none where it doesn't.
func TestGlancesSparklinesAgreeWithTheMoment(t *testing.T) {
	g := glanceOf(trio(), Look{})
	for _, a := range g.m.Accounts {
		for _, cells := range []int{sparkCells, 40} {
			key := GraphsWindow.KeyIn(g.m)
			w, _ := a.Window(key)
			c := newCanvas(cells, 1)
			g.accountGauge(a, true).spark.spark(c, Look{}, 0, 0, cells)
			x, _, crossed := crossOn(c, cells, 1)
			if crossed != w.RunsOut {
				t.Errorf("%s's week, %d cells, has ✕ %v, want %v", a.ID, cells, crossed, w.RunsOut)
				continue
			}
			tl := views.Timeline{Start: w.Start, End: w.Start.Add(w.Length), Columns: cells}
			if want := tl.Column(w.Out.At); crossed && x != want {
				t.Errorf("%s's week, %d cells, has ✕ in column %d, want %d, where Out.At falls", a.ID, cells, x, want)
			}
		}
	}
}

// Each account's row says how it stands as its card does: not read, or
// without a usable token, or lacking the week, as one whose reads carry the
// 5-hour alone, its sparkline empty, its figure —, and usage's words for it;
// stale, its sparkline faded and its figure in text.subtle; out of the pool,
// faint, its name, figure and words in text.faint, its sparkline faded; and
// one the config has that the document doesn't hold yet, as one not read.
func TestGlanceSaysEachAccountsStateAsItsCardDoes(t *testing.T) {
	unread := trio(account("work", func(a *status.Account) { a.FetchedAt = time.Time{} }))
	weekless := trio(account("work", func(a *status.Account) {
		a.Windows = slices.DeleteFunc(a.Windows, func(w quota.Window) bool { return w.Key == "7d" })
	}))
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
		{name: "not read", doc: unread, row: 2, want: "work                              —  not read yet", figure: faintInk, words: dimInk, label: titleInk},
		{name: "without a usable token", doc: tokenless, row: 2, want: "work                              —  no token", figure: faintInk, words: errorInk, label: titleInk},
		{name: "lacking the week", doc: weekless, row: 2, want: "work                              —  no week limit", figure: faintInk, words: dimInk, label: titleInk},
		{name: "stale", doc: stale, row: 2, want: "work          ▇▇▇▇▇▆▆▆▆         66%  not read since 14:42", spark: &faded, figure: dimInk, words: dimInk, label: titleInk},
		{name: "out of the pool", doc: out, row: 2, want: "work          ▇▇▇▇▇▆▆▆▆▅▅▄▄▄▃▃  66%  19%/day till Sun 21:00", spark: &faded, figure: ink{token: theme.TextFaint, bold: true}, words: faintInk, label: ink{token: theme.TextFaint, bold: true}},
		{name: "not in the document yet", doc: trio(), accounts: added, row: 8, want: "spare                             —  not read yet", figure: faintInk, words: dimInk, label: titleInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts := tt.accounts
			if accounts == nil {
				accounts = trioConfigured()
			}
			c, _ := glanceIn(glanceOf(tt.doc, Look{}), accounts, 61, 0)
			if row := rowOf(c, tt.row); !strings.HasPrefix(row, tt.want) {
				t.Fatalf("the row is %q, want it to start %q", row, tt.want)
			}
			if tt.spark != nil {
				if got := c.at(14, tt.row).ink; !sameFade(got, *tt.spark) {
					t.Errorf("its sparkline is in %+v, want %+v", got, *tt.spark)
				}
			}
			for _, at := range []struct {
				what string
				x    int
				want ink
			}{{"its label", 0, tt.label}, {"its figure", 34, tt.figure}, {"its words", 37, tt.words}} {
				if got := c.at(at.x, tt.row).ink; got != at.want {
					t.Errorf("%s, %q, is in %+v, want %+v", at.what, c.at(at.x, tt.row).glyph, got, at.want)
				}
			}
		})
	}
}

// The pool's row, under usage's rule, has the sparkline of the pool's room
// over the week's span, each column its room of all its accounts' in
// eighths, before now's column in a lasting window's ink, from now's as it
// heads; and its room, as usage words it, pinned after it. With one account
// there's no pool, nor a row for it; with two configured that the document
// doesn't hold yet, its rows are kept, blank.
func TestGlancesPoolRowIsThePoolsRoomOverTheWeeksSpan(t *testing.T) {
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

	pinned, _ := glanceIn(glanceOf(trio(pinnedTo("personal", "side")), Look{}), trioConfigured(), 61, 0)
	if got, want := rowOf(pinned, 8), "Σ pool                ▂▂▁▁▁    0.6 of 2 left · pinned"; !strings.HasPrefix(got, want) || strings.Count(got, "pinned") != 1 {
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
	if got := layGlance(trioConfigured(), 113).key; len(got) != 1 || got[0].width() != want.width() || got[0].plain() != want.plain() {
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
	if got := len(layGlance(trioConfigured(), 52).key); got != 2 {
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
			given := graphsGiven{width: width, rows: 200, shown: GraphsSectionsNamed([]string{"glance"}), accounts: accounts}
			l := given.shape().glance
			counted := l.rows()
			rows := glanceRows(glanceDrawnIn(glanceOf(trio(), Look{}), l, width, 3))
			key := rows[counted-len(l.key) : counted]
			for i, k := range l.key {
				if key[i] != strings.TrimRight(k.plain(), " ") {
					t.Fatalf("of %d accounts, at %d columns, the key's row %d is %q, want %q, on the last of the %d rows counted", len(accounts), width, i, key[i], k.plain(), counted)
				}
			}
			if rows[counted-len(l.key)-1] != "" || slices.ContainsFunc(rows[counted:], func(r string) bool { return r != "" }) {
				t.Fatalf("of %d accounts, at %d columns, GLANCE is\n%s\nwant its key on the last of %d rows, under a blank row", len(accounts), width, strings.Join(rows, "\n"), counted)
			}
		}
	}
}
