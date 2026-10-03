package theme_test

import (
	"os"
	"path/filepath"
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

	if got := prefs.Read(); got != (theme.Prefs{}) {
		t.Errorf("Read() without a file = %+v, want the defaults", got)
	}
	if err := prefs.Update(func(p *theme.Prefs) { p.Choice = theme.Choice{Light: "exchange", Dark: "amber"} }); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got, want := prefs.Read(), (theme.Prefs{Light: "exchange", Dark: "amber"}); got != want {
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

func TestTheViewShownIsKeptBesideTheThemes(t *testing.T) {
	state := t.TempDir()
	prefs := theme.NewPrefsFile(state, clock)
	if err := prefs.Update(func(p *theme.Prefs) { p.Choice = theme.One("amber") }); err != nil {
		t.Fatal(err)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.View = "accounts" }); err != nil {
		t.Fatal(err)
	}
	if got, want := prefs.Read(), (theme.Prefs{Choice: theme.One("amber"), View: "accounts"}); got != want {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(filepath.Join(state, "prefs.json"))
	if err != nil || !strings.Contains(string(data), `"view": "accounts"`) {
		t.Errorf("prefs.json holds %s, %v; want the view kept", data, err)
	}
}

func TestTheFeaturedWindowIsKeptBesideTheView(t *testing.T) {
	state := t.TempDir()
	prefs := theme.NewPrefsFile(state, clock)
	if err := prefs.Update(func(p *theme.Prefs) { p.View = "accounts" }); err != nil {
		t.Fatal(err)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.Featured = "7d" }); err != nil {
		t.Fatal(err)
	}
	if got, want := prefs.Read(), (theme.Prefs{View: "accounts", Featured: "7d"}); got != want {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(filepath.Join(state, "prefs.json"))
	if err != nil || !strings.Contains(string(data), `"featured": "7d"`) {
		t.Errorf("prefs.json holds %s, %v; want the featured window kept", data, err)
	}
	if err := prefs.Update(func(p *theme.Prefs) { p.Featured = "" }); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(state, "prefs.json")); err != nil || strings.Contains(string(data), "featured") {
		t.Errorf("prefs.json holds %s, %v; want auto, the default, left out", data, err)
	}
}

func TestTheChartStyleIsKeptBesideTheFeaturedWindow(t *testing.T) {
	state := t.TempDir()
	prefs := theme.NewPrefsFile(state, clock)
	if err := prefs.Update(func(p *theme.Prefs) { p.Featured = "7d" }); err != nil {
		t.Fatal(err)
	}

	if err := prefs.Update(func(p *theme.Prefs) { p.Chart = "hourglass" }); err != nil {
		t.Fatal(err)
	}
	if got, want := prefs.Read(), (theme.Prefs{Featured: "7d", Chart: "hourglass"}); got != want {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(filepath.Join(state, "prefs.json"))
	if err != nil || !strings.Contains(string(data), `"chart": "hourglass"`) {
		t.Errorf("prefs.json holds %s, %v; want the chart style kept", data, err)
	}
	if err := prefs.Update(func(p *theme.Prefs) { p.Chart = "" }); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(state, "prefs.json")); err != nil || strings.Contains(string(data), "chart") {
		t.Errorf("prefs.json holds %s, %v; want burn-down, the default, left out", data, err)
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

			if got := theme.NewPrefsFile(state, clock).Read(); got != (theme.Prefs{}) {
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

	if got := theme.NewPrefsFile(state, clock).Read(); got != (theme.Prefs{}) {
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

func TestUpdateKeepsTheKeysOfANewerBuild(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "prefs.json")
	write(t, path, `{"theme": "amber", "featured": "7d", "lens": "wide", "pane": {"split": 0.5}}`)
	prefs := theme.NewPrefsFile(state, clock)

	if err := prefs.Update(func(p *theme.Prefs) { p.View, p.Featured = "accounts", "" }); err != nil {
		t.Fatal(err)
	}
	if got, want := prefs.Read(), (theme.Prefs{Choice: theme.One("amber"), View: "accounts"}); got != want {
		t.Errorf("Read() = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"lens\": \"wide\",\n  \"pane\": {\n    \"split\": 0.5\n  },\n  \"theme\": \"amber\",\n  \"view\": \"accounts\"\n}\n"
	if string(data) != want {
		t.Errorf("prefs.json holds\n%s\nwant\n%s", data, want)
	}
}

func TestAPreferencesFileThatCantBeReadLeavesTheDefaults(t *testing.T) {
	log := logstest.Capture(t)
	state := t.TempDir()
	if err := os.Mkdir(filepath.Join(state, "prefs.json"), 0o700); err != nil {
		t.Fatal(err)
	}

	if got := theme.NewPrefsFile(state, clock).Read(); got != (theme.Prefs{}) {
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
