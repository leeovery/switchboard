package testguard

import (
	"fmt"
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
			watched := watchReal(home, noEnv)

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
			watched := watchReal(home, noEnv)

			write(t, config, "listen = \"127.0.0.1:4748\"\n")

			want := []string{"the real ~/.config/switchboard/config.toml was modified"}
			if got := watched.changes(); !slices.Equal(got, want) {
				t.Errorf("changes() = %q, want %q", got, want)
			}
		})
	}
}

func TestChangesToTheRealLaunchAgents(t *testing.T) {
	agent := filepath.Join(launchAgentsDir, "io.github.leeovery.switchboard.plist")
	other := filepath.Join(launchAgentsDir, "com.example.other.plist")
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
			files:  []string{agent, other},
			change: func(*testing.T, string) {},
		},
		{
			name:  "installed where there was none",
			files: []string{other},
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, agent), "<plist/>\n")
			},
			want: []string{"the real ~/Library/LaunchAgents/io.github.leeovery.switchboard.plist was created"},
		},
		{
			name:  "rewritten",
			files: []string{agent},
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, agent), "<plist version=\"1.0\"/>\n")
			},
			want: []string{"the real ~/Library/LaunchAgents/io.github.leeovery.switchboard.plist was modified"},
		},
		{
			name:  "removed",
			files: []string{agent},
			change: func(t *testing.T, home string) {
				if err := os.Remove(filepath.Join(home, agent)); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"the real ~/Library/LaunchAgents/io.github.leeovery.switchboard.plist was removed"},
		},
		{
			name: "any file naming switchboard, in any case",
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, launchAgentsDir, "com.example.Switchboard-helper.plist"), "<plist/>\n")
			},
			want: []string{"the real ~/Library/LaunchAgents/com.example.Switchboard-helper.plist was created"},
		},
		{
			name:  "another program's",
			files: []string{agent, other},
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, other), "<plist version=\"1.0\"/>\n")
				write(t, filepath.Join(home, launchAgentsDir, "com.example.new.plist"), "<plist/>\n")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			for _, file := range tt.files {
				write(t, filepath.Join(home, file), "<plist/>\n")
			}
			backdate(t, home)
			watched := watchReal(home, noEnv)

			tt.change(t, home)

			if got := watched.changes(); !slices.Equal(got, tt.want) {
				t.Errorf("changes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChangesToTheRealLaunchAgentThroughALink(t *testing.T) {
	home, dotfiles := t.TempDir(), t.TempDir()
	kept := filepath.Join(dotfiles, "io.github.leeovery.switchboard.plist")
	write(t, kept, "<plist/>\n")
	symlink(t, kept, filepath.Join(home, launchAgentsDir, "io.github.leeovery.switchboard.plist"))
	backdate(t, dotfiles)
	watched := watchReal(home, noEnv)

	write(t, kept, "<plist version=\"1.0\"/>\n")

	want := []string{"the real ~/Library/LaunchAgents/io.github.leeovery.switchboard.plist was modified"}
	if got := watched.changes(); !slices.Equal(got, want) {
		t.Errorf("changes() = %q, want %q", got, want)
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
			watched := watchReal(home, noEnv)

			tt.during(t, state)

			if got := watched.changes(); !slices.Equal(got, tt.want) {
				t.Errorf("changes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNoHomeWatchesNothing(t *testing.T) {
	if got := watchReal("", noEnv).changes(); got != nil {
		t.Errorf("changes() with no home = %q, want none", got)
	}
}

func TestChangesWhereTheEnvironmentPutsTheConfigAndState(t *testing.T) {
	tests := []struct {
		name string
		// before lays out elsewhere, where the environment puts the config
		// and state, before the tests begin.
		before func(t *testing.T, elsewhere string)
		// during changes it as the tests run.
		during func(t *testing.T, elsewhere string)
		// want are the changes reported, each with %[1]s standing for
		// elsewhere.
		want []string
	}{
		{
			name:   "the config file SWITCHBOARD_CONFIG names, overwritten",
			before: func(t *testing.T, elsewhere string) { write(t, filepath.Join(elsewhere, "work.toml"), "before\n") },
			during: func(t *testing.T, elsewhere string) {
				write(t, filepath.Join(elsewhere, "work.toml"), "after, and longer\n")
			},
			want: []string{"the real %[1]s/work.toml was modified"},
		},
		{
			name:   "the config file SWITCHBOARD_CONFIG names, written where there was none",
			during: func(t *testing.T, elsewhere string) { write(t, filepath.Join(elsewhere, "work.toml"), "after\n") },
			want:   []string{"the real %[1]s/work.toml was created"},
		},
		{
			name: "a config written where XDG_CONFIG_HOME puts it",
			during: func(t *testing.T, elsewhere string) {
				write(t, filepath.Join(elsewhere, "config", "switchboard", "config.toml"), "after\n")
			},
			want: []string{"the real %[1]s/config/switchboard was created", "the real %[1]s/config/switchboard/config.toml was created"},
		},
		{
			name:   "a state directory appearing where XDG_STATE_HOME puts it",
			during: func(t *testing.T, elsewhere string) { writeState(t, filepath.Join(elsewhere, "state", "switchboard")) },
			want:   []string{"the real %[1]s/state/switchboard appeared"},
		},
		{
			name:   "a live router writing the state there all along",
			before: func(t *testing.T, elsewhere string) { writeState(t, filepath.Join(elsewhere, "state", "switchboard")) },
			during: func(t *testing.T, elsewhere string) {
				write(t, filepath.Join(elsewhere, "state", "switchboard", "logs", "router.log"), "level=INFO msg=routed\nlevel=INFO msg=routed\n")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elsewhere := t.TempDir()
			if tt.before != nil {
				tt.before(t, elsewhere)
			}
			backdate(t, elsewhere)
			env := map[string]string{
				"SWITCHBOARD_CONFIG": filepath.Join(elsewhere, "work.toml"),
				"XDG_CONFIG_HOME":    filepath.Join(elsewhere, "config"),
				"XDG_STATE_HOME":     filepath.Join(elsewhere, "state"),
			}
			watched := watchReal(t.TempDir(), func(key string) string { return env[key] })

			tt.during(t, elsewhere)

			var want []string
			for _, line := range tt.want {
				want = append(want, fmt.Sprintf(line, elsewhere))
			}
			if got := watched.changes(); !slices.Equal(got, want) {
				t.Errorf("changes() = %q, want %q", got, want)
			}
		})
	}
}

func TestTheConfigIsWatchedWhereItsLinkLedAsTheTestsBegan(t *testing.T) {
	dotfiles := t.TempDir()
	config := filepath.Join(dotfiles, "work.toml")
	write(t, config, "before\n")
	link := filepath.Join(t.TempDir(), "work.toml")
	symlink(t, config, link)
	backdate(t, dotfiles)
	watched := watchReal("", func(key string) string { return map[string]string{"SWITCHBOARD_CONFIG": link}[key] })

	// The link moved to lead elsewhere, and the config it led to changed.
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	symlink(t, filepath.Join(t.TempDir(), "other.toml"), link)
	write(t, config, "after, and longer\n")

	if got, want := watched.changes(), []string{"the real " + link + " was modified"}; !slices.Equal(got, want) {
		t.Errorf("changes() = %q, want %q", got, want)
	}
}

func TestRelativePlacesAreNoneTestguardCanKnow(t *testing.T) {
	env := map[string]string{"SWITCHBOARD_CONFIG": "work.toml", "XDG_CONFIG_HOME": "config", "XDG_STATE_HOME": "state"}
	if got := watchReal("", func(key string) string { return env[key] }); len(got) > 0 {
		t.Errorf("watchReal() watches %d places, want none: a relative one leads wherever switchboard runs", len(got))
	}
}

// noEnv is an environment without a variable set.
func noEnv(string) string { return "" }

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
