package views_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// namesOfTeam name work configured, labelled Work, and removedTeam.
var namesOfTeam = status.NamesOf(config.Accounts{{ID: "work", Label: "Work"}}, status.RemovedAccounts{removedTeam})

// teamNamed is removedTeam as namesOfTeam name it.
var teamNamed = status.AccountName{Label: "team", Other: true, Removed: removedTeam.Said()}

// A request's line names its account as the names given name it, and the
// line the router answered itself none.
func TestARequestsLineNamesItsAccount(t *testing.T) {
	lines := []ledger.Held{
		{Request: "r1", Kind: ledger.KindMessage, Account: "work", Status: 200},
		{Request: "r2", Kind: ledger.KindMessage, Account: "h7c2", Status: 200},
		{Request: "r3", Kind: ledger.KindMessage, Status: 429},
	}
	var got []status.AccountName
	for r := range views.RequestLines(slices.Values(lines), namesOfTeam) {
		got = append(got, r.AccountName)
	}
	if want := []status.AccountName{{Label: "Work"}, teamNamed, {}}; !slices.Equal(got, want) {
		t.Errorf("the lines' accounts are named %+v, want %+v", got, want)
	}
}

// The windows' readings, named, name each one's account as the names given
// name it, leaving the windows named from unchanged.
func TestTheWindowsNameTheirAccounts(t *testing.T) {
	windows := views.Windows{Windows: []views.WindowReadings{{Account: "work", Window: "5h"}, {Account: "h7c2", Window: "5h"}}}
	named := windows.Named(namesOfTeam)
	if got := []status.AccountName{named.Windows[0].AccountName, named.Windows[1].AccountName}; !slices.Equal(got, []status.AccountName{{Label: "Work"}, teamNamed}) {
		t.Errorf("the windows' accounts are named %+v, want Work and team, removed", got)
	}
	if windows.Windows[1].Label != "" {
		t.Errorf("the windows named from are %+v, want them unchanged", windows.Windows)
	}
}

// The List, named, names each account it gives: each session's, each of its
// models', the account its move left, and those its routing's words name.
func TestTheListNamesEveryAccountItGives(t *testing.T) {
	list := views.SessionList{Sessions: []views.ListedSession{{
		Session: "s1", Account: "h7c2",
		Models:  []views.SessionModel{{Model: "claude-opus-5-5", Account: "h7c2"}, {Model: "claude-haiku-4-5", Account: "work"}},
		Moved:   &views.Move{From: "work"},
		Routing: views.RoutingSaid{From: "work", Words: status.Words{status.Plain("from "), status.Named("work"), status.Plain(", at its limit")}},
	}}}
	s := list.Named(namesOfTeam).Sessions[0]
	if s.AccountName != teamNamed || s.Models[0].AccountName != teamNamed || s.Models[1].AccountName != (status.AccountName{Label: "Work"}) {
		t.Errorf("the session is named %+v, its models %+v, want team, removed, on team and Work", s.AccountName, s.Models)
	}
	if s.Moved.FromLabel != "Work" || s.Routing.Said().String() != "from Work, at its limit" {
		t.Errorf("the session's move is from %q, its routing %q; want from Work, and from Work, at its limit", s.Moved.FromLabel, s.Routing.Said().String())
	}
	if was := list.Sessions[0]; was.Label != "" || was.Models[0].Label != "" || was.Moved.FromLabel != "" || was.Routing.Words[1].Text != "work" {
		t.Errorf("the List named from is %+v, want it unchanged", was)
	}
}

// A session's page, named, names each account it gives: the session's, its
// pin's, its models' and where they go, each it ran on, each move's from and
// to, and each turn's, in order.
func TestASessionsPageNamesEveryAccountItGives(t *testing.T) {
	page := views.SessionPage{
		Session: "s1", Account: "h7c2", Pin: "work",
		Models:   []views.SessionModel{{Model: "claude-opus-5-5", Account: "h7c2"}},
		Goes:     []views.SessionModel{{Model: "claude-opus-5-5", Account: "h7c2"}},
		Accounts: []views.SessionAccount{{Account: "work"}, {Account: "h7c2"}},
		Moves:    []views.SessionMove{{From: "work", To: "h7c2"}},
		Turns:    []views.Turn{{Turn: 1, Accounts: []string{"work", "h7c2"}}},
	}
	p := page.Named(namesOfTeam)
	if p.AccountName != teamNamed || p.PinLabel != "Work" || p.Models[0].AccountName != teamNamed || p.Goes[0].AccountName != teamNamed {
		t.Errorf("the page is named %+v, its pin %q, its models %+v and goes %+v; want team, removed, pinned to Work", p.AccountName, p.PinLabel, p.Models, p.Goes)
	}
	if p.Accounts[0].AccountName != (status.AccountName{Label: "Work"}) || p.Accounts[1].AccountName != teamNamed {
		t.Errorf("the accounts it ran on are named %+v, want Work then team, removed", p.Accounts)
	}
	if m := p.Moves[0]; m.FromLabel != "Work" || m.ToLabel != "team" {
		t.Errorf("its move is labelled from %q to %q, want Work to team", m.FromLabel, m.ToLabel)
	}
	if got := p.Turns[0].Labels; !slices.Equal(got, []string{"Work", "team"}) {
		t.Errorf("its turn's accounts are labelled %q, want Work then team", got)
	}
	if page.Accounts[0].Label != "" || page.Moves[0].FromLabel != "" || page.Turns[0].Labels != nil {
		t.Errorf("the page named from is %+v, want it unchanged", page)
	}
}

// History's blocks label each configured account they give as the config
// does, and every account's none.
func TestHistorysBlocksLabelEachAccountTheyGive(t *testing.T) {
	days := daysFrom("2026-10-05", day("2026-10-05", account("work", sent("claude-opus-5-5", 1, inputUsage))))
	h := views.NewHistory(withRemoved(days, removedTeam))
	labels := map[string]string{}
	for _, a := range h.Totals.Accounts {
		labels["totals "+a.Account] = a.Label
	}
	for _, p := range h.Plans {
		labels["plans "+p.Account] = p.Label
	}
	for _, v := range h.Capacity.Accounts {
		labels["capacity "+v.Account] = v.Label
	}
	for _, a := range h.Weeks[len(h.Weeks)-1].Accounts {
		labels["weeks "+a.Account] = a.Label
	}
	for _, block := range []string{"totals", "plans", "capacity", "weeks"} {
		if got := labels[block+" work"]; got != "team" {
			t.Errorf("%s labels work %q, want team, as configured", block, got)
		}
		if got := labels[block+" "]; got != "" {
			t.Errorf("%s labels every account's %q, want none", block, got)
		}
	}
}
