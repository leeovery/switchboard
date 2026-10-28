package atomicfile_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
	// A relative link whose text goes through a linked directory and back
	// out of it, which the system takes back from where that link leads.
	write(t, at("elsewhere", "kept"))
	write(t, at("elsewhere", "deep", "placeholder"))
	symlink(t, at("elsewhere", "deep"), at("links", "sub"))
	symlink(t, "sub/../kept", at("links", "through"))
	// A relative link, in a linked directory, whose ".." leads on to a
	// directory that isn't there yet.
	write(t, at("real", "cfgdir", "placeholder"))
	symlink(t, at("real", "cfgdir"), at("config"))
	symlink(t, "../sb/config.toml", at("config", "config.toml"))
	// Links that go back, by "..", out of a directory that isn't there,
	// which the system can't follow: relative, and not.
	sep := string(filepath.Separator)
	symlink(t, "gone"+sep+".."+sep+"token", at("links", "back"))
	symlink(t, at("dotfiles", "gone")+sep+".."+sep+"token", at("links", "back-whole"))
	// Links whose whole text goes back, by "..", out of a linked directory.
	symlink(t, at("state")+sep+".."+sep+"token", at("links", "whole-through"))
	symlink(t, at("state")+sep+".."+sep+"absent", at("links", "whole-absent"))
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
		{name: "a relative link back out of a linked directory, from where that leads", path: at("links", "through"), want: at("elsewhere", "kept")},
		{name: "a link to a file that isn't there yet, where it would be", path: at("links", "missing"), want: at("dotfiles", "missing")},
		{name: "a relative link into a directory that isn't there yet, from where its link leads", path: at("config", "config.toml"), want: at("real", "sb", "config.toml")},
		{name: "a link back out of a linked directory, from where that leads", path: at("links", "whole-through"), want: token},
		{name: "a link back out of a linked directory to a file not there yet, from where that leads", path: at("links", "whole-absent"), want: at("dotfiles", "absent")},
		{name: "a relative link back out of a directory that isn't there, nowhere", path: at("links", "back"), wantErr: at("links", "back") + ": leads back out of " + at("links", "gone") + ", which isn't there"},
		{name: "a link back out of a directory that isn't there, nowhere", path: at("links", "back-whole"), wantErr: at("links", "back-whole") + ": leads back out of " + at("dotfiles", "gone") + ", which isn't there"},
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

func TestWriteFrom(t *testing.T) {
	failed := errors.New("the source failed")
	tests := []struct {
		name  string
		write func(w io.Writer) error
		// want is what the file holds after, and wantErr what WriteFrom
		// fails with, if it does.
		want    string
		wantErr error
	}{
		{
			name: "what's written, in several writes",
			write: func(w io.Writer) error {
				for _, part := range []string{"new", "\n", strings.Repeat("x", 100<<10)} {
					if _, err := io.WriteString(w, part); err != nil {
						return err
					}
				}
				return nil
			},
			want: "new\n" + strings.Repeat("x", 100<<10),
		},
		{
			name: "nothing, where writing fails partway",
			write: func(w io.Writer) error {
				if _, err := io.WriteString(w, strings.Repeat("x", 100<<10)); err != nil {
					return err
				}
				return failed
			},
			want:    "old\n",
			wantErr: failed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "file")
			write(t, path)

			if err := atomicfile.WriteFrom(path, 0o600, tt.write); !errors.Is(err, tt.wantErr) {
				t.Fatalf("WriteFrom() error = %v, want %v", err, tt.wantErr)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != tt.want {
				t.Errorf("the file holds %d bytes (%v), want %d", len(data), err, len(tt.want))
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
				t.Errorf("the directory holds %v (%v), want the file alone", entries, err)
			}
		})
	}
}

func TestWriteChecked(t *testing.T) {
	refused := errors.New("the file changed")
	for _, checkErr := range []error{nil, refused} {
		t.Run(errorText(checkErr), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "file")
			write(t, path)
			checked := false
			check := func() error {
				checked = true
				// Checked once the new file is written whole beside the old,
				// the old still in place.
				if data, err := os.ReadFile(path); err != nil || string(data) != "old\n" {
					t.Errorf("at the check, the file holds %q (%v), want the old", data, err)
				}
				written, err := filepath.Glob(filepath.Join(dir, ".file.*"))
				if err != nil || len(written) != 1 {
					t.Fatalf("at the check, beside the file: %v (%v), want the new one", written, err)
				}
				if data, err := os.ReadFile(written[0]); err != nil || string(data) != "new\n" {
					t.Errorf("at the check, the new file holds %q (%v), want it whole", data, err)
				}
				return checkErr
			}

			err := atomicfile.WriteChecked(path, 0o600, func(w io.Writer) error {
				_, err := io.WriteString(w, "new\n")
				return err
			}, check)
			if !errors.Is(err, checkErr) || !checked {
				t.Fatalf("WriteChecked() error = %v, checked %v; want %v, checked", err, checked, checkErr)
			}
			want := "new\n"
			if checkErr != nil {
				want = "old\n"
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != want {
				t.Errorf("the file holds %q (%v), want %q", data, err, want)
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
				t.Errorf("the directory holds %v (%v), want the file alone", entries, err)
			}
		})
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
