package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// SetupAccountsPage is what the Accounts step's page has of its own, beside
// the review and why the step failed, which are the Summary's: whether it's
// a first run, setup having first read no account; the row chosen, an
// account's by its place among them, or past the last, + add an account;
// the card open over it, the card adding an account, the chosen account's
// edit card, the card asking before it's removed, or the token card, where
// one is; and what the step failed at, where it did.
type SetupAccountsPage struct {
	FirstRun bool
	Chosen   int
	Add      *SetupAddCard
	Edit     *SetupEditCard
	Remove   *SetupRemoveCard
	Token    *SetupTokenCard
	Failed   AccountsFailed
}

// AccountsFailed is what the Accounts step failed at.
type AccountsFailed int

const (
	// AccountsNotRead is reading the accounts.
	AccountsNotRead AccountsFailed = iota
	// AccountsNotSaved is saving a change to them.
	AccountsNotSaved
	// AccountsNotDeleted is deleting a removed account's token file, the
	// account taken out of the config.
	AccountsNotDeleted
	// AccountsNotChecked is checking an account to add, as accounts add
	// would, for a reason no field's value gives.
	AccountsNotChecked
)

// accountsFailedLabels are the labels of the failed fact, by what the step
// failed at.
var accountsFailedLabels = map[AccountsFailed]string{
	AccountsNotRead:    "Not read",
	AccountsNotSaved:   "Not saved",
	AccountsNotDeleted: "Not deleted",
	AccountsNotChecked: "Not checked",
}

// What the Accounts page says: what the review is, or on a first run, what
// to add, saying so where there's no account yet; and the row that adds one.
const (
	reviewIntro     = "Each token was checked with the API just now."
	firstRunIntro   = "Add each Claude subscription you want switchboard to route across."
	noAccountsIntro = "No accounts yet. " + firstRunIntro
	addAccount      = "add an account"
	// notSet is what's said of a plan that isn't set.
	notSet = "not set"
)

// The keys of the Accounts page: those of an account chosen, and of + add
// an account.
var (
	selectKey   = Key{Key: "↑↓", Does: "select"}
	accountKeys = []Key{selectKey, {Key: "t", Does: "new token"}, {Key: "e", Does: "edit"}, {Key: "r", Does: "remove"}, {Key: "p", Does: "make primary"}}
	addingKeys  = []Key{{Key: "enter", Does: addAccount}, Quit}
)

// accountsPage is the Accounts step's page, drawn in look: ACCOUNTS, the
// accounts counted at its right; once they're read, what's said over them,
// as accountsIntro has it, muted, and the table of them, the notes under it;
// or, where the step failed, what it failed at, as its page says, and why.
// With an account chosen, its keys, and enter moving on to Priming, but
// where two accounts or more have none the primary, which enter waits for;
// with + add an account chosen, select alone, and enter adding one.
func (s Setup) accountsPage(look Look) setupPage {
	p := setupPage{name: "ACCOUNTS"}
	switch {
	case s.Failure != "":
		var failed AccountsFailed
		if s.Accounts != nil {
			failed = s.Accounts.Failed
		}
		p.facts = []setup.Said{{Mark: setup.Failed, Label: accountsFailedLabels[failed], Detail: s.Failure}}
	case s.Review != nil:
		t := reviewTable{review: *s.Review, chosen: min(max(s.chosen(), 0), len(s.Review.Accounts)), look: look}
		if len(t.review.Accounts) > 0 {
			p.tail = prose.Counted(len(t.review.Accounts), "account")
		}
		p.purpose, p.table = []line{{{s.accountsIntro(len(t.review.Accounts)), mutedInk}}}, &t
		p.keys, p.onward = t.keys()
	}
	return p
}

// accountsIntro is what the Accounts page says over its table of accounts,
// as many as given: with none, that there are none yet, and what to add; on
// a first run, what to add; else what the review is.
func (s Setup) accountsIntro(accounts int) string {
	switch {
	case accounts == 0:
		return noAccountsIntro
	case s.Accounts != nil && s.Accounts.FirstRun:
		return firstRunIntro
	}
	return reviewIntro
}

// chosen is the Accounts page's row chosen: the first, where it has no page.
func (s Setup) chosen() int {
	if s.Accounts == nil {
		return 0
	}
	return s.Accounts.Chosen
}

// reviewTable is the Accounts page's table of the review's accounts, the
// row chosen among them, and the notes under it, drawn in look.
type reviewTable struct {
	review setup.Review
	chosen int
	look   Look
}

// adding reports whether + add an account is the row chosen.
func (t reviewTable) adding() bool {
	return t.chosen == len(t.review.Accounts)
}

// keys are the keys at the key line's left, and its onward keys, at its
// right, as accountsPage has them.
func (t reviewTable) keys() (left, right []Key) {
	switch {
	case t.adding():
		return []Key{selectKey}, addingKeys
	case t.review.Unmarked:
		return accountKeys, []Key{Quit}
	}
	return accountKeys, onward("next: " + SetupPriming.Title())
}

// The rows of the table and its notes, from the heads: the first account's,
// two on, each two rows apart; and the first note's, three rows past + add
// an account.
const (
	reviewRowsApart = 2
	notesUnderAdd   = 3
)

// draw draws the table from row y, width cells wide: its heads, then from
// reviewRowsApart rows on, a row an account, reviewRowsApart apart, then +
// add an account; then the notes, notesUnderAdd rows under it, as notes has
// them. It returns the row after the last it drew on, the heads kept over
// the rows, and the row to show the row chosen: its own, but the page's
// first for the first, so what's over the table shows with it.
func (t reviewTable) draw(c *canvas, y, width int) (next int, kept []keptHeading, chosen int) {
	cols := t.columns(width)
	t.heads(c, y, cols)
	first := y + reviewRowsApart
	add := first + reviewRowsApart*len(t.review.Accounts)
	c.heading(y, first)
	for i, a := range t.review.Accounts {
		t.row(c, first+reviewRowsApart*i, width, cols, i, a)
	}
	t.addRow(c, add, width, cols)
	next = t.notes(c, add+notesUnderAdd, width)
	if t.chosen > 0 {
		chosen = first + reviewRowsApart*t.chosen
	}
	return next, []keptHeading{{at: y, end: add + 1}}, chosen
}

// The table's parts, as columns lays them out: an account's number, its id,
// its label, its plan and its reserve, each but the number padded out to the
// frames' width, as pads have them; then its token, its year's bar and the
// words after it.
const (
	reviewIndent = iota
	reviewNumber
	reviewID
	reviewIDPad
	reviewLabel
	reviewLabelPad
	reviewPlan
	reviewPlanPad
	reviewReserve
	reviewReservePad
	reviewBar
	reviewWords
)

// The table's figures: the frames' widths of its id, label, plan and
// reserve columns, padding them out where they're narrower; the gap between
// its columns; and a year's bar, whole and at its least.
const (
	framedID       = 14
	framedLabel    = 12
	framedPlan     = 9
	framedReserve  = 9
	reviewGap      = 2
	yearBarCells   = 24
	leastYearCells = 12
)

// The turns the table's parts narrow in, as the room runs short: first the
// columns' padding; then a year's bar; then the labels, cut; then the ids,
// cut; and last, the words after a token's bar, to their shortest forms, as
// wordsOf has them, so the days left are the last a row gives up.
const (
	padsNarrow = iota + 1
	barNarrows
	labelsNarrow
	idsNarrow
	wordsNarrow
)

// The heads of the table's columns.
const (
	accountHead = "ACCOUNT"
	labelHead   = "LABEL"
	planHead    = "PLAN"
	reserveHead = "RESERVE"
	tokenHead   = "TOKEN"
)

// columns lays the table's parts out across width cells: from setupIndent,
// the number, as wide as the widest; two cells on, the id, then its label,
// plan and reserve, each a gap apart, as wide as its widest, head and all,
// padded out to the frames' width; then a gap on, a year's bar, and a gap
// on, the words after it, as wide as the widest. The labels, then the ids,
// narrow to their heads, cut, in the turns after the bar's; then the words,
// to the widest of their shortest forms. The bar and the words are as wide,
// whole and at their least, as what's told in place of a bar, as toldForBar
// has it, so an id narrows before that's cut.
func (t reviewTable) columns(width int) []flex.Span {
	number := ansi.StringWidth(strconv.Itoa(len(t.review.Accounts)))
	id, label := ansi.StringWidth(accountHead), ansi.StringWidth(labelHead)
	plan, reserve := ansi.StringWidth(planHead), ansi.StringWidth(reserveHead)
	var words, leastWords int
	for _, a := range t.review.Accounts {
		id, label = max(id, idCells(a)), max(label, ansi.StringWidth(a.Label))
		plan, reserve = max(plan, planShown(a).width()), max(reserve, reserveShown(a).width())
		if forms := t.wordsOf(a); len(forms) > 0 {
			words, leastWords = max(words, forms[0].width()), max(leastWords, forms[len(forms)-1].width())
		}
		leastWords = max(leastWords, toldForBar(a).width()-leastYearCells-reviewGap)
	}
	spans, _ := flex.Lay(width, []flex.Part{
		reviewIndent:     {Least: setupIndent, Natural: setupIndent},
		reviewNumber:     {Least: number, Natural: number},
		reviewID:         {Least: ansi.StringWidth(accountHead), Natural: id, Narrows: idsNarrow, Gap: reviewGap},
		reviewIDPad:      framedPad(framedID, id),
		reviewLabel:      {Least: ansi.StringWidth(labelHead), Natural: label, Narrows: labelsNarrow, Gap: reviewGap},
		reviewLabelPad:   framedPad(framedLabel, label),
		reviewPlan:       reviewColumn(plan),
		reviewPlanPad:    framedPad(framedPlan, plan),
		reviewReserve:    reviewColumn(reserve),
		reviewReservePad: framedPad(framedReserve, reserve),
		reviewBar:        {Least: leastYearCells, Natural: yearBarCells, Narrows: barNarrows, Gap: reviewGap},
		reviewWords:      {Least: leastWords, Natural: words, Narrows: wordsNarrow, Gap: reviewGap},
	}...)
	return spans
}

// reviewColumn is a column of the table cells wide, a gap after the one before.
func reviewColumn(cells int) flex.Part {
	return flex.Part{Least: cells, Natural: cells, Gap: reviewGap}
}

// framedPad is the padding after a column cells wide out to the frames' width
// of it, framed, the first to narrow, as the room runs short.
func framedPad(framed, cells int) flex.Part {
	return flex.Part{Natural: max(framed-cells, 0), Narrows: padsNarrow}
}

// heads draws the columns' heads along row y, in text.subtle: the token's
// over its year's bar.
func (t reviewTable) heads(c *canvas, y int, cols []flex.Span) {
	for _, h := range []struct {
		at   int
		head string
	}{{reviewID, accountHead}, {reviewLabel, labelHead}, {reviewPlan, planHead}, {reviewReserve, reserveHead}, {reviewBar, tokenHead}} {
		c.line(cols[h.at].At, y, line{{h.head, dimInk}}.fit(cols[h.at].Size))
	}
}

// reviewChosenOn is the surface the row chosen is drawn on.
var reviewChosenOn = hue{token: theme.BgSelection}

// row draws the account a, i-th of the table's, along row y, width cells
// wide, at the columns cols: its number, subtle; its id, bold text.primary,
// and ◆ after the primary's; its label, muted, cut to its column; its plan
// and its reserve; and its token, as token has it. The row chosen is on
// bg.selection across the width, ▌ in accent.key at its first cell.
func (t reviewTable) row(c *canvas, y, width int, cols []flex.Span, i int, a setup.Reviewed) {
	c.line(cols[reviewNumber].At, y, line{{strconv.Itoa(i + 1), dimInk}})
	c.line(cols[reviewID].At, y, idShown(a, cols[reviewID].Size))
	label := cols[reviewLabel]
	c.line(label.At, y, line{{a.Label, mutedInk}}.fit(label.Size))
	c.line(cols[reviewPlan].At, y, planShown(a))
	c.line(cols[reviewReserve].At, y, reserveShown(a))
	t.token(c, y, width, cols, a)
	if i == t.chosen {
		reviewChosen(c, y, width)
	}
}

// primaryMark is what follows the primary's id.
var primaryMark = line{{" ◆", mutedInk}}

// idShown is the account's id as its row has it, cells wide at most: bold
// text.primary, cut where it's wider, and primaryMark after the primary's,
// kept whole.
func idShown(a setup.Reviewed, cells int) line {
	id := line{{a.ID, titleInk}}
	if !a.Primary {
		return id.fit(cells)
	}
	return append(id.fit(cells-primaryMark.width()), primaryMark...)
}

// idCells is how wide the account's id is, as idShown has it whole.
func idCells(a setup.Reviewed) int {
	cells := ansi.StringWidth(a.ID)
	if a.Primary {
		cells += primaryMark.width()
	}
	return cells
}

// planShown is the account's plan, as setup names it, in text.secondary; or
// not set, faint.
func planShown(a setup.Reviewed) line {
	if name := a.PlanName(); name != "" {
		return line{{name, secondaryInk}}
	}
	return line{{notSet, faintInk}}
}

// reserveShown is the account's reserve, as the percentage it was set, in
// text.secondary; or none, faint.
func reserveShown(a setup.Reviewed) line {
	if a.Reserve == 0 {
		return line{{"none", faintInk}}
	}
	return line{{prose.PercentSet(a.Reserve), secondaryInk}}
}

// token draws the account's token along row y, from its year's bar's
// column, as setup tells it: a token told by its year, as yearShown draws
// it; one working, its year unknown, a bar of ╌, faint; each with its words
// after it in the words' column, as wordsOf has them, the first form that
// fits to the row's end; and any other, what toldForBar says in place of the
// bar, run on to the row's end, cut there.
func (t reviewTable) token(c *canvas, y, width int, cols []flex.Span, a setup.Reviewed) {
	bar, words := cols[reviewBar], cols[reviewWords]
	if told := toldForBar(a); told != nil {
		c.line(bar.At, y, told.fit(width-bar.At))
		return
	}
	if told, _ := a.Token.Told(); told == setup.ToldUndated {
		c.line(bar.At, y, line{{strings.Repeat("╌", bar.Size), faintInk}})
	} else {
		t.yearOf(a).draw(c, t.look, bar.At, y, bar.Size)
	}
	c.line(words.At, y, firstFit(width-words.At, t.wordsOf(a)...))
}

// toldForBar is what's said of the account's token in place of a bar: one
// refused, ✗ in bold state.destructive and its words in text.secondary; one
// told by its words alone, those, muted; nil for one with a bar.
func toldForBar(a setup.Reviewed) line {
	switch told, said := a.Token.Told(); told {
	case setup.ToldRefused:
		return line{{"✗ ", exhaustedInk}, {said, secondaryInk}}
	case setup.ToldInWords:
		return line{{said, mutedInk}}
	}
	return nil
}

// yearOf is the account's token's year as its row shows it, at the review's
// time.
func (t reviewTable) yearOf(a setup.Reviewed) yearShown {
	return yearShown{year: a.Token.Year, at: t.review.At}
}

// wordsOf are the forms of the words after the account's token's bar, each
// narrower than the one before: a year's, as yearShown.forms has them; one
// working, its year unknown, its words, muted; and none of any other token.
func (t reviewTable) wordsOf(a setup.Reviewed) []line {
	switch told, said := a.Token.Told(); told {
	case setup.ToldByYear:
		return t.yearOf(a).forms()
	case setup.ToldUndated:
		return []line{{{said, mutedInk}}}
	}
	return nil
}

// yearShown is a token's year as the table shows it, at the time at.
type yearShown struct {
	year status.TokenYear
	at   time.Time
}

// draw draws the year as a bar cells long from x along row y, in look: the
// share of it gone filled, a whole cell at a time, each cell in the ramp's
// colour at its place along the bar, as barCell draws it; the rest the
// track.
func (s yearShown) draw(c *canvas, look Look, x, y, cells int) {
	filled := halfEven(s.year.Gone(s.at) * float64(cells))
	for i := range cells {
		eighths := 0
		if i < filled {
			eighths = len(blocks) - 1
		}
		glyph, k := barCell(look, eighths, along(i, cells))
		c.text(x+i, y, glyph, k)
	}
}

// forms are the forms of what's said after the year's bar, the first whole,
// each narrower than the one before: expires, muted, and the date, in
// text.secondary, as prose.Expires says them; then, where the year isn't up,
// · and the days left, as prose.DaysLeft says them, in daysLeftInk; and
// where they don't fit, the days left alone, which a row gives up last.
func (s yearShown) forms() []line {
	verb, date, _ := strings.Cut(prose.Expires(s.at, s.year.GoodUntil, s.year.Up(s.at)), " ")
	said := line{{verb + " ", mutedInk}, {date, secondaryInk}}
	left, counted := s.year.DaysLeft(s.at)
	if !counted {
		return []line{said}
	}
	days := line{{prose.DaysLeft(left), s.daysLeftInk()}}
	return []line{slices.Concat(said, line{{"  ·  ", faintInk}}, days), days}
}

// daysLeftInk is the ink of the days left of the year: bold
// state.destructive within its last warning's days, bold accent.attention
// within its first's, as status.TokenYear.Warned has them, else muted.
func (s yearShown) daysLeftInk() ink {
	switch days, warned := s.year.Warned(s.at); {
	case !warned:
		return mutedInk
	case days == status.TokenLastWarning:
		return exhaustedInk
	}
	return alertInk
}

// reviewChosen lays bg.selection under row y, width cells wide, but under the
// cells on a surface of their own, as a year's bar's, and draws ▌ in
// accent.key at its first cell.
func reviewChosen(c *canvas, y, width int) {
	for x := range width {
		if here := c.at(x, y); here != nil && here.ink.on == (hue{}) {
			here.ink.on = reviewChosenOn
		}
	}
	c.line(0, y, line{{"▌", ink{token: theme.AccentKey, on: reviewChosenOn}}})
}

// addRow draws + add an account along row y, width cells wide: + in bold
// accent.key at the number's column, and its words at the id's, muted, or
// text.primary where it's the row chosen.
func (t reviewTable) addRow(c *canvas, y, width int, cols []flex.Span) {
	words := mutedInk
	if t.adding() {
		words = ink{token: theme.TextPrimary}
	}
	c.line(cols[reviewNumber].At, y, line{{"+", keyInk}})
	c.line(cols[reviewID].At, y, line{{addAccount, words}}.fit(width-cols[reviewID].At))
	if t.adding() {
		reviewChosen(c, y, width)
	}
}

// notes draws the review's notes from row y, width cells wide, as
// setup.Review.Notes has them: each as noteLines has it, wrapped from
// setupIndent, a blank row before each but the first, and those after
// another of the same account's. It returns the row after the last.
func (t reviewTable) notes(c *canvas, y, width int) int {
	notes := t.review.Notes()
	for i, n := range notes {
		if i > 0 && (n.Account == "" || n.Account != notes[i-1].Account) {
			y++
		}
		for _, said := range noteLines(n) {
			for _, l := range said.wrap(width - setupIndent) {
				c.line(setupIndent, y, l)
				y++
			}
		}
	}
	return y
}

// noteLines are the note's lines, the id of its account leading it, bold
// text.primary, where it's of one: a note of what stops an account's
// requests, or enter, its words in text.secondary, and how to see to it on
// a line of its own, muted; any other, muted, how to see to it after its
// words.
func noteLines(n setup.Note) []line {
	var lead line
	if n.Account != "" {
		lead = line{{n.Account, titleInk}, {" ", mutedInk}}
	}
	if n.Need == setup.NeedToken || n.Need == setup.NeedPrimary {
		return []line{append(lead, span{n.Says, secondaryInk}), {{n.Press, mutedInk}}}
	}
	return []line{append(lead, span{n.Says + " " + n.Press, mutedInk})}
}
