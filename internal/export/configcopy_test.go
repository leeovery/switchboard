package export_test

import (
	"bytes"
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

// wantNoConfig fails t where the Mac's folder at folder holds a copy of the
// config.
func wantNoConfig(t *testing.T, folder string) {
	t.Helper()
	if _, err := os.Lstat(export.ConfigFile(folder)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the folder holds a config (%v), want none", err)
	}
}

// copyOf is the copy of the config text gives, as config.Copy makes it.
func copyOf(t *testing.T, text string) []byte {
	t.Helper()
	copied, err := config.Copy("config.toml", []byte(text), func() (string, error) { return "/Users/someone", nil })
	if err != nil {
		t.Fatal(err)
	}
	return copied
}

// The config is exported as a copy, with its time, before any day's lines,
// so a sync in the export's order never carries the days ahead of it, and
// export.json says the folder holds it; passed over while the folder's copy
// is as long and was last modified when the config was, told by those alone,
// as a day's compressed file is, so nothing in the folder is read; and
// written again once the config changes.
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
	copied := copyOf(t, copiedConfig)
	wantCopy(t, export.ConfigFile(folder), copied, configuredAt)
	wantConfigHeld(t, folder, true)

	writeFile(t, export.ConfigFile(folder), bytes.Repeat([]byte("x"), len(copied)), configuredAt)
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
	wantCopy(t, export.ConfigFile(folder), copyOf(t, changed), configuredAt.Add(time.Hour))
}

// No comment reaches the folder's copy, a token in one included.
func TestTheConfigsCopyHoldsNoComment(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	path := putConfig(t, t.TempDir(), "# test-token: sk-ant-oat01-fake_token-shaped\n"+copiedConfig, configuredAt)

	if _, err := configExporter(state, path).Export(dir); err != nil {
		t.Fatal(err)
	}
	wantCopy(t, export.ConfigFile(folderOf(t, state, dir)), copyOf(t, copiedConfig), configuredAt)
}

// A config that can't be copied, as one that doesn't load, one giving a
// value shaped like a token or one that's gone, fails no export: the folder
// is left holding no copy, as an older config's would read as current, and
// says so, and the export says why it copied none, but of one that's gone,
// which is none to copy.
func TestAConfigThatCantBeCopiedLeavesTheFolderNone(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   error
	}{
		{name: "one that doesn't load", config: "[[account]]\nid = 7\n", want: errors.New("any")},
		{name: "one giving a token", config: `upstream = "https://gateway.example.com/sk-ant-oat01-fake_token-shaped"` + "\n" + copiedConfig, want: config.ErrSecretValue},
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
				if errors.Is(tt.want, config.ErrSecretValue) && !errors.Is(r.Uncopied, tt.want) || r.Uncopied != nil && strings.Contains(r.Uncopied.Error(), "sk-ant-") {
					t.Errorf("Export() said the config wasn't copied as %v, want %v, without the token", r.Uncopied, tt.want)
				}
				wantConfigHeld(t, folder, false)
				wantNoConfig(t, folder)
			})
		}
	}
}

// An export given no config file copies none, removes the copy the folder
// holds, and says nothing of it.
func TestAnExportGivenNoConfigCopiesNone(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	folder := folderOf(t, state, dir)
	writeFile(t, export.ConfigFile(folder), copyOf(t, copiedConfig), configuredAt)

	r := exportInto(t, state, dir)
	if r.Uncopied != nil {
		t.Errorf("Export() = %+v, want it to say nothing of the config", r)
	}
	wantConfigHeld(t, folder, false)
	wantNoConfig(t, folder)
}
