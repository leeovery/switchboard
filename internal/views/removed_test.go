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

// elsewhereHome is an account only another Mac configures, labelled home.
var elsewhereHome = status.RemovedAccount{ID: "p4m2", Label: "home"}

// withRemoved is the input of the days given, work configured under the
// label team since, and the removed accounts given.
func withRemoved(days []ledger.Summary, removed ...status.RemovedAccount) views.HistoryInput {
	in := input(days)
	in.Accounts = config.Accounts{{ID: "work", Label: "team"}}
	in.Removed = removed
	return in
}

// Tokens names an account removed by the label it showed last, set apart,
// told apart from the configured account given that label since; one only
// another Mac configures alike, by its label; and one neither configured nor
// in the accounts record by its id.
func TestTokensNamesAnAccountRemovedByItsLastLabel(t *testing.T) {
	days := daysFrom("2026-10-05", day("2026-10-05",
		account("work", sent("claude-opus-5-5", 1, inputUsage)),
		account("h7c2", sent("claude-opus-5-5", 1, inputUsage)),
		account("p4m2", sent("claude-opus-5-5", 1, inputUsage)),
		account("zzzz", sent("claude-opus-5-5", 1, inputUsage))))

	got := views.NewHistory(withRemoved(days, removedTeam, elsewhereHome)).Tokens[0].ByAccount
	want := []views.AccountTokens{{Account: "work", Label: "team"}, {Account: "h7c2", Label: "team", Other: true}, {Account: "p4m2", Label: "home", Other: true}, {Account: "zzzz", Label: "zzzz"}}
	if len(got) != len(want) {
		t.Fatalf("this week's tokens are by %+v, want %+v", got, want)
	}
	for i, w := range want {
		if g := got[i]; g.Account != w.Account || g.Label != w.Label || g.Other != w.Other {
			t.Errorf("this week's account %d is %s, labelled %q, other %t; want %s, labelled %q, other %t", i, g.Account, g.Label, g.Other, w.Account, w.Label, w.Other)
		}
	}
}

// History's JSON gives each account removed that its days name by its id,
// the label it showed last and when it was removed, one only another Mac
// configures with no time removed, and none it doesn't name: an empty list
// where its days name none.
func TestHistorysJSONGivesTheAccountsRemovedItsDaysName(t *testing.T) {
	spare := status.RemovedAccount{ID: "k3f9", Label: "spare", Removed: onAt("2026-08-01", "09:00").UTC()}
	days := daysFrom("2026-10-05", day("2026-10-05",
		account("h7c2", sent("claude-opus-5-5", 1, inputUsage)),
		account("p4m2", sent("claude-opus-5-5", 1, inputUsage))))

	got := jsonOf(t, views.NewHistory(withRemoved(days, spare, removedTeam, elsewhereHome)))
	want := `"removed":[{"id":"h7c2","label":"team","removed":` + jsonOf(t, removedTeam.Removed) + `},{"id":"p4m2","label":"home"}]`
	if !strings.HasSuffix(got, want+"}") {
		t.Errorf("history's JSON ends\n%s\nwant it to end\n%s}", got[max(0, len(got)-200):], want)
	}
	if none := jsonOf(t, views.NewHistory(withRemoved(daysFrom("2026-10-05"), removedTeam))); !strings.HasSuffix(none, `"removed":[]}`) {
		t.Errorf("history's JSON of days naming no account removed ends %s, want an empty removed list", none[max(0, len(none)-200):])
	}
	upper := daysFrom("2026-10-05", day("2026-10-05", account("H7C2", sent("claude-opus-5-5", 1, inputUsage))))
	if got := views.NewHistory(withRemoved(upper, removedTeam)).Removed; len(got) != 1 || got[0].ID != "h7c2" {
		t.Errorf("history of days naming H7C2 gives the accounts removed %+v, want h7c2, its ids compared ignoring case", got)
	}
}
