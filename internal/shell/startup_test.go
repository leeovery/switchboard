package shell_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/shell"
)

func TestTheStartupLine(t *testing.T) {
	const home = "/home/tester"
	tests := []struct {
		name string
		sh   shell.Shell
		// file is the init file's path.
		file string
		want string
	}{
		{
			name: "zsh, in the home: written from $HOME",
			sh:   shell.Zsh, file: home + "/.local/share/switchboard/shell/init.zsh",
			want: `if [ -r "$HOME/.local/share/switchboard/shell/init.zsh" ]; then . "$HOME/.local/share/switchboard/shell/init.zsh"; fi  # switchboard`,
		},
		{
			name: "bash, in the home: written from $HOME",
			sh:   shell.Bash, file: home + "/.local/share/switchboard/shell/init.bash",
			want: `if [ -r "$HOME/.local/share/switchboard/shell/init.bash" ]; then . "$HOME/.local/share/switchboard/shell/init.bash"; fi  # switchboard`,
		},
		{
			name: "fish, in the home: written from $HOME",
			sh:   shell.Fish, file: home + "/.local/share/switchboard/shell/init.fish",
			want: `if test -r "$HOME/.local/share/switchboard/shell/init.fish"; source "$HOME/.local/share/switchboard/shell/init.fish"; end  # switchboard`,
		},
		{
			name: "zsh, outside the home: written whole",
			sh:   shell.Zsh, file: "/data/switchboard/shell/init.zsh",
			want: `if [ -r "/data/switchboard/shell/init.zsh" ]; then . "/data/switchboard/shell/init.zsh"; fi  # switchboard`,
		},
		{
			name: "zsh, beside the home, not in it: written whole",
			sh:   shell.Zsh, file: home + "-other/switchboard/shell/init.zsh",
			want: `if [ -r "/home/tester-other/switchboard/shell/init.zsh" ]; then . "/home/tester-other/switchboard/shell/init.zsh"; fi  # switchboard`,
		},
		{
			name: "bash, what it reads specially between double quotes escaped",
			sh:   shell.Bash, file: home + "/it's \"$x\" `y` \\z/switchboard/shell/init.bash",
			want: strings.ReplaceAll(`if [ -r "F" ]; then . "F"; fi  # switchboard`, "F", "$HOME/it's \\\"\\$x\\\" \\`y\\` \\\\z/switchboard/shell/init.bash"),
		},
		{
			name: "fish, what it reads specially between double quotes escaped, and nothing else",
			sh:   shell.Fish, file: home + "/it's \"$x\" `y` \\z/switchboard/shell/init.fish",
			want: strings.ReplaceAll(`if test -r "F"; source "F"; end  # switchboard`, "F", "$HOME/it's \\\"\\$x\\\" `y` \\\\z/switchboard/shell/init.fish"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shell.Line(tt.sh, tt.file, home); got != tt.want {
				t.Errorf("Line() = %s\nwant      %s", got, tt.want)
			}
		})
	}
}

func TestTheStartupFile(t *testing.T) {
	tests := []struct {
		name string
		sh   shell.Shell
		env  map[string]string
		// configDir is where the shell said it reads its startup file from,
		// from the home.
		configDir string
		// there are the files in the home, by their names.
		there []string
		// want is the startup file, from the home.
		want string
	}{
		{name: "zsh: .zshrc in the home", sh: shell.Zsh, want: ".zshrc"},
		{name: "zsh: .zshrc in ZDOTDIR", sh: shell.Zsh, env: map[string]string{"ZDOTDIR": "<home>/.config/zsh"}, want: ".config/zsh/.zshrc"},
		{name: "zsh: a relative ZDOTDIR passed over", sh: shell.Zsh, env: map[string]string{"ZDOTDIR": ".config/zsh"}, want: ".zshrc"},
		{
			name: "zsh: .zshrc where the shell said, over ZDOTDIR, which a startup file can set unexported",
			sh:   shell.Zsh, env: map[string]string{"ZDOTDIR": "<home>/elsewhere"}, configDir: "<home>/.config/zsh", want: ".config/zsh/.zshrc",
		},
		{name: "zsh: a relative directory the shell said passed over", sh: shell.Zsh, configDir: "zsh", want: ".zshrc"},
		{name: "fish: config.fish in ~/.config", sh: shell.Fish, want: ".config/fish/config.fish"},
		{name: "fish: config.fish in XDG_CONFIG_HOME", sh: shell.Fish, env: map[string]string{"XDG_CONFIG_HOME": "<home>/xdg"}, want: "xdg/fish/config.fish"},
		{name: "fish: a relative XDG_CONFIG_HOME passed over", sh: shell.Fish, env: map[string]string{"XDG_CONFIG_HOME": "xdg"}, want: ".config/fish/config.fish"},
		{
			name: "fish: config.fish where the shell said, over XDG_CONFIG_HOME",
			sh:   shell.Fish, env: map[string]string{"XDG_CONFIG_HOME": "<home>/xdg"}, configDir: "<home>/fish-config", want: "fish-config/config.fish",
		},
		{name: "bash: none there, .bash_profile made", sh: shell.Bash, want: ".bash_profile"},
		{name: "bash: .bash_profile, read first", sh: shell.Bash, there: []string{".bash_profile", ".bash_login", ".profile"}, want: ".bash_profile"},
		{name: "bash: .bash_login, without .bash_profile", sh: shell.Bash, there: []string{".bash_login", ".profile"}, want: ".bash_login"},
		{
			name: "bash: .profile alone, never a .bash_profile beside it, which would stop it loading",
			sh:   shell.Bash, there: []string{".profile"}, want: ".profile",
		},
		{name: "bash: .bashrc isn't one a login bash reads", sh: shell.Bash, there: []string{".bashrc"}, want: ".bash_profile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			for _, name := range tt.there {
				write(t, filepath.Join(home, name), "")
			}
			atHome := func(path string) string { return strings.ReplaceAll(path, "<home>", home) }
			getenv := func(key string) string { return atHome(tt.env[key]) }
			if got := shell.StartupFile(tt.sh, getenv, home, atHome(tt.configDir)); got != filepath.Join(home, tt.want) {
				t.Errorf("StartupFile() = %s, want ~/%s", got, tt.want)
			}
		})
	}
}

func TestBashsStartupFileIsOneThatLeadsSomewhere(t *testing.T) {
	home := t.TempDir()
	link(t, filepath.Join(home, "gone"), filepath.Join(home, ".bash_profile"))
	write(t, filepath.Join(home, ".profile"), "")

	if got, want := shell.StartupFile(shell.Bash, noEnv, home, ""), filepath.Join(home, ".profile"); got != want {
		t.Errorf("StartupFile() = %s, want %s: bash passes over a .bash_profile that leads nowhere, as it can't read it", got, want)
	}
}

func TestFindingTheLine(t *testing.T) {
	ours := map[shell.Shell]string{
		shell.Zsh:  `if [ -r "$HOME/.local/share/switchboard/shell/init.zsh" ]; then . "$HOME/.local/share/switchboard/shell/init.zsh"; fi  # switchboard`,
		shell.Bash: `if [ -r "/data/switchboard/shell/init.bash" ]; then . "/data/switchboard/shell/init.bash"; fi  # switchboard`,
		shell.Fish: `if test -r "$HOME/.local/share/switchboard/shell/init.fish"; source "$HOME/.local/share/switchboard/shell/init.fish"; end  # switchboard`,
	}
	const zshFile = "/home/tester/.local/share/switchboard/shell/init.zsh"
	tests := []struct {
		name string
		sh   shell.Shell
		// text is the startup file's, or none where there's no file.
		text *string
		// want is the file the line found sources, "" for none found.
		want string
	}{
		{name: "no file", sh: shell.Zsh},
		{name: "an empty file", sh: shell.Zsh, text: new("")},
		{name: "ours, among others", sh: shell.Zsh, text: new("export EDITOR=vi\n" + ours[shell.Zsh] + "\nalias ll='ls -l'\n"), want: zshFile},
		{name: "ours, last, without a newline", sh: shell.Zsh, text: new("export EDITOR=vi\n" + ours[shell.Zsh]), want: zshFile},
		{name: "ours, indented", sh: shell.Zsh, text: new("  " + ours[shell.Zsh] + "\n"), want: zshFile},
		{name: "ours from another directory", sh: shell.Bash, text: new(ours[shell.Bash] + "\n"), want: "/data/switchboard/shell/init.bash"},
		{name: "fish's", sh: shell.Fish, text: new(ours[shell.Fish] + "\n"), want: "/home/tester/.local/share/switchboard/shell/init.fish"},
		{
			name: "ours, a path with what's escaped in it",
			sh:   shell.Zsh,
			text: new(`if [ -r "$HOME/it's \"\$x\" \` + "`y\\`" + `/switchboard/shell/init.zsh" ]; then . "$HOME/it's \"\$x\" \` + "`y\\`" + `/switchboard/shell/init.zsh"; fi  # switchboard` + "\n"),
			want: "/home/tester/it's \"$x\" `y`/switchboard/shell/init.zsh",
		},
		{
			name: "ours, written whole, $HOME in it escaped",
			sh:   shell.Zsh,
			text: new(`if [ -r "/data/\$HOME/switchboard/shell/init.zsh" ]; then . "/data/\$HOME/switchboard/shell/init.zsh"; fi  # switchboard` + "\n"),
			want: "/data/$HOME/switchboard/shell/init.zsh",
		},
		{name: "the user's own # switchboard comment", sh: shell.Zsh, text: new("# switchboard\n")},
		{name: "the user's own export line, marked", sh: shell.Zsh, text: new(`export PATH="$HOME/.local/share/switchboard/bin:$PATH"  # switchboard` + "\n")},
		{name: "ours, commented out", sh: shell.Zsh, text: new("# " + ours[shell.Zsh] + "\n")},
		{name: "ours, without the mark", sh: shell.Zsh, text: new(strings.TrimSuffix(ours[shell.Zsh], "  # switchboard") + "\n")},
		{name: "another shell's", sh: shell.Bash, text: new(ours[shell.Zsh] + "\n")},
		{name: "zsh's in fish", sh: shell.Fish, text: new(ours[shell.Zsh] + "\n")},
		{
			name: "reading one file and sourcing another",
			sh:   shell.Zsh,
			text: new(`if [ -r "/a/switchboard/shell/init.zsh" ]; then . "/b/switchboard/shell/init.zsh"; fi  # switchboard` + "\n"),
		},
		{
			name: "sourcing a file that isn't switchboard's",
			sh:   shell.Zsh,
			text: new(`if [ -r "$HOME/init.zsh" ]; then . "$HOME/init.zsh"; fi  # switchboard` + "\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "rc")
			if tt.text != nil {
				write(t, file, *tt.text)
			}
			if got, err := shell.FindLine(tt.sh, file, "/home/tester"); err != nil || got != tt.want {
				t.Errorf("FindLine() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestTheLineSetupAddsIsFoundAgain(t *testing.T) {
	for _, sh := range shell.Shells {
		t.Run(string(sh), func(t *testing.T) {
			home := t.TempDir()
			file := filepath.Join(home, "rc")
			for _, init := range []string{
				shell.InitFile(filepath.Join(home, ".local", "share", "switchboard", "shell"), sh),
				shell.InitFile(filepath.Join("/data", `it's "$x"`, "switchboard", "shell"), sh),
			} {
				write(t, file, shell.Line(sh, init, home)+"\n")
				if found, err := shell.FindLine(sh, file, home); err != nil || found != init {
					t.Errorf("FindLine() = %q, %v for the line for %s; want it found, sourcing that", found, err, init)
				}
			}
		})
	}
}

func TestAddingTheLine(t *testing.T) {
	const line = `if [ -r "$HOME/init.zsh" ]; then . "$HOME/init.zsh"; fi  # switchboard`
	tests := []struct {
		name string
		// text is the file's, or none where there's no file.
		text *string
		want string
	}{
		{name: "a file ending in a newline", text: new("export EDITOR=vi\n"), want: "export EDITOR=vi\n" + line + "\n"},
		{name: "a file without a final newline: one added first", text: new("export EDITOR=vi"), want: "export EDITOR=vi\n" + line + "\n"},
		{name: "an empty file", text: new(""), want: line + "\n"},
		{name: "no file: made", want: line + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "dir", "rc")
			if tt.text != nil {
				write(t, file, *tt.text)
			}
			if err := shell.AddLine(file, line); err != nil {
				t.Fatal(err)
			}
			checkFile(t, file, tt.want)
		})
	}
}

func TestAddingTheLineKeepsTheFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "rc")
	write(t, file, "export EDITOR=vi\n")
	if err := os.Chmod(file, 0o640); err != nil {
		t.Fatal(err)
	}
	hard := filepath.Join(filepath.Dir(file), "hard")
	if err := os.Link(file, hard); err != nil {
		t.Fatal(err)
	}
	before := stat(t, file)

	if err := shell.AddLine(file, "the line"); err != nil {
		t.Fatal(err)
	}
	after := stat(t, file)
	if !os.SameFile(before, after) || after.Mode() != 0o640 {
		t.Errorf("the file is %v, mode %v; want the same file, mode %v, appended to in place", after, after.Mode(), before.Mode())
	}
	checkFile(t, hard, "export EDITOR=vi\nthe line\n")
}

func TestANewStartupFileIsMadeReadableByAll(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, ".config", "fish", "config.fish")
	if err := shell.AddLine(file, "the line"); err != nil {
		t.Fatal(err)
	}
	// What 0644 comes to under this process's umask.
	made, err := os.OpenFile(filepath.Join(dir, "made"), os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_ = made.Close()
	if got, want := stat(t, file).Mode(), stat(t, made.Name()).Mode(); got != want {
		t.Errorf("the new file's mode is %v, want %v, 0644 under the umask", got, want)
	}
}

func TestAddingTheLineThroughALink(t *testing.T) {
	dir := t.TempDir()
	kept := filepath.Join(dir, "dotfiles", "zshrc")
	write(t, kept, "export EDITOR=vi\n")
	file := filepath.Join(dir, "home", ".zshrc")
	link(t, filepath.Join("..", "dotfiles", "zshrc"), file)

	if err := shell.AddLine(file, "the line"); err != nil {
		t.Fatal(err)
	}
	checkFile(t, kept, "export EDITOR=vi\nthe line\n")
	if info, err := os.Lstat(file); err != nil || info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("the link is %v (%v), want it a link still", info, err)
	}
}

func TestAddingTheLineFailsSayingWhy(t *testing.T) {
	tests := []struct {
		name string
		lay  func(t *testing.T, file string)
		want string
	}{
		{
			name: "a link that leads nowhere",
			lay:  func(t *testing.T, file string) { link(t, filepath.Join(filepath.Dir(file), "gone"), file) },
			want: "is a link to <dir>/gone, which isn't there",
		},
		{
			name: "a file that can't be written",
			lay: func(t *testing.T, file string) {
				if os.Getuid() == 0 {
					t.Skip("root can write to any file")
				}
				write(t, file, "export EDITOR=vi\n")
				if err := os.Chmod(file, 0o444); err != nil {
					t.Fatal(err)
				}
			},
			want: "permission denied",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "rc")
			tt.lay(t, file)
			before, _ := os.ReadFile(file)

			err := shell.AddLine(file, "the line")
			if want := strings.ReplaceAll(tt.want, "<dir>", dir); err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("AddLine() error = %v, want one saying %q", err, want)
			}
			if after, _ := os.ReadFile(file); string(after) != string(before) {
				t.Errorf("the file reads %q, want it as it was, %q", after, before)
			}
			if _, err := os.Stat(filepath.Join(dir, "gone")); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("where the link leads: %v, want nothing made there", err)
			}
		})
	}
}

func noEnv(string) string { return "" }

// write writes text to a file at path, making its directory.
func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// link makes a link at path leading to target, making its directory.
func link(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func stat(t *testing.T, path string) fs.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// checkFile checks the file at path reads want.
func checkFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Errorf("%s reads %q (%v), want %q", path, got, err, want)
	}
}
