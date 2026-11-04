package routercopy_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/routercopy"
)

func TestFind(t *testing.T) {
	home := func() (string, error) { return "/home/tester", nil }
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "XDG_DATA_HOME", env: map[string]string{"XDG_DATA_HOME": "/xdg/data"}, want: "/xdg/data/switchboard/router/switchboard"},
		{name: "the home directory", want: "/home/tester/.local/share/switchboard/router/switchboard"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			own, err := routercopy.Find(func(key string) string { return tt.env[key] }, home)
			if err != nil || own.Path() != tt.want {
				t.Errorf("Find() = %q, %v; want %q", own.Path(), err, tt.want)
			}
		})
	}
}

func TestAtIsTheCopyByThePathItWasStartedAs(t *testing.T) {
	const path = "/test/data/switchboard/router/switchboard"
	if got := routercopy.At(path).Path(); got != path {
		t.Errorf("At(%q).Path() = %q, want it as given", path, got)
	}
}

func TestMakeCopiesTheBinaryItsLinksLeadTo(t *testing.T) {
	f := newFixture(t)
	version := f.binary(t, "Cellar/switchboard/1.2.3/bin/switchboard", "version 1.2.3\n", modified)
	link := filepath.Join(f.root, "opt", "bin", "switchboard")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(version, link); err != nil {
		t.Fatal(err)
	}

	if err := f.own.Make(link); err != nil {
		t.Fatalf("Make() error = %v", err)
	}
	if data, err := os.ReadFile(f.own.Path()); err != nil || string(data) != "version 1.2.3\n" {
		t.Errorf("the copy holds %q (%v), want the binary the link leads to", data, err)
	}
	info, err := os.Lstat(f.own.Path())
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o755 || !info.ModTime().Equal(modified) {
		t.Errorf("the copy is %v (%v), want a file, 0755, modified at %v as the binary was", info, err, modified)
	}
	if info, err := os.Stat(filepath.Dir(f.own.Path())); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the copy's directory is %v (%v), want it 0700", info, err)
	}
	f.checkAlone(t)
}

func TestMakeReplacesTheCopyWithAnotherFile(t *testing.T) {
	f := newFixture(t)
	if err := f.own.Make(f.binary(t, "1/switchboard", "version 1\n", modified)); err != nil {
		t.Fatal(err)
	}
	// The copy as a router started from it holds it.
	running, err := os.Open(f.own.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = running.Close() }()
	before, err := running.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if err := f.own.Make(f.binary(t, "2/switchboard", "version 2\n", modified.Add(time.Hour))); err != nil {
		t.Fatalf("Make() error = %v", err)
	}
	after, err := os.Stat(f.own.Path())
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Error("the copy was written into, want another file renamed over it")
	}
	if data, err := io.ReadAll(running); err != nil || string(data) != "version 1\n" {
		t.Errorf("the copy held open reads %q (%v), want the one before, untouched", data, err)
	}
	if data, err := os.ReadFile(f.own.Path()); err != nil || string(data) != "version 2\n" {
		t.Errorf("the copy holds %q (%v), want the new binary", data, err)
	}
	f.checkAlone(t)
}

func TestMakeOfWhatIsntAFileFails(t *testing.T) {
	f := newFixture(t)

	if err := f.own.Make(f.root); err == nil {
		t.Error("Make() of a directory: no error, want one")
	}
	if err := f.own.Make(filepath.Join(f.root, "gone")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Make() of nothing: error = %v, want %v", err, fs.ErrNotExist)
	}
	if _, err := os.Stat(f.own.Path()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the copy: %v, want none made", err)
	}
}

func TestStageThenCommit(t *testing.T) {
	f := newFixture(t)
	if err := f.own.Make(f.binary(t, "1/switchboard", "version 1\n", modified)); err != nil {
		t.Fatal(err)
	}

	staged, err := f.own.Stage(f.binary(t, "2/switchboard", "version 2\n", modified))
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	if filepath.Dir(staged.Path()) != filepath.Dir(f.own.Path()) || filepath.Base(staged.Path())[0] != '.' {
		t.Errorf("staged at %s, want a hidden file beside the copy, %s", staged.Path(), f.own.Path())
	}
	if data, err := os.ReadFile(staged.Path()); err != nil || string(data) != "version 2\n" {
		t.Errorf("the staged copy holds %q (%v), want the new binary", data, err)
	}
	if data, err := os.ReadFile(f.own.Path()); err != nil || string(data) != "version 1\n" {
		t.Errorf("before the commit, the copy holds %q (%v), want the one before", data, err)
	}

	if err := staged.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if data, err := os.ReadFile(f.own.Path()); err != nil || string(data) != "version 2\n" {
		t.Errorf("the copy holds %q (%v), want the one staged", data, err)
	}
	f.checkAlone(t)
}

func TestAStagedCopySaysTheFileItWasMadeOf(t *testing.T) {
	f := newFixture(t)
	version := f.binary(t, "Cellar/switchboard/2/bin/switchboard", "version 2\n", modified)
	link := filepath.Join(f.root, "opt", "bin", "switchboard")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(version, link); err != nil {
		t.Fatal(err)
	}

	staged, err := f.own.Stage(link)
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	want, err := os.Stat(version)
	if err != nil {
		t.Fatal(err)
	}
	if from := staged.From(); from == nil || !os.SameFile(from, want) || !from.ModTime().Equal(modified) {
		t.Errorf("From() = %v, want the file the link leads to, %s, modified at %v", from, version, modified)
	}
}

func TestACommitThatFailsRemovesTheStagedCopy(t *testing.T) {
	f := newFixture(t)
	staged, err := f.own.Stage(f.binary(t, "1/switchboard", "version 1\n", modified))
	if err != nil {
		t.Fatal(err)
	}
	// A directory, with something in it, where the copy goes, which no
	// rename replaces.
	if err := os.MkdirAll(filepath.Join(f.own.Path(), "kept"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := staged.Commit(); err == nil {
		t.Fatal("Commit() succeeded, want it to fail to replace a directory")
	}
	entries, err := os.ReadDir(filepath.Dir(f.own.Path()))
	if err != nil || len(entries) != 1 || entries[0].Name() != "switchboard" || !entries[0].IsDir() {
		t.Errorf("the copy's directory holds %v (%v), want what was there alone, the staged copy removed", entries, err)
	}
}

func TestStageThenDiscard(t *testing.T) {
	f := newFixture(t)
	if err := f.own.Make(f.binary(t, "1/switchboard", "version 1\n", modified)); err != nil {
		t.Fatal(err)
	}
	staged, err := f.own.Stage(f.binary(t, "2/switchboard", "version 2\n", modified))
	if err != nil {
		t.Fatal(err)
	}

	if err := staged.Discard(); err != nil {
		t.Fatalf("Discard() error = %v", err)
	}
	if data, err := os.ReadFile(f.own.Path()); err != nil || string(data) != "version 1\n" {
		t.Errorf("the copy holds %q (%v), want the one before, untouched", data, err)
	}
	f.checkAlone(t)
	if err := staged.Discard(); err != nil {
		t.Errorf("Discard() again: error = %v, want none, as it's gone", err)
	}
}

func TestMatches(t *testing.T) {
	tests := []struct {
		name string
		// change changes the binary at path once it's copied, nil leaving it.
		change func(t *testing.T, path string)
		want   bool
	}{
		{name: "the binary copied", want: true},
		{
			name: "modified since, the same size",
			change: func(t *testing.T, path string) {
				if err := os.Chtimes(path, time.Time{}, modified.Add(time.Second)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "another size, modified at the same time",
			change: func(t *testing.T, path string) {
				if err := os.WriteFile(path, []byte("version 1, longer\n"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(path, time.Time{}, modified); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			binary := f.binary(t, "1/switchboard", "version 1\n", modified)
			if err := f.own.Make(binary); err != nil {
				t.Fatal(err)
			}
			if tt.change != nil {
				tt.change(t, binary)
			}

			if got, err := f.own.Matches(binary); err != nil || got != tt.want {
				t.Errorf("Matches() = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestMatchesTheBinaryALinkLeadsTo(t *testing.T) {
	f := newFixture(t)
	version := f.binary(t, "Cellar/switchboard/1.2.3/bin/switchboard", "version 1.2.3\n", modified)
	link := filepath.Join(f.root, "opt", "bin", "switchboard")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(version, link); err != nil {
		t.Fatal(err)
	}
	if err := f.own.Make(link); err != nil {
		t.Fatal(err)
	}

	if got, err := f.own.Matches(link); err != nil || !got {
		t.Errorf("Matches() of the link = %v, %v; want true, judged by the binary it leads to", got, err)
	}
}

func TestMissing(t *testing.T) {
	f := newFixture(t)
	if !f.own.Missing() {
		t.Error("Missing() before the copy is made = false, want true")
	}
	if err := f.own.Make(f.binary(t, "1/switchboard", "version 1\n", modified)); err != nil {
		t.Fatal(err)
	}
	if f.own.Missing() {
		t.Error("Missing() once the copy is made = true, want false")
	}
	if err := os.Remove(f.own.Path()); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(f.root, "gone"), f.own.Path()); err != nil {
		t.Fatal(err)
	}
	if !f.own.Missing() {
		t.Error("Missing() of a link that leads nowhere = false, want true")
	}
}

func TestMatchesWithoutACopy(t *testing.T) {
	f := newFixture(t)
	binary := f.binary(t, "1/switchboard", "version 1\n", modified)

	if got, err := f.own.Matches(binary); err != nil || got {
		t.Errorf("Matches() = %v, %v; want false, as there's no copy", got, err)
	}
	if _, err := f.own.Matches(filepath.Join(f.root, "gone")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Matches() of no binary: error = %v, want %v", err, fs.ErrNotExist)
	}
}

func TestRemoveLeftovers(t *testing.T) {
	f := newFixture(t)
	if err := f.own.RemoveLeftovers(); err != nil {
		t.Errorf("RemoveLeftovers() with no directory: error = %v, want none", err)
	}
	binary := f.binary(t, "1/switchboard", "version 1\n", modified)
	if err := f.own.Make(binary); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := f.own.Stage(binary); err != nil {
			t.Fatal(err)
		}
	}

	if err := f.own.RemoveLeftovers(); err != nil {
		t.Fatalf("RemoveLeftovers() error = %v", err)
	}
	f.checkAlone(t)
	if data, err := os.ReadFile(f.own.Path()); err != nil || string(data) != "version 1\n" {
		t.Errorf("the copy holds %q (%v), want it kept", data, err)
	}
}

func TestRemove(t *testing.T) {
	f := newFixture(t)
	binary := f.binary(t, "1/switchboard", "version 1\n", modified)
	if err := f.own.Make(binary); err != nil {
		t.Fatal(err)
	}
	if _, err := f.own.Stage(binary); err != nil {
		t.Fatal(err)
	}

	if err := f.own.Remove(); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Stat(filepath.Dir(f.own.Path())); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the copy's directory: %v, want it gone", err)
	}
	if _, err := os.Stat(filepath.Dir(filepath.Dir(f.own.Path()))); err != nil {
		t.Errorf("switchboard's data directory: %v, want it kept", err)
	}
	if err := f.own.Remove(); err != nil {
		t.Errorf("Remove() again: error = %v, want none, as it's gone", err)
	}
}

// modified is when the binaries the tests copy were last modified, finer
// than a second, as the copy keeps it whole.
var modified = time.Date(2026, 9, 28, 13, 12, 0, 123456789, time.UTC)

// fixture is the router's copy, in a data directory under root, which holds
// the binaries copied too.
type fixture struct {
	root string
	own  routercopy.Copy
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	data := filepath.Join(root, "data")
	own, err := routercopy.Find(func(key string) string {
		if key == "XDG_DATA_HOME" {
			return data
		}
		return ""
	}, func() (string, error) { return "", errors.New("no home directory: the copy's under XDG_DATA_HOME") })
	if err != nil {
		t.Fatal(err)
	}
	return fixture{root: root, own: own}
}

// binary writes a binary holding content at name under root, last modified
// at at, and returns its path.
func (f fixture) binary(t *testing.T, name, content string, at time.Time) string {
	t.Helper()
	path := filepath.Join(f.root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Time{}, at); err != nil {
		t.Fatal(err)
	}
	return path
}

// checkAlone checks the copy's directory holds the copy alone, nothing
// staged left beside it.
func (f fixture) checkAlone(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(f.own.Path()))
	if err != nil || len(entries) != 1 || entries[0].Name() != "switchboard" {
		t.Errorf("the copy's directory holds %v (%v), want the copy alone", entries, err)
	}
}
