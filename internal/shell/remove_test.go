package shell_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/shell"
)

func TestRemovingTheLine(t *testing.T) {
	const home = "/home/tester"
	initDir := filepath.Join(home, ".local", "share", "switchboard", "shell")
	for _, sh := range shell.Shells {
		ours := shell.Line(sh, shell.InitFile(initDir, sh), home)
		another := shell.Line(sh, shell.InitFile("/data/switchboard/shell", sh), home)
		tests := []struct {
			name string
			// text is the startup file's, or none where there's no file.
			text *string
			want string
			// wantRemoved is whether RemoveLine says it took one out, the
			// file left as it was where it didn't.
			wantRemoved bool
		}{
			{name: "no file"},
			{name: "an empty file", text: new("")},
			{name: "the user's lines alone", text: new("export EDITOR=vi\n# switchboard\n"), want: "export EDITOR=vi\n# switchboard\n"},
			{
				name: "ours last, as setup adds it: every other line kept, byte for byte",
				text: new("export EDITOR=vi\r\n\talias ll='ls -l'  \n\n" + ours + "\n"),
				want: "export EDITOR=vi\r\n\talias ll='ls -l'  \n\n", wantRemoved: true,
			},
			{
				name: "ours among others, indented",
				text: new("export EDITOR=vi\n  " + ours + "\nalias ll='ls -l'"),
				want: "export EDITOR=vi\nalias ll='ls -l'", wantRemoved: true,
			},
			{name: "ours last, without a newline", text: new("export EDITOR=vi\n" + ours), want: "export EDITOR=vi\n", wantRemoved: true},
			{name: "ours twice: both taken out", text: new(ours + "\nexport EDITOR=vi\n" + ours + "\n"), want: "export EDITOR=vi\n", wantRemoved: true},
			{name: "ours alone: the file left empty", text: new(ours + "\n"), want: "", wantRemoved: true},
			{name: "another switchboard's, kept", text: new(another + "\n" + ours + "\n"), want: another + "\n", wantRemoved: true},
			{name: "ours, commented out: kept", text: new("# " + ours + "\n"), want: "# " + ours + "\n"},
		}
		for _, tt := range tests {
			t.Run(string(sh)+", "+tt.name, func(t *testing.T) {
				file := filepath.Join(t.TempDir(), "rc")
				if tt.text != nil {
					write(t, file, *tt.text)
					if err := os.Chmod(file, 0o640); err != nil {
						t.Fatal(err)
					}
				}

				removed, err := shell.RemoveLine(sh, file, home, shell.InitFile(initDir, sh))
				if err != nil || removed != tt.wantRemoved {
					t.Fatalf("RemoveLine() = %v, %v; want %v", removed, err, tt.wantRemoved)
				}
				if tt.text == nil {
					if _, err := os.Stat(file); !errors.Is(err, fs.ErrNotExist) {
						t.Errorf("the startup file: %v, want none made", err)
					}
					return
				}
				checkFile(t, file, tt.want)
				if mode := stat(t, file).Mode(); mode != 0o640 {
					t.Errorf("the file's mode is %v, want its own, %v, kept", mode, fs.FileMode(0o640))
				}
			})
		}
	}
}

func TestTheLineSetupAddsIsRemovedAgain(t *testing.T) {
	for _, sh := range shell.Shells {
		t.Run(string(sh), func(t *testing.T) {
			home := t.TempDir()
			file := filepath.Join(home, "rc")
			const before = "export EDITOR=vi\n"
			for _, init := range []string{
				shell.InitFile(filepath.Join(home, ".local", "share", "switchboard", "shell"), sh),
				shell.InitFile(filepath.Join("/data", `it's "$x"`, "switchboard", "shell"), sh),
			} {
				write(t, file, before)
				if err := shell.AddLine(file, shell.Line(sh, init, home)); err != nil {
					t.Fatal(err)
				}
				if removed, err := shell.RemoveLine(sh, file, home, init); err != nil || !removed {
					t.Errorf("RemoveLine() = %v, %v for the line for %s; want it removed", removed, err, init)
				}
				checkFile(t, file, before)
			}
		})
	}
}

func TestRemovingTheLineThroughALink(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	init := shell.InitFile(filepath.Join(home, ".local", "share", "switchboard", "shell"), shell.Zsh)
	kept := filepath.Join(dir, "dotfiles", "zshrc")
	write(t, kept, "export EDITOR=vi\n"+shell.Line(shell.Zsh, init, home)+"\n")
	file := filepath.Join(home, ".zshrc")
	link(t, filepath.Join("..", "dotfiles", "zshrc"), file)

	if removed, err := shell.RemoveLine(shell.Zsh, file, home, init); err != nil || !removed {
		t.Fatalf("RemoveLine() = %v, %v; want it removed", removed, err)
	}
	checkFile(t, kept, "export EDITOR=vi\n")
	if info, err := os.Lstat(file); err != nil || info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("the link is %v (%v), want it a link still", info, err)
	}
}

func TestRemovingTheLineFailsSayingWhy(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to any directory")
	}
	dir := filepath.Join(t.TempDir(), "home")
	init := shell.InitFile(filepath.Join(dir, ".local", "share", "switchboard", "shell"), shell.Zsh)
	file := filepath.Join(dir, ".zshrc")
	text := "export EDITOR=vi\n" + shell.Line(shell.Zsh, init, dir) + "\n"
	write(t, file, text)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	removed, err := shell.RemoveLine(shell.Zsh, file, dir, init)
	if err == nil || !strings.Contains(err.Error(), "remove switchboard's line from "+file) || !errors.Is(err, fs.ErrPermission) {
		t.Errorf("RemoveLine() = %v, %v; want it to fail, saying why", removed, err)
	}
	checkFile(t, file, text)
}

func TestTheStartupFilesTheLineMayBeIn(t *testing.T) {
	const home = "/home/tester"
	tests := []struct {
		name string
		sh   shell.Shell
		env  map[string]string
		want []string
	}{
		{name: "zsh", sh: shell.Zsh, want: []string{home + "/.zshrc"}},
		{name: "zsh, with a ZDOTDIR", sh: shell.Zsh, env: map[string]string{"ZDOTDIR": "/home/tester/.config/zsh"}, want: []string{home + "/.config/zsh/.zshrc", home + "/.zshrc"}},
		{name: "zsh, with a ZDOTDIR of the home", sh: shell.Zsh, env: map[string]string{"ZDOTDIR": home}, want: []string{home + "/.zshrc"}},
		{name: "zsh, with a relative ZDOTDIR, passed over", sh: shell.Zsh, env: map[string]string{"ZDOTDIR": "zsh"}, want: []string{home + "/.zshrc"}},
		{name: "bash: every file a login bash may read first", sh: shell.Bash, want: []string{home + "/.bash_profile", home + "/.bash_login", home + "/.profile"}},
		{name: "fish", sh: shell.Fish, want: []string{home + "/.config/fish/config.fish"}},
		{name: "fish, with an XDG_CONFIG_HOME", sh: shell.Fish, env: map[string]string{"XDG_CONFIG_HOME": "/data/config"}, want: []string{"/data/config/fish/config.fish", home + "/.config/fish/config.fish"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shell.StartupFiles(tt.sh, home, func(key string) string { return tt.env[key] }); !slices.Equal(got, tt.want) {
				t.Errorf("StartupFiles() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRemovingTheLines(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".local", "share", "switchboard", "shell")
	zdotdir := filepath.Join(home, ".config", "zsh")
	getenv := func(key string) string { return map[string]string{"ZDOTDIR": zdotdir}[key] }
	const theirs = "export EDITOR=vi\n"
	files := map[string]shell.Shell{
		filepath.Join(zdotdir, ".zshrc"):                      shell.Zsh,
		filepath.Join(home, ".zshrc"):                         shell.Zsh,
		filepath.Join(home, ".profile"):                       shell.Bash,
		filepath.Join(home, ".config", "fish", "config.fish"): shell.Fish,
	}
	for file, sh := range files {
		write(t, file, theirs+shell.Line(sh, shell.InitFile(dir, sh), home)+"\n")
	}
	write(t, filepath.Join(home, ".bash_profile"), theirs)

	removed, err := shell.RemoveLines(home, dir, getenv)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(zdotdir, ".zshrc"), filepath.Join(home, ".zshrc"), filepath.Join(home, ".profile"), filepath.Join(home, ".config", "fish", "config.fish")}
	if !slices.Equal(removed, want) {
		t.Errorf("RemoveLines() = %q, want %q", removed, want)
	}
	for file := range files {
		checkFile(t, file, theirs)
	}
	checkFile(t, filepath.Join(home, ".bash_profile"), theirs)

	if removed, err := shell.RemoveLines(home, dir, getenv); err != nil || removed != nil {
		t.Errorf("RemoveLines(), again = %q, %v; want none taken out", removed, err)
	}
}

func TestRemovingTheLinesGoesOnPastAFileItCant(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".local", "share", "switchboard", "shell")
	// A .zshrc that's a directory can't be read.
	if err := os.MkdirAll(filepath.Join(home, ".zshrc"), 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(home, ".bash_profile"), shell.Line(shell.Bash, shell.InitFile(dir, shell.Bash), home)+"\n")

	removed, err := shell.RemoveLines(home, dir, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), filepath.Join(home, ".zshrc")) {
		t.Errorf("RemoveLines() error = %v, want it to name the .zshrc it can't read", err)
	}
	if want := []string{filepath.Join(home, ".bash_profile")}; !slices.Equal(removed, want) {
		t.Errorf("RemoveLines() = %q, want %q", removed, want)
	}
}

func TestRemovingTheFiles(t *testing.T) {
	tests := []struct {
		name string
		// there is what's in the directory before, by its name, as lay
		// reads it, and other a file of someone else's in it, where set.
		there map[string]string
		other bool
		want  map[string]shell.State
		// wantDir is whether the directory is left.
		wantDir bool
	}{
		{
			name: "none there",
			want: map[string]shell.State{"init.zsh": shell.Absent, "init.bash": shell.Absent, "completion.bash": shell.Absent, "init.fish": shell.Absent},
		},
		{
			name:  "every one there: each removed, and the directory",
			there: map[string]string{"init.zsh": "current", "init.bash": "current", "completion.bash": "current", "init.fish": "current"},
			want:  map[string]shell.State{"init.zsh": shell.Removed, "init.bash": shell.Removed, "completion.bash": shell.Removed, "init.fish": shell.Removed},
		},
		{
			name:    "some there, beside a file of someone else's: the directory kept for it",
			there:   map[string]string{"init.zsh": "an older one"},
			other:   true,
			want:    map[string]shell.State{"init.zsh": shell.Removed, "init.bash": shell.Absent, "completion.bash": shell.Absent, "init.fish": shell.Absent},
			wantDir: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := files(t)
			lay(t, f, tt.there)
			if tt.other {
				write(t, filepath.Join(f.Dir, "notes"), "someone else's")
			}

			got, err := shell.RemoveFiles(f.Dir)
			if err != nil {
				t.Fatal(err)
			}
			checkStates(t, f, got, tt.want)
			if _, err := os.Stat(f.Dir); (err == nil) != tt.wantDir {
				t.Errorf("the directory: %v, want it there: %v", err, tt.wantDir)
			}
		})
	}
}

func TestRemovingTheFilesRemovesEveryOneWritten(t *testing.T) {
	f := files(t)
	if _, err := f.Write(); err != nil {
		t.Fatal(err)
	}

	if _, err := shell.RemoveFiles(f.Dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.Dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the directory: %v, want it gone, every file written removed", err)
	}
}

func TestRemovingTheFilesGoesOnPastOneItCantRemove(t *testing.T) {
	f := files(t)
	lay(t, f, map[string]string{"init.zsh": "current", "init.fish": "current"})
	// A directory with something in it where bash's file goes can't be
	// removed as a file is.
	write(t, filepath.Join(shell.InitFile(f.Dir, shell.Bash), "kept"), "")

	got, err := shell.RemoveFiles(f.Dir)
	if err == nil || !strings.Contains(err.Error(), "remove switchboard's file "+shell.InitFile(f.Dir, shell.Bash)) {
		t.Errorf("RemoveFiles() error = %v, want it to say bash's file couldn't be removed", err)
	}
	want := []shell.File{
		{Shell: shell.Zsh, Path: shell.InitFile(f.Dir, shell.Zsh), State: shell.Removed},
		{Shell: shell.Bash, Path: shell.CompletionFile(f.Dir), State: shell.Absent},
		{Shell: shell.Fish, Path: shell.InitFile(f.Dir, shell.Fish), State: shell.Removed},
	}
	if !slices.Equal(got, want) {
		t.Errorf("RemoveFiles() = %+v, want %+v", got, want)
	}
}
