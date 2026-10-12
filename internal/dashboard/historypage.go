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
// picked out, set tight on a phone, as the rule Accounts and History share
// has them; after [ ], what the Year is shown for, where it's shown for one
// version or account; and at its right, the period of the sub-tab shown, as
// Period names it, between < and >; else eight weeks, which Weeks and Days
// keep to.
func (p HistoryPage) heading() heading {
	h := heading{name: strings.ToUpper(History.Title()), subTabs: historyViews, shown: int(p.Shown), tight: true, state: p.state()}
	if h.period = p.Period(); h.period == "" {
		h.right = line{{"eight weeks", mutedInk}}
	}
	return h
}

// state is what the heading says after [ ] that the screen can't otherwise
// show: on the Year shown for one version, or one account, which it shows,
// and that v shows the rest, as "Opus 5.5 only, every account  ·  v for all,
// a model or an account"; and nothing otherwise.
func (p HistoryPage) state() line {
	shown := p.yearShown()
	if p.Shown != HistoryYear || shown == "" {
		return nil
	}
	every := "every account"
	if p.YearAsk.Account != "" {
		every = "every model"
	}
	return line{{shown + " only, " + every + "  ·  ", faintInk}, {"v", keyInk}, {" for all, a model or an account", faintInk}}
}

// yearPeriod is the Year's period as the heading names it: the calendar year
// given, or where it's 0, the last 12 months.
func yearPeriod(year int) string {
	if year == 0 {
		return "last 12 months"
	}
	return strconv.Itoa(year)
}

// history draws History's sub-tab shown from row top, once the year's days
// are read: till then, the view is blank. The Year is drawn whole, then as
// many of its rows as the view has, from as far down as it's scrolled.
func (f Frame) history(c *canvas, top int) {
	if !f.History.Read {
		return
	}
	switch f.History.Shown {
	case HistoryYear:
		year := f.wholeYear()
		scrolled := byRows(year.height(), f.viewRows())
		c.paste(year, scrolled.clamped(f.Scroll), top, scrolled.Page)
	case HistoryWeeks:
		f.historyWeeks(c, top)
	case HistoryDays:
		f.historyDays(c, top)
	case HistoryTokens:
		f.historyTokens(c, top)
	}
}

// wholeYear is the Year drawn whole on a canvas of its own, from its first
// row, as tall as it runs.
func (f Frame) wholeYear() *canvas {
	year := growing(f.Width)
	f.historyYear(year, 0)
	return year
}

// historyScrolling is how far History's sub-tab shown, from row top,
// scrolls, scrolled to each offset given: the Year by rows, as far as it
// runs past the view; Weeks by accounts, and Days, within BY ACCOUNT; and
// Tokens by periods. None scrolls before its page is built.
func (f Frame) historyScrolling(top int) func(offset int) Scrolling {
	var s Scrolling
	switch f.History.Shown {
	case HistoryYear:
		s = byRows(f.wholeYear().height(), f.viewRows())
	case HistoryWeeks:
		s = f.weeksScrolling()
	case HistoryDays:
		s = f.daysScrolling()
	case HistoryTokens:
		return f.tokensScrolling(top)
	}
	return func(int) Scrolling { return s }
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

// moreBelow says how many more of a view's rows, each one of the thing noun
// names, are out of view below, and how to scroll to them, what toScroll says
// following j/k: "▼ 3 more weeks below   ·   j/k to scroll".
func moreBelow(n int, noun, toScroll string) line {
	return line{
		{"▼", keyMarkInk},
		{" " + prose.Counted(n, "more "+noun) + " below", mutedInk},
		{"   ·   ", faintInk},
		{"j/k", keyInk},
		{toScroll, mutedInk},
	}
}

// accountsToScroll is how History says to scroll to its accounts out of
// view, following j/k.
const accountsToScroll = " or the wheel to scroll them"

// accountsOutOfView says how many of a view's accounts are out of view above
// and below, and how to scroll to them: "▼ 4 more accounts below   ·   j/k
// or the wheel to scroll them".
func accountsOutOfView(above, below int) line {
	var l line
	if above > 0 {
		l = line{{"▲", keyMarkInk}, {" " + moreAccounts(above) + " above", mutedInk}, {"   ·   ", faintInk}}
	}
	if below > 0 {
		return append(l, moreBelow(below, "account", accountsToScroll)...)
	}
	return append(l, span{"j/k", keyInk}, span{accountsToScroll, mutedInk})
}

// groupHead is a group's heading, as BY MONTH and BY MODEL, in text.subtle,
// and after it, but on a phone, what it shows, in text.faint.
func groupHead(name, says string, phone bool) line {
	if phone {
		return line{{name, dimInk}}
	}
	return line{{name, dimInk}, {"  " + says, faintInk}}
}
