package watch

import (
	"slices"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// PageKeys are the keys a page takes, as its help lists them: the page, and
// its keys, each worded as the key line words it.
type PageKeys struct {
	Page string
	Keys []dashboard.Key
}

// EveryPagesKeys are the dashboard's keys as its help lists them, for what
// tells of them outside it, as the dashboard command's help, to read here,
// so the two never drift: those every page takes first; then each tab's own,
// in the title row's order, then a session's page's. A page lists every key
// it takes in any state, whatever is chosen, shown or set, each by its key
// and what it does alone, once.
func EveryPagesKeys() []PageKeys {
	pages := []PageKeys{{Page: "every page", Keys: slices.Concat([]dashboard.Key{tabViews, pinWord, themesKey, refreshKey, helpKey, dashboard.Quit}, scrollKeys)}}
	for _, v := range dashboard.Views() {
		pages = append(pages, PageKeys{Page: v.Title(), Keys: toldOnce(tabListings(v)...)})
	}
	return append(pages, PageKeys{Page: "a session's page", Keys: toldOnce(sessionListings(false), sessionListings(true))})
}

// tabListings are the tab's own keys, as the model lists them showing it, in
// each state that words one of them otherwise: the Log's ↑↓ on Requests and
// on Events, and Runway's ←→ in the 5-hour view and the week's.
func tabListings(v dashboard.View) [][]keyListing {
	m := Model{view: v}
	switch v {
	case dashboard.Overview:
		return [][]keyListing{m.overviewListings()}
	case dashboard.Accounts:
		return [][]keyListing{m.accountsListings(nil)}
	case dashboard.History:
		return [][]keyListing{m.historyListings()}
	case dashboard.Sessions:
		return [][]keyListing{m.sessionsListings()}
	case dashboard.Runway:
		week := m
		week.runwayTab.shown.Window = views.RunwayWeek
		return [][]keyListing{m.runwayListings(), week.runwayListings()}
	case dashboard.Log:
		events := m
		events.log.page.Shown = dashboard.LogEvents
		return [][]keyListing{m.logListings(), events.logListings()}
	}
	return nil
}

// toldOnce are the keys a page lists in each of its states, each by its key
// and what it does alone, once, in their order: each state's in turn at each
// place, so a key a state words otherwise follows the word it takes in the
// others, as ↑↓ request, ↑↓ event.
func toldOnce(states ...[]keyListing) []dashboard.Key {
	longest := 0
	for _, listed := range states {
		longest = max(longest, len(listed))
	}
	var keys []dashboard.Key
	for i := range longest {
		for _, listed := range states {
			if i >= len(listed) {
				continue
			}
			if told := (dashboard.Key{Key: listed[i].Key.Key, Does: listed[i].Does}); !slices.Contains(keys, told) {
				keys = append(keys, told)
			}
		}
	}
	return keys
}
