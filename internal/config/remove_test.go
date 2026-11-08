package config_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

func TestDirAndDataDir(t *testing.T) {
	home := func() (string, error) { return "/home/tester", nil }
	tests := []struct {
		name              string
		env               map[string]string
		wantDir, wantData string
	}{
		{name: "home directory", wantDir: "/home/tester/.config/switchboard", wantData: "/home/tester/.local/share/switchboard"},
		{
			name:     "XDG's",
			env:      map[string]string{"XDG_CONFIG_HOME": "/xdg/config", "XDG_DATA_HOME": "/xdg/data"},
			wantDir:  "/xdg/config/switchboard",
			wantData: "/xdg/data/switchboard",
		},
		{
			name:     "the config and themes' own variables don't move the config directory",
			env:      map[string]string{"SWITCHBOARD_CONFIG": "/etc/switchboard.toml", "SWITCHBOARD_THEMES_DIR": "/etc/themes"},
			wantDir:  "/home/tester/.config/switchboard",
			wantData: "/home/tester/.local/share/switchboard",
		},
		{
			name:     "relative XDG directories are ignored",
			env:      map[string]string{"XDG_CONFIG_HOME": "config", "XDG_DATA_HOME": "data"},
			wantDir:  "/home/tester/.config/switchboard",
			wantData: "/home/tester/.local/share/switchboard",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := config.Dir(envFrom(tt.env), home); err != nil || got != tt.wantDir {
				t.Errorf("Dir() = %q, %v; want %q", got, err, tt.wantDir)
			}
			if got, err := config.DataDir(envFrom(tt.env), home); err != nil || got != tt.wantData {
				t.Errorf("DataDir() = %q, %v; want %q", got, err, tt.wantData)
			}
		})
	}
}

func TestPlaces(t *testing.T) {
	home := func() (string, error) { return "/home/tester", nil }
	const dir = "/home/tester/.config/switchboard"
	tests := []struct {
		name string
		env  map[string]string
		file string
		want config.Places
	}{
		{name: "where they usually are: the directory alone", file: dir + "/config.toml", want: config.Places{Dir: dir}},
		{
			name: "a config file and themes elsewhere: each as well",
			env:  map[string]string{"SWITCHBOARD_THEMES_DIR": "/data/themes"},
			file: "/etc/switchboard.toml",
			want: config.Places{Dir: dir, File: "/etc/switchboard.toml", Themes: "/data/themes"},
		},
		{
			name: "themes in the directory, where a variable puts them",
			env:  map[string]string{"SWITCHBOARD_THEMES_DIR": dir + "/mine"},
			file: dir + "/config.toml",
			want: config.Places{Dir: dir},
		},
		{
			name: "a config file beside the directory, by a name it begins",
			file: dir + ".toml",
			want: config.Places{Dir: dir, File: dir + ".toml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := config.PlacesOf(envFrom(tt.env), home, tt.file); err != nil || got != tt.want {
				t.Errorf("PlacesOf() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestPlacesFindsAConfigFileInTheDirectoryThroughALink(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".config", "switchboard")
	writeIn(t, filepath.Join(dir, "config.toml"))
	link := filepath.Join(root, "linked")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"XDG_CONFIG_HOME": filepath.Join(root, ".config")}

	got, err := config.PlacesOf(envFrom(env), func() (string, error) { return root, nil }, filepath.Join(link, "config.toml"))
	if want := (config.Places{Dir: dir}); err != nil || got != want {
		t.Errorf("PlacesOf() = %+v, %v; want %+v", got, err, want)
	}
}

func TestRemoveFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "work.toml")
	writeIn(t, file)

	if got, err := config.RemoveFile(file); err != nil || got != config.Removed {
		t.Errorf("RemoveFile() = %v, %v; want %v", got, err, config.Removed)
	}
	if got, err := config.RemoveFile(file); err != nil || got != config.NotThere {
		t.Errorf("RemoveFile(), again = %v, %v; want %v", got, err, config.NotThere)
	}
	writeIn(t, filepath.Join(dir, "configs", "work.toml"))
	if got, err := config.RemoveFile(filepath.Join(dir, "configs")); err == nil || !strings.Contains(err.Error(), "is a directory, not a config file, so it's left alone") {
		t.Errorf("RemoveFile() of a directory = %v, %v; want it refused", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "configs", "work.toml")); err != nil {
		t.Errorf("what's in the directory: %v, want it kept", err)
	}
}

func TestExportFolderOf(t *testing.T) {
	home := func() (string, error) { return "/home/tester", nil }
	tests := []struct {
		name string
		// text is the config file's, or none where there's no file.
		text    *string
		want    string
		wantErr string
	}{
		{name: "no config"},
		{name: "no export", text: new("[[account]]\nid = \"work\"\n")},
		{name: "a folder in the home", text: new("[export]\ndir = \"~/Dropbox/Switchboard\"\n"), want: "/home/tester/Dropbox/Switchboard"},
		{name: "a folder named whole", text: new("[export]\ndir = \"/Volumes/Backup/switchboard/\"\n"), want: "/Volumes/Backup/switchboard"},
		{
			name: "a config otherwise invalid: read all the same",
			text: new(invalidOtherwise),
			want: "/home/tester/Dropbox/Switchboard",
		},
		{name: "not TOML", text: new("[export\n"), wantErr: "parse config "},
		{name: "a relative folder", text: new("[export]\ndir = \"exports\"\n"), wantErr: "must be an absolute path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if tt.text != nil {
				if err := os.WriteFile(path, []byte(*tt.text), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			got, err := config.ExportFolderOf(path, home)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ExportFolderOf() = %q, %v; want an error saying %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("ExportFolderOf() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

// invalidOtherwise is a config Load refuses, for its account's token_env and
// a week_starts of the wrong type, that names an export's folder.
const invalidOtherwise = "week_starts = 3\n[[account]]\nid = \"work\"\ntoken_env = \"CLAUDE_TOKEN_WORK\"\n[export]\ndir = \"~/Dropbox/Switchboard\"\n"

func TestRemoveAll(t *testing.T) {
	tests := []struct {
		name string
		// keep is the folder kept, from the directory's parent, "" for none.
		keep string
		want config.Removal
		// wantLeft is what's left under the directory's parent.
		wantLeft []string
	}{
		{name: "nothing kept: removed whole", want: config.Removed},
		{name: "a folder kept elsewhere: removed whole", keep: "elsewhere", want: config.Removed, wantLeft: []string{"elsewhere"}},
		{name: "a folder kept beside it, by a name it begins: removed whole", keep: "switchboard-exports", want: config.Removed, wantLeft: []string{"switchboard-exports"}},
		{
			name:     "a folder kept in it: that kept, with the directories on the way there, and nothing else",
			keep:     "switchboard/ledger/exports",
			want:     config.KeptFor,
			wantLeft: []string{"switchboard", "switchboard/ledger", "switchboard/ledger/exports", "switchboard/ledger/exports/mac-1", "switchboard/ledger/exports/mac-1/export.json"},
		},
		{
			name: "the directory itself kept: left whole",
			keep: "switchboard",
			want: config.KeptFor,
			wantLeft: []string{
				"switchboard", "switchboard/ledger", "switchboard/ledger/day-2026-11-08.json", "switchboard/ledger/exports",
				"switchboard/ledger/exports/mac-1", "switchboard/ledger/exports/mac-1/export.json", "switchboard/logs", "switchboard/logs/cli.log",
				"switchboard/state.json", "switchboard/tokens", "switchboard/tokens/work",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "switchboard")
			for _, file := range []string{"state.json", "tokens/work", "logs/cli.log", "ledger/day-2026-11-08.json", "ledger/exports/mac-1/export.json"} {
				writeIn(t, filepath.Join(dir, file))
			}
			keep := ""
			if tt.keep != "" {
				keep = filepath.Join(parent, tt.keep)
				if err := os.MkdirAll(keep, 0o700); err != nil {
					t.Fatal(err)
				}
			}

			got, err := config.RemoveAll(dir, keep)
			if err != nil || got != tt.want {
				t.Fatalf("RemoveAll() = %v, %v; want %v", got, err, tt.want)
			}
			if left := tree(t, parent); !slices.Equal(left, tt.wantLeft) {
				t.Errorf("RemoveAll() left %q, want %q", left, tt.wantLeft)
			}
			if got, err := config.RemoveAll(filepath.Join(parent, "gone"), keep); err != nil || got != config.NotThere {
				t.Errorf("RemoveAll() of a directory that isn't there = %v, %v; want %v", got, err, config.NotThere)
			}
		})
	}
}

func TestRemoveAllKeepsAFolderNamedThroughALink(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "switchboard")
	writeIn(t, filepath.Join(dir, "state.json"))
	writeIn(t, filepath.Join(dir, "exports", "mac-1", "export.json"))
	// The export's folder, named through a link to the directory's parent.
	link := filepath.Join(t.TempDir(), "state")
	if err := os.Symlink(parent, link); err != nil {
		t.Fatal(err)
	}

	got, err := config.RemoveAll(dir, filepath.Join(link, "switchboard", "exports"))
	if err != nil || got != config.KeptFor {
		t.Fatalf("RemoveAll() = %v, %v; want %v", got, err, config.KeptFor)
	}
	want := []string{"switchboard", "switchboard/exports", "switchboard/exports/mac-1", "switchboard/exports/mac-1/export.json"}
	if left := tree(t, parent); !slices.Equal(left, want) {
		t.Errorf("RemoveAll() left %q, want %q", left, want)
	}
}

func TestRemoveAllGoesOnPastWhatItCantRemove(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can remove from any directory")
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "switchboard")
	writeIn(t, filepath.Join(dir, "locked", "kept"))
	writeIn(t, filepath.Join(dir, "state.json"))
	writeIn(t, filepath.Join(dir, "exports", "export.json"))
	locked := filepath.Join(dir, "locked")
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	_, err := config.RemoveAll(dir, filepath.Join(dir, "exports"))
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		t.Errorf("RemoveAll() error = %v, want it to fail, for want of permission", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("state.json: %v, want it removed all the same", err)
	}
}

// writeIn writes an empty file at path, making its directory.
func writeIn(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// tree lists everything under root, from root, in order.
func tree(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil || path == root {
			return err
		}
		rel, err := filepath.Rel(root, path)
		paths = append(paths, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}
