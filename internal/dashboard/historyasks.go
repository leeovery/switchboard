package dashboard

import (
	"cmp"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// allUse is the name the preferences and the key line give the Year shown
// for all use, every version and every account.
const allUse = "all"

// The names the preferences keep Weeks' shapes and Tokens' splits by, in
// v's order, and the key line's words for Tokens' splits.
var (
	weeksShapeNames  = []string{"table", "columns"}
	tokensSplitNames = []string{"account", "model", "account+model", "none"}
	tokensSplitWords = []string{"by account", "by model", "by account and model", "none"}
)

// tokensPeriods are Tokens' periods, the shortest first, as < and > step
// them.
var tokensPeriods = []views.TokensPeriod{views.TokensByDay, views.TokensByWeek, views.TokensByMonth}

// HistoryKept is what History's views show as the preferences kept them, the
// accounts configured given: the sub-tab, Weeks' shape, and Tokens' period
// and split, each by its name; and what the Year shows, all use, an account
// by its id, where it's one v cycles through, else, but a configured
// account's id, a version by its name, for the watch to check against those
// used once it reads them. What names nothing there is, as an account no
// longer configured, or the one of one alone, shows as History opens.
func HistoryKept(kept theme.HistoryPrefs, accounts config.Accounts) HistoryPage {
	p := HistoryPage{
		Shown:       HistoryView(placeAmong(kept.View, historyNames())),
		WeeksShape:  WeeksShape(placeAmong(kept.Weeks, weeksShapeNames)),
		TokensAsk:   views.TokensAsk{Period: keptPeriod(kept.Tokens.Period)},
		TokensSplit: TokensSplit(placeAmong(kept.Tokens.Split, tokensSplitNames)),
	}
	configured := slices.Contains(accounts.IDs(), kept.Year)
	switch {
	case kept.Year == "" || kept.Year == allUse:
	case configured && len(accounts) > 1:
		p.YearAsk.Account = kept.Year
	case !configured:
		p.YearAsk.Version = kept.Year
	}
	return p
}

// Kept is what the preferences keep of what History's views show, each as
// HistoryKept reads it: never a calendar year stepped back to.
func (p HistoryPage) Kept() theme.HistoryPrefs {
	return theme.HistoryPrefs{
		View:   historyNames()[p.Shown],
		Year:   cmp.Or(p.YearAsk.Version, p.YearAsk.Account, allUse),
		Weeks:  weeksShapeNames[p.WeeksShape],
		Tokens: theme.TokensPrefs{Period: p.TokensAsk.Period.Name(), Split: tokensSplitNames[p.TokensSplit]},
	}
}

// historyNames are the names the preferences keep History's sub-tabs by, in
// the heading row's order: year, weeks, days and tokens.
func historyNames() []string {
	names := make([]string, len(historyViews))
	for i, name := range historyViews {
		names[i] = strings.ToLower(name)
	}
	return names
}

// placeAmong is the place of name among names: the first's, 0, where it's
// none of them.
func placeAmong(name string, names []string) int {
	return max(slices.Index(names, name), 0)
}

// keptPeriod is Tokens' period with the given name, as Name names it: by
// week, as Tokens opens, where it's none of them.
func keptPeriod(name string) views.TokensPeriod {
	if i := slices.IndexFunc(tokensPeriods, func(p views.TokensPeriod) bool { return p.Name() == name }); i >= 0 {
		return tokensPeriods[i]
	}
	return views.TokensByWeek
}

// Period is the period of the sub-tab shown, as the heading and the key line
// name it: the Year's, the last 12 months or a calendar year; Tokens', by
// week, by day or by month; and "" for Weeks and Days, which keep to eight
// weeks.
func (p HistoryPage) Period() string {
	switch p.Shown {
	case HistoryYear:
		return yearPeriod(p.YearAsk.Year)
	case HistoryTokens:
		return "by " + p.TokensAsk.Period.Name()
	}
	return ""
}

// Split is what v has the sub-tab shown show, as the key line names it: the
// Year's use, all, or what it's shown for alone, as yearShown names it;
// Weeks' shape, table or columns; Tokens' split, by account, by model, by
// account and model, or none; and "" for Days, which v doesn't cycle.
func (p HistoryPage) Split() string {
	switch p.Shown {
	case HistoryYear:
		return cmp.Or(p.yearShown(), allUse)
	case HistoryWeeks:
		return weeksShapeNames[p.WeeksShape]
	case HistoryTokens:
		return tokensSplitWords[p.TokensSplit]
	}
	return ""
}

// yearShown names what the Year is shown for alone: an account, as label
// names it, or a version, by its name, as Opus 5.5, once the last 12 months
// are read, as one kept is checked against those they used; "" for all use,
// and for a version till then.
func (p HistoryPage) yearShown() string {
	switch {
	case p.YearAsk.Account != "":
		return p.label(p.YearAsk.Account)
	case p.Read:
		return status.Clean(p.YearAsk.Version)
	}
	return ""
}

// label is the account with the given id as History names it: by its label
// as the config gives it, as labelled names it.
func (p HistoryPage) label(id string) string {
	var label string
	if i := slices.IndexFunc(p.Accounts, func(a config.Account) bool { return a.ID == id }); i >= 0 {
		label = p.Accounts[i].Label
	}
	return labelled(label, id)
}

// Showing is the page with its sub-tab i shown, as a click on it shows it. It
// reports false where that's the one shown, or there's no such sub-tab.
func (p HistoryPage) Showing(i int) (HistoryPage, bool) {
	if i < 0 || i >= len(historyViews) || HistoryView(i) == p.Shown {
		return p, false
	}
	p.Shown = HistoryView(i)
	return p, true
}

// Turned is the page with the sub-tab by places on from the one shown shown,
// round, as ] and [ move, a place each.
func (p HistoryPage) Turned(by int) (HistoryPage, bool) {
	return p.Showing(round(int(p.Shown)+by, len(historyViews)))
}

// Stepped is the page with the period of the sub-tab shown stepped a step,
// back where by is -1, as < steps it, or on where it's 1, as > does: the
// Year's from the last 12 months to the calendar year latest, back a year at
// a time to earliest, and forward again; and Tokens' to a shorter period, a
// day the shortest, or a longer one, a month the longest. It reports false
// where the period stands as it was: at either end, or on a sub-tab without
// one.
func (p HistoryPage) Stepped(by, earliest, latest int) (HistoryPage, bool) {
	switch p.Shown {
	case HistoryYear:
		year, ok := steppedYear(p.YearAsk.Year, by, earliest, latest)
		p.YearAsk.Year = year
		return p, ok
	case HistoryTokens:
		i := slices.Index(tokensPeriods, p.TokensAsk.Period) + by
		if i < 0 || i >= len(tokensPeriods) {
			return p, false
		}
		p.TokensAsk.Period = tokensPeriods[i]
		return p, true
	}
	return p, false
}

// steppedYear is the Year's year, 0 for the last 12 months, stepped as
// Stepped steps it.
func steppedYear(year, by, earliest, latest int) (int, bool) {
	switch {
	case by < 0 && year == 0:
		return latest, true
	case by < 0 && year > earliest:
		return year - 1, true
	case by > 0 && year == latest:
		return 0, true
	case by > 0 && year != 0:
		return year + 1, true
	}
	return year, false
}

// Cycled is the page with what v cycles on the sub-tab shown cycled on,
// round, as v does: what the Year shows, from all use to each version its
// page lists, then each account it lists, and back; Weeks' shape; and
// Tokens' split. It reports false where nothing changes: on Days, which v
// doesn't cycle, and on the Year before its page is built, or with nothing
// to show alone.
func (p HistoryPage) Cycled() (HistoryPage, bool) {
	switch {
	case p.Shown == HistoryYear && p.Year != nil:
		shown := views.YearAsk{Version: p.YearAsk.Version, Account: p.YearAsk.Account}
		choices := yearChoices(*p.Year)
		next := choices[(slices.Index(choices, shown)+1)%len(choices)]
		p.YearAsk.Version, p.YearAsk.Account = next.Version, next.Account
		return p, next != shown
	case p.Shown == HistoryWeeks:
		p.WeeksShape = WeeksShape(round(int(p.WeeksShape)+1, len(weeksShapeNames)))
	case p.Shown == HistoryTokens:
		p.TokensSplit = TokensSplit(round(int(p.TokensSplit)+1, len(tokensSplitNames)))
	default:
		return p, false
	}
	return p, true
}

// yearChoices are what v cycles the Year through, in turn: all use, then
// each version the page lists, then each account.
func yearChoices(page views.YearPage) []views.YearAsk {
	choices := []views.YearAsk{{}}
	for _, v := range page.Versions {
		choices = append(choices, views.YearAsk{Version: v})
	}
	for _, a := range page.Accounts {
		choices = append(choices, views.YearAsk{Account: a})
	}
	return choices
}

// Moved is the page with the Year, shown for one account, shown for the
// account by places on from it, among those configured, round, as ← and →
// step, a place each. It reports false elsewhere.
func (p HistoryPage) Moved(by int) (HistoryPage, bool) {
	ids := p.Accounts.IDs()
	i := slices.Index(ids, p.YearAsk.Account)
	if p.Shown != HistoryYear || i < 0 || len(ids) < 2 {
		return p, false
	}
	p.YearAsk.Account = ids[round(i+by, len(ids))]
	return p, true
}

// round is i among n places, round: as far on from the first as i is past
// the last, or back from the last as it's before the first.
func round(i, n int) int {
	return (i%n + n) % n
}
