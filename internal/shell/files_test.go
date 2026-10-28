package shell_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/shell"
)

// files are switchboard's files for the shells in a directory of the test's
// own, made but empty, each with a completion script of its own.
func files(t *testing.T) shell.Files {
	t.Helper()
	return shell.Files{
		Dir: filepath.Join(t.TempDir(), "switchboard", "shell"),
		Bin: "/data/switchboard/bin",
		Completion: func(sh shell.Shell) ([]byte, error) {
			return []byte("# " + string(sh) + "'s completions\n"), nil
		},
	}
}

// initText is sh's file as f writes it.
func initText(t *testing.T, f shell.Files, sh shell.Shell) string {
	t.Helper()
	completion, err := f.Completion(sh)
	if err != nil {
		t.Fatal(err)
	}
	text, err := shell.Init(sh, f.Bin, completion)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

func TestWritingTheFiles(t *testing.T) {
	tests := []struct {
		name string
		// there is what each shell's file holds before, where it's there, as
		// lay reads it.
		there map[shell.Shell]string
		want  map[shell.Shell]shell.State
	}{
		{
			name: "none there: each written, the directory made",
			want: map[shell.Shell]shell.State{shell.Zsh: shell.Created, shell.Bash: shell.Created, shell.Fish: shell.Created},
		},
		{
			name:  "each this switchboard's already: left as they are",
			there: map[shell.Shell]string{shell.Zsh: "current", shell.Bash: "current", shell.Fish: "current"},
			want:  map[shell.Shell]shell.State{shell.Zsh: shell.Current, shell.Bash: shell.Current, shell.Fish: shell.Current},
		},
		{
			name:  "one another's, one missing: brought up to date, and written",
			there: map[shell.Shell]string{shell.Zsh: "# an older switchboard's\n", shell.Bash: "current"},
			want:  map[shell.Shell]shell.State{shell.Zsh: shell.Updated, shell.Bash: shell.Current, shell.Fish: shell.Created},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := files(t)
			lay(t, f, tt.there)

			written, err := f.Write()
			if err != nil {
				t.Fatal(err)
			}
			checkStates(t, f, written, tt.want)
			for _, sh := range shell.Shells {
				checkFile(t, shell.InitFile(f.Dir, sh), initText(t, f, sh))
			}
		})
	}
}

func TestRefreshingTheFiles(t *testing.T) {
	tests := []struct {
		name  string
		there map[shell.Shell]string
		want  map[shell.Shell]shell.State
	}{
		{
			name: "none there: none written, nor the directory made",
			want: map[shell.Shell]shell.State{shell.Zsh: shell.Absent, shell.Bash: shell.Absent, shell.Fish: shell.Absent},
		},
		{
			name:  "one another's, one current, one missing: the one brought up to date alone",
			there: map[shell.Shell]string{shell.Zsh: "# an older switchboard's\n", shell.Bash: "current"},
			want:  map[shell.Shell]shell.State{shell.Zsh: shell.Updated, shell.Bash: shell.Current, shell.Fish: shell.Absent},
		},
		{
			name:  "one differing by a byte: rewritten",
			there: map[shell.Shell]string{shell.Fish: "current\n"},
			want:  map[shell.Shell]shell.State{shell.Zsh: shell.Absent, shell.Bash: shell.Absent, shell.Fish: shell.Updated},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := files(t)
			lay(t, f, tt.there)

			refreshed, err := f.Refresh()
			if err != nil {
				t.Fatal(err)
			}
			checkStates(t, f, refreshed, tt.want)
			for _, sh := range shell.Shells {
				path := shell.InitFile(f.Dir, sh)
				if tt.want[sh] == shell.Absent {
					if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
						t.Errorf("%s: %v, want it not there still", path, err)
					}
					continue
				}
				checkFile(t, path, initText(t, f, sh))
			}
			if len(tt.there) == 0 {
				if _, err := os.Lstat(f.Dir); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("the shell directory: %v, want it not there still", err)
				}
			}
		})
	}
}

func TestWritingTheFilesFailsWithoutCompletions(t *testing.T) {
	f := files(t)
	f.Completion = func(shell.Shell) ([]byte, error) { return nil, errors.New("no completions") }
	if _, err := f.Write(); err == nil {
		t.Error("Write() succeeded, want it to fail without the completions to write")
	}
	if _, err := os.Lstat(shell.InitFile(f.Dir, shell.Zsh)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("zsh's file: %v, want none written", err)
	}
}

// lay puts each shell's file in f's directory as there says: "current" for
// this switchboard's, "current\n" for one a byte longer, else the text given.
func lay(t *testing.T, f shell.Files, there map[shell.Shell]string) {
	t.Helper()
	for sh, text := range there {
		switch text {
		case "current":
			text = initText(t, f, sh)
		case "current\n":
			text = initText(t, f, sh) + "\n"
		}
		write(t, shell.InitFile(f.Dir, sh), text)
	}
}

// checkStates checks what became of each of f's files, as got says, is want,
// each named for its shell, in its turn.
func checkStates(t *testing.T, f shell.Files, got []shell.File, want map[shell.Shell]shell.State) {
	t.Helper()
	var wanted []shell.File
	for _, sh := range shell.Shells {
		wanted = append(wanted, shell.File{Shell: sh, Path: shell.InitFile(f.Dir, sh), State: want[sh]})
	}
	if !slices.Equal(got, wanted) {
		t.Errorf("the files became %+v, want %+v", got, wanted)
	}
}
