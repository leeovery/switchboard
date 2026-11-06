package notify

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/logs/logstest"
)

func TestWhatWritesCutShortLeftIsSweptAwayUnderTheLock(t *testing.T) {
	zipped := zipOf(t, appEntries("new")...)
	s := newAppSetup(t, zipped)
	for _, current := range []bool{false, true} {
		if current {
			put(t, zipped, s.path)
		}
		left := filepath.Join(filepath.Dir(s.path), ".Switchboard.app.new-123", appName)
		write(t, filepath.Join(left, executable))
		s.desktop.app = newApp(zipped)

		if err := s.desktop.Notify(message); err != nil {
			t.Fatalf("Notify() error = %v", err)
		}

		if got, want := s.beside(t), []string{lockName, sumName, appName}; !slices.Equal(got, want) {
			t.Errorf("the app current %v: beside it are %q, want %q alone", current, got, want)
		}
	}
}

func TestUninstallUnregistersTheAppAndRemovesIt(t *testing.T) {
	tests := []struct {
		name string
		// fails is how unregistering it fails, or nil.
		fails error
		// zipped is the app the build embeds, which removing it needs none of.
		zipped bool
	}{
		{name: "a build with the app", zipped: true},
		{name: "a build without it"},
		{name: "unregistering failing", zipped: true, fails: exitStatus(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			var zipped []byte
			if tt.zipped {
				zipped = zipOf(t, appEntries("new")...)
			}
			s := newAppSetup(t, zipped)
			put(t, zipOf(t, appEntries("new")...), s.path)
			holdLock(t, s.path)()
			s.programs.fail("lsregister", tt.fails)

			if err := s.desktop.Uninstall(); err != nil {
				t.Fatalf("Uninstall() error = %v", err)
			}

			if got, want := s.programs.ran(), [][]string{{lsregister, "-u", s.path}}; !slices.EqualFunc(got, want, slices.Equal) {
				t.Errorf("ran %q, want %q", got, want)
			}
			if got := s.beside(t); len(got) > 0 {
				t.Errorf("beside the app are %q, want the app, its sum and its lock gone", got)
			}
			const cantUnregister = `msg="can't unregister the notifications' app from Launch Services; removing it all the same"`
			if warned := log.Has("level=WARN", cantUnregister, "error=\"exit status 1\""); warned != (tt.fails != nil) {
				t.Errorf("log reads\n%s\nwant a warning that it couldn't be unregistered: %v", log, tt.fails != nil)
			}
		})
	}
}

func TestUninstallWithoutAnAppDoesNothing(t *testing.T) {
	s := newAppSetup(t, zipOf(t, appEntries("new")...))
	write(t, sumPath(s.path))
	write(t, lockPath(s.path))

	if err := s.desktop.Uninstall(); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	if ran := s.programs.ran(); len(ran) > 0 {
		t.Errorf("ran %q, want nothing run without an app", ran)
	}
	if got, want := s.beside(t), []string{lockName, sumName}; !slices.Equal(got, want) {
		t.Errorf("beside the app are %q, want %q left alone", got, want)
	}
	if _, err := os.Stat(s.path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the app is there (%v), want none", err)
	}
}

// write writes an empty file at path, making its directory.
func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o755); err != nil {
		t.Fatal(err)
	}
}
