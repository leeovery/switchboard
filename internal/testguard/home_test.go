package testguard

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestChangesToTheRealThemes(t *testing.T) {
	theme := filepath.Join(configDir, themesDir, "lake.theme")
	const themes = "the real ~/.config/switchboard/themes"
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
			files:  []string{theme},
			change: func(*testing.T, string) {},
		},
		{
			name: "written where there was none",
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, theme), "canvas = #2E3440\n")
			},
			want: []string{"the real ~/.config/switchboard was modified", themes + " was created", themes + "/lake.theme was created"},
		},
		{
			name:  "rewritten",
			files: []string{theme},
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, theme), "canvas = #102030\nborder = #4C566A\n")
			},
			want: []string{themes + "/lake.theme was modified"},
		},
		{
			name:  "removed",
			files: []string{theme},
			change: func(t *testing.T, home string) {
				if err := os.Remove(filepath.Join(home, theme)); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{themes + " was modified", themes + "/lake.theme was removed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			write(t, filepath.Join(home, configDir, "config.toml"), "listen = \"127.0.0.1:4747\"\n")
			for _, file := range tt.files {
				write(t, filepath.Join(home, file), "canvas = #2E3440\n")
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

func TestChangesToTheRealThemesThroughLinks(t *testing.T) {
	tests := []struct {
		name string
		// link links the themes, kept in dotfiles, into home.
		link func(t *testing.T, home, dotfiles string)
	}{
		{
			name: "the themes directory linked in",
			link: func(t *testing.T, home, dotfiles string) {
				symlink(t, filepath.Join(dotfiles, "themes"), filepath.Join(home, configDir, themesDir))
			},
		},
		{
			name: "a theme linked in",
			link: func(t *testing.T, home, dotfiles string) {
				symlink(t, filepath.Join(dotfiles, "themes", "lake.theme"), filepath.Join(home, configDir, themesDir, "lake.theme"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, dotfiles := t.TempDir(), t.TempDir()
			theme := filepath.Join(dotfiles, "themes", "lake.theme")
			write(t, theme, "canvas = #2E3440\n")
			tt.link(t, home, dotfiles)
			backdate(t, dotfiles)
			watched := watchReal(home, noEnv)

			write(t, theme, "canvas = #102030\nborder = #4C566A\n")

			want := []string{"the real ~/.config/switchboard/themes/lake.theme was modified"}
			if got := watched.changes(); !slices.Equal(got, want) {
				t.Errorf("changes() = %q, want %q", got, want)
			}
		})
	}
}

func TestALiveDashboardWritingItsPreferencesIsNoTestsChange(t *testing.T) {
	tests := []struct {
		name string
		// before lays out the state directory, at state, and what else the
		// home holds, at home, before the tests begin.
		before func(t *testing.T, home, state string)
		// during changes them as the tests run.
		during func(t *testing.T, home, state string)
	}{
		{
			name:   "written where there was none",
			before: func(t *testing.T, _, state string) { writeState(t, state) },
			during: func(t *testing.T, _, state string) { writePrefs(t, state) },
		},
		{
			name:   "rewritten",
			before: func(t *testing.T, _, state string) { writePrefs(t, state) },
			during: func(t *testing.T, _, state string) {
				write(t, filepath.Join(state, prefsFile), "{\n  \"theme_dark\": \"amber\"\n}\n")
			},
		},
		{
			name:   "removed",
			before: func(t *testing.T, _, state string) { writePrefs(t, state) },
			during: func(t *testing.T, _, state string) {
				if err := os.Remove(filepath.Join(state, prefsFile)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "rewritten where its link leads",
			before: func(t *testing.T, home, state string) {
				writeState(t, state)
				write(t, filepath.Join(home, "dotfiles", prefsFile), "{\n  \"theme\": \"nord\"\n}\n")
				symlink(t, filepath.Join(home, "dotfiles", prefsFile), filepath.Join(state, prefsFile))
			},
			during: func(t *testing.T, home, _ string) {
				write(t, filepath.Join(home, "dotfiles", prefsFile), "{\n  \"theme\": \"amber\"\n}\n")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			state := filepath.Join(home, stateDir)
			tt.before(t, home, state)
			backdate(t, home)
			watched := watchReal(home, noEnv)

			tt.during(t, home, state)

			if got := watched.changes(); len(got) > 0 {
				t.Errorf("changes() = %q, want none: a live dashboard writes it as it's used, and the sandbox denies a test any write there", got)
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

func TestChangesToTheRealSkill(t *testing.T) {
	skill := filepath.Join(claudeDir, skillDir, "SKILL.md")
	other := filepath.Join(claudeDir, "skills", "other", "SKILL.md")
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
			files:  []string{skill, other},
			change: func(*testing.T, string) {},
		},
		{
			name:  "installed where there was none",
			files: []string{other},
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, skill), "---\nname: switchboard\n---\n")
			},
			want: []string{"the real ~/.claude/skills/switchboard was created", "the real ~/.claude/skills/switchboard/SKILL.md was created"},
		},
		{
			name:  "rewritten",
			files: []string{skill},
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, skill), "---\nname: switchboard\ndescription: rewritten\n---\n")
			},
			want: []string{"the real ~/.claude/skills/switchboard/SKILL.md was modified"},
		},
		{
			name:  "removed",
			files: []string{skill},
			change: func(t *testing.T, home string) {
				if err := os.Remove(filepath.Join(home, skill)); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"the real ~/.claude/skills/switchboard was modified", "the real ~/.claude/skills/switchboard/SKILL.md was removed"},
		},
		{
			name:  "Claude Code's own files and other skills",
			files: []string{skill, other},
			change: func(t *testing.T, home string) {
				write(t, filepath.Join(home, other), "---\nname: other\ndescription: rewritten\n---\n")
				write(t, filepath.Join(home, claudeDir, "skills", "new", "SKILL.md"), "---\nname: new\n---\n")
				write(t, filepath.Join(home, claudeDir, "settings.json"), "{}\n")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			for _, file := range tt.files {
				write(t, filepath.Join(home, file), "---\nname: before\n---\n")
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

func TestChangesToTheRealSkillThroughLinks(t *testing.T) {
	tests := []struct {
		name string
		// link links what's kept in dotfiles into home.
		link func(t *testing.T, home, dotfiles string)
	}{
		{
			name: "Claude Code's config directory linked in",
			link: func(t *testing.T, home, dotfiles string) {
				symlink(t, filepath.Join(dotfiles, "claude"), filepath.Join(home, claudeDir))
			},
		},
		{
			name: "the skills linked in",
			link: func(t *testing.T, home, dotfiles string) {
				symlink(t, filepath.Join(dotfiles, "claude", "skills"), filepath.Join(home, claudeDir, "skills"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, dotfiles := t.TempDir(), t.TempDir()
			skill := filepath.Join(dotfiles, "claude", skillDir, "SKILL.md")
			write(t, skill, "---\nname: switchboard\n---\n")
			tt.link(t, home, dotfiles)
			backdate(t, dotfiles)
			watched := watchReal(home, noEnv)

			write(t, skill, "---\nname: switchboard\ndescription: rewritten\n---\n")

			want := []string{"the real ~/.claude/skills/switchboard/SKILL.md was modified"}
			if got := watched.changes(); !slices.Equal(got, want) {
				t.Errorf("changes() = %q, want %q", got, want)
			}
		})
	}
}

func TestChangesToTheRealBinDirectory(t *testing.T) {
	link := filepath.Join(binDir, "claude")
	tests := []struct {
		name string
		// before lays out the home, at home, before the tests begin.
		before func(t *testing.T, home string)
		// during changes it as the tests run.
		during func(t *testing.T, home string)
		want   []string
	}{
		{
			name: "nothing",
			before: func(t *testing.T, home string) {
				symlink(t, filepath.Join(home, "brew", "switchboard"), filepath.Join(home, link))
			},
			during: func(*testing.T, string) {},
		},
		{
			name:   "the claude link made where there was none",
			during: func(t *testing.T, home string) { symlink(t, filepath.Join(home, "gone"), filepath.Join(home, link)) },
			want:   []string{"the real ~/.local/share/switchboard/bin was created", "the real ~/.local/share/switchboard/bin/claude was created"},
		},
		{
			name: "the claude link made to lead elsewhere",
			before: func(t *testing.T, home string) {
				symlink(t, filepath.Join(home, "brew", "switchboard"), filepath.Join(home, link))
			},
			during: func(t *testing.T, home string) {
				if err := os.Remove(filepath.Join(home, link)); err != nil {
					t.Fatal(err)
				}
				symlink(t, filepath.Join(home, "go", "switchboard"), filepath.Join(home, link))
			},
			want: []string{"the real ~/.local/share/switchboard/bin was modified", "the real ~/.local/share/switchboard/bin/claude was modified"},
		},
		{
			name: "what the claude link leads to upgraded, which is no change of the link's",
			before: func(t *testing.T, home string) {
				write(t, filepath.Join(home, "brew", "switchboard"), "#!/bin/sh\n")
				symlink(t, filepath.Join(home, "brew", "switchboard"), filepath.Join(home, link))
			},
			during: func(t *testing.T, home string) {
				write(t, filepath.Join(home, "brew", "switchboard"), "#!/bin/sh\nexit 0\n")
			},
		},
		{
			name: "other programs' data beside it",
			during: func(t *testing.T, home string) {
				write(t, filepath.Join(home, ".local", "share", "other", "bin", "claude"), "#!/bin/sh\n")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.before != nil {
				tt.before(t, home)
			}
			backdate(t, home)
			watched := watchReal(home, noEnv)

			tt.during(t, home)

			if got := watched.changes(); !slices.Equal(got, tt.want) {
				t.Errorf("changes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestALinkIsNotedAsALinkWithWhereItLeads(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "claude")
	symlink(t, filepath.Join(dir, "gone"), link)

	noted, ok := asItIs(link)
	if !ok || noted.kind != fs.ModeSymlink || noted.target != filepath.Join(dir, "gone") {
		t.Errorf("asItIs() = %+v, %v; want the link noted as a link, leading to %s", noted, ok, filepath.Join(dir, "gone"))
	}
}

func TestChangesToClaudeOnTheRealPath(t *testing.T) {
	tests := []struct {
		name string
		// before lays out the directories on PATH, first and second, before
		// the tests begin.
		before func(t *testing.T, first, second string)
		// during changes them as the tests run.
		during func(t *testing.T, first, second string)
		// want are the changes reported, %[1]s standing for first and %[2]s
		// for second.
		want []string
	}{
		{
			name: "nothing",
			before: func(t *testing.T, first, second string) {
				write(t, filepath.Join(second, "claude"), "#!/bin/sh\n")
				symlink(t, filepath.Join(second, "switchboard"), filepath.Join(first, "claude"))
			},
			during: func(*testing.T, string, string) {},
		},
		{
			name:   "a link made where there was none",
			before: func(t *testing.T, _, second string) { write(t, filepath.Join(second, "claude"), "#!/bin/sh\n") },
			during: func(t *testing.T, first, second string) {
				symlink(t, filepath.Join(second, "claude"), filepath.Join(first, "claude"))
			},
			want: []string{"the real %[1]s/claude was created"},
		},
		{
			name: "a link made that leads nowhere",
			during: func(t *testing.T, first, _ string) {
				symlink(t, filepath.Join(first, "gone"), filepath.Join(first, "claude"))
			},
			want: []string{"the real %[1]s/claude was created"},
		},
		{
			name: "a link made to lead elsewhere",
			before: func(t *testing.T, first, _ string) {
				symlink(t, filepath.Join(first, "switchboard"), filepath.Join(first, "claude"))
			},
			during: func(t *testing.T, first, _ string) {
				if err := os.Remove(filepath.Join(first, "claude")); err != nil {
					t.Fatal(err)
				}
				symlink(t, filepath.Join(first, "sb"), filepath.Join(first, "claude"))
			},
			want: []string{"the real %[1]s/claude was modified"},
		},
		{
			name: "a link to Claude Code made to lead to switchboard, the link watched rather than where it led",
			before: func(t *testing.T, first, second string) {
				write(t, filepath.Join(second, "versions", "2.1.300"), "#!/bin/sh\n")
				write(t, filepath.Join(first, "switchboard"), "#!/bin/sh\n")
				symlink(t, filepath.Join(second, "versions", "2.1.300"), filepath.Join(first, "claude"))
			},
			during: func(t *testing.T, first, _ string) {
				if err := os.Remove(filepath.Join(first, "claude")); err != nil {
					t.Fatal(err)
				}
				symlink(t, filepath.Join(first, "switchboard"), filepath.Join(first, "claude"))
			},
			want: []string{"the real %[1]s/claude was modified"},
		},
		{
			name:   "a link put in claude's place",
			before: func(t *testing.T, _, second string) { write(t, filepath.Join(second, "claude"), "#!/bin/sh\n") },
			during: func(t *testing.T, first, second string) {
				if err := os.Remove(filepath.Join(second, "claude")); err != nil {
					t.Fatal(err)
				}
				symlink(t, filepath.Join(first, "switchboard"), filepath.Join(second, "claude"))
			},
			want: []string{"the real %[2]s/claude was modified"},
		},
		{
			name: "a link removed",
			before: func(t *testing.T, first, _ string) {
				symlink(t, filepath.Join(first, "switchboard"), filepath.Join(first, "claude"))
			},
			during: func(t *testing.T, first, _ string) {
				if err := os.Remove(filepath.Join(first, "claude")); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"the real %[1]s/claude was removed"},
		},
		{
			name: "other programs on PATH, made and changed",
			before: func(t *testing.T, _, second string) {
				write(t, filepath.Join(second, "claude"), "#!/bin/sh\n")
				write(t, filepath.Join(second, "tmux"), "#!/bin/sh\n")
			},
			during: func(t *testing.T, first, second string) {
				write(t, filepath.Join(first, "switchboard"), "#!/bin/sh\n")
				write(t, filepath.Join(second, "tmux"), "#!/bin/sh\nexit 0\n")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
			for _, dir := range []string{first, second} {
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if tt.before != nil {
				tt.before(t, first, second)
			}
			backdate(t, root)
			// A relative directory, which names none testguard can know, and
			// first again, which is watched once.
			path := strings.Join([]string{first, "bin", second, first}, string(os.PathListSeparator))
			watched := watchReal("", func(key string) string { return map[string]string{"PATH": path}[key] })

			tt.during(t, first, second)

			var want []string
			for _, line := range tt.want {
				want = append(want, fmt.Sprintf(line, first, second))
			}
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
			watched := watchReal(home, noEnv)

			tt.during(t, state)

			if got := watched.changes(); !slices.Equal(got, tt.want) {
				t.Errorf("changes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChangesToTheRealTokenFiles(t *testing.T) {
	const tokens = "the real ~/.local/state/switchboard/tokens"
	tests := []struct {
		name string
		// before lays out the state directory, at state, before the tests
		// begin.
		before func(t *testing.T, state string)
		// during changes it as the tests run.
		during func(t *testing.T, state string)
		want   []string
	}{
		{
			name:   "nothing",
			before: writeTokens,
			during: func(*testing.T, string) {},
		},
		{
			name:   "one written where there was none",
			before: writeState,
			during: func(t *testing.T, state string) {
				write(t, filepath.Join(state, tokensDir, "work"), "test-token-work\n")
			},
			want: []string{tokens + " was created", tokens + "/work was created"},
		},
		{
			name:   "one rewritten",
			before: writeTokens,
			during: func(t *testing.T, state string) {
				write(t, filepath.Join(state, tokensDir, "work"), "test-token-other\n")
			},
			want: []string{tokens + "/work was modified"},
		},
		{
			name:   "one removed",
			before: writeTokens,
			during: func(t *testing.T, state string) {
				if err := os.Remove(filepath.Join(state, tokensDir, "work")); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{tokens + " was modified", tokens + "/work was removed"},
		},
		{
			name:   "a live router writing the rest of its state",
			before: writeTokens,
			during: func(t *testing.T, state string) {
				write(t, filepath.Join(state, "logs", "router.log"), "level=INFO msg=routed\nlevel=INFO msg=routed\n")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			state := filepath.Join(home, stateDir)
			tt.before(t, state)
			backdate(t, home)
			watched := watchReal(home, noEnv)

			tt.during(t, state)

			if got := watched.changes(); !slices.Equal(got, tt.want) {
				t.Errorf("changes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChangesToARealTokenFileThroughALink(t *testing.T) {
	home, secrets := t.TempDir(), t.TempDir()
	kept := filepath.Join(secrets, "work")
	write(t, kept, "test-token-work\n")
	writeState(t, filepath.Join(home, stateDir))
	symlink(t, kept, filepath.Join(home, stateDir, tokensDir, "work"))
	backdate(t, secrets)
	watched := watchReal(home, noEnv)

	write(t, kept, "test-token-other\n")

	want := []string{"the real ~/.local/state/switchboard/tokens/work was modified"}
	if got := watched.changes(); !slices.Equal(got, want) {
		t.Errorf("changes() = %q, want %q", got, want)
	}
}

func TestNoHomeWatchesNothing(t *testing.T) {
	if got := watchReal("", noEnv).changes(); got != nil {
		t.Errorf("changes() with no home = %q, want none", got)
	}
}

func TestChangesWhereTheEnvironmentPutsTheConfigStateAndSkill(t *testing.T) {
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
		{
			name:   "a token file written in the state there all along",
			before: func(t *testing.T, elsewhere string) { writeState(t, filepath.Join(elsewhere, "state", "switchboard")) },
			during: func(t *testing.T, elsewhere string) {
				write(t, filepath.Join(elsewhere, "state", "switchboard", tokensDir, "work"), "test-token-work\n")
			},
			want: []string{"the real %[1]s/state/switchboard/tokens was created", "the real %[1]s/state/switchboard/tokens/work was created"},
		},
		{
			name: "a theme written where SWITCHBOARD_THEMES_DIR puts them",
			before: func(t *testing.T, elsewhere string) {
				write(t, filepath.Join(elsewhere, "themes", "nord.theme"), "before\n")
			},
			during: func(t *testing.T, elsewhere string) {
				write(t, filepath.Join(elsewhere, "themes", "lake.theme"), "after\n")
			},
			want: []string{"the real %[1]s/themes was modified", "the real %[1]s/themes/lake.theme was created"},
		},
		{
			name: "a theme written where XDG_CONFIG_HOME puts them",
			during: func(t *testing.T, elsewhere string) {
				write(t, filepath.Join(elsewhere, "config", "switchboard", "themes", "lake.theme"), "after\n")
			},
			want: []string{
				"the real %[1]s/config/switchboard was created",
				"the real %[1]s/config/switchboard/themes was created",
				"the real %[1]s/config/switchboard/themes/lake.theme was created",
			},
		},
		{
			name:   "the preferences file written in the state there all along, as a live dashboard does",
			before: func(t *testing.T, elsewhere string) { writeState(t, filepath.Join(elsewhere, "state", "switchboard")) },
			during: func(t *testing.T, elsewhere string) {
				write(t, filepath.Join(elsewhere, "state", "switchboard", prefsFile), "{\"theme\": \"amber\"}\n")
			},
		},
		{
			name: "the skill installed where CLAUDE_CONFIG_DIR puts it",
			during: func(t *testing.T, elsewhere string) {
				write(t, filepath.Join(elsewhere, "claude", skillDir, "SKILL.md"), "---\nname: switchboard\n---\n")
			},
			want: []string{"the real %[1]s/claude/skills/switchboard was created", "the real %[1]s/claude/skills/switchboard/SKILL.md was created"},
		},
		{
			name: "the skill rewritten there",
			before: func(t *testing.T, elsewhere string) {
				write(t, filepath.Join(elsewhere, "claude", skillDir, "SKILL.md"), "before\n")
			},
			during: func(t *testing.T, elsewhere string) {
				write(t, filepath.Join(elsewhere, "claude", skillDir, "SKILL.md"), "after, and longer\n")
			},
			want: []string{"the real %[1]s/claude/skills/switchboard/SKILL.md was modified"},
		},
		{
			name: "the claude link made in the bin directory where XDG_DATA_HOME puts it",
			during: func(t *testing.T, elsewhere string) {
				symlink(t, filepath.Join(elsewhere, "switchboard"), filepath.Join(elsewhere, "data", "switchboard", "bin", "claude"))
			},
			want: []string{"the real %[1]s/data/switchboard/bin was created", "the real %[1]s/data/switchboard/bin/claude was created"},
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
				"SWITCHBOARD_CONFIG":     filepath.Join(elsewhere, "work.toml"),
				"SWITCHBOARD_THEMES_DIR": filepath.Join(elsewhere, "themes"),
				"XDG_CONFIG_HOME":        filepath.Join(elsewhere, "config"),
				"XDG_STATE_HOME":         filepath.Join(elsewhere, "state"),
				"XDG_DATA_HOME":          filepath.Join(elsewhere, "data"),
				"CLAUDE_CONFIG_DIR":      filepath.Join(elsewhere, "claude"),
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

func TestWhatsWatchedIsWhereItsLinkLedAsTheTestsBegan(t *testing.T) {
	tests := []struct {
		name string
		// variable names the link, and file is what's watched in what the
		// link leads to, "" for that itself.
		variable, file string
	}{
		{name: "the config", variable: "SWITCHBOARD_CONFIG"},
		{name: "a theme", variable: "SWITCHBOARD_THEMES_DIR", file: "lake.theme"},
		{name: "a token file", variable: "XDG_STATE_HOME", file: filepath.Join("switchboard", tokensDir, "work")},
		{name: "the skill", variable: "CLAUDE_CONFIG_DIR", file: filepath.Join(skillDir, "SKILL.md")},
		{name: "claude on PATH", variable: "PATH", file: "claude"},
		{name: "switchboard's bin directory", variable: "XDG_DATA_HOME", file: filepath.Join("switchboard", "bin", "claude")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dotfiles := t.TempDir()
			kept := filepath.Join(dotfiles, "kept")
			write(t, filepath.Join(kept, tt.file), "before\n")
			link := filepath.Join(t.TempDir(), "link")
			symlink(t, kept, link)
			backdate(t, dotfiles)
			watched := watchReal("", func(key string) string { return map[string]string{tt.variable: link}[key] })

			// The link moved to lead elsewhere, and what it led to changed.
			if err := os.Remove(link); err != nil {
				t.Fatal(err)
			}
			symlink(t, filepath.Join(t.TempDir(), "other"), link)
			write(t, filepath.Join(kept, tt.file), "after, and longer\n")

			if got, want := watched.changes(), []string{"the real " + filepath.Join(link, tt.file) + " was modified"}; !slices.Equal(got, want) {
				t.Errorf("changes() = %q, want %q", got, want)
			}
		})
	}
}

func TestRelativePlacesAreNoneTestguardCanKnow(t *testing.T) {
	env := map[string]string{
		"SWITCHBOARD_CONFIG":     "work.toml",
		"SWITCHBOARD_THEMES_DIR": "themes",
		"XDG_CONFIG_HOME":        "config",
		"XDG_STATE_HOME":         "state",
		"XDG_DATA_HOME":          "data",
		"CLAUDE_CONFIG_DIR":      "claude",
		"PATH":                   strings.Join([]string{"bin", "", "."}, string(os.PathListSeparator)),
	}
	if got := watchReal("", func(key string) string { return env[key] }); len(got) > 0 {
		t.Errorf("watchReal() watches %d places, want none: a relative one leads wherever the program reading it runs", len(got))
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

// prefsFile is the dashboard's preferences file, in the state directory.
const prefsFile = "prefs.json"

// writePrefs lays out a state directory at state as writeState does, with
// the dashboard's preferences file in it.
func writePrefs(t *testing.T, state string) {
	t.Helper()
	writeState(t, state)
	write(t, filepath.Join(state, prefsFile), "{\n  \"theme\": \"nord\"\n}\n")
}

// writeTokens lays out a state directory at state as writeState does, with
// work's token file in it.
func writeTokens(t *testing.T, state string) {
	t.Helper()
	writeState(t, state)
	write(t, filepath.Join(state, tokensDir, "work"), "test-token-work\n")
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
// in the same tick as it was written still moves its time: everything but a
// link, which can't be set without setting where it leads, which may not be
// there.
func backdate(t *testing.T, dir string) {
	t.Helper()
	then := time.Now().Add(-time.Hour)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.Type()&fs.ModeSymlink != 0 {
			return err
		}
		return os.Chtimes(path, then, then)
	})
	if err != nil {
		t.Fatal(err)
	}
}
