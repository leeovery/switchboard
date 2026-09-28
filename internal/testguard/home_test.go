package testguard

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestChangesToTheRealHome(t *testing.T) {
	config := filepath.Join(".config", "switchboard", "config.toml")
	cliLog := filepath.Join(".local", "state", "switchboard", "logs", "cli.log")
	state := filepath.Join(".local", "state", "switchboard", "state.json")
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
			files:  []string{config, cliLog, state},
			change: func(*testing.T, string) {},
		},
		{
			name: "the config written where there was none",
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, config), "listen = \"127.0.0.1:4747\"\n")
			},
			want: []string{"the real ~/.config/switchboard was created", "the real ~/.config/switchboard/config.toml was created"},
		},
		{
			name:  "the config overwritten in place",
			files: []string{config},
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, config), "listen = \"127.0.0.1:4748\"\n")
			},
			want: []string{"the real ~/.config/switchboard/config.toml was modified"},
		},
		{
			name:  "a log appended to",
			files: []string{cliLog},
			change: func(t *testing.T, home string) {
				f, err := os.OpenFile(filepath.Join(home, cliLog), os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = f.Close() }()
				if _, err := f.WriteString("level=INFO msg=start\n"); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"the real ~/.local/state/switchboard/logs/cli.log was modified"},
		},
		{
			name:  "the state file touched",
			files: []string{state},
			change: func(t *testing.T, home string) {
				now := time.Now()
				if err := os.Chtimes(filepath.Join(home, state), now, now); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"the real ~/.local/state/switchboard/state.json was modified"},
		},
		{
			name:  "the state file removed",
			files: []string{state},
			change: func(t *testing.T, home string) {
				if err := os.Remove(filepath.Join(home, state)); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"the real ~/.local/state/switchboard was modified", "the real ~/.local/state/switchboard/state.json was removed"},
		},
		{
			name: "only outside switchboard's directories",
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, ".config", "other", "config.toml"), "")
				write(t, filepath.Join(home, ".local", "state", "other.log"), "")
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
			before := take(home)

			tt.change(t, home)

			if got := changes(before, take(home)); !slices.Equal(got, tt.want) {
				t.Errorf("changes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChangesThroughLinks(t *testing.T) {
	tests := []struct {
		name string
		// link links the config, kept in dotfiles, into home.
		link func(t *testing.T, home, dotfiles string)
	}{
		{
			name: "the config directory linked in",
			link: func(t *testing.T, home, dotfiles string) {
				symlink(t, filepath.Join(dotfiles, "switchboard"), filepath.Join(home, ".config", "switchboard"))
			},
		},
		{
			name: "the config file linked in",
			link: func(t *testing.T, home, dotfiles string) {
				symlink(t, filepath.Join(dotfiles, "switchboard", "config.toml"), filepath.Join(home, ".config", "switchboard", "config.toml"))
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
			before := take(home)

			write(t, config, "listen = \"127.0.0.1:4748\"\n")

			want := []string{"the real ~/.config/switchboard/config.toml was modified"}
			if got := changes(before, take(home)); !slices.Equal(got, want) {
				t.Errorf("changes() = %q, want %q", got, want)
			}
		})
	}
}

func TestNoHomeHoldsNothing(t *testing.T) {
	if s := take(""); s != nil {
		t.Errorf("take(\"\") = %v, want nothing", s)
	}
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
