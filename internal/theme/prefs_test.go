package theme_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/theme"
)

// prefsNow is when the tests' preferences files are read.
var prefsNow = time.Date(2026, 10, 2, 9, 30, 0, 0, time.UTC)

func clock() time.Time { return prefsNow }

func TestThePreferencesAreWrittenWholeAndReadBack(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	prefs := theme.NewPrefsFile(state, clock)

	if got := prefs.Read(); !reflect.DeepEqual(got, theme.Prefs{}) {
		t.Errorf("Read() without a file = %+v, want the defaults", got)
	}
	if err := prefs.Update(func(p *theme.Prefs) { p.Choice = theme.Choice{Light: "exchange", Dark: "amber"} }); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got, want := prefs.Read(), (theme.Prefs{Light: "exchange", Dark: "amber"}); !reflect.DeepEqual(got, want) {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(filepath.Join(state, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"theme_dark\": \"amber\",\n  \"theme_light\": \"exchange\"\n}\n"; string(data) != want {
		t.Errorf("prefs.json holds\n%s\nwant\n%s", data, want)
	}
	info, err := os.Stat(filepath.Join(state, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("prefs.json has mode %v, want it its owner's alone", info.Mode().Perm())
	}
	if entries, _ := os.ReadDir(state); len(entries) != 1 {
		t.Errorf("the state directory holds %d files, want prefs.json alone, nothing left beside it", len(entries))
	}
}

func TestUpdateKeepsWhatAnotherDashboardWroteSince(t *testing.T) {
	state := t.TempDir()
	mine, theirs := theme.NewPrefsFile(state, clock), theme.NewPrefsFile(state, clock)
	if err := theirs.Update(func(p *theme.Prefs) { p.Choice = theme.One("amber") }); err != nil {
		t.Fatal(err)
	}

	err := mine.Update(func(p *theme.Prefs) {
		if p.Theme != "amber" {
			t.Errorf("Update() changes %+v, want what the other dashboard wrote", *p)
		}
		p.Choice = p.WithHalf(true, "nord")
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := mine.Read().Choice, (theme.Choice{Dark: "nord"}); got != want {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
}

func TestTheTabShownIsKeptBesideTheThemes(t *testing.T) {
	state := t.TempDir()
	prefs := theme.NewPrefsFile(state, clock)
	if err := prefs.Update(func(p *theme.Prefs) { p.Choice = theme.One("amber") }); err != nil {
		t.Fatal(err)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.Tab = "accounts" }); err != nil {
		t.Fatal(err)
	}
	if got, want := prefs.Read(), (theme.Prefs{Choice: theme.One("amber"), Tab: "accounts"}); !reflect.DeepEqual(got, want) {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(filepath.Join(state, "prefs.json"))
	if err != nil || !strings.Contains(string(data), `"tab": "accounts"`) {
		t.Errorf("prefs.json holds %s, %v; want the tab kept", data, err)
	}
}

func TestTheOverviewsSectionsAndGroupingAreKeptBesideTheTab(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "prefs.json")
	prefs := theme.NewPrefsFile(state, clock)
	if err := prefs.Update(func(p *theme.Prefs) { p.Tab = "overview" }); err != nil {
		t.Fatal(err)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.Sections, p.By = []string{"usage", "sessions"}, "account" }); err != nil {
		t.Fatal(err)
	}
	if got, want := prefs.Read(), (theme.Prefs{Tab: "overview", Sections: []string{"usage", "sessions"}, By: "account"}); !reflect.DeepEqual(got, want) {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "\"sections\": [\n    \"usage\",\n    \"sessions\"\n  ]") || !strings.Contains(string(data), `"by": "account"`) {
		t.Errorf("prefs.json holds %s, %v; want the sections shown and their grouping kept", data, err)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.Sections = []string{} }); err != nil {
		t.Fatal(err)
	}
	if got := prefs.Read().Sections; got == nil || len(got) > 0 {
		t.Errorf("Read().Sections = %#v, want none shown, kept apart from the defaults", got)
	}
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), `"sections": []`) {
		t.Errorf("prefs.json holds %s, %v; want none shown kept as []", data, err)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.Sections, p.By = nil, "" }); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || strings.Contains(string(data), "sections") || strings.Contains(string(data), `"by"`) {
		t.Errorf("prefs.json holds %s, %v; want the defaults left out", data, err)
	}
}

// A file v0.2.7 wrote, with the Overview's GRAPHS shown, or its burn-down
// chosen, or both, reads cleanly, as its other sections and the rest as it
// kept them, and the next write keeps them and drops what's gone: GRAPHS
// among the sections and its chart. With GRAPHS alone shown, none of the rest
// is, kept as [].
func TestAFileOfV027WithGRAPHSReadsWithoutItAndTheNextWriteDropsIt(t *testing.T) {
	tests := []struct {
		name, file string
		read       theme.Prefs
		written    string
	}{
		{
			name: "GRAPHS shown and its burn-down",
			file: `{"by": "account", "graphs": "burn-down", "models": true, "sections": ["usage", "graphs", "routing", "lately"], "tab": "overview", "theme": "nord"}`,
			read: theme.Prefs{Choice: theme.Choice{Theme: "nord"}, Tab: "overview", Sections: []string{"usage", "routing", "lately"}, By: "account", Models: true},
			written: "{\n  \"by\": \"account\",\n  \"models\": true,\n  \"sections\": [\n    \"usage\",\n    \"routing\",\n    \"lately\"\n  ],\n" +
				"  \"tab\": \"runway\",\n  \"theme\": \"nord\"\n}\n",
		},
		{
			name:    "its burn-down alone",
			file:    `{"graphs": "burn-down", "tab": "overview"}`,
			read:    theme.Prefs{Tab: "overview"},
			written: "{\n  \"tab\": \"runway\"\n}\n",
		},
		{
			name:    "GRAPHS alone shown",
			file:    `{"sections": ["graphs"], "tab": "overview"}`,
			read:    theme.Prefs{Tab: "overview", Sections: []string{}},
			written: "{\n  \"sections\": [],\n  \"tab\": \"runway\"\n}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			path := filepath.Join(state, "prefs.json")
			write(t, path, tt.file)
			prefs := theme.NewPrefsFile(state, clock)

			if got := prefs.Read(); !reflect.DeepEqual(got, tt.read) {
				t.Errorf("Read() = %+v, want %+v", got, tt.read)
			}
			if _, err := os.Stat(path); err != nil {
				t.Errorf("the file is gone after a read, %v; want it read cleanly, not set aside", err)
			}
			if err := prefs.Update(func(p *theme.Prefs) { p.Tab = "runway" }); err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != tt.written {
				t.Errorf("after the next write, prefs.json holds\n%s\nwant\n%s", data, tt.written)
			}
		})
	}
}

// Whether the live views show each model's own window is kept beside the
// rest and read back; left out while they're hidden, as a file without it
// reads, and as one of an earlier build does.
func TestWhetherModelsWindowsShowIsKeptAndHiddenWithout(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "prefs.json")
	prefs := theme.NewPrefsFile(state, clock)

	if prefs.Read().Models {
		t.Error("without a file, Read().Models is set; want them hidden")
	}
	if err := os.WriteFile(path, []byte(`{"tab": "runway", "graphs": "burn-down"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if prefs.Read().Models {
		t.Error("a file of an earlier build reads Models set; want them hidden")
	}
	if err := prefs.Update(func(p *theme.Prefs) { p.Models = true }); err != nil {
		t.Fatal(err)
	}
	if got := prefs.Read(); !got.Models || got.Tab != "runway" {
		t.Errorf("Read() = %+v, want Models read back beside the tab", got)
	}
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), `"models": true`) {
		t.Errorf("prefs.json holds %s, %v; want models kept", data, err)
	}
	if err := prefs.Update(func(p *theme.Prefs) { p.Models = false }); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || strings.Contains(string(data), "models") {
		t.Errorf("prefs.json holds %s, %v; want models left out while hidden", data, err)
	}
}

// What Accounts shows is kept under a key of its own, the view and the
// period by name, beside the rest and the keys of another build, and left
// out where none is made, as this week is.
func TestAccountsViewAndPeriodAreKeptUnderAKeyOfTheirOwn(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "prefs.json")
	write(t, path, `{"theme": "amber", "pane": {"split": 0.5}}`)
	prefs := theme.NewPrefsFile(state, clock)
	accounts := theme.AccountsPrefs{View: "detail", Period: "30-days"}

	if err := prefs.Update(func(p *theme.Prefs) { p.Accounts = accounts }); err != nil {
		t.Fatal(err)
	}
	if got, want := prefs.Read(), (theme.Prefs{Choice: theme.One("amber"), Accounts: accounts}); !reflect.DeepEqual(got, want) {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(path)
	want := "{\n  \"accounts\": {\n    \"view\": \"detail\",\n    \"period\": \"30-days\"\n  },\n" +
		"  \"pane\": {\n    \"split\": 0.5\n  },\n  \"theme\": \"amber\"\n}\n"
	if err != nil || string(data) != want {
		t.Errorf("prefs.json holds\n%s\nwant\n%s", data, want)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.Accounts = theme.AccountsPrefs{} }); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || strings.Contains(string(data), "accounts") {
		t.Errorf("prefs.json holds %s, %v; want no choice of Accounts' kept", data, err)
	}
}

// What Sessions shows is kept under a key of its own, the view by name,
// beside the rest and the keys of another build, and left out where none is
// made, as the Switchboard is.
func TestSessionsViewIsKeptUnderAKeyOfItsOwn(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "prefs.json")
	write(t, path, `{"theme": "amber", "pane": {"split": 0.5}}`)
	prefs := theme.NewPrefsFile(state, clock)
	sessions := theme.SessionsPrefs{View: "list"}

	if err := prefs.Update(func(p *theme.Prefs) { p.Sessions = sessions }); err != nil {
		t.Fatal(err)
	}
	if got, want := prefs.Read(), (theme.Prefs{Choice: theme.One("amber"), Sessions: sessions}); !reflect.DeepEqual(got, want) {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(path)
	want := "{\n  \"pane\": {\n    \"split\": 0.5\n  },\n  \"sessions\": {\n    \"view\": \"list\"\n  },\n  \"theme\": \"amber\"\n}\n"
	if err != nil || string(data) != want {
		t.Errorf("prefs.json holds\n%s\nwant\n%s", data, want)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.Sessions = theme.SessionsPrefs{} }); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || strings.Contains(string(data), "sessions") {
		t.Errorf("prefs.json holds %s, %v; want no choice of Sessions' kept", data, err)
	}
}

// Runway's window is kept under a key of its own, by name, beside the rest
// and the keys of another build, and left out where none is made, as the
// 5-hour is.
func TestRunwaysWindowIsKeptUnderAKeyOfItsOwn(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "prefs.json")
	write(t, path, `{"theme": "amber", "pane": {"split": 0.5}}`)
	prefs := theme.NewPrefsFile(state, clock)
	runway := theme.RunwayPrefs{Window: "week"}

	if err := prefs.Update(func(p *theme.Prefs) { p.Runway = runway }); err != nil {
		t.Fatal(err)
	}
	if got, want := prefs.Read(), (theme.Prefs{Choice: theme.One("amber"), Runway: runway}); !reflect.DeepEqual(got, want) {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(path)
	want := "{\n  \"pane\": {\n    \"split\": 0.5\n  },\n  \"runway\": {\n    \"window\": \"week\"\n  },\n  \"theme\": \"amber\"\n}\n"
	if err != nil || string(data) != want {
		t.Errorf("prefs.json holds\n%s\nwant\n%s", data, want)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.Runway = theme.RunwayPrefs{} }); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || strings.Contains(string(data), "runway") {
		t.Errorf("prefs.json holds %s, %v; want no window of Runway's kept", data, err)
	}
}

// Whether Runway's account column is collapsed is kept beside its window,
// read back as it was kept, and read as expanded from a file without it, as
// an earlier build's, and left out once it's expanded again.
func TestWhetherRunwaysAccountColumnIsCollapsedIsKeptAndExpandedWithout(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "prefs.json")
	write(t, path, `{"runway": {"window": "5-hour"}}`)
	prefs := theme.NewPrefsFile(state, clock)
	if prefs.Read().Runway.Collapsed {
		t.Error("a file of an earlier build reads Runway collapsed; want it expanded")
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.Runway.Collapsed = true }); err != nil {
		t.Fatal(err)
	}
	if got, want := prefs.Read().Runway, (theme.RunwayPrefs{Window: "5-hour", Collapsed: true}); got != want {
		t.Errorf("Read().Runway = %+v, want %+v", got, want)
	}
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), `"collapsed": true`) {
		t.Errorf("prefs.json holds %s, %v; want Runway's column kept collapsed", data, err)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.Runway.Collapsed = false }); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || strings.Contains(string(data), "collapsed") {
		t.Errorf("prefs.json holds %s, %v; want no collapsed kept once it's expanded", data, err)
	}
}

// What History's views show is kept under a key of its own, each choice by
// its name, beside the rest and the keys of another build, and left out
// where none is made.
func TestHistorysViewsAreKeptUnderAKeyOfTheirOwn(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "prefs.json")
	write(t, path, `{"theme": "amber", "pane": {"split": 0.5}}`)
	prefs := theme.NewPrefsFile(state, clock)
	history := theme.HistoryPrefs{View: "tokens", Year: "Opus 5.5", Weeks: "columns", Tokens: theme.TokensPrefs{Period: "month", Split: "account+model"}}

	if err := prefs.Update(func(p *theme.Prefs) { p.History = history }); err != nil {
		t.Fatal(err)
	}
	if got, want := prefs.Read(), (theme.Prefs{Choice: theme.One("amber"), History: history}); !reflect.DeepEqual(got, want) {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(path)
	want := "{\n  \"history\": {\n    \"view\": \"tokens\",\n    \"year\": \"Opus 5.5\",\n    \"weeks\": \"columns\",\n" +
		"    \"tokens\": {\n      \"period\": \"month\",\n      \"split\": \"account+model\"\n    }\n  },\n" +
		"  \"pane\": {\n    \"split\": 0.5\n  },\n  \"theme\": \"amber\"\n}\n"
	if err != nil || string(data) != want {
		t.Errorf("prefs.json holds\n%s\nwant\n%s", data, want)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.History = theme.HistoryPrefs{} }); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || strings.Contains(string(data), "history") {
		t.Errorf("prefs.json holds %s, %v; want no choice of History's kept", data, err)
	}
}

// The Log's columns are kept under a key of their own, by name, beside the
// rest and the keys of another build: none shown kept as [], apart from the
// defaults, which are left out.
func TestTheLogsColumnsAreKeptUnderAKeyOfTheirOwn(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "prefs.json")
	write(t, path, `{"theme": "amber", "pane": {"split": 0.5}}`)
	prefs := theme.NewPrefsFile(state, clock)
	log := theme.LogPrefs{Columns: []string{"session", "worth", "request id"}}

	if err := prefs.Update(func(p *theme.Prefs) { p.Log = log }); err != nil {
		t.Fatal(err)
	}
	if got, want := prefs.Read(), (theme.Prefs{Choice: theme.One("amber"), Log: log}); !reflect.DeepEqual(got, want) {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(path)
	want := "{\n  \"log\": {\n    \"columns\": [\n      \"session\",\n      \"worth\",\n      \"request id\"\n    ]\n  },\n" +
		"  \"pane\": {\n    \"split\": 0.5\n  },\n  \"theme\": \"amber\"\n}\n"
	if err != nil || string(data) != want {
		t.Errorf("prefs.json holds\n%s\nwant\n%s", data, want)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.Log = theme.LogPrefs{Columns: []string{}} }); err != nil {
		t.Fatal(err)
	}
	if got := prefs.Read().Log.Columns; got == nil || len(got) > 0 {
		t.Errorf("Read().Log.Columns = %#v, want none shown, kept apart from the defaults", got)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.Log = theme.LogPrefs{} }); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || strings.Contains(string(data), "log") {
		t.Errorf("prefs.json holds %s, %v; want the Log's defaults left out", data, err)
	}
}

// Graphs' sections shown are kept under a key of their own, by name, apart
// from "graphs", where a file of before them keeps the Overview's GRAPHS'
// chart: none shown kept as [], apart from the defaults, which are left out.
func TestGraphsSectionsAreKeptUnderAKeyOfTheirOwn(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "prefs.json")
	write(t, path, `{"graphs": "burn-down", "tab": "graphs"}`)
	prefs := theme.NewPrefsFile(state, clock)
	if got, want := prefs.Read(), (theme.Prefs{Tab: "graphs"}); !reflect.DeepEqual(got, want) {
		t.Errorf("a file of before Graphs' sections reads %+v, want %+v", got, want)
	}
	graphs := theme.GraphsPrefs{Sections: []string{"week", "pool", "when"}}

	if err := prefs.Update(func(p *theme.Prefs) { p.GraphsTab = graphs }); err != nil {
		t.Fatal(err)
	}
	if got, want := prefs.Read(), (theme.Prefs{Tab: "graphs", GraphsTab: graphs}); !reflect.DeepEqual(got, want) {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(path)
	want := "{\n  \"graphs_tab\": {\n    \"sections\": [\n      \"week\",\n      \"pool\",\n      \"when\"\n    ]\n  },\n" +
		"  \"tab\": \"graphs\"\n}\n"
	if err != nil || string(data) != want {
		t.Errorf("prefs.json holds\n%s\nwant\n%s", data, want)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.GraphsTab = theme.GraphsPrefs{Sections: []string{}} }); err != nil {
		t.Fatal(err)
	}
	if got := prefs.Read().GraphsTab.Sections; got == nil || len(got) > 0 {
		t.Errorf("Read().GraphsTab.Sections = %#v, want none shown, kept apart from the defaults", got)
	}
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "\"graphs_tab\": {\n    \"sections\": []") {
		t.Errorf("prefs.json holds %s, %v; want none shown kept as []", data, err)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.GraphsTab = theme.GraphsPrefs{} }); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || strings.Contains(string(data), "graphs_tab") {
		t.Errorf("prefs.json holds %s, %v; want Graphs' defaults left out", data, err)
	}
}

func TestACorruptPreferencesFileIsSetAsideAndTheDefaultsStand(t *testing.T) {
	tests := []struct {
		name, content string
	}{
		{name: "not JSON", content: "{\"theme\": \"amber\""},
		{name: "a theme that isn't a string", content: "{\"theme\": 7}"},
		{name: "not an object", content: "[\"amber\"]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			state := t.TempDir()
			path := filepath.Join(state, "prefs.json")
			write(t, path, tt.content)

			if got := theme.NewPrefsFile(state, clock).Read(); !reflect.DeepEqual(got, theme.Prefs{}) {
				t.Errorf("Read() = %+v, want the defaults", got)
			}
			aside := path + ".corrupt-1790933400"
			if data, err := os.ReadFile(aside); err != nil || string(data) != tt.content {
				t.Errorf("set aside: %q, %v; want the corrupt file at %s, as it was", data, err, aside)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("prefs.json: %v, want it gone, set aside", err)
			}
			if !log.Has("level=WARN", `msg="preferences file corrupt; set aside, the defaults stand"`, "aside="+aside) {
				t.Errorf("the log reads\n%s\nwant it to say the file was set aside, and where", log)
			}
		})
	}
}

func TestACorruptPreferencesFileLinkedElsewhereIsSetAsideWhereItLeads(t *testing.T) {
	log := logstest.Capture(t)
	state, elsewhere := t.TempDir(), t.TempDir()
	kept, link := filepath.Join(elsewhere, "prefs.json"), filepath.Join(state, "prefs.json")
	write(t, kept, "{\"theme\": ")
	symlink(t, kept, link)

	if got := theme.NewPrefsFile(state, clock).Read(); !reflect.DeepEqual(got, theme.Prefs{}) {
		t.Errorf("Read() = %+v, want the defaults", got)
	}
	aside := kept + ".corrupt-1790933400"
	if data, err := os.ReadFile(aside); err != nil || string(data) != "{\"theme\": " {
		t.Errorf("set aside: %q, %v; want the corrupt file at %s, beside where it was", data, err, aside)
	}
	if target, err := os.Readlink(link); err != nil || target != kept {
		t.Errorf("prefs.json: link to %q, %v; want the link kept, leading to %s", target, err, kept)
	}
	if !log.Has("level=WARN", `msg="preferences file corrupt; set aside, the defaults stand"`, "aside="+aside) {
		t.Errorf("the log reads\n%s\nwant it to say the file was set aside, and where", log)
	}
}

func TestUpdateKeepsTheKeysOfAnOlderOrANewerBuild(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "prefs.json")
	write(t, path, `{"theme": "amber", "featured": "7d", "chart": "hourglass", "lens": "wide", "pane": {"split": 0.5}}`)
	prefs := theme.NewPrefsFile(state, clock)

	if err := prefs.Update(func(p *theme.Prefs) { p.Tab = "accounts" }); err != nil {
		t.Fatal(err)
	}
	if got, want := prefs.Read(), (theme.Prefs{Choice: theme.One("amber"), Tab: "accounts"}); !reflect.DeepEqual(got, want) {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"chart\": \"hourglass\",\n  \"featured\": \"7d\",\n  \"lens\": \"wide\",\n  \"pane\": {\n    \"split\": 0.5\n  },\n  \"tab\": \"accounts\",\n  \"theme\": \"amber\"\n}\n"
	if string(data) != want {
		t.Errorf("prefs.json holds\n%s\nwant\n%s", data, want)
	}
}

func TestMilestone5sCardViewKeptAsTheViewIsNoTab(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "prefs.json")
	write(t, path, `{"theme": "amber", "view": "accounts"}`)
	prefs := theme.NewPrefsFile(state, clock)

	if got, want := prefs.Read(), (theme.Prefs{Choice: theme.One("amber")}); !reflect.DeepEqual(got, want) {
		t.Errorf("Read() = %+v, want %+v: no tab, for the Overview, as milestone 5's card view, kept, names none", got, want)
	}
	if err := prefs.Update(func(p *theme.Prefs) { p.Tab = "runway" }); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if want := "{\n  \"tab\": \"runway\",\n  \"theme\": \"amber\",\n  \"view\": \"accounts\"\n}\n"; err != nil || string(data) != want {
		t.Errorf("prefs.json holds\n%s\nwant\n%s", data, want)
	}
}

func TestAPreferencesFileThatCantBeReadLeavesTheDefaults(t *testing.T) {
	log := logstest.Capture(t)
	state := t.TempDir()
	if err := os.Mkdir(filepath.Join(state, "prefs.json"), 0o700); err != nil {
		t.Fatal(err)
	}

	if got := theme.NewPrefsFile(state, clock).Read(); !reflect.DeepEqual(got, theme.Prefs{}) {
		t.Errorf("Read() = %+v, want the defaults", got)
	}
	if _, err := os.Stat(filepath.Join(state, "prefs.json")); err != nil {
		t.Errorf("prefs.json: %v, want it left where it is, as it's no file to set aside", err)
	}
	if !log.Has("level=WARN", `msg="can't read the preferences file; the defaults stand"`) {
		t.Errorf("the log reads\n%s\nwant it to say the file can't be read", log)
	}
}

func TestAPreferencesFileLinkedElsewhereIsWrittenWhereItLeads(t *testing.T) {
	state, elsewhere := t.TempDir(), t.TempDir()
	kept := filepath.Join(elsewhere, "prefs.json")
	write(t, kept, "{\"theme\": \"amber\"}\n")
	symlink(t, kept, filepath.Join(state, "prefs.json"))
	prefs := theme.NewPrefsFile(state, clock)

	if err := prefs.Update(func(p *theme.Prefs) { p.Choice = theme.One("exchange") }); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(filepath.Join(state, "prefs.json")); err != nil || target != kept {
		t.Errorf("prefs.json: link to %q, %v; want the link kept, leading to %s", target, err, kept)
	}
	if data, _ := os.ReadFile(kept); !strings.Contains(string(data), `"theme": "exchange"`) {
		t.Errorf("the file the link leads to holds %s, want the new choice", data)
	}
}
