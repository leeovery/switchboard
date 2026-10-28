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
// own, not made, each with a completion script of its own.
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

// fileNames are the files, by name, in turn, each with its shell.
var fileNames = []struct {
	name string
	sh   shell.Shell
}{
	{"init.zsh", shell.Zsh}, {"init.bash", shell.Bash}, {"completion.bash", shell.Bash}, {"init.fish", shell.Fish},
}

// fileText is the file named name as f writes it: bash's completion script
// as it is, and each shell's file as Init writes it.
func fileText(t *testing.T, f shell.Files, name string, sh shell.Shell) string {
	t.Helper()
	completion, err := f.Completion(sh)
	if err != nil {
		t.Fatal(err)
	}
	if name == "completion.bash" {
		return string(completion)
	}
	text, err := shell.Init(sh, f.Dir, f.Bin, completion)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

func TestWritingTheFiles(t *testing.T) {
	tests := []struct {
		name string
		// there is what each file holds before, by its name, where it's
		// there, as lay reads it.
		there map[string]string
		want  map[string]shell.State
	}{
		{
			name: "none there: each written, the directory made",
			want: map[string]shell.State{"init.zsh": shell.Created, "init.bash": shell.Created, "completion.bash": shell.Created, "init.fish": shell.Created},
		},
		{
			name:  "each this switchboard's already: left as they are",
			there: map[string]string{"init.zsh": "current", "init.bash": "current", "completion.bash": "current", "init.fish": "current"},
			want:  map[string]shell.State{"init.zsh": shell.Current, "init.bash": shell.Current, "completion.bash": shell.Current, "init.fish": shell.Current},
		},
		{
			name:  "one another's, others missing: brought up to date, and written",
			there: map[string]string{"init.zsh": "# an older switchboard's\n", "init.bash": "current"},
			want:  map[string]shell.State{"init.zsh": shell.Updated, "init.bash": shell.Current, "completion.bash": shell.Created, "init.fish": shell.Created},
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
			for _, n := range fileNames {
				checkFile(t, filepath.Join(f.Dir, n.name), fileText(t, f, n.name, n.sh))
			}
		})
	}
}

func TestRefreshingTheFiles(t *testing.T) {
	tests := []struct {
		name  string
		there map[string]string
		want  map[string]shell.State
	}{
		{
			name: "none there: none written, nor the directory made",
			want: map[string]shell.State{"init.zsh": shell.Absent, "init.bash": shell.Absent, "completion.bash": shell.Absent, "init.fish": shell.Absent},
		},
		{
			name:  "one another's, one current, others missing: the one brought up to date alone",
			there: map[string]string{"init.zsh": "# an older switchboard's\n", "init.bash": "current", "completion.bash": "current"},
			want:  map[string]shell.State{"init.zsh": shell.Updated, "init.bash": shell.Current, "completion.bash": shell.Current, "init.fish": shell.Absent},
		},
		{
			name:  "bash's completions missing beside bash's file, as an older switchboard's setup wrote none: written",
			there: map[string]string{"init.bash": "current"},
			want:  map[string]shell.State{"init.zsh": shell.Absent, "init.bash": shell.Current, "completion.bash": shell.Created, "init.fish": shell.Absent},
		},
		{
			name:  "bash's completions missing beside bash's file, itself missing: neither written",
			there: map[string]string{"init.zsh": "current"},
			want:  map[string]shell.State{"init.zsh": shell.Current, "init.bash": shell.Absent, "completion.bash": shell.Absent, "init.fish": shell.Absent},
		},
		{
			name:  "bash's completions differing by a byte: rewritten",
			there: map[string]string{"completion.bash": "current\n"},
			want:  map[string]shell.State{"init.zsh": shell.Absent, "init.bash": shell.Absent, "completion.bash": shell.Updated, "init.fish": shell.Absent},
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
			for _, n := range fileNames {
				path := filepath.Join(f.Dir, n.name)
				if tt.want[n.name] == shell.Absent {
					if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
						t.Errorf("%s: %v, want it not there still", path, err)
					}
					continue
				}
				checkFile(t, path, fileText(t, f, n.name, n.sh))
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

// lay puts each file in f's directory as there says, by its name: "current"
// for this switchboard's, "current\n" for one a byte longer, else the text
// given.
func lay(t *testing.T, f shell.Files, there map[string]string) {
	t.Helper()
	for _, n := range fileNames {
		text, ok := there[n.name]
		if !ok {
			continue
		}
		switch text {
		case "current":
			text = fileText(t, f, n.name, n.sh)
		case "current\n":
			text = fileText(t, f, n.name, n.sh) + "\n"
		}
		write(t, filepath.Join(f.Dir, n.name), text)
	}
}

// checkStates checks what became of each of f's files, as got says, is want,
// by the file's name, in its turn.
func checkStates(t *testing.T, f shell.Files, got []shell.File, want map[string]shell.State) {
	t.Helper()
	var wanted []shell.File
	for _, n := range fileNames {
		wanted = append(wanted, shell.File{Shell: n.sh, Path: filepath.Join(f.Dir, n.name), State: want[n.name]})
	}
	if !slices.Equal(got, wanted) {
		t.Errorf("the files became %+v, want %+v", got, wanted)
	}
}
