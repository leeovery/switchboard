package views_test

import (
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// removedTeam is an account removed on 4 September, labelled team.
var removedTeam = status.RemovedAccount{ID: "h7c2", Label: "team", Removed: onAt("2026-09-04", "16:00").UTC()}

// withRemoved is the input of the days given, work configured under the
// label team since, and the removed accounts given.
func withRemoved(days []ledger.Summary, removed ...status.RemovedAccount) views.HistoryInput {
	in := input(days)
	in.Accounts = config.Accounts{{ID: "work", Label: "team"}}
	in.Removed = removed
	return in
}

// Tokens names an account removed by the label it showed last, said removed,
// told apart from the configured account given that label since; and one
// neither configured nor removed by none.
func TestTokensNamesAnAccountRemovedByItsLastLabel(t *testing.T) {
	days := daysFrom("2026-10-05", day("2026-10-05",
		account("work", sent("claude-opus-5-5", 1, inputUsage)),
		account("h7c2", sent("claude-opus-5-5", 1, inputUsage)),
		account("zzzz", sent("claude-opus-5-5", 1, inputUsage))))

	got := views.NewHistory(withRemoved(days, removedTeam)).Tokens[0].ByAccount
	want := []views.AccountTokens{{Account: "work", Label: "team"}, {Account: "h7c2", Label: "team", Removed: true}, {Account: "zzzz"}}
	if len(got) != len(want) {
		t.Fatalf("this week's tokens are by %+v, want %+v", got, want)
	}
	for i, w := range want {
		if g := got[i]; g.Account != w.Account || g.Label != w.Label || g.Removed != w.Removed {
			t.Errorf("this week's account %d is %s, labelled %q, removed %t; want %s, labelled %q, removed %t", i, g.Account, g.Label, g.Removed, w.Account, w.Label, w.Removed)
		}
	}
}

// History's JSON gives each account removed that its days name by its id,
// the label it showed last and when it was removed, and none it doesn't name.
func TestHistorysJSONGivesTheAccountsRemovedItsDaysName(t *testing.T) {
	spare := status.RemovedAccount{ID: "k3f9", Label: "spare", Removed: onAt("2026-08-01", "09:00").UTC()}
	days := daysFrom("2026-10-05", day("2026-10-05", account("h7c2", sent("claude-opus-5-5", 1, inputUsage))))

	got := jsonOf(t, views.NewHistory(withRemoved(days, spare, removedTeam)))
	want := `"removed":[{"id":"h7c2","label":"team","removed":` + jsonOf(t, removedTeam.Removed) + `}]`
	if !strings.HasSuffix(got, want+"}") {
		t.Errorf("history's JSON ends\n%s\nwant it to end\n%s}", got[max(0, len(got)-200):], want)
	}
	if none := jsonOf(t, views.NewHistory(withRemoved(daysFrom("2026-10-05"), removedTeam))); strings.Contains(none, `"removed"`) {
		t.Errorf("history's JSON of days naming no account removed gives %s, want no removed", none[max(0, len(none)-200):])
	}
}
