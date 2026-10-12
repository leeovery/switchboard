package dashboard

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/prose"
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
// the columns worth and against its price start at, a column at 0 not shown.
type tokensColumns struct {
	of      int
	counts  []tokensCount
	worth   int
	against int
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

// tokensColumns are the frame's columns of Tokens: a phone's, or the wide
// ones, cut at the right below 160 columns.
func (f Frame) tokensColumns() tokensColumns {
	if f.phone() {
		return phoneTokens
	}
	return wideTokens
}

// tokensRow is a row of Tokens, a period's, or one it's split into: what it
// names at 0; at the split's column, its versions' count or the accounts it
// ran on, what says which; its counts and its worth, in their ink; and its
// worth against its price, a period's alone. A row of the period still
// running is lit, its figures numbered as Tokens', or where it's split,
// as its account's, its version's, or both.
type tokensRow struct {
	name        line
	of          line
	ofIs        string
	tokens      views.Tokens
	worth       views.Worth
	ink         ink
	against     line
	lit, split  bool
	account, id string
}

// figure is the row's figure in the column named, as l draws it: numbered
// where the row is lit.
func (r tokensRow) figure(column string, l line) line {
	if !r.lit {
		return l
	}
	n := Number{Kind: NumberTokens, Of: column}
	if r.split {
		n = Number{Kind: NumberTokensSplit, Account: r.account, Of: strings.TrimSpace(r.id + " " + column)}
	}
	return l.numbered(n)
}

// historyTokens draws Tokens from row top: its heads, then its periods,
// newest first, a blank row between them, as many as fit whole, scrolled as
// the frame has it, with how many more are below; and the footnote on the
// view's last rows. With nothing recorded, it says so.
func (f Frame) historyTokens(c *canvas, top int) {
	page := f.History.Tokens
	if page == nil || len(page.Periods) == 0 {
		c.text(0, top, "nothing recorded yet", dimInk)
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
	body := c.within(bounds{x: 0, y: placed.first, width: f.Width, rows: placed.rows - min(below, 1)})
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
		c.line(0, min(y, placed.first+placed.rows-1), moreBelow(below, f.History.TokensAsk.Period.Name(), " to scroll"))
	}
}

// tokensHeads draws Tokens' heads along row y: the period, then models, or
// split by model, on; each count's; worth; and against its price.
func (f Frame) tokensHeads(c *canvas, y int, cols tokensColumns) {
	c.text(0, y, f.History.TokensAsk.Period.Name(), dimInk)
	if cols.of > 0 {
		of := "models"
		if f.History.TokensSplit == TokensByModel {
			of = "on"
		}
		c.text(cols.of, y, of, dimInk)
	}
	for _, count := range cols.counts {
		c.right(count.end, y, line{{count.head, dimInk}})
	}
	c.text(cols.worth, y, "worth", dimInk)
	if cols.against > 0 {
		c.text(cols.against, y, "against its price", dimInk)
	}
}

// tokensFootnote says what worth is, as of the price table's date, and
// where the rows are split by model, that each version is at its own price.
func (f Frame) tokensFootnote(asOf string) line {
	if day, ok := civilDay(asOf); ok {
		asOf = day.Format("2 Jan 2006")
	}
	note := "worth: what the same tokens would cost through the API, at its prices as of " + asOf
	if f.History.TokensSplit == TokensByModel || f.History.TokensSplit == TokensByAccountAndModel {
		note += ", each version at its own price"
	}
	return line{{note, faintInk}}
}

// draw draws the row along row y.
func (cols tokensColumns) draw(c *canvas, y int, r tokensRow) {
	nameEnd := cols.counts[0].end - tokensCountCells
	if cols.of > 0 && r.of != nil {
		nameEnd = cols.of
		c.line(cols.of, y, r.figure(r.ofIs, r.of.fit(cols.counts[0].end-tokensCountCells-cols.of)))
	}
	c.line(0, y, r.name.fit(nameEnd-1))
	for _, count := range cols.counts {
		c.right(count.end, y, r.figure(count.head, line{{tokenCount(count.count(r.tokens)), r.ink}}))
	}
	c.line(cols.worth, y, r.figure("worth", worthSaid(r.worth, r.ink)))
	if cols.against > 0 && r.against != nil {
		c.line(cols.against, y, r.figure("against", r.against))
	}
}

// tokensRows are the period's rows: its own, then those Tokens splits it
// into, as the frame has it split.
func (f Frame) tokensRows(t views.TokenPeriod) []tokensRow {
	split := tokensRow{ink: mutedInk, lit: t.SoFar, split: true}
	rows := []tokensRow{{
		name: line{{periodName(t.First, f.History.TokensAsk.Period), titleInk}}, of: line{{prose.Counted(t.Versions, "model"), mutedInk}}, ofIs: "models",
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
	r.name = line{{"└ ", faintInk}, {tokensAccount(a), secondaryInk}}
	r.of, r.ofIs = line{{prose.Counted(len(a.ByModel), "model"), dimInk}}, "models"
	r.tokens, r.worth, r.account = a.Tokens, a.Worth, a.Account
	return r
}

// ofModel is the row, as split, of a model version, after the branch given:
// its id, as ccusage names it, its counts and its worth.
func (r tokensRow) ofModel(m views.ModelTokens, branch string) tokensRow {
	r.name = line{{branch, faintInk}, {strings.TrimPrefix(m.Model, "claude-"), secondaryInk}}
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
	r.of, r.ofIs = line{{strings.Join(names, " "), dimInk}}, "on"
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
// "4.6× its $138 week". It's a dash where no plan is known, or they cost
// nothing.
func againstItsPrice(a views.Against, unit string) line {
	if a.Ratio == nil || a.Cost == nil {
		return line{noFigure}
	}
	return append(againstSaid(a.Ratio), span{" its " + a.Cost.GroupedDollars() + " " + unit, dimInk})
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
	note := f.tokensFootnote(page.PricesAsOf).wrap(f.Width)
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
