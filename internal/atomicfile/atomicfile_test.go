package atomicfile_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

func TestWrite(t *testing.T) {
	tests := []struct {
		name string
		// before is what the file holds before it's written, when it's there.
		before *string
		perm   fs.FileMode
	}{
		{name: "a new file", perm: 0o600},
		{name: "in place of a file", before: new("old"), perm: 0o600},
		{name: "in place of a file, with other permissions", before: new("old"), perm: 0o644},
		{name: "with permissions a umask would take away", perm: 0o666},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "file")
			if tt.before != nil {
				if err := os.WriteFile(path, []byte(*tt.before), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			if err := atomicfile.Write(path, []byte("new\n"), tt.perm); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != "new\n" {
				t.Errorf("the file holds %q (%v), want %q", data, err, "new\n")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode() != tt.perm {
				t.Errorf("the file's mode = %v, want %v", info.Mode(), tt.perm)
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
				t.Errorf("the directory holds %v (%v), want the file alone", entries, err)
			}
		})
	}
}

func TestTarget(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	at := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }
	token := at("dotfiles", "token")
	write(t, token)
	symlink(t, token, at("links", "token"))
	symlink(t, token, at("dotfiles", "current"))
	symlink(t, at("dotfiles", "current"), at("links", "chain"))
	// A relative link, reached through a link to its directory, where ".."
	// leads on from the directory it's really in.
	symlink(t, at("dotfiles", "linked"), at("state"))
	symlink(t, filepath.Join("..", "token"), at("dotfiles", "linked", "relative"))
	symlink(t, at("dotfiles", "missing"), at("links", "missing"))
	symlink(t, at("links", "loop-back"), at("links", "loop"))
	symlink(t, at("links", "loop"), at("links", "loop-back"))
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr string
	}{
		{name: "nothing there, itself", path: at("none"), want: at("none")},
		{name: "a file, itself", path: token, want: token},
		{name: "a link, where it leads", path: at("links", "token"), want: token},
		{name: "a link to a link, where the last leads", path: at("links", "chain"), want: token},
		{name: "a relative link, through a link to its directory, from where it really is", path: at("state", "relative"), want: token},
		{name: "a link to a file that isn't there yet, where it would be", path: at("links", "missing"), want: at("dotfiles", "missing")},
		{name: "a loop of links, nowhere", path: at("links", "loop"), wantErr: at("links", "loop") + ": too many links"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := atomicfile.Target(tt.path)
			if got != tt.want || errorText(err) != tt.wantErr {
				t.Errorf("Target() = %q, %v; want %q, and the error %q", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestWriteLeavesNothingBehindWhenItFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	// A directory, with something in it, where the file goes.
	if err := os.MkdirAll(filepath.Join(path, "kept"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := atomicfile.Write(path, []byte("new\n"), 0o600); err == nil {
		t.Fatal("Write() succeeded, want it to fail to replace a directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "file" || !entries[0].IsDir() {
		t.Errorf("the directory holds %v (%v), want what was there alone", entries, err)
	}
}

// write writes a file at path, making its directory.
func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// symlink makes a symbolic link at path leading to target, making its
// directory.
func symlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

// errorText is err's text, or "" for none.
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
