package export_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/export"
)

// copiedConfig is the config the tests export a copy of, a pool's folder in
// the home directory.
const copiedConfig = `[[account]]
id    = "b6s7"
label = "Work"
pool  = "b2n8"

[[pool]]
id    = "b2n8"
label = "Default"
dirs  = ["~/Code"]
`

// configuredAt is when the tests' config was last modified.
var configuredAt = time.Date(2026, 10, 5, 9, 30, 0, 0, time.UTC)

// configExporter exports the history in the state directory stateDir, as
// exporter does, with a copy of the config file at path.
func configExporter(stateDir, path string) export.Exporter {
	x := exporter(stateDir)
	x.Config, x.HomeDir = path, func() (string, error) { return "/Users/someone", nil }
	return x
}

// putConfig writes a config file holding data in dir, last modified at
// modified, and returns where.
func putConfig(t *testing.T, dir, data string, modified time.Time) string {
	t.Helper()
	path := filepath.Join(dir, "config.toml")
	writeFile(t, path, []byte(data), modified)
	return path
}

// wantConfigHeld fails t where the export.json of folder doesn't say, as
// held says, that the folder holds a copy of the config.
func wantConfigHeld(t *testing.T, folder string, held bool) {
	t.Helper()
	if m, err := export.ReadManifest(folder); err != nil || m.Config != held {
		t.Errorf("the folder's export.json reads %+v (%v), want it to say it holds the config: %v", m, err, held)
	}
}

// The config is exported as a copy, with its time, before any day's lines,
// so a sync in the export's order never carries the days ahead of it, and
// export.json says the folder holds it; passed over while it's as it was, and
// written again once it changes.
func TestTheConfigIsExportedBeforeTheDaysAndSaidHeld(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	path := putConfig(t, t.TempDir(), copiedConfig, configuredAt)
	writeFile(t, filepath.Join(state, requests.compressed("", day1)), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
	folder := folderOf(t, state, dir)
	looked := 0
	x := export.WithLooked(configExporter(state, path), func(string) {
		looked++
		if _, err := os.Stat(export.ConfigFile(folder)); err != nil {
			t.Errorf("as the day's lines are looked at, the folder's config: %v, want it written", err)
		}
	})

	r, err := x.Export(dir)
	if err != nil || looked != 1 || r.Uncopied != nil {
		t.Fatalf("Export() = %+v, %v, looking at %d days; want the day exported, and the config copied", r, err, looked)
	}
	wantCopy(t, export.ConfigFile(folder), []byte(copiedConfig), configuredAt)
	wantConfigHeld(t, folder, true)
	was := look(t, export.ConfigFile(folder))

	if _, err := x.Export(dir); err != nil {
		t.Fatal(err)
	}
	wantUnwritten(t, export.ConfigFile(folder), was)

	changed := strings.Replace(copiedConfig, `"Work"`, `"Office"`, 1)
	putConfig(t, filepath.Dir(path), changed, configuredAt.Add(time.Hour))
	if _, err := x.Export(dir); err != nil {
		t.Fatal(err)
	}
	wantCopy(t, export.ConfigFile(folder), []byte(changed), configuredAt.Add(time.Hour))
}

// The copy never holds a token: one in a comment is hidden.
func TestTheConfigsCopyHidesATokenInAComment(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	path := putConfig(t, t.TempDir(), "# test-token: sk-ant-oat01-fake_token-shaped\n"+copiedConfig, configuredAt)

	if _, err := configExporter(state, path).Export(dir); err != nil {
		t.Fatal(err)
	}
	wantCopy(t, export.ConfigFile(folderOf(t, state, dir)), []byte("# test-token: [redacted]\n"+copiedConfig), configuredAt)
}

// A config that can't be copied, as one that doesn't load, one giving a
// value shaped like a token or one that's gone, fails no export: the folder
// keeps the copy it has, as a backup keeps the last config that loaded, and
// says it holds one where it does, and the export says why it copied none,
// but of one that's gone, which is none to copy.
func TestAConfigThatCantBeCopiedLeavesTheFoldersCopy(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   error
	}{
		{name: "one that doesn't load", config: "[[account]]\nid = 7\n", want: errors.New("any")},
		{name: "one giving a token", config: `upstream = "https://gateway.example.com/sk-ant-oat01-fake_token-shaped"` + "\n" + copiedConfig, want: config.ErrTokenValue},
		{name: "one that's gone"},
	}
	for _, tt := range tests {
		for _, before := range []bool{false, true} {
			name := tt.name + ", the folder holding none"
			if before {
				name = tt.name + ", the folder holding one"
			}
			t.Run(name, func(t *testing.T) {
				state, dir := t.TempDir(), t.TempDir()
				path := putConfig(t, t.TempDir(), copiedConfig, configuredAt)
				folder := folderOf(t, state, dir)
				if before {
					if _, err := configExporter(state, path).Export(dir); err != nil {
						t.Fatal(err)
					}
				}
				if tt.config == "" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				} else {
					putConfig(t, filepath.Dir(path), tt.config, configuredAt.Add(time.Hour))
				}

				r, err := configExporter(state, path).Export(dir)
				if err != nil || (r.Uncopied != nil) != (tt.want != nil) {
					t.Fatalf("Export() = %+v, %v; want it exported, saying why the config wasn't copied: %v", r, err, tt.want != nil)
				}
				if errors.Is(tt.want, config.ErrTokenValue) && !errors.Is(r.Uncopied, tt.want) || r.Uncopied != nil && strings.Contains(r.Uncopied.Error(), "sk-ant-") {
					t.Errorf("Export() said the config wasn't copied as %v, want %v, without the token", r.Uncopied, tt.want)
				}
				wantConfigHeld(t, folder, before)
				if before {
					wantCopy(t, export.ConfigFile(folder), []byte(copiedConfig), configuredAt)
				} else if _, err := os.Lstat(export.ConfigFile(folder)); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("the folder holds a config (%v), want none", err)
				}
			})
		}
	}
}

// An export given no config file copies none and says nothing of it.
func TestAnExportGivenNoConfigCopiesNone(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()

	r := exportInto(t, state, dir)
	if r.Uncopied != nil {
		t.Errorf("Export() = %+v, want it to say nothing of the config", r)
	}
	wantConfigHeld(t, folderOf(t, state, dir), false)
}
