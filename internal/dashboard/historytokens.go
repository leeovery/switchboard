package dashboard

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/views"
)

// tokensCount is a column of Tokens' counts: its head, the column it ends
// before, its counts right-aligned in the ten cells before it, and the count
// of a row's tokens it shows.
type tokensCount struct {
	head  string
	end   int
	count func(views.Tokens) int
}

// tokensCountCells is how many cells a column of counts is, its head and
// counts right-aligned in them.
const tokensCountCells = 10

// The columns of counts: input, output, cache write, both lifetimes' writes
// together, cache read and total, the four kinds.
var (
	inputCount      = tokensCount{"input", 42, func(t views.Tokens) int { return t.Input }}
	outputCount     = tokensCount{"output", 54, func(t views.Tokens) int { return t.Output }}
	cacheWriteCount = tokensCount{"cache write", 66, func(t views.Tokens) int { return t.CacheWrite }}
	cacheReadCount  = tokensCount{"cache read", 80, func(t views.Tokens) int { return t.CacheRead }}
	totalCount      = tokensCount{"total", 94, func(t views.Tokens) int { return t.Total }}
)

// tokensColumns are where Tokens' columns are: the column a row's split
// names its versions' count or the accounts it ran on at, its counts, and
// the columns worth and against its price start at, a column at 0 not shown;
// and the width, the frame's, a row's worth and against its price are cut at.
type tokensColumns struct {
	of      int
	counts  []tokensCount
	worth   int
	against int
	width   int
}

var (
	// wideTokens are Tokens' columns as history-tokens-160 draws them.
	wideTokens = tokensColumns{
		of:      14,
		counts:  []tokensCount{inputCount, outputCount, cacheWriteCount, cacheReadCount, totalCount},
		worth:   96,
		against: 106,
	}
	// phoneTokens are a phone's: each row's total and worth alone.
	phoneTokens = tokensColumns{counts: []tokensCount{{"total", 30, totalCount.count}}, worth: 32}
)

// tokensColumns are the frame's columns of Tokens, cut at its width: the wide
// ones where their heads fit, else a phone's.
func (f Frame) tokensColumns() tokensColumns {
	cols := wideTokens
	if f.phoneForm(wideTokens.reach) {
		cols = phoneTokens
	}
	cols.width = f.Width
	return cols
}

// tokensRow is a row of Tokens, a period's, or one it's split into: what it
// names at 0; at the split's column, its versions' count or the accounts it
// ran on, what says which; its counts and its worth, in their ink; and its
// worth against its price, a period's alone. A row of the period still
// running is lit, its figures numbered as Tokens', or where it's split,
// as its account's, its version's, or both.
type tokensRow struct {
	name        []part
	of          []part
	ofIs        string
	tokens      views.Tokens
	worth       views.Worth
	ink         ink
	against     []part
	lit, split  bool
	account, id string
}

// number is the number the row's figure in the column named is, where the
// row is lit: none where it isn't.
func (r tokensRow) number(column string) Number {
	switch {
	case !r.lit:
		return Number{}
	case r.split:
		return Number{Kind: NumberTokensSplit, Account: r.account, Of: strings.TrimSpace(r.id + " " + column)}
	}
	return Number{Kind: NumberTokens, Of: column}
}

// historyTokens draws Tokens from row top: its heads, then its periods,
// newest first, a blank row between them, as many as fit whole, scrolled as
// the frame has it, with how many more are below; and the footnote on the
// view's last rows. With nothing recorded, it says so.
func (f Frame) historyTokens(c *canvas, top int) {
	page := f.History.Tokens
	if page == nil || len(page.Periods) == 0 {
		c.line(0, top, line{{"nothing recorded yet", dimInk}}.fit(f.Width))
		return
	}
	cols := f.tokensColumns()
	f.tokensHeads(c, top, cols)
	placed, note := f.tokensPlaced(top)
	for i, l := range note {
		c.line(0, top+f.viewRows()-len(note)+i, l)
	}
	if placed.rows < 1 {
		return
	}
	from := min(max(f.Scroll, 0), placed.most())
	shown := placed.shown(from)
	below := len(page.Periods) - from - shown
	body := c.within(bounds{x: 0, y: placed.first, width: c.width, rows: placed.rows - min(below, 1)})
	y := placed.first
	for i, period := range page.Periods[from : from+shown] {
		if i > 0 {
			y++
		}
		for _, r := range f.tokensRows(period) {
			cols.draw(body, y, r)
			y++
		}
	}
	if below > 0 {
		c.line(0, min(y, placed.first+placed.rows-1), moreBelow(below, f.History.TokensAsk.Period.Name(), " to scroll", f.Width))
	}
}

// tokensHeads draws Tokens' heads along row y, each cut to its column, as
// the row under it is: the period, then models, or split by model, on; each
// count's; worth; and against its price.
func (f Frame) tokensHeads(c *canvas, y int, cols tokensColumns) {
	c.line(0, y, line{{f.History.TokensAsk.Period.Name(), dimInk}}.fit(cols.nameCells(cols.of > 0)))
	if cols.of > 0 {
		of := "models"
		if f.History.TokensSplit == TokensByModel {
			of = "on"
		}
		c.line(cols.of, y, line{{of, dimInk}}.fit(cols.ofCells()))
	}
	for _, count := range cols.counts {
		c.rightFit(count.end, cols.width, y, wordsIn(count.head, dimInk))
	}
	c.line(cols.worth, y, line{{worthHead, dimInk}}.fit(cols.worthCells()))
	if cols.against > 0 {
		c.line(cols.against, y, line{{againstHead, dimInk}}.fit(cols.width-cols.against))
	}
}

// The heads of Tokens' worth, and of its worth against its price.
const (
	worthHead   = "worth"
	againstHead = "against its price"
)

// reach is the width Tokens, at the columns, takes, drawn to the frame's
// width: where its widest row ends, its heads', or the counts', each
// right-aligned to its column's end. A row's worth and against its price
// are cut at the frame's width, what names it and its split short of the
// counts, and the footnote wraps, so they're left out.
func (cols tokensColumns) reach() int {
	end := cols.worth + ansi.StringWidth(worthHead)
	for _, count := range cols.counts {
		end = max(end, count.end)
	}
	if cols.against > 0 {
		end = max(end, cols.against+ansi.StringWidth(againstHead))
	}
	return end
}

// tokensFootnote says what worth is, as of the price table's date, and
// where the rows are split by model, that each version is at its own price:
// the date a figure, so it's never cut or broken across rows.
func (f Frame) tokensFootnote(asOf string) []part {
	if day, ok := civilDay(asOf); ok {
		asOf = day.Format("2 Jan 2006")
	}
	note := []part{wordsIn("worth: what the same tokens would cost through the API, at its prices as of ", faintInk), figureIn(asOf, faintInk)}
	if f.History.TokensSplit == TokensByModel || f.History.TokensSplit == TokensByAccountAndModel {
		note = append(note, wordsIn(", each version at its own price", faintInk))
	}
	return note
}

// draw draws the row along row y, each column cut to its cells.
func (cols tokensColumns) draw(c *canvas, y int, r tokensRow) {
	of := cols.of > 0 && r.of != nil
	if of {
		c.line(cols.of, y, fitParts(cols.ofCells(), r.numbered(r.ofIs, r.of...)...))
	}
	c.line(0, y, fitParts(cols.nameCells(of), r.name...))
	for _, count := range cols.counts {
		c.rightFit(count.end, cols.width, y, figurePart(line{{tokenCount(count.count(r.tokens)), r.ink}}, r.number(count.head)))
	}
	c.line(cols.worth, y, fitParts(cols.worthCells(), r.numbered("worth", worthPart(r.worth, r.ink))...))
	if cols.against > 0 && r.against != nil {
		c.line(cols.against, y, fitParts(cols.width-cols.against, r.numbered("against", r.against...)...))
	}
}

// nameCells is how many cells what names a row has: to a cell before the
// split's column, where of says the row has something there, else before
// the first count's cells; and never past the frame's width.
func (cols tokensColumns) nameCells(of bool) int {
	if of {
		return min(cols.of-1, cols.width)
	}
	return min(cols.counts[0].end-tokensCountCells-1, cols.width)
}

// ofCells is how many cells the split's column has: to the first count's
// cells, and never past the frame's width.
func (cols tokensColumns) ofCells() int {
	return min(cols.counts[0].end-tokensCountCells, cols.width) - cols.of
}

// worthCells is how many cells a row's worth has: to a cell before against
// its price, where it's shown, else to the frame's width.
func (cols tokensColumns) worthCells() int {
	if cols.against > 0 {
		return cols.against - cols.worth - 1
	}
	return cols.width - cols.worth
}

// numbered are the parts with the row's number in the column named, which
// fitParts marks a figure of them with where it's drawn whole.
func (r tokensRow) numbered(column string, parts ...part) []part {
	numbered := slices.Clone(parts)
	for i := range numbered {
		numbered[i].number = r.number(column)
	}
	return numbered
}

// tokensRows are the period's rows: its own, then those Tokens splits it
// into, as the frame has it split.
func (f Frame) tokensRows(t views.TokenPeriod) []tokensRow {
	split := tokensRow{ink: mutedInk, lit: t.SoFar, split: true}
	rows := []tokensRow{{
		name: []part{figureIn(periodName(t.First, f.History.TokensAsk.Period), titleInk)}, of: countedParts(t.Versions, "model", mutedInk), ofIs: "models",
		tokens: t.Tokens, worth: t.Worth, ink: titleInk, against: againstItsPrice(t.Against, f.History.TokensAsk.Period.Name()), lit: t.SoFar,
	}}
	switch f.History.TokensSplit {
	case TokensByAccount:
		for _, a := range t.ByAccount {
			rows = append(rows, split.ofAccount(a))
		}
	case TokensByModel:
		named := tokensAccounts(t.ByAccount)
		for _, m := range t.ByModel {
			rows = append(rows, split.ofModel(m, "└ ").on(m.On, named))
		}
	case TokensByAccountAndModel:
		for _, a := range t.ByAccount {
			nested := split
			nested.account = a.Account
			rows = append(rows, split.ofAccount(a))
			for _, m := range a.ByModel {
				rows = append(rows, nested.ofModel(m, "  └ "))
			}
		}
	}
	return rows
}

// ofAccount is the row, as split, of an account: its name, its versions'
// count, its counts and its worth.
func (r tokensRow) ofAccount(a views.AccountTokens) tokensRow {
	r.name = []part{wordsPart(line{{"└ ", faintInk}, {tokensAccount(a), secondaryInk}})}
	r.of, r.ofIs = countedParts(len(a.ByModel), "model", dimInk), "models"
	r.tokens, r.worth, r.account = a.Tokens, a.Worth, a.Account
	return r
}

// ofModel is the row, as split, of a model version, after the branch given:
// its id, as ccusage names it, its counts and its worth.
func (r tokensRow) ofModel(m views.ModelTokens, branch string) tokensRow {
	r.name = []part{wordsPart(line{{branch, faintInk}, {strings.TrimPrefix(m.Model, "claude-"), secondaryInk}})}
	r.tokens, r.worth, r.id = m.Tokens, m.Worth, m.Model
	return r
}

// on is the row naming the accounts given, by their ids, those its version
// ran on, at the split's column, each as named names it, else by its id.
func (r tokensRow) on(accounts []string, named map[string]string) tokensRow {
	names := make([]string, len(accounts))
	for i, id := range accounts {
		names[i] = cmp.Or(named[id], tokensAccount(views.AccountTokens{Account: id}))
	}
	r.of, r.ofIs = []part{wordsPart(line{{strings.Join(names, " "), dimInk}})}, "on"
	return r
}

// tokensAccount is an account's name as Tokens shows it: as labelled names
// it, as Weeks and Days do; "no account" for the requests none answered.
func tokensAccount(a views.AccountTokens) string {
	if a.Account == "" {
		return "no account"
	}
	return labelled(a.Label, a.Account)
}

// tokensAccounts are the accounts' names, as Tokens shows them, by their ids.
func tokensAccounts(accounts []views.AccountTokens) map[string]string {
	named := make(map[string]string, len(accounts))
	for _, a := range accounts {
		named[a.Account] = tokensAccount(a)
	}
	return named
}

// periodName names a period by its first day, as 28 Sep, or by day its day,
// Thu 1 Oct, or by month its month, Oct 2026.
func periodName(first string, period views.TokensPeriod) string {
	if period == views.TokensByMonth {
		return monthName(first, true)
	}
	day, ok := civilDay(first)
	switch {
	case !ok:
		return first
	case period == views.TokensByDay:
		return day.Format(dayLayout)
	}
	return day.Format(weekLayout)
}

// tokenCount writes a count of tokens: under 100 million in millions to one
// place, 4.2M; then in whole millions, 615M; and from a billion in billions
// to two places, 1.06B.
func tokenCount(n int) string {
	millions := float64(n) / 1e6
	switch {
	case math.Round(millions*10) < 1000:
		return fmt.Sprintf("%.1fM", millions)
	case math.Round(millions) < 1000:
		return fmt.Sprintf("%.0fM", millions)
	}
	return fmt.Sprintf("%.2fB", millions/1000)
}

// againstItsPrice is a period's worth against what the plans cost over it,
// as againstSaid writes it, then the price, the period named by unit:
// "4.6× its $138 week", the ratio and the price its figures, the rest its
// words. It's a dash where no plan is known, or they cost nothing.
func againstItsPrice(a views.Against, unit string) []part {
	if a.Ratio == nil || a.Cost == nil {
		return []part{figurePart(line{noFigure}, Number{})}
	}
	return []part{
		figurePart(againstSaid(a.Ratio), Number{}),
		wordsPart(line{{" its ", dimInk}}),
		figurePart(line{{a.Cost.GroupedDollars(), dimInk}}, Number{}),
		wordsPart(line{{" " + unit, dimInk}}),
	}
}

// tokensPlaced is how Tokens' periods fit in the rows they have, from row
// first, a blank row between each and a row under the last shown where more
// are below: each period's height, as the frame has them split.
type tokensPlaced struct {
	heights     []int
	first, rows int
}

// tokensPlaced is the frame's periods of Tokens, its view from row top,
// placed from two rows under its heads to two over its footnote, and the
// footnote, wrapped to the frame's width, which ends on the view's last row.
func (f Frame) tokensPlaced(top int) (tokensPlaced, []line) {
	page := f.History.Tokens
	if page == nil {
		return tokensPlaced{}, nil
	}
	note := wrapParts(f.Width, f.tokensFootnote(page.PricesAsOf)...)
	p := tokensPlaced{first: top + 2}
	p.rows = top + f.viewRows() - len(note) - 1 - p.first
	for _, t := range page.Periods {
		p.heights = append(p.heights, tokensHeight(t, f.History.TokensSplit))
	}
	return p, note
}

// tokensHeight is how many rows a period takes, split as given, as
// tokensRows makes them.
func tokensHeight(t views.TokenPeriod, split TokensSplit) int {
	switch split {
	case TokensByAccount:
		return 1 + len(t.ByAccount)
	case TokensByModel:
		return 1 + len(t.ByModel)
	case TokensByAccountAndModel:
		n := 1
		for _, a := range t.ByAccount {
			n += 1 + len(a.ByModel)
		}
		return n
	}
	return 1
}

// shown is how many periods show scrolled to the one from, whole, with a row
// for how many more are below where they don't all show: at least one.
func (p tokensPlaced) shown(from int) int {
	used, n := -1, 0
	for i := from; i < len(p.heights); i++ {
		need := used + 1 + p.heights[i]
		if i < len(p.heights)-1 {
			need++
		}
		if need > p.rows {
			break
		}
		used, n = used+1+p.heights[i], n+1
	}
	return max(n, 1)
}

// most is the furthest Tokens scrolls, in periods: the first from which the
// rest all show.
func (p tokensPlaced) most() int {
	used := -1
	for i, height := range slices.Backward(p.heights) {
		if used += 1 + height; used > p.rows {
			return min(i+1, len(p.heights)-1)
		}
	}
	return 0
}

// tokensScrolling is how far Tokens, from row top, scrolls, a period at a
// time, scrolled to each offset given: as far as most says, and as many
// periods at once as show from there.
func (f Frame) tokensScrolling(top int) func(offset int) Scrolling {
	placed, _ := f.tokensPlaced(top)
	if len(placed.heights) == 0 || placed.rows < 1 {
		return func(int) Scrolling { return Scrolling{} }
	}
	most := placed.most()
	return func(offset int) Scrolling {
		return Scrolling{Most: most, Page: placed.shown(offset)}
	}
}
