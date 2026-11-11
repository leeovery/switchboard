package status_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/status"
)

// The accounts removed are those the accounts record keeps that the config
// doesn't configure, by the label each showed last: those removed, but for
// one the config configures again, as by an edit by hand, and those only
// another Mac configures, never removed, in the record's order; and each is
// found by its id, and none else is.
func TestTheAccountsRemovedAreTheRecordsTheConfigDoesntConfigure(t *testing.T) {
	added := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	gone := time.Date(2026, 9, 4, 16, 30, 0, 0, time.UTC)
	record := identity.Record{Accounts: []identity.Entry{
		{ID: "b6s7", Added: added},
		{ID: "h7c2", Label: "team", Added: added, Removed: gone},
		{ID: "m2x7", Label: "side", Added: added, Removed: gone},
		{ID: "p4m2", Label: "home", Added: added},
		{ID: "k3f9", Label: "spare", Added: added, Removed: gone},
	}}
	configured := config.Accounts{{ID: "b6s7", Label: "work"}, {ID: "M2X7", Label: "side"}, {ID: "qgr2", Label: "team"}}

	removed := status.RemovedOf(record, configured)
	want := status.RemovedAccounts{
		{ID: "h7c2", Label: "team", Removed: gone},
		{ID: "p4m2", Label: "home"},
		{ID: "k3f9", Label: "spare", Removed: gone},
	}
	if !slices.Equal(removed, want) {
		t.Errorf("RemovedOf() = %+v, want %+v", removed, want)
	}
	if got, ok := removed.Of("h7c2"); !ok || got != want[0] {
		t.Errorf("Of(h7c2) = %+v, %v; want %+v", got, ok, want[0])
	}
	for _, id := range []string{"qgr2", "m2x7", ""} {
		if got, ok := removed.Of(id); ok {
			t.Errorf("Of(%q) = %+v, want none", id, got)
		}
	}
}

// inZone has the test's local time zone the one given until it ends. A test
// that calls it mustn't run in parallel.
func inZone(t *testing.T, zone *time.Location) {
	was := time.Local
	time.Local = zone
	t.Cleanup(func() { time.Local = was })
}

// westOfUTC is a zone whose local day, late in the evening, is the day
// before UTC's.
var westOfUTC = time.FixedZone("UTC-10", -10*60*60)

// A removed account says when it was removed, by its local day, however the
// record keeps the time: one removed late on the 4th where UTC is already on
// the 5th says the 4th; and one never removed says nothing.
func TestARemovedAccountSaysWhenItWasRemoved(t *testing.T) {
	inZone(t, westOfUTC)
	a := status.RemovedAccount{ID: "h7c2", Label: "team", Removed: time.Date(2026, 9, 4, 23, 30, 0, 0, westOfUTC).UTC()}
	if got := a.Said(); got != "removed 4 Sep" {
		t.Errorf("Said() = %q, want removed 4 Sep", got)
	}
	if got := (status.RemovedAccount{ID: "p4m2", Label: "home"}).Said(); got != "" {
		t.Errorf("Said() of an account never removed = %q, want none", got)
	}
}

// An account is named by its label as configured, where it's configured;
// else, set apart as other, by the label the accounts record keeps, saying
// when it was removed where it was; else by its id: alike of the configured
// accounts given and of a moment's.
func TestAnAccountIsNamedByTheLabelItsConfiguredOrRecordedBy(t *testing.T) {
	gone := time.Date(2026, 9, 4, 16, 30, 0, 0, time.UTC)
	removed := status.RemovedAccounts{
		{ID: "h7c2", Label: "team", Removed: gone},
		{ID: "p4m2", Label: "home"},
		{ID: "z9k4"},
	}
	said := removed[0].Said()
	tests := []struct {
		id   string
		want status.AccountName
	}{
		{id: "work", want: status.AccountName{Label: "Work"}},
		{id: "side", want: status.AccountName{Label: "side"}},
		{id: "h7c2", want: status.AccountName{Label: "team", Other: true, Removed: said}},
		{id: "p4m2", want: status.AccountName{Label: "home", Other: true}},
		{id: "z9k4", want: status.AccountName{Label: "z9k4", Other: true}},
		{id: "qgr2", want: status.AccountName{Label: "qgr2"}},
	}
	configured := config.Accounts{{ID: "work", Label: "Work"}, {ID: "side"}}
	m := routerDocOf(status.Account{ID: "work", Label: "Work", TokenSet: true, FetchedAt: built}, status.Account{ID: "side", TokenSet: true, FetchedAt: built}).
		WithRemoved(removed).At(built, policy)
	for _, tt := range tests {
		if got := removed.Named(configured, tt.id); got != tt.want {
			t.Errorf("Named(%s) of the configured accounts = %+v, want %+v", tt.id, got, tt.want)
		}
		if got := m.Named(tt.id); got != tt.want {
			t.Errorf("Named(%s) of the moment = %+v, want %+v", tt.id, got, tt.want)
		}
	}
	if got := m.At(built.Add(time.Minute)).Named("h7c2"); got != tests[2].want {
		t.Errorf("Named(h7c2) of the moment a minute on = %+v, want %+v", got, tests[2].want)
	}
}

// Ids are compared ignoring case, as a config edited by hand may spell one.
func TestAnAccountIsNamedWhateverTheCaseOfItsID(t *testing.T) {
	removed := status.RemovedAccounts{{ID: "h7c2", Label: "team"}}
	if got := removed.Named(config.Accounts{{ID: "B6S7", Label: "Work"}}, "b6s7"); got != (status.AccountName{Label: "Work"}) {
		t.Errorf("Named(b6s7) of B6S7 configured = %+v, want it configured, labelled Work", got)
	}
	if got := removed.Named(nil, "H7C2"); got != (status.AccountName{Label: "team", Other: true}) {
		t.Errorf("Named(H7C2) = %+v, want the account kept as h7c2, labelled team", got)
	}
	m := routerDocOf(status.Account{ID: "B6S7", Label: "Work", TokenSet: true, FetchedAt: built}).WithRemoved(removed).At(built, policy)
	if got := m.Named("b6s7"); got != (status.AccountName{Label: "Work"}) {
		t.Errorf("the moment's Named(b6s7) of B6S7 = %+v, want it configured, labelled Work", got)
	}
}

// Names find an account given by its id, ignoring case, as the config or the
// accounts record spells it, and nothing else, a label among them.
func TestNamesFindAnAccountByItsIDWhateverItsCase(t *testing.T) {
	names := status.NamesOf(config.Accounts{{ID: "B6S7", Label: "work"}}, status.RemovedAccounts{{ID: "h7c2", Label: "team"}})
	for given, want := range map[string]string{"b6s7": "B6S7", "B6S7": "B6S7", "H7C2": "h7c2"} {
		if got, ok := names.ID(given); !ok || got != want {
			t.Errorf("ID(%s) = %q, %t; want %s", given, got, ok, want)
		}
	}
	for _, given := range []string{"work", "team", "zzzz"} {
		if got, ok := names.ID(given); ok {
			t.Errorf("ID(%s) = %q, want none", given, got)
		}
	}
}

// The verbs' text names an account by its label, cleaned, and one removed
// says when, as Days does: "team (removed 4 Sep)"; one only another Mac
// configures, never removed, by its label alone.
func TestAnAccountsNameIsSaidByItsLabelAndWhenItWasRemoved(t *testing.T) {
	tests := []struct {
		name status.AccountName
		want string
	}{
		{name: status.AccountName{Label: "work"}, want: "work"},
		{name: status.AccountName{Label: "team", Other: true, Removed: "removed 4 Sep"}, want: "team (removed 4 Sep)"},
		{name: status.AccountName{Label: "home", Other: true}, want: "home"},
		{name: status.AccountName{Label: "side\nproject"}, want: "side project"},
	}
	for _, tt := range tests {
		if got := tt.name.Said(); got != tt.want {
			t.Errorf("%+v.Said() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// Names name accounts as RemovedAccounts' Named does, and their zero value
// names each by its id; a configured account unlabelled is named by its id.
func TestNamesNameAccountsAsTheConfigAndTheRecordDo(t *testing.T) {
	removed := status.RemovedAccounts{{ID: "h7c2", Label: "team", Removed: time.Date(2026, 9, 4, 16, 30, 0, 0, time.UTC)}}
	configured := config.Accounts{{ID: "work", Label: "Work"}, {ID: "side"}}
	names := status.NamesOf(configured, removed)
	for _, id := range []string{"work", "side", "h7c2", "qgr2"} {
		if got, want := names.Named(id), removed.Named(configured, id); got != want {
			t.Errorf("Named(%s) = %+v, want %+v", id, got, want)
		}
		if got := (status.Names{}).Named(id); got != (status.AccountName{Label: id}) {
			t.Errorf("Named(%s) of no names = %+v, want it named by its id", id, got)
		}
	}
	if got := status.ConfiguredName(configured[1]); got != (status.AccountName{Label: "side"}) {
		t.Errorf("ConfiguredName(side, unlabelled) = %+v, want it named by its id", got)
	}
}

// Words name each account they name by its id as the names given say it,
// leaving the rest, and the words given, as they are.
func TestWordsNameTheirAccountsAsTheNamesGivenSayThem(t *testing.T) {
	removed := status.RemovedAccounts{{ID: "h7c2", Label: "team", Removed: time.Date(2026, 9, 4, 16, 30, 0, 0, time.UTC)}}
	names := status.NamesOf(config.Accounts{{ID: "c4rf", Label: "work"}}, removed)
	w := status.Words{status.Plain("moved from "), status.Named("c4rf"), status.Plain(" to "), status.Named("h7c2")}
	named := w.Named(names)
	if got, want := named.String(), "moved from work to team ("+removed[0].Said()+")"; got != want {
		t.Errorf("Named().String() = %q, want %q", got, want)
	}
	if named[1].Kind != status.PartAccount || w[1].Text != "c4rf" {
		t.Errorf("Named() = %+v of %+v, want each account still a PartAccount, and the words given unchanged", named, w)
	}
}
