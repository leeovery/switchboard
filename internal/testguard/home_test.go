package testguard

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestChangesToTheRealConfig(t *testing.T) {
	config := filepath.Join(configDir, "config.toml")
	tests := []struct {
		name string
		// files are what the home holds before, by path from it.
		files []string
		// change changes the home at home.
		change func(t *testing.T, home string)
		want   []string
	}{
		{
			name:   "nothing",
			files:  []string{config},
			change: func(*testing.T, string) {},
		},
		{
			name: "written where there was none",
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, config), "listen = \"127.0.0.1:4747\"\n")
			},
			want: []string{"the real ~/.config/switchboard was created", "the real ~/.config/switchboard/config.toml was created"},
		},
		{
			name:  "overwritten in place",
			files: []string{config},
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, config), "listen = \"127.0.0.1:4748\"\n")
			},
			want: []string{"the real ~/.config/switchboard/config.toml was modified"},
		},
		{
			name:  "touched",
			files: []string{config},
			change: func(t *testing.T, home string) {
				now := time.Now()
				if err := os.Chtimes(filepath.Join(home, config), now, now); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"the real ~/.config/switchboard/config.toml was modified"},
		},
		{
			name:  "removed",
			files: []string{config},
			change: func(t *testing.T, home string) {
				if err := os.Remove(filepath.Join(home, config)); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"the real ~/.config/switchboard was modified", "the real ~/.config/switchboard/config.toml was removed"},
		},
		{
			name: "only elsewhere",
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, ".config", "other", "config.toml"), "")
				write(t, filepath.Join(home, "switchboard"), "")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			for _, file := range tt.files {
				write(t, filepath.Join(home, file), "before\n")
			}
			backdate(t, home)
			watched := watchHome(home)

			tt.change(t, home)

			if got := watched.changes(); !slices.Equal(got, tt.want) {
				t.Errorf("changes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChangesToTheRealConfigThroughLinks(t *testing.T) {
	tests := []struct {
		name string
		// link links the config, kept in dotfiles, into home.
		link func(t *testing.T, home, dotfiles string)
	}{
		{
			name: "the directory linked in",
			link: func(t *testing.T, home, dotfiles string) {
				symlink(t, filepath.Join(dotfiles, "switchboard"), filepath.Join(home, configDir))
			},
		},
		{
			name: "the file linked in",
			link: func(t *testing.T, home, dotfiles string) {
				symlink(t, filepath.Join(dotfiles, "switchboard", "config.toml"), filepath.Join(home, configDir, "config.toml"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, dotfiles := t.TempDir(), t.TempDir()
			config := filepath.Join(dotfiles, "switchboard", "config.toml")
			write(t, config, "listen = \"127.0.0.1:4747\"\n")
			tt.link(t, home, dotfiles)
			backdate(t, dotfiles)
			watched := watchHome(home)

			write(t, config, "listen = \"127.0.0.1:4748\"\n")

			want := []string{"the real ~/.config/switchboard/config.toml was modified"}
			if got := watched.changes(); !slices.Equal(got, want) {
				t.Errorf("changes() = %q, want %q", got, want)
			}
		})
	}
}

func TestTheRealStateDirectoryAppearing(t *testing.T) {
	tests := []struct {
		name string
		// before lays out the state directory, when it's there at the start.
		before func(t *testing.T, state string)
		// during changes the state directory as the tests run.
		during func(t *testing.T, state string)
		want   []string
	}{
		{
			name:   "appeared during the run",
			during: writeState,
			want:   []string{"the real ~/.local/state/switchboard appeared"},
		},
		{
			name:   "a live router rewrote state.json and logged during the run",
			before: writeState,
			during: func(t *testing.T, state string) {
				write(t, filepath.Join(state, "state.json.tmp"), "{\"version\": 1, \"sessions\": []}\n")
				if err := os.Rename(filepath.Join(state, "state.json.tmp"), filepath.Join(state, "state.json")); err != nil {
					t.Fatal(err)
				}
				write(t, filepath.Join(state, "logs", "router.log"), "level=INFO msg=routed\nlevel=INFO msg=routed\n")
			},
		},
		{
			name:   "there all along",
			before: writeState,
			during: func(*testing.T, string) {},
		},
		{
			name:   "never there",
			during: func(*testing.T, string) {},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			state := filepath.Join(home, stateDir)
			if tt.before != nil {
				tt.before(t, state)
			}
			backdate(t, home)
			watched := watchHome(home)

			tt.during(t, state)

			if got := watched.changes(); !slices.Equal(got, tt.want) {
				t.Errorf("changes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNoHomeWatchesNothing(t *testing.T) {
	if got := watchHome("").changes(); got != nil {
		t.Errorf("changes() with no home = %q, want none", got)
	}
}

// writeState lays out a state directory at state as a router leaves it.
func writeState(t *testing.T, state string) {
	t.Helper()
	write(t, filepath.Join(state, "state.json"), "{\"version\": 1, \"sessions\": []}\n")
	write(t, filepath.Join(state, "logs", "router.log"), "level=INFO msg=routed\n")
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

// backdate sets everything under dir as last modified an hour ago, so a change
// in the same tick as it was written still moves its time.
func backdate(t *testing.T, dir string) {
	t.Helper()
	then := time.Now().Add(-time.Hour)
	err := filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(path, then, then)
	})
	if err != nil {
		t.Fatal(err)
	}
}
