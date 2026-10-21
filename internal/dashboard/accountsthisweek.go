package dashboard

import (
	"maps"
	"slices"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// Detail's THIS WEEK, wide: its column's width, and its items, each figure's
// parts over the rows they need.

// thisWeekCells are the cells of THIS WEEK's column: its labels, and its
// figures' cells, at least thisWeekFigures, and as many as the widest
// bounded part of a figure takes, so each such part reads whole on a row of
// its own: a limit hit, of any window limitedWindows has, as its short name
// says it and counted as often as views.LimitsHitMost has it, "5h ×403",
// "Fable wk ×12"; and all accounts' extra use, on for one fewer than every
// configured account, "on for 2 of 3". The same for every choice in the
// list, so nothing moves as the choice does. Of every account, the plans known, the
// other bounded part, says it shortest in fewer cells, "2/3"; and all
// accounts' limits, how many, in fewer still. The requests, the plan's name,
// from the price table, or the plans' summed price, and the worth and its
// ratio have no bound, and are cut at the weeks' edge with ….
func (f Frame) thisWeekCells(m status.Moment) int {
	figures, n := thisWeekFigures, len(f.Accounts.Accounts)
	if n > 1 {
		figures = max(figures, partsWidth(extraShort(status.AllExtra{State: status.ExtraOn, On: n - 1, Of: n})))
	}
	short := shortWindows(f.windowNamer(m))
	for _, key := range f.limitedWindows(m) {
		hit := views.LimitsHit(map[string]int{key: views.LimitsHitMost(key)}, short)
		figures = max(figures, partsWidth([]part{figureIn(hit[0], errorInk)}))
	}
	return thisWeekLabels + figures
}

// limitedWindows are the windows a limit may be named of in THIS WEEK, by
// their keys: every one the moment m lists, and every one any row of the
// period's page records a limit of, listed or not.
func (f Frame) limitedWindows(m status.Moment) []string {
	keys := m.Listed(true)
	if page := f.Accounts.Page; page != nil {
		for _, row := range append(slices.Clone(page.Rows), page.All) {
			keys = append(keys, slices.Collect(maps.Keys(row.Totals.Limits))...)
		}
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

// thisWeekForms are a part of a THIS WEEK figure in its forms, the full first,
// then each shorter.
type thisWeekForms [][]part

// fitting is the first of the forms that fits the cells given, else the
// last, for fitParts to cut.
func (f thisWeekForms) fitting(cells int) []part {
	for _, form := range f {
		if partsWidth(form) <= cells {
			return form
		}
	}
	return f[len(f)-1]
}

// thisWeekSaid is an item of THIS WEEK, wide: its label; its figure's parts,
// in their forms; and what sets them apart on a row.
type thisWeekSaid struct {
	label string
	parts []thisWeekForms
	apart part
}

// figure is an item whose figure is one part, in one form.
func figure(label string, said []part) thisWeekSaid {
	return thisWeekSaid{label: label, parts: []thisWeekForms{{said}}}
}

// rows are the item's figure over the rows it needs, cells wide: each part
// after the first on the row before, set apart, in its full form, where it
// fits there; else on a row of its own, in the first of its forms that fits.
func (s thisWeekSaid) rows(cells int) [][]part {
	var rows [][]part
	for i, forms := range s.parts {
		if i > 0 {
			last := len(rows) - 1
			if with := slices.Concat(rows[last], []part{s.apart}, forms[0]); partsWidth(with) <= cells {
				rows[last] = with
				continue
			}
		}
		rows = append(rows, forms.fitting(cells))
	}
	return rows
}

// thisWeekSaid are THIS WEEK's items, wide, as the words of Accounts say
// them, in text.secondary: the requests; the limits, by window, each hit
// named in full, or by its short name, "5h ×2", where that doesn't fit a row,
// or all accounts' how many; the plan, by its name, or all accounts' summed;
// the worth, with its ratio, and of every account where some plans aren't
// known, over those known, as knownForms says it, its forms shorter where
// they don't fit; and the extra usage of the accounts shown. Each of the
// period's figures is a dash where the period records nothing, as
// periodParts has it; the plan and extra use are none of them.
func (d detailDrawn) thisWeekSaid() []thisWeekSaid {
	totals, page := d.row.Totals, *d.page
	limits, worth := d.limitsSaid(), d.worthSaid()
	if !page.Recorded {
		limits, worth = figure("limits", noneOf(noFigure.text, secondaryInk)), figure("worth", noneOf(noFigure.text, secondaryInk))
	}
	return []thisWeekSaid{
		figure("requests", periodParts(page, []part{figureIn(prose.Grouped(totals.Requests), secondaryInk)}, secondaryInk)),
		limits,
		figure("plan", planNamed(d.row, secondaryInk)),
		worth,
		figure("extra use", extraShort(d.extra())),
	}
}

// limitsSaid is THIS WEEK's limits, as limitsParts says them: of an account
// that hit any, each window's hits a part, in full, "5-hour ×2", or short,
// "5h ×2", set apart by " · " in text.faint.
func (d detailDrawn) limitsSaid() thisWeekSaid {
	totals, name := d.row.Totals, d.f.windowNamer(d.m)
	if totals.All || totals.LimitCount() == 0 {
		return figure("limits", limitsParts(totals, name, secondaryInk))
	}
	full, short := totals.LimitsHit(name), totals.LimitsHit(shortWindows(name))
	said := thisWeekSaid{label: "limits", apart: wordsIn(accountsApart, faintInk)}
	for i := range full {
		said.parts = append(said.parts, thisWeekForms{{figureIn(full[i], errorInk)}, {figureIn(short[i], errorInk)}})
	}
	return said
}

// worthSaid is THIS WEEK's worth: as worthParts says it, then two cells on,
// its worth against its price, as againstShort says it, "$635  4.3×"; and
// of every account's where some plans aren't known, the plans known, as
// knownForms says them, "2 of 3 plans known", "2 of 3 plans" or "2/3", set
// apart by " · " in text.subtle; the worth alone where no plan is known.
func (d detailDrawn) worthSaid() thisWeekSaid {
	worth := worthParts(d.row, secondaryInk)
	if d.row.Plan.Ratio == nil {
		return figure("worth", worth)
	}
	said := thisWeekSaid{
		label: "worth", apart: wordsIn(accountsApart, dimInk),
		parts: []thisWeekForms{{slices.Concat(worth, []part{wordsIn("  ", secondaryInk)}, againstShort(d.row))}},
	}
	if known := d.row.Plan.Plans; known.Said() != "" {
		said.parts = append(said.parts, thisWeekForms{
			{wordsIn(known.Said(), dimInk)}, {wordsIn(known.SaidShort(), dimInk)}, {wordsIn(known.SaidCompact(), dimInk)},
		})
	}
	return said
}

// thisWeekRows are the rows THIS WEEK's items take under its title, each
// figure's as its rows have them: thisWeekLeastRows, a row an item, where the
// period's page isn't read.
func (d detailDrawn) thisWeekRows() int {
	if d.page == nil {
		return thisWeekLeastRows
	}
	rows, cells := 0, d.thisWeekFigureCells()
	for _, item := range d.thisWeekSaid() {
		rows += len(item.rows(cells))
	}
	return rows
}

// thisWeekFigureCells are the cells THIS WEEK's figures take, from after its
// labels to the weeks' edge.
func (d detailDrawn) thisWeekFigureCells() int {
	return d.l.edge() - d.l.thisWeekAt() - thisWeekLabels
}

// thisWeek draws THIS WEEK from row y, at its column: its title, naming the
// period, then each item, its label thisWeekLabels cells wide in
// text.subtle, and its figure over the rows its rows give it, each cut at
// the weeks' edge, as fitParts cuts it, which only an unbounded figure
// reaches.
func (d detailDrawn) thisWeek(c *canvas, y int) {
	x, cells := d.l.thisWeekAt(), d.thisWeekFigureCells()
	c.line(x, y, line{{d.page.Period.Title(), dimInk}})
	for _, item := range d.thisWeekSaid() {
		for i, row := range item.rows(cells) {
			y++
			if i == 0 {
				c.line(x, y, line{{padded(item.label, thisWeekLabels), dimInk}})
			}
			c.line(x+thisWeekLabels, y, fitParts(cells, periodNumbered(row, d.id, item.label)...))
		}
	}
}
