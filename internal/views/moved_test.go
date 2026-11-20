package views_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The moved accounts' pools: Default, which work left for Acme, keeping
// personal; and Acme, where acme-1 comes before work in the config.
var (
	movedPools   = config.Pools{{ID: "b2n8", Label: "Default"}, {ID: "q7mt", Label: "Acme"}}
	poolAccounts = config.Accounts{
		{ID: "acme1", Label: "acme-1", Pool: "q7mt"},
		{ID: "work", Label: "work", Pool: "q7mt"},
		{ID: "personal", Label: "personal", Pool: "b2n8"},
	}
)

// seenFrom is the input of the days given read in the pool with the given
// id: its accounts those configured in it, and those moved to another, as
// status.MovedOf gives them.
func seenFrom(pool string, days []ledger.Summary) views.HistoryInput {
	in := input(days)
	in.Accounts = poolAccounts.InPool(pool)
	in.Moved = status.MovedOf(movedPools, poolAccounts, pool)
	return in
}

// movedDays are Default's days: work's 9 requests before it moved to Acme,
// and personal's 6.
func movedDays() []ledger.Summary {
	return daysFrom("2026-10-05", day("2026-10-05",
		account("personal", sent("claude-opus-5-5", 6, inputUsage)),
		account("work", sent("claude-opus-5-5", 9, inputUsage))))
}

// totalsBy are the totals' accounts, each by its id, "" every account's.
func totalsBy(h views.History) map[string]views.AccountTotals {
	by := make(map[string]views.AccountTotals, len(h.Totals.Accounts))
	for _, a := range h.Totals.Accounts {
		by[a.Account] = a
	}
	return by
}

// Seen from the pool it left, an account moved keeps its work there: the
// totals give it a column of its own, after the pool's accounts and before
// every account's, named by its label and where it is now, so each of
// these days' 15 requests is in a column; acme-1, which did no work there,
// has none; and its tokens name it so.
func TestTheTotalsOfThePoolAnAccountLeftGiveItAColumn(t *testing.T) {
	h := views.NewHistory(seenFrom("b2n8", movedDays()))
	var ids []string
	for _, a := range h.Totals.Accounts {
		ids = append(ids, a.Account)
	}
	if !slices.Equal(ids, []string{"personal", "work", ""}) {
		t.Fatalf("Default's totals are of %q, want personal, then work, then every account's", ids)
	}
	moved := status.AccountName{Label: "work", Other: true, Moved: "now in Acme"}
	totals := totalsBy(h)
	if got := totals["work"]; got.AccountName != moved || got.Requests != 9 {
		t.Errorf("work's totals in Default are named %+v, of %d requests; want %+v, of 9", got.AccountName, got.Requests, moved)
	}
	if all, columns := totals[""].Requests, totals["work"].Requests+totals["personal"].Requests; all != 15 || columns != all {
		t.Errorf("every account's requests are %d, its columns' %d; want 15 of each", all, columns)
	}
	tokens := h.Tokens[len(h.Tokens)-1].ByAccount
	i := slices.IndexFunc(tokens, func(a views.AccountTokens) bool { return a.Account == "work" })
	if i < 0 || tokens[i].AccountName != moved {
		t.Errorf("this week's tokens are by %+v, want work among them, named %+v", tokens, moved)
	}
}

// Seen from the pool it's in now, an account moved is one of the pool's: its
// column comes in the config's order, after acme-1 though the days name it
// first, named by its label alone, with the work it did there; and the
// History lists no account moved.
func TestTheTotalsOfThePoolAnAccountJoinedGiveItAColumnAsItsOwn(t *testing.T) {
	days := daysFrom("2026-10-05", day("2026-10-05",
		account("work", sent("claude-opus-5-5", 5, inputUsage)),
		account("acme1", sent("claude-opus-5-5", 3, inputUsage))))
	h := views.NewHistory(seenFrom("q7mt", days))
	var ids []string
	for _, a := range h.Totals.Accounts {
		ids = append(ids, a.Account)
	}
	if !slices.Equal(ids, []string{"acme1", "work", ""}) {
		t.Errorf("Acme's totals are of %q, want acme1, then work, in the config's order, then every account's", ids)
	}
	if got := totalsBy(h)["work"]; got.AccountName != (status.AccountName{Label: "work"}) || got.Requests != 5 {
		t.Errorf("work's totals in Acme are named %+v, of %d requests; want work, configured, of 5", got.AccountName, got.Requests)
	}
	if len(h.Moved) != 0 {
		t.Errorf("Acme's history lists %+v moved, want none", h.Moved)
	}
}

// An account moved that the days don't name has no column, as it did no
// work in the pool read.
func TestAnAccountMovedWithNoWorkInThePoolHasNoColumn(t *testing.T) {
	days := daysFrom("2026-10-05", day("2026-10-05", account("personal", sent("claude-opus-5-5", 6, inputUsage))))
	if _, ok := totalsBy(views.NewHistory(seenFrom("b2n8", days)))["work"]; ok {
		t.Error("Default's totals give work a column, want none, as it did no work there")
	}
}

// History's JSON gives each account moved that its days name, by its id,
// its label, and the pool it's in now, by its id and label: an empty list
// where its days name none.
func TestHistorysJSONGivesTheAccountsMovedItsDaysName(t *testing.T) {
	got := jsonOf(t, views.NewHistory(seenFrom("b2n8", movedDays())))
	want := `"moved":[{"id":"work","label":"work","pool":"q7mt","pool_label":"Acme"}]}`
	if !strings.HasSuffix(got, want) {
		t.Errorf("Default's history's JSON ends\n%s\nwant it to end\n%s", got[max(0, len(got)-200):], want)
	}
	acme := daysFrom("2026-10-05", day("2026-10-05", account("work", sent("claude-opus-5-5", 5, inputUsage))))
	if none := jsonOf(t, views.NewHistory(seenFrom("q7mt", acme))); !strings.HasSuffix(none, `"moved":[]}`) {
		t.Errorf("Acme's history's JSON ends %s, want an empty moved list", none[max(0, len(none)-200):])
	}
}

// Accounts' Tokens names an account moved by its label and where it is now,
// set apart, over any period.
func TestTokensNamesAnAccountMovedByItsLabelAndWhereItIsNow(t *testing.T) {
	page := views.NewPast(seenFrom("b2n8", nil), movedDays()).TokensOver(views.LengthTwelveWeeks.Now())
	if page.Total == nil {
		t.Fatal("Tokens over 12 weeks has no total")
	}
	moved := status.AccountName{Label: "work", Other: true, Moved: "now in Acme"}
	i := slices.IndexFunc(page.Total.ByAccount, func(a views.AccountTokens) bool { return a.Account == "work" })
	if i < 0 || page.Total.ByAccount[i].AccountName != moved {
		t.Errorf("Tokens' total is by %+v, want work among them, named %+v", page.Total.ByAccount, moved)
	}
}
