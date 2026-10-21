package dashboard

import (
	"strconv"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// HistoryView is one of History's views, its sub-tabs: the Year, the
// default, Weeks, Days and Tokens.
type HistoryView int

const (
	// HistoryYear is the year a day at a time, a column a week.
	HistoryYear HistoryView = iota
	// HistoryWeeks is how full each account's week got, and whether there
	// are as many accounts as the use needs.
	HistoryWeeks
	// HistoryDays is eight weeks a day at a time, by model and by account.
	HistoryDays
	// HistoryTokens is the tokens by period, as ccusage reports them, split
	// by account.
	HistoryTokens
)

// historyViews are History's sub-tabs' names, in the heading row's order.
var historyViews = []string{"Year", "Weeks", "Days", "Tokens"}

// WeeksShape is the shape Weeks is drawn in, as v cycles it: a table, the
// default, or columns.
type WeeksShape int

const (
	// WeeksTable is a row an account, each week's peak a percentage.
	WeeksTable WeeksShape = iota
	// WeeksColumns is a block an account, each week's peak a column.
	WeeksColumns
)

// TokensSplit is how Tokens splits each period, as v cycles it: by account,
// the default, by model, by account and model, or not at all.
type TokensSplit int

const (
	// TokensByAccount is a row an account.
	TokensByAccount TokensSplit = iota
	// TokensByModel is a row a model version.
	TokensByModel
	// TokensByAccountAndModel is a row an account, its versions nested
	// under it.
	TokensByAccountAndModel
	// TokensUnsplit is the periods alone.
	TokensUnsplit
)

// HistoryPage is History as the watch gives it: the sub-tab shown, what each
// view is asked to show, and the pages built of the ledger's days.
type HistoryPage struct {
	// Shown is the sub-tab shown.
	Shown HistoryView
	// YearAsk is what the Year shows; WeeksShape how Weeks is drawn; and
	// TokensAsk the period Tokens' rows are, and TokensSplit how each is
	// split.
	YearAsk     views.YearAsk
	WeeksShape  WeeksShape
	TokensAsk   views.TokensAsk
	TokensSplit TokensSplit
	// Accounts are the configured accounts, in the config's order, which ←
	// and → step the Year shown for one through, and whose labels name it.
	Accounts config.Accounts
	// Read is set once the year's days are read: till then, the view is
	// blank.
	Read bool
	// Year, Weeks, Days and Tokens are the views' pages, as built of the
	// days read: nil where one isn't.
	Year   *views.YearPage
	Weeks  *views.WeeksPage
	Days   *views.DaysPage
	Tokens *views.TokensPage
}

// heading is History's heading row: its name and its sub-tabs, the one shown
// picked out; after [ ], what the Year is shown for, where it's shown for one
// version or account, in the forms filtered says it in, its least as
// leastFiltered reckons it; and at its right, the period of the sub-tab
// shown, as Period names it, between < and >; else eight weeks, which Weeks
// and Days keep to.
func (p HistoryPage) heading() heading {
	h := heading{
		name: strings.ToUpper(History.Title()), subTabs: historyViews, shown: int(p.Shown),
		least: p.leastFiltered(), widest: widestOf(p.periods()),
	}
	if forms := p.filtered(); forms != nil {
		h.state, h.shorter = forms[0], forms[1:]
	}
	if h.period = p.Period(); h.period == "" {
		h.right = line{{"eight weeks", mutedInk}}
	}
	return h
}

// filtered is what the heading says after [ ] that the screen can't
// otherwise show: on the Year shown for one version, or one account, which
// it shows, and that v shows the rest, as "Opus 5.5 only, every account  ·
// v for all, a model or an account", then for less room, which it shows
// alone, "Opus 5.5 only"; and nothing otherwise.
func (p HistoryPage) filtered() []line {
	shown := p.yearShown()
	if p.Shown != HistoryYear || shown == "" {
		return nil
	}
	every := "every account"
	if p.YearAsk.Account != "" {
		every = "every model"
	}
	return []line{
		{{shown + " only, " + every + "  ·  ", faintInk}, {"v", keyInk}, {" for all, a model or an account", faintInk}},
		{{onlyForm(shown), faintInk}},
	}
}

// leastFiltered is the widest the Year's filter takes in its short form as
// what can change it without v steps it: shown for an account, the widest of
// every account configured, as ← and → step through them; shown for a
// version, its own, whether the last 12 months are read yet or not; and ""
// unfiltered, or on another sub-tab.
func (p HistoryPage) leastFiltered() string {
	switch {
	case p.Shown != HistoryYear:
		return ""
	case p.YearAsk.Account != "":
		forms := []string{onlyForm(p.yearShown())}
		for _, a := range p.Accounts {
			forms = append(forms, onlyForm(p.label(a.ID)))
		}
		return widestOf(forms)
	case p.YearAsk.Version != "":
		return onlyForm(prose.Clean(p.YearAsk.Version))
	}
	return ""
}

// onlyForm is the Year's filter in its short form, as "Opus 5.5 only".
func onlyForm(shown string) string {
	return shown + " only"
}

// widestYear is a calendar year at its widest, as the heading names it.
const widestYear = 9999

// yearPeriod is the Year's period as the heading names it: the calendar year
// given, or where it's 0, the last 12 months.
func yearPeriod(year int) string {
	if year == 0 {
		return "last 12 months"
	}
	return strconv.Itoa(year)
}

// history draws History's sub-tab shown from row top, in the view's rows
// alone, once the year's days are read: till then, the view is blank. The
// Year is drawn whole, once its page is built, then as many of its rows as
// the view has, from as far down as it's scrolled.
func (f Frame) history(c *canvas, top int) {
	if !f.History.Read {
		return
	}
	c = c.within(bounds{y: top, width: f.Width, rows: f.viewRows()})
	switch f.History.Shown {
	case HistoryYear:
		if y, ok := f.yearDrawn(); ok {
			f.pasteWhole(c, y.whole(), top, y.paged())
		}
	case HistoryWeeks:
		f.historyWeeks(c, top)
	case HistoryDays:
		f.historyDays(c, top)
	case HistoryTokens:
		f.historyTokens(c, top)
	}
}

// historyRows is how tall History's frames are: shorter, Weeks and Days
// scroll whole, as the Year does, keeping their foot rows.
const historyRows = 37

// shortOfHistory reports whether the frame is shorter than History's
// frames, where Weeks and Days scroll whole.
func (f Frame) shortOfHistory() bool {
	return f.Height < historyRows
}

// pasteWhole draws whole, a view of History drawn whole, from row top, as
// paged pages it, scrolled as the frame has it, saying what's out of view, as
// historyRowsOut says it, where it says.
func (f Frame) pasteWhole(c, whole *canvas, top int, paged rowsPaged) {
	paged.paste(c, whole, top, f.Scroll, f.edge(), historyRowsOut...)
}

// wholePaged is how a view of History drawn whole, rows tall, pages in the
// view's rows over its foot rows, foot of them, its heads as given, saying
// how many rows are out of view, as Weeks and Days do where they scroll
// whole.
func (f Frame) wholePaged(rows, foot int, heads []headed) rowsPaged {
	return rowsPaged{rows: f.viewRows(), foot: foot, content: rows, heads: heads, says: true}
}

// paged is how the Year, drawn whole, pages in the view's rows: its
// numbers' heading where they're under the key, and BY MONTH's, with their
// first rows; and where it doesn't fit, its last row saying how many rows
// are out of view, as Weeks and Days say it where they're drawn whole.
func (y yearDrawn) paged() rowsPaged {
	p := y.laid.byMonth
	heads := []headed{{at: p.heading, first: p.first}}
	if !y.laid.beside {
		heads = []headed{{at: yearNumbersRow, first: yearNumbersRow + 1}, heads[0]}
	}
	return rowsPaged{rows: y.viewRows(), content: y.height(), heads: heads, says: true}
}

// historyScrollsRows reports whether History's sub-tab shown scrolls a row at
// a time: the Year; Weeks and Days where they're drawn whole, as weeksWhole
// and layOutDays have it; and Tokens where a period doesn't fit, as byRows
// has it.
func (f Frame) historyScrollsRows() bool {
	switch f.History.Shown {
	case HistoryYear:
		return true
	case HistoryWeeks:
		p := f.History.Weeks
		return p != nil && p.Capacity.Weeks > 0 && f.weeksWhole(f.weeksDrawn(*p))
	case HistoryDays:
		p := f.History.Days
		return p != nil && p.Recorded() && f.layOutDays(*p, f.top()).whole
	case HistoryTokens:
		placed, _ := f.tokensPlaced(f.top())
		return placed.byRows()
	}
	return false
}

// phoneForm reports whether a sub-tab of History draws its phone form: on a
// phone, or where the width its wide form takes, as reach gives it, its
// widest row and any column after it it keeps clear, is wider than the
// frame, as its parts are at fixed columns.
func (f Frame) phoneForm(reach func() int) bool {
	return f.phone() || reach() > f.Width
}

// historyScrolling is how far History's sub-tab shown, from row top,
// scrolls, scrolled to each offset given: the Year by rows, as far as it
// runs past the view; Weeks by accounts, and Days, within BY ACCOUNT, but
// by rows where they're drawn whole; and Tokens by periods, or by rows where
// a period doesn't fit. None scrolls before its page is built.
func (f Frame) historyScrolling(top int) func(offset int) Scrolling {
	switch f.History.Shown {
	case HistoryYear:
		if y, ok := f.yearDrawn(); ok {
			return y.paged().scrolling
		}
	case HistoryWeeks:
		return f.weeksScrolling
	case HistoryDays:
		return f.daysScrolling
	case HistoryTokens:
		return f.tokensScrolling(top)
	}
	return func(int) Scrolling { return Scrolling{} }
}

// historyScrolls is what of History's sub-tab shown scrolls, from row y,
// rows tall, across the frame: the Year's view whole; the accounts Weeks
// shows, and BY ACCOUNT's on Days; and the periods Tokens shows.
func (f Frame) historyScrolls() (y, rows int) {
	switch f.History.Shown {
	case HistoryWeeks:
		return f.weeksScrolls()
	case HistoryDays:
		return f.daysScrolls()
	case HistoryTokens:
		placed, _ := f.tokensPlaced(f.top())
		return placed.first, placed.rows
	}
	return f.top(), f.viewRows()
}

// periodsOut is the form of the line saying how many more of Tokens'
// periods, each one of the thing noun names, are out of view below, and how
// to scroll to them: "▼ 3 more weeks below   ·   j/k to scroll".
func periodsOut(noun string) outForm {
	return outForm{noun: noun, below: " below", gap: "   ·   ", keys: "j/k", does: " to scroll", inks: keyedOut}
}

// historyRowsOut are the forms of the line saying how many of a view's rows
// are out of view, where Weeks and Days are drawn whole and Tokens scrolls a
// row at a time, each shorter than the one before: "▲ 3 more above   ·   ▼ 12
// more below   ·   j/k to scroll"; spaced as Compare's is, "▲ 3 more above ·
// ▼ 12 more below · j/k"; and without how to scroll.
var historyRowsOut = []outForm{
	{above: " above", below: " below", gap: "   ·   ", keys: "j/k", does: " to scroll", inks: keyedOut},
	{above: " above", below: " below", gap: " · ", keys: "j/k", inks: keyedOut},
	{above: " above", below: " below", gap: " · ", inks: keyedOut},
}

// historyAccountsOut are the forms of the line saying how many of Weeks' and Days'
// accounts are out of view, each shorter than the one before: the design's,
// "▼ 4 more accounts below   ·   j/k or the wheel to scroll them"; spaced as
// Sessions' and Runway's scroll line is, "▼ 4 more accounts below · j/k or
// wheel to scroll"; without how to scroll; and as USAGE's rows say it, "▼ 4
// more accounts".
var historyAccountsOut = []outForm{
	{noun: "account", above: " above", below: " below", gap: "   ·   ", keys: "j/k", does: " or the wheel to scroll them", inks: keyedOut},
	{noun: "account", above: " above", below: " below", gap: " · ", keys: "j/k", does: " or wheel to scroll", inks: keyedOut},
	{noun: "account", above: " above", below: " below", gap: " · ", inks: keyedOut},
	{noun: "account", gap: " · ", inks: keyedOut},
}

// groupHead is a group's heading, as BY MONTH and BY MODEL, in text.subtle,
// and after it, but on a phone, what it shows, in text.faint.
func groupHead(name, says string, phone bool) line {
	if phone {
		return line{{name, dimInk}}
	}
	return line{{name, dimInk}, {"  " + says, faintInk}}
}
