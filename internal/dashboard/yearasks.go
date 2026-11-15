package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// allUse is the name the preferences and the key line give the Year shown
// for all use, every version and every account.
const allUse = "all"

// yearShownOf names what the Year asked is shown for alone, of the
// configured accounts given: an account, as accountLabel names it, or a
// version, by its name, as Opus 5.5, once the last 12 months are read, as
// read says, as one kept is checked against those they used; "" for all
// use, and for a version till then.
func yearShownOf(ask views.YearAsk, accounts config.Accounts, read bool) string {
	switch {
	case ask.Account != "":
		return accountLabel(accounts, ask.Account)
	case read:
		return versionShown(ask.Version)
	}
	return ""
}

// versionShown names a version as the Year's filter says it, as Opus 5.5.
func versionShown(version string) string {
	return prose.Clean(version)
}

// accountLabel is the account with the given id, of the configured accounts
// given, as the Year names it: by its label as the config gives it, as
// labelled names it.
func accountLabel(accounts config.Accounts, id string) string {
	var label string
	if i := slices.IndexFunc(accounts, func(a config.Account) bool { return a.ID == id }); i >= 0 {
		label = accounts[i].Label
	}
	return labelled(label, id)
}

// cycledAsk is the Year asked shown for the next of what v cycles it
// through, round, as yearChoices has them of its page, reporting false where
// that's what it shows already, as with nothing to show alone.
func cycledAsk(ask views.YearAsk, page views.YearPage) (views.YearAsk, bool) {
	choices := yearChoices(page)
	next := choices[(slices.Index(choices, ask)+1)%len(choices)]
	return next, next != ask
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

// movedAsk is the Year asked, shown for one account, shown for the account
// by places on from it, among the configured accounts given, round,
// reporting false where it's shown for none, or there's no other.
func movedAsk(ask views.YearAsk, accounts config.Accounts, by int) (views.YearAsk, bool) {
	ids := accounts.IDs()
	i := slices.Index(ids, ask.Account)
	if i < 0 || len(ids) < 2 {
		return ask, false
	}
	ask.Account = ids[round(i+by, len(ids))]
	return ask, true
}
