package theme_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestLoadFindsABuiltInElseItsFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "lake.theme"), file(nil))
	// A file taking a built-in's slug, which the built-in keeps.
	write(t, filepath.Join(dir, "nord.theme"), file([]string{"canvas"}, "canvas = #000000"))
	lib := theme.NewLibrary(dir)

	lake, err := lib.Load("lake")
	if err != nil || lake.Slug != "lake" || hex(lake.Colour(theme.Canvas)) != "#2E3440" {
		t.Errorf("Load(lake) = %s, %v; want lake from its file", lake.Slug, err)
	}
	nord, err := lib.Load("nord")
	if err != nil || hex(nord.Colour(theme.Canvas)) != "#2E3440" {
		t.Errorf("Load(nord) = canvas %s, %v; want the built-in's #2E3440, never the file taking its slug", hex(nord.Colour(theme.Canvas)), err)
	}
}

func TestLoadSaysWhyAThemeDoesntLoad(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "themes")
	write(t, filepath.Join(dir, "broken.theme"), file([]string{"canvas"}))
	symlink(t, filepath.Join(dir, "gone.theme"), filepath.Join(dir, "dangling.theme"))
	write(t, filepath.Join(root, "outside.theme"), file(nil))
	tests := []struct {
		name, slug string
		dir        string
		// wantReason is the problem's reason, as the picker shows it.
		wantReason string
	}{
		{name: "no such theme", slug: "lake", dir: dir, wantReason: "not found"},
		{name: "no themes directory", slug: "lake", wantReason: "not found"},
		{name: "a themes directory that isn't there", slug: "lake", dir: filepath.Join(dir, "missing"), wantReason: "not found"},
		{name: "a broken file", slug: "broken", dir: dir, wantReason: "missing tokens"},
		{name: "a link that leads nowhere", slug: "dangling", dir: dir, wantReason: "unreadable"},
		{name: "a slug that would leave the directory", slug: "../outside", dir: dir, wantReason: "bad name"},
		{name: "a slug that isn't one", slug: "Lake", dir: dir, wantReason: "bad name"},
		{name: "no slug", dir: dir, wantReason: "bad name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := theme.NewLibrary(tt.dir).Load(tt.slug)
			p, ok := errors.AsType[*theme.Problem](err)
			if !ok || p.Reason != tt.wantReason {
				t.Errorf("Load(%q) error = %v, want a problem of %s", tt.slug, err, tt.wantReason)
			}
		})
	}
}

func TestListListsEveryThemeLoadedOrNot(t *testing.T) {
	dir, elsewhere := t.TempDir(), t.TempDir()
	write(t, filepath.Join(dir, "lake.theme"), file(nil))
	write(t, filepath.Join(dir, "broken.theme"), file([]string{"canvas"}))
	write(t, filepath.Join(dir, "Upper.theme"), file(nil))
	write(t, filepath.Join(dir, "shout.THEME"), file(nil))
	write(t, filepath.Join(dir, "nord.theme"), file(nil))
	write(t, filepath.Join(dir, "notes.txt"), "not a theme\n")
	write(t, filepath.Join(dir, "nested", "deep.theme"), file(nil))
	write(t, filepath.Join(elsewhere, "kept.theme"), file(nil))
	symlink(t, filepath.Join(elsewhere, "kept.theme"), filepath.Join(dir, "linked.theme"))
	symlink(t, filepath.Join(elsewhere, "gone.theme"), filepath.Join(dir, "dangling.theme"))
	if err := os.Mkdir(filepath.Join(dir, "folder.theme"), 0o700); err != nil {
		t.Fatal(err)
	}

	got := theme.NewLibrary(dir).List()

	if got.Problem != nil {
		t.Errorf("List() problem = %v, want none", got.Problem)
	}
	want := []string{
		"amber", "broken: missing tokens", "dangling: unreadable", "exchange", "lake", "linked", "nord",
		"nord.theme: reserved name", "shout.THEME: bad name", "terminal", "tokyo-night", "tokyo-night-day", "Upper.theme: bad name",
	}
	if listed := listed(got); !slices.Equal(listed, want) {
		t.Errorf("List() lists\n%q\nwant\n%q", listed, want)
	}
	if i := slices.IndexFunc(got.Entries, func(e theme.Entry) bool { return e.Name == "linked" }); i < 0 || got.Entries[i].Slug != "linked" {
		t.Errorf("a link to a theme kept elsewhere isn't listed under its link's slug")
	}
}

func TestAThemesDirectoryLinkedInIsFollowed(t *testing.T) {
	dotfiles, config := t.TempDir(), t.TempDir()
	write(t, filepath.Join(dotfiles, "themes", "lake.theme"), file(nil))
	symlink(t, filepath.Join(dotfiles, "themes"), filepath.Join(config, "themes"))
	lib := theme.NewLibrary(filepath.Join(config, "themes"))

	if got := listed(lib.List()); !slices.Contains(got, "lake") {
		t.Errorf("List() lists %q, want lake, from the directory the link leads to", got)
	}
	if _, err := lib.Load("lake"); err != nil {
		t.Errorf("Load(lake) error = %v, want the theme from the directory the link leads to", err)
	}
}

func TestListWithoutAThemesDirectoryListsTheBuiltIns(t *testing.T) {
	for _, dir := range []string{"", filepath.Join(t.TempDir(), "missing")} {
		got := theme.NewLibrary(dir).List()
		if got.Problem != nil {
			t.Errorf("List() of %q: problem = %v, want none, as no directory is no theme", dir, got.Problem)
		}
		if listed, want := listed(got), []string{"amber", "exchange", "nord", "terminal", "tokyo-night", "tokyo-night-day"}; !slices.Equal(listed, want) {
			t.Errorf("List() of %q lists %q, want the built-ins, %q", dir, listed, want)
		}
	}
}

func TestListSaysWhenTheThemesDirectoryCantBeRead(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "themes")
	write(t, notADir, "a file where the directory goes\n")

	got := theme.NewLibrary(notADir).List()

	if got.Problem == nil {
		t.Error("List() of a file where the themes directory goes: no problem, want why it couldn't be read")
	}
	if len(got.Entries) != len(theme.Builtins()) {
		t.Errorf("List() lists %d themes, want the %d built-ins all the same", len(got.Entries), len(theme.Builtins()))
	}
}

func TestListTellsTheLogOfABrokenThemeOnce(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "broken.theme"), file([]string{"canvas"}))
	lib := theme.NewLibrary(dir)

	lib.List()
	lib.List()

	var told []string
	for _, line := range log.Lines() {
		if strings.Contains(line, `msg="a theme doesn't load"`) {
			told = append(told, line)
		}
	}
	if len(told) != 1 || !strings.Contains(told[0], "level=WARN") || !strings.Contains(told[0], "missing canvas") {
		t.Errorf("the log tells of the broken theme in\n%s\nwant a warning, once, saying why", strings.Join(told, "\n"))
	}
}

func TestRowsListTheThemesAChoiceNamesThatArentThere(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "lake.theme"), file(nil))
	unreadable := filepath.Join(t.TempDir(), "themes")
	write(t, unreadable, "a file where the directory goes\n")
	tests := []struct {
		name    string
		dir     string
		choice  theme.Choice
		wantNew []string
	}{
		{name: "a theme there", dir: dir, choice: theme.One("lake")},
		{name: "a built-in", dir: dir, choice: theme.Choice{Dark: "amber"}},
		{name: "one theme not there", dir: dir, choice: theme.One("gone"), wantNew: []string{"gone: not found"}},
		{name: "both halves not there", dir: dir, choice: theme.Choice{Light: "gone", Dark: "lost"}, wantNew: []string{"gone: not found", "lost: not found"}},
		{name: "a slug that names no theme", dir: dir, choice: theme.One("../lake"), wantNew: []string{"../lake: bad name"}},
		{name: "a theme in a directory that can't be read", dir: unreadable, choice: theme.One("lake"), wantNew: []string{"lake: unreadable"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listing := theme.NewLibrary(tt.dir).List()
			rows := theme.Listing{Entries: listing.Rows(tt.choice)}
			var added []string
			for _, row := range listed(rows) {
				if !slices.Contains(listed(listing), row) {
					added = append(added, row)
				}
			}
			if !slices.Equal(added, tt.wantNew) {
				t.Errorf("Rows() adds %q to the listing, want %q", added, tt.wantNew)
			}
			if !slices.IsSortedFunc(rows.Entries, func(a, b theme.Entry) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) }) {
				t.Errorf("Rows() = %q, want them in order of their names", listed(rows))
			}
		})
	}
}

func TestReadFileReadsAThemeWhateverItsNamed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "My Theme.theme")
	write(t, path, file(nil))

	got, err := theme.ReadFile(path)
	if err != nil || got.Slug != "My Theme" {
		t.Errorf("ReadFile() = %q, %v; want the theme, named by its file", got.Slug, err)
	}
	if _, err := theme.ReadFile(filepath.Join(t.TempDir(), "gone.theme")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile() of no file: error = %v, want it not to exist", err)
	}
}

// listed is each theme the listing lists, by name, and why it doesn't load
// where it doesn't.
func listed(l theme.Listing) []string {
	var names []string
	for _, e := range l.Entries {
		if e.Problem != nil {
			names = append(names, e.Name+": "+e.Problem.Reason)
			continue
		}
		names = append(names, e.Name)
	}
	return names
}

// write writes content to a file at path, creating its directories.
func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// symlink links path to target, creating path's directories.
func symlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}
