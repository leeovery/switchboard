package ledger_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
)

func TestTheVersionTableNamesEachVersionByFamilyNewestFirst(t *testing.T) {
	want := []struct {
		name, family string
		ids          []string
	}{
		{name: "Opus 5.5", family: "opus", ids: []string{"claude-opus-5-5"}},
		{name: "Opus 5", family: "opus", ids: []string{"claude-opus-5"}},
		{name: "Opus 4.8", family: "opus", ids: []string{"claude-opus-4-8"}},
		{name: "Opus 4.7", family: "opus", ids: []string{"claude-opus-4-7"}},
		{name: "Opus 4.6", family: "opus", ids: []string{"claude-opus-4-6"}},
		{name: "Opus 4.5", family: "opus", ids: []string{"claude-opus-4-5-20251101", "claude-opus-4-5"}},
		{name: "Sonnet 5.5", family: "sonnet", ids: []string{"claude-sonnet-5-5"}},
		{name: "Sonnet 5", family: "sonnet", ids: []string{"claude-sonnet-5"}},
		{name: "Sonnet 4.6", family: "sonnet", ids: []string{"claude-sonnet-4-6"}},
		{name: "Sonnet 4.5", family: "sonnet", ids: []string{"claude-sonnet-4-5-20250929", "claude-sonnet-4-5"}},
		{name: "Fable 5.1", family: "fable", ids: []string{"claude-fable-5-1"}},
		{name: "Fable 5", family: "fable", ids: []string{"claude-fable-5"}},
		{name: "Haiku 5.5", family: "haiku", ids: []string{"claude-haiku-5-5"}},
		{name: "Haiku 4.5", family: "haiku", ids: []string{"claude-haiku-4-5-20251001", "claude-haiku-4-5"}},
	}
	got := ledger.Pricing.Versions
	if len(got) != len(want) {
		t.Fatalf("the version table has %d rows, want %d", len(got), len(want))
	}
	for i, w := range want {
		if v := got[i]; v.Name != w.name || v.Family != w.family || !slices.Equal(v.IDs, w.ids) {
			t.Errorf("row %d is %+v, want %s, of the %s family, by the ids %q", i+1, v, w.name, w.family, w.ids)
		}
	}
}

// The table gives viz.version.1 to viz.version.6 to Opus 5.5 and Opus 5,
// Sonnet 5.5 and Sonnet 5, Fable 5.1, and Haiku 5.5, which took Haiku 4.5's
// as it was priced; and marks each version older than its family's newest.
func TestTheVersionTableShadesEachFamilysNewestFirst(t *testing.T) {
	want := map[string]struct {
		shade int
		older bool
	}{
		"Opus 5.5": {shade: 1}, "Opus 5": {shade: 2, older: true}, "Opus 4.8": {older: true}, "Opus 4.7": {older: true},
		"Opus 4.6": {older: true}, "Opus 4.5": {older: true},
		"Sonnet 5.5": {shade: 3}, "Sonnet 5": {shade: 4, older: true}, "Sonnet 4.6": {older: true}, "Sonnet 4.5": {older: true},
		"Fable 5.1": {shade: 5}, "Fable 5": {older: true},
		"Haiku 5.5": {shade: 6}, "Haiku 4.5": {older: true},
	}
	for _, v := range ledger.Pricing.Versions {
		w, ok := want[v.Name]
		if !ok {
			t.Errorf("the version table has %s, which this test doesn't know", v.Name)
			continue
		}
		if v.Shade != w.shade || v.Older != w.older {
			t.Errorf("%s has shade %d, older %v; want shade %d, older %v", v.Name, v.Shade, v.Older, w.shade, w.older)
		}
	}
}

func TestTheVersionTableNamesEachModelThePriceTableDoesByItsIDs(t *testing.T) {
	for _, m := range ledger.Pricing.Models {
		named := slices.IndexFunc(ledger.Pricing.Versions, func(v ledger.Version) bool { return slices.Equal(v.IDs, m.IDs) })
		if named < 0 {
			t.Errorf("the version table has no row by the ids %q, as the price table holds them", m.IDs)
		}
	}
	if got, want := len(ledger.Pricing.Versions), len(ledger.Pricing.Models); got != want {
		t.Errorf("the version table has %d rows, want one for each of the price table's %d models", got, want)
	}
}

func TestTheVersionTablesRowsGoByFamilyThenNewestFirst(t *testing.T) {
	families := ledger.Families()
	// launched is the date of the day the version with the given ids
	// launched: that of its first prices.
	launched := func(ids []string) string {
		i := slices.IndexFunc(ledger.Pricing.Models, func(m ledger.Model) bool { return slices.Equal(m.IDs, ids) })
		if i < 0 {
			t.Fatalf("the price table has no model by the ids %q", ids)
		}
		return ledger.Pricing.Models[i].Prices[0].From
	}
	rows := ledger.Pricing.Versions
	for i := 1; i < len(rows); i++ {
		before, after := rows[i-1], rows[i]
		order := slices.Index(families, before.Family) - slices.Index(families, after.Family)
		if order > 0 || order == 0 && launched(before.IDs) <= launched(after.IDs) {
			t.Errorf("%s, launched %s, comes before %s, launched %s, want rows by family, %q, then newest first",
				before.Name, launched(before.IDs), after.Name, launched(after.IDs), families)
		}
	}
}

// Every family the version table names is one of the families the router
// knows, and the one it reads from each of the version's ids.
func TestEveryFamilyTheVersionTableNamesIsOneTheRouterKnows(t *testing.T) {
	for _, v := range ledger.Pricing.Versions {
		if !slices.Contains(ledger.Families(), v.Family) {
			t.Errorf("%s is of the %s family, which isn't one of the router's, %q", v.Name, v.Family, ledger.Families())
		}
		for _, id := range v.IDs {
			if read := (claude.Provider{}).Family(id); read != v.Family {
				t.Errorf("%s is of the %s family, want %s, as the router reads it from %s", v.Name, v.Family, read, id)
			}
		}
	}
}

// A model named by its id is named by its id less claude-, cleaned first, so
// a recorded id with stray spaces or control characters is named as any
// other, wherever it's named: the one owner the CLI, the views and the
// dashboard call.
func TestAModelsShortIDIsItsIDLessClaudeCleaned(t *testing.T) {
	for id, want := range map[string]string{
		"claude-opus-5-5":        "opus-5-5",
		"  claude-opus-5-5\t":    "opus-5-5",
		"claude-opus-5-5\x1b[2J": "opus-5-5 [2J",
		"gpt-x":                  "gpt-x",
		"":                       "",
	} {
		if got := ledger.ShortModelID(id); got != want {
			t.Errorf("ShortModelID(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestAVersionIsNamedByTheTableElseByItsID(t *testing.T) {
	tests := []struct {
		id, want string
	}{
		{id: "claude-opus-5-5", want: "Opus 5.5"},
		{id: "claude-opus-4-5-20251101", want: "Opus 4.5"},
		{id: "claude-opus-4-5", want: "Opus 4.5"},
		{id: "claude-fable-5", want: "Fable 5"},
		{id: "claude-opus-6", want: "opus-6"},
		{id: "claude-sonnet-5-5-20270101", want: "sonnet-5-5-20270101"},
		{id: "some-other-model", want: "some-other-model"},
		{id: "", want: ""},
	}
	for _, tt := range tests {
		if got := ledger.Pricing.VersionName(tt.id); got != tt.want {
			t.Errorf("VersionName(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
	if v, ok := ledger.Pricing.Version("claude-opus-6"); ok {
		t.Errorf("Version(claude-opus-6) = %+v, want none: the table doesn't name it", v)
	}
}
