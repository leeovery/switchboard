package setup_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/setup"
)

func TestRemovingTheClaudeLink(t *testing.T) {
	tests := []struct {
		name string
		// lay puts what's named claude in bin, where there's anything.
		lay  func(t *testing.T, bin string)
		want setup.LinkRemoval
		// wantClaude is whether what's named claude is left, and wantBin
		// whether the bin directory is.
		wantClaude, wantBin bool
	}{
		{name: "no bin directory", lay: func(*testing.T, string) {}, want: setup.NoLink},
		{name: "an empty bin directory: removed", lay: func(t *testing.T, bin string) { mkdir(t, bin) }, want: setup.NoLink},
		{
			name: "switchboard's link: removed, with the directory",
			lay: func(t *testing.T, bin string) {
				symlink(t, "/opt/homebrew/bin/switchboard", filepath.Join(bin, "claude"))
			},
			want: setup.LinkRemoved,
		},
		{
			name: "switchboard's link, to a switchboard no longer there: removed",
			lay: func(t *testing.T, bin string) {
				symlink(t, "/Users/test/go/bin/switchboard", filepath.Join(bin, "claude"))
			},
			want: setup.LinkRemoved,
		},
		{
			name: "switchboard's link, beside a file of someone else's: the directory kept for it",
			lay: func(t *testing.T, bin string) {
				symlink(t, "/opt/homebrew/bin/switchboard", filepath.Join(bin, "claude"))
				writeFile(t, filepath.Join(bin, "notes"), "", 0o644)
			},
			want:    setup.LinkRemoved,
			wantBin: true,
		},
		{
			name: "a link to another claude: left alone",
			lay: func(t *testing.T, bin string) {
				symlink(t, "/Users/test/.local/bin/claude", filepath.Join(bin, "claude"))
			},
			want:       setup.LinkNotOurs,
			wantClaude: true, wantBin: true,
		},
		{
			name:       "a file: left alone",
			lay:        func(t *testing.T, bin string) { writeFile(t, filepath.Join(bin, "claude"), "", 0o755) },
			want:       setup.LinkNotOurs,
			wantClaude: true, wantBin: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), "switchboard", "bin")
			tt.lay(t, bin)

			got, err := setup.RemoveLink(bin)
			if err != nil || got != tt.want {
				t.Fatalf("RemoveLink() = %v, %v, want %v", got, err, tt.want)
			}
			if _, err := os.Lstat(filepath.Join(bin, "claude")); (err == nil) != tt.wantClaude {
				t.Errorf("what's named claude: %v, want it there: %v", err, tt.wantClaude)
			}
			if _, err := os.Lstat(bin); (err == nil) != tt.wantBin {
				t.Errorf("the bin directory: %v, want it there: %v", err, tt.wantBin)
			}
		})
	}
}

func TestRemovingTheClaudeLinkFailsSayingWhy(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can remove from any directory")
	}
	bin := filepath.Join(t.TempDir(), "bin")
	symlink(t, "/opt/homebrew/bin/switchboard", filepath.Join(bin, "claude"))
	if err := os.Chmod(bin, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(bin, 0o700) })

	if got, err := setup.RemoveLink(bin); err == nil || !errors.Is(err, fs.ErrPermission) {
		t.Errorf("RemoveLink() = %v, %v, want it to fail, for want of permission", got, err)
	}
}

func symlink(t *testing.T, target, path string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}
