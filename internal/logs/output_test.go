package logs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnOutputLogPastItsMostIsCutToItsNewestLines(t *testing.T) {
	const most, kept = 40, 20
	tests := []struct {
		name, before, want string
	}{
		{name: "no further than its most", before: strings.Repeat("123456789\n", 4), want: strings.Repeat("123456789\n", 4)},
		{
			name:   "past its most, from the first line to start in what's kept",
			before: "one 56789\ntwo 56789\nthree 7890\nfour 6789\nfive 789\n",
			want:   "four 6789\nfive 789\n",
		},
		{
			name:   "past its most, from a line that starts just where what's kept does",
			before: "one 56789\ntwo 56789\nthree 789\nfour 6789\nfive 6789\n",
			want:   "four 6789\nfive 6789\n",
		},
		{
			name:   "past its most, from where what's kept starts, with no line starting after it",
			before: strings.Repeat("x", 50),
			want:   strings.Repeat("x", 20),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "launchd.log")
			if err := os.WriteFile(path, []byte(tt.before), 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			held := []io.Writer{openHeld(t, path, os.O_WRONLY|os.O_APPEND)}

			if err := trimOutput(path, most, kept, held); err != nil {
				t.Fatalf("trimOutput() = %v", err)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != tt.want {
				t.Errorf("the log holds %q (%v), want %q", data, err, tt.want)
			}
			if after, err := os.Stat(path); err != nil || !os.SameFile(before, after) {
				t.Errorf("the log is %v (%v), want the file it was, cut in place", after, err)
			}
		})
	}
}

func TestWhatsWrittenToAnOutputLogHeldOpenLandsAfterWhatsKept(t *testing.T) {
	const most, kept = 40, 20
	before := "one 56789\ntwo 56789\nthree 789\nfour 6789\nfive 6789\n"
	tests := []struct {
		name string
		// flags are what the log is held open with, for the process's output
		// and its errors.
		flags []int
	}{
		{name: "held to append, as launchd holds it", flags: []int{os.O_WRONLY | os.O_APPEND}},
		{name: "held to append twice, its output and its errors", flags: []int{os.O_WRONLY | os.O_APPEND, os.O_WRONLY | os.O_APPEND}},
		{name: "held to write at its end, not to append", flags: []int{os.O_WRONLY}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "launchd.log")
			if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
				t.Fatal(err)
			}
			var held []io.Writer
			for _, flags := range tt.flags {
				held = append(held, openHeld(t, path, flags))
			}
			// Another file the process writes to, as a terminal, which is no
			// business of the trim's.
			other := filepath.Join(dir, "other")
			if err := os.WriteFile(other, []byte("$ "), 0o600); err != nil {
				t.Fatal(err)
			}
			terminal := openHeld(t, other, os.O_WRONLY)
			if _, err := terminal.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			held = append(held, terminal)

			if err := trimOutput(path, most, kept, held); err != nil {
				t.Fatalf("trimOutput() = %v", err)
			}
			for _, w := range held[:len(tt.flags)] {
				if _, err := io.WriteString(w, "panic: a start that failed\n"); err != nil {
					t.Fatal(err)
				}
			}
			want := "four 6789\nfive 6789\n" + strings.Repeat("panic: a start that failed\n", len(tt.flags))
			if data, err := os.ReadFile(path); err != nil || string(data) != want {
				t.Errorf("the log holds %q (%v), want what's kept, and what was written after it: %q", data, err, want)
			}
			if at, err := terminal.Seek(0, io.SeekCurrent); err != nil || at != 0 {
				t.Errorf("another file held is at %d (%v), want it left where it was, at 0", at, err)
			}
		})
	}
}

func TestAnOutputLogThatIsntTheProcesssOutputIsLeftAlone(t *testing.T) {
	const most, kept = 40, 20
	before := "one 56789\ntwo 56789\nthree 789\nfour 6789\nfive 6789\n"
	tests := []struct {
		name string
		// held are the process's output, as the test opens them in dir.
		held func(t *testing.T, dir string) []io.Writer
	}{
		{name: "with none held", held: func(*testing.T, string) []io.Writer { return nil }},
		{name: "with its output another file, as a terminal", held: func(t *testing.T, dir string) []io.Writer {
			return []io.Writer{openHeld(t, filepath.Join(dir, "terminal"), os.O_WRONLY|os.O_CREATE)}
		}},
		{name: "with its output no file", held: func(*testing.T, string) []io.Writer { return []io.Writer{io.Discard} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "launchd.log")
			if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
				t.Fatal(err)
			}

			if err := trimOutput(path, most, kept, tt.held(t, dir)); err != nil {
				t.Fatalf("trimOutput() = %v", err)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != before {
				t.Errorf("the log holds %q (%v), want it as it was: %q", data, err, before)
			}
		})
	}
}

func TestAnOutputLogThatIsntThereIsLeftSo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launchd.log")
	if err := TrimOutput(path); err != nil {
		t.Errorf("TrimOutput() = %v, want nothing to trim", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the log: %v, want it still not there", err)
	}
}

// openHeld opens the file at path with flags, as a service manager holds a
// process's output open, at its end, until the test ends.
func openHeld(t *testing.T, path string, flags int) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	return f
}
