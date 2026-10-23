package dashboard

import (
	"cmp"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// Tokens' parts across a row, in their order: what names the row; the
// split's column, the row's versions' count or the accounts it ran on; its
// counts, as tokensCounts has them; its worth; and its worth against its
// price; and how many they are.
const (
	tokensName = iota
	tokensOf
	tokensInput
	tokensOutput
	tokensCacheWrite
	tokensCacheRead
	tokensTotal
	tokensWorth
	tokensAgainst
	tokensPartsCount
)

// The turns Tokens' parts give way in as the frame narrows, in this order,
// one at a time, so its names are the last to give: the split's column; the
// counts of cache write, input and output; against its price; the count of
// cache read; and then the names, to their least. Total and worth stay.
const (
	ofLeaves = iota + 1
	cacheWriteLeaves
	inputLeaves
	outputLeaves
	againstLeaves
	cacheReadLeaves
	namesNarrow
)

// The cells of Tokens' fixed parts and the gaps before them: the names'
// natural at the least, which holds a period's name and a version's id after
// its branch, └ sonnet-5-5; the split's column's least, two cells after
// them, running to the first count's cells; total a cell after the column
// before it; worth, two cells after total; and against its price a cell
// after worth.
const (
	tokensNamesCells = 12
	tokensOfCells    = 18
	tokensWorthCells = 9
	tokensOfGap      = 2
	tokensTotalGap   = 1
	tokensWorthGap   = 2
	tokensAgainstGap = 1
)

// tokensRatioMost is the ratio against its price's least is reckoned at:
// 99.9×, 5 cells, the most prose.Ratio writes up to 9999×, as it writes
// whole from 100×.
const tokensRatioMost = 99.9

// The branches before a row's name under its period: an account's or a
// version's, and a version's nested under its account.
const (
	tokensBranch = "└ "
	tokensNested = "  └ "
)

// tokensCount is a column of Tokens' counts: its head; its part; the cells
// its head and counts are right-aligned in, from the part's start, those to
// their left keeping it apart from the column before; the cells after them
// it keeps clear, which leave with it; the count of a row's tokens it shows;
// and the turn it leaves in, 0 for never.
type tokensCount struct {
	head   string
	part   int
	cells  int
	after  int
	count  func(views.Tokens) int
	leaves int
}

// tokensCounts are the columns of counts: input, output, cache write, both
// lifetimes' writes together, cache read and total, the four kinds. Cache
// read keeps total apart from it, so where it leaves, total sits a cell
// after the column before, the names among them.
var tokensCounts = []tokensCount{
	{"input", tokensInput, 10, 0, func(t views.Tokens) int { return t.Input }, inputLeaves},
	{"output", tokensOutput, 12, 0, func(t views.Tokens) int { return t.Output }, outputLeaves},
	{"cache write", tokensCacheWrite, 12, 0, func(t views.Tokens) int { return t.CacheWrite }, cacheWriteLeaves},
	{"cache read", tokensCacheRead, 14, 3, func(t views.Tokens) int { return t.CacheRead }, cacheReadLeaves},
	{"total", tokensTotal, 10, 0, func(t views.Tokens) int { return t.Total }, 0},
}

// tokensColumns are where Tokens' parts lie across the frame, each by its
// part, as flex lays them; and the frame's width, where every row is cut.
type tokensColumns struct {
	spans []flex.Span
	width int
}

// tokensColumns are Tokens' parts, as tokensParts has them, laid out across
// the frame's width.
func (f Frame) tokensColumns() tokensColumns {
	spans, _ := flex.Lay(f.Width, f.tokensParts()...)
	return tokensColumns{spans: spans, width: f.Width}
}

// tokensParts are Tokens' parts, in their order: the names, from their
// least, labelsLeast of their natural, as tokensNamesNatural has it, holding
// it until every column that leaves has left; the split's column, raised toward its
// natural, as tokensOfNatural has it, before against its price grows; each
// count's; worth; and against its price, from its least, as tokensAgainst
// has it, which takes the rest of the row.
func (f Frame) tokensParts() []flex.Part {
	parts := make([]flex.Part, tokensPartsCount)
	names := f.tokensNamesNatural()
	parts[tokensName] = flex.Part{Least: labelsLeast(names), Natural: names, Narrows: namesNarrow}
	parts[tokensOf] = flex.Part{Least: tokensOfCells, Natural: f.tokensOfNatural(), Leaves: ofLeaves, Gap: tokensOfGap}
	for _, count := range tokensCounts {
		parts[count.part] = flex.Part{Least: count.cells + count.after, Natural: count.cells + count.after, Leaves: count.leaves}
	}
	parts[tokensTotal].Gap = tokensTotalGap
	parts[tokensWorth] = flex.Part{Least: tokensWorthCells, Natural: tokensWorthCells, Gap: tokensWorthGap}
	against := f.tokensAgainstLeast()
	parts[tokensAgainst] = flex.Part{Least: against, Natural: against, Grows: true, Leaves: againstLeaves, Gap: tokensAgainstGap}
	return parts
}

// tokensNamesNatural is the names' natural, of what the split names and
// what's configured, never of the rows shown: tokensNamesCells, which holds
// the periods' names and the versions'; split by account, or by account and
// model, each account configured, as Tokens names it, after its branch, as
// labelColumnCells counts it; and by account and model, a version's id
// nested under its account.
func (f Frame) tokensNamesNatural() int {
	natural := tokensNamesCells
	split := f.History.TokensSplit
	if split == TokensByAccountAndModel {
		natural += ansi.StringWidth(tokensNested) - ansi.StringWidth(tokensBranch)
	}
	if split == TokensByAccount || split == TokensByAccountAndModel {
		for _, a := range f.History.Accounts {
			natural = max(natural, ansi.StringWidth(tokensBranch)+labelColumnCells(labelled(a.Label, a.ID)))
		}
	}
	return natural
}

// tokensOfNatural is the split's column's natural: its least, and split by
// model, where it names the accounts a version ran on, every account
// configured, as Tokens names it, a space apart, each as labelColumnCells
// counts it.
func (f Frame) tokensOfNatural() int {
	if f.History.TokensSplit != TokensByModel || len(f.History.Accounts) == 0 {
		return tokensOfCells
	}
	listed := len(f.History.Accounts) - 1
	for _, a := range f.History.Accounts {
		listed += labelColumnCells(labelled(a.Label, a.ID))
	}
	return max(listed, tokensOfCells)
}

// tokensAgainstLeast is against its price's least, of what's configured,
// never of the rows shown: its head; and where the plans configured cost
// anything, its words at their widest for what they cost over a whole
// period, the ratio at tokensRatioMost, so where they don't fit it leaves,
// never cut.
func (f Frame) tokensAgainstLeast() int {
	least := ansi.StringWidth(againstHead)
	if page := f.History.Tokens; page != nil && page.PeriodCost != nil {
		widest := againstItsPrice(views.Against{Ratio: new(float64(tokensRatioMost)), Cost: page.PeriodCost}, f.History.TokensAsk.Period.Name(), 0)
		least = max(least, partsWidth(widest))
	}
	return least
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

// historyTokens draws Tokens from row top: its heads; then its periods,
// newest first, a blank row between them, as periods or byRows draws them,
// as each fits whole or not; and the footnote on the view's last rows. With
// nothing recorded, it says so.
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
	switch {
	case placed.rows < 1:
	case placed.byRows():
		f.tokensByRows(c, cols, placed)
	default:
		f.tokensByPeriods(c, cols, placed)
	}
}

// tokensByPeriods draws Tokens' periods, every one of which fits whole, as
// placed places them: as many as fit, scrolled as the frame has it, a period
// at a time, and under them how many more are below.
func (f Frame) tokensByPeriods(c *canvas, cols tokensColumns, placed tokensPlaced) {
	periods := f.History.Tokens.Periods
	from := min(max(f.Scroll, 0), placed.most())
	shown := placed.shown(from)
	below := len(periods) - from - shown
	out := min(below, 1)
	body := c.within(bounds{y: placed.first, width: c.width, rows: placed.rows - out})
	y := placed.first
	for i, period := range periods[from : from+shown] {
		if i > 0 {
			y++
		}
		for _, r := range f.tokensRows(period) {
			cols.draw(body, y, r)
			y++
		}
	}
	if out > 0 {
		c.line(0, y, outOfView(0, below, f.Width, periodsOut(f.History.TokensAsk.Period.Name())))
	}
}

// tokensByRows draws Tokens' periods, where one doesn't fit whole, a row at
// a time, as placed pages them: the rows shown, scrolled as the frame has
// it, and after them how many rows are out of view, above and below.
func (f Frame) tokensByRows(c *canvas, cols tokensColumns, placed tokensPlaced) {
	paged := placed.paged()
	from, to := paged.shown(f.Scroll)
	body := c.within(bounds{y: placed.first, width: c.width, rows: to - from})
	y := placed.first - from
	for i, period := range f.History.Tokens.Periods {
		if height := placed.heights[i]; y+height > placed.first && y < placed.first+to-from {
			for j, r := range f.tokensRows(period) {
				cols.draw(body, y+j, r)
			}
		}
		y += placed.heights[i] + 1
	}
	paged.said(c, placed.first, from, to, f.Width, historyRowsOut...)
}

// tokensHeads draws Tokens' heads along row y, each cut to its column, as
// the row under it is: the period, then models, or split by model, on; each
// count's; worth; and against its price.
func (f Frame) tokensHeads(c *canvas, y int, cols tokensColumns) {
	c.line(0, y, line{{f.History.TokensAsk.Period.Name(), dimInk}}.fit(cols.cells(tokensName)))
	if cols.shows(tokensOf) {
		of := "models"
		if f.History.TokensSplit == TokensByModel {
			of = "on"
		}
		c.line(cols.at(tokensOf), y, line{{of, dimInk}}.fit(cols.cells(tokensOf)))
	}
	for _, count := range cols.counts() {
		c.rightFit(cols.countEnd(count), cols.width, y, wordsIn(count.head, dimInk))
	}
	c.line(cols.at(tokensWorth), y, line{{worthHead, dimInk}}.fit(cols.worthCells()))
	if cols.shows(tokensAgainst) {
		c.line(cols.at(tokensAgainst), y, line{{againstHead, dimInk}}.fit(cols.cells(tokensAgainst)))
	}
}

// The heads of Tokens' worth, and of its worth against its price.
const (
	worthHead   = "worth"
	againstHead = "against its price"
)

// tokensFootnote says what worth is, as of the price table's date, and
// where the rows are split by model, that each version is at its own price:
// the date a figure, so it's never cut or broken across rows.
func (f Frame) tokensFootnote(asOf string) []part {
	note := []part{wordsIn("worth: what the same tokens would cost through the API, at its prices as of ", faintInk), figureIn(pricesDated(asOf), faintInk)}
	if f.History.TokensSplit == TokensByModel || f.History.TokensSplit == TokensByAccountAndModel {
		note = append(note, wordsIn(", each version at its own price", faintInk))
	}
	return note
}

// pricesDated is the price table's date as worth is said to be priced as
// of it, as "1 Oct 2026": as given where it isn't a date.
func pricesDated(asOf string) string {
	if day, ok := dayfile.Civil(asOf); ok {
		return prose.DayMonthYear(day)
	}
	return asOf
}

// draw draws the row along row y, each column cut to its cells.
func (cols tokensColumns) draw(c *canvas, y int, r tokensRow) {
	if cols.shows(tokensOf) && r.of != nil {
		c.line(cols.at(tokensOf), y, fitParts(cols.cells(tokensOf), r.numbered(r.ofIs, r.of...)...))
	}
	c.line(0, y, fitParts(cols.cells(tokensName), r.name...))
	for _, count := range cols.counts() {
		c.rightFit(cols.countEnd(count), cols.width, y, figurePart(line{{prose.Millions(count.count(r.tokens)), r.ink}}, r.number(count.head)))
	}
	c.line(cols.at(tokensWorth), y, fitParts(cols.worthCells(), r.numbered("worth", worthPart(r.worth, r.ink))...))
	if cols.shows(tokensAgainst) && r.against != nil {
		c.line(cols.at(tokensAgainst), y, fitParts(cols.cells(tokensAgainst), r.numbered("against", r.against...)...))
	}
}

// shows reports whether the part shows: all but one that left.
func (cols tokensColumns) shows(part int) bool {
	return !cols.spans[part].Left
}

// at is the column the part starts at.
func (cols tokensColumns) at(part int) int {
	return cols.spans[part].At
}

// end is the column the part ends before.
func (cols tokensColumns) end(part int) int {
	return cols.spans[part].At + cols.spans[part].Size
}

// countEnd is the column the count's head and counts end before.
func (cols tokensColumns) countEnd(count tokensCount) int {
	return cols.at(count.part) + count.cells
}

// cells is how many cells the part has: its own, never past the frame's
// width.
func (cols tokensColumns) cells(part int) int {
	return max(min(cols.end(part), cols.width)-cols.at(part), 0)
}

// worthCells is how many cells a row's worth has: its own, where against its
// price shows after it, else to the frame's width.
func (cols tokensColumns) worthCells() int {
	if cols.shows(tokensAgainst) {
		return cols.cells(tokensWorth)
	}
	return max(cols.width-cols.at(tokensWorth), 0)
}

// counts are the columns of counts that show.
func (cols tokensColumns) counts() []tokensCount {
	return slices.DeleteFunc(slices.Clone(tokensCounts), func(count tokensCount) bool { return !cols.shows(count.part) })
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
		tokens: t.Tokens, worth: t.Worth, ink: titleInk, against: againstItsPrice(t.Against, f.History.TokensAsk.Period.Name(), f.tokensColumns().cells(tokensAgainst)), lit: t.SoFar,
	}}
	switch f.History.TokensSplit {
	case TokensByAccount:
		for _, a := range t.ByAccount {
			rows = append(rows, split.ofAccount(a))
		}
	case TokensByModel:
		named := tokensAccounts(t.ByAccount)
		for _, m := range t.ByModel {
			rows = append(rows, split.ofModel(m, tokensBranch).on(m.On, named))
		}
	case TokensByAccountAndModel:
		for _, a := range t.ByAccount {
			nested := split
			nested.account = a.Account
			rows = append(rows, split.ofAccount(a))
			for _, m := range a.ByModel {
				rows = append(rows, nested.ofModel(m, tokensNested))
			}
		}
	}
	return rows
}

// ofAccount is the row, as split, of an account: its name, its versions'
// count, its counts and its worth.
func (r tokensRow) ofAccount(a views.AccountTokens) tokensRow {
	r.name = []part{wordsPart(line{{tokensBranch, faintInk}, {tokensAccount(a), secondaryInk}})}
	r.of, r.ofIs = countedParts(len(a.ByModel), "model", dimInk), "models"
	r.tokens, r.worth, r.account = a.Tokens, a.Worth, a.Account
	return r
}

// ofModel is the row, as split, of a model version, after the branch given:
// its id, as ccusage names it, its counts and its worth.
func (r tokensRow) ofModel(m views.ModelTokens, branch string) tokensRow {
	r.name = []part{wordsPart(line{{branch, faintInk}, {modelNamed(m.Model), secondaryInk}})}
	r.tokens, r.worth, r.id = m.Tokens, m.Worth, m.Model
	return r
}

// on is the row naming the accounts given, by their ids, those its version
// ran on, at the split's column, each as named names it, else by its id, as
// asLabelled cuts it.
func (r tokensRow) on(accounts []string, named map[string]string) tokensRow {
	names := make([]string, len(accounts))
	for i, id := range accounts {
		name := cmp.Or(named[id], tokensAccount(views.AccountTokens{Account: id}))
		names[i] = asLabelled(name)
	}
	r.of, r.ofIs = []part{wordsPart(line{{strings.Join(names, " "), dimInk}})}, "on"
	return r
}

// tokensAccount is an account's name as Tokens shows it: as labelled names
// it, as Weeks and Days do; views.NoAccount for the requests none answered.
func tokensAccount(a views.AccountTokens) string {
	if a.Account == "" {
		return views.NoAccount
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
	day, ok := dayfile.Civil(first)
	switch {
	case !ok:
		return first
	case period == views.TokensByDay:
		return prose.Day(day)
	}
	return prose.DayMonth(day)
}

// againstItsPrice is a period's worth against what the plans cost over it,
// as againstSaid writes it, then the price, the period named by unit:
// "4.6× its $138 week", the ratio and the price its figures, the rest its
// words; then, of every account's, as knownParts says it in the cells given
// that are left: "4.4× its $92 week · 2 of 3 plans known". It's a dash where
// no plan is known, or they cost nothing.
func againstItsPrice(a views.Against, unit string, cells int) []part {
	said := againstPriced(a, unit)
	if !pricedAgainst(a) {
		return said
	}
	return append(said, knownParts(a.Plans, cells-partsWidth(said))...)
}

// againstPriced is a period's worth against what the plans cost over it, as
// againstItsPrice says it, without the plans known: "4.6× its $138 week"; a
// dash where no plan is known, or they cost nothing.
func againstPriced(a views.Against, unit string) []part {
	if !pricedAgainst(a) {
		return []part{figurePart(line{noFigure}, Number{})}
	}
	return []part{
		figurePart(againstSaid(a.Ratio), Number{}),
		wordsPart(line{{" its ", dimInk}}),
		figurePart(line{{a.Cost.GroupedDollars(), dimInk}}, Number{}),
		wordsPart(line{{" " + unit, dimInk}}),
	}
}

// pricedAgainst reports whether the worth against the plans' price has a
// ratio and a price to say.
func pricedAgainst(a views.Against) bool {
	return a.Ratio != nil && a.Cost != nil
}

// knownParts say, after a ratio of every account's, that it's over the plans
// known, where some accounts' plans aren't, as views.PlansKnown says it, in
// text.subtle, in the longest of knownForms' forms the cells given hold: in
// full, " · 2 of 3 plans known", short, " · 2 of 3 plans", or compact, " ·
// 2/3". They're none where none fits, the ratio alone, or every plan is
// known.
func knownParts(k views.PlansKnown, cells int) []part {
	for _, said := range knownForms(k) {
		if ansi.StringWidth(said) <= cells {
			return []part{wordsIn(said, dimInk)}
		}
	}
	return nil
}

// knownForms are the forms a ratio of every account's says, after it, that
// it's over the plans known, as views.PlansKnown says it, longest first: "
// · 2 of 3 plans known", " · 2 of 3 plans", " · 2/3"; none where every plan
// is known.
func knownForms(k views.PlansKnown) []string {
	if k.Said() == "" {
		return nil
	}
	return []string{accountsApart + k.Said(), accountsApart + k.SaidShort(), accountsApart + k.SaidCompact()}
}

// tokensPlaced is how Tokens' periods fit in the rows they have, from row
// first, a blank row between each and a row under the last shown where more
// are below: each period's height, as the frame has them split; and their
// headings, laid out one after another, as periodHeads has them.
type tokensPlaced struct {
	heights     []int
	heads       []headed
	first, rows int
}

// Tokens' parts down its view, in their order: its heads' row, the periods'
// rows and the footnote's; and how many they are. The footnote leaves in
// tokensNoteLeaves, the one turn.
const (
	tokensHeadsRow = iota
	tokensPeriodsRows
	tokensNoteRows
	tokensRowsCount
)

const tokensNoteLeaves = 1

// tokensPlaced is the frame's periods of Tokens, its view from row top,
// placed from two rows under its heads to two over its footnote, and the
// footnote, wrapped to the frame's width, which ends on the view's last row,
// as flex lays them down the view. Where that leaves the periods fewer than
// leastPaged rows, a period's first and the line saying what's below, the
// footnote goes, as it would soon reach the heads, and the periods run to the
// view's last row.
func (f Frame) tokensPlaced(top int) (tokensPlaced, []line) {
	page := f.History.Tokens
	if page == nil {
		return tokensPlaced{}, nil
	}
	note := wrapParts(f.Width, f.tokensFootnote(page.PricesAsOf)...)
	rows := make([]flex.Part, tokensRowsCount)
	rows[tokensHeadsRow] = flex.Part{Least: 1, Natural: 1}
	rows[tokensPeriodsRows] = flex.Part{Least: leastPaged, Natural: leastPaged, Grows: true, Gap: 1}
	rows[tokensNoteRows] = flex.Part{Least: len(note), Natural: len(note), Leaves: tokensNoteLeaves, Gap: min(len(note), 1)}
	spans, _ := flex.Lay(f.viewRows(), rows...)
	if spans[tokensNoteRows].Left {
		note = nil
	}
	periods := spans[tokensPeriodsRows]
	p := tokensPlaced{first: top + periods.At, rows: min(periods.Size, f.viewRows()-periods.At)}
	y := 0
	for _, t := range page.Periods {
		height := tokensHeight(t, f.History.TokensSplit)
		p.heights = append(p.heights, height)
		p.heads = append(p.heads, periodHeads(t, f.History.TokensSplit, y)...)
		y += height + 1
	}
	return p, note
}

// byRows reports whether Tokens' periods show a row at a time: where one of
// them doesn't fit whole in the rows they have, with a row under it where
// more are below.
func (p tokensPlaced) byRows() bool {
	for i, height := range p.heights {
		if i < len(p.heights)-1 {
			height++
		}
		if height > p.rows {
			return true
		}
	}
	return false
}

// paged is how Tokens' periods page a row at a time in the rows they have,
// laid out one after another, a blank row between each, as heads holds their
// headings, saying how many rows are out of view.
func (p tokensPlaced) paged() rowsPaged {
	content := max(len(p.heights)-1, 0)
	for _, height := range p.heights {
		content += height
	}
	return rowsPaged{rows: p.rows, content: content, heads: p.heads, says: true}
}

// periodHeads are the headings of a period, split as given, from row y, as
// tokensRows makes its rows, each over the first row under it: its own row,
// where it's split into any; and by account and model, each account's row,
// where it has versions under it.
func periodHeads(t views.TokenPeriod, split TokensSplit, y int) []headed {
	if tokensHeight(t, split) < 2 {
		return nil
	}
	heads := []headed{{at: y, first: y + 1}}
	if split != TokensByAccountAndModel {
		return heads
	}
	for _, a := range t.ByAccount {
		y++
		if len(a.ByModel) > 0 {
			heads = append(heads, headed{at: y, first: y + 1})
		}
		y += len(a.ByModel)
	}
	return heads
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
// for how many more are below where they don't all show: none where the
// first doesn't fit, as it does wherever byRows says they show a period at
// a time.
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
	return n
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

// tokensScrolling is how far Tokens, from row top, scrolls, scrolled to each
// offset given: a row at a time, where byRows says so, as paged has it; else
// a period at a time, as far as most says, and as many periods at once as
// show from there, paged as thingsPaged has it.
func (f Frame) tokensScrolling(top int) func(offset int) Scrolling {
	placed, _ := f.tokensPlaced(top)
	switch {
	case len(placed.heights) == 0 || placed.rows < 1:
		return func(int) Scrolling { return Scrolling{} }
	case placed.byRows():
		return placed.paged().scrolling
	}
	most := placed.most()
	return func(offset int) Scrolling {
		return thingsPaged(most, offset, placed.shown)
	}
}
