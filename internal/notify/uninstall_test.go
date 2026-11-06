package notify

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// leftover is a folder a write cut short left beside the app.
const leftover = ".Switchboard.app.new-123"

func TestWhatWritesCutShortLeftIsSweptAwayUnderTheLock(t *testing.T) {
	tests := []struct {
		name string
		// current is whether the app there is the one embedded, which isn't
		// written again.
		current bool
	}{
		{name: "the app written afresh"},
		{name: "the app current", current: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				zipped := zipOf(t, appEntries("new")...)
				s := newAppSetup(t, zipped)
				if tt.current {
					put(t, zipped, s.path)
				}
				write(t, filepath.Join(filepath.Dir(s.path), leftover, appName, executable))
				release := holdLock(t, s.path)
				posted := make(chan error, 1)
				go func() { posted <- s.desktop.Notify(t.Context(), message) }()
				synctest.Wait()

				if !slices.Contains(s.beside(t), leftover) {
					t.Errorf("beside the app are %q while another writer held the lock, want %s left", s.beside(t), leftover)
				}
				release()
				if err := <-posted; err != nil {
					t.Fatalf("Notify() error = %v", err)
				}

				if got, want := s.beside(t), []string{lockName, sumName, appName}; !slices.Equal(got, want) {
					t.Errorf("beside the app are %q, want %q alone", got, want)
				}
			})
		})
	}
}

func TestSweepingTakesTheAppsDirectoryAsItIs(t *testing.T) {
	for _, name := range []string{"data[", "da*ta", "da?ta"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir, sibling := filepath.Join(root, name), filepath.Join(root, "daXta")
			for _, d := range []string{dir, sibling} {
				write(t, filepath.Join(d, leftover, appName, executable))
			}
			write(t, filepath.Join(dir, "kept"))

			if err := sweep(dir); err != nil {
				t.Fatalf("sweep(%q) error = %v", dir, err)
			}

			if got := names(t, dir); !slices.Equal(got, []string{"kept"}) {
				t.Errorf("sweep(%q) left %q, want kept alone", dir, got)
			}
			if got := names(t, sibling); !slices.Equal(got, []string{leftover}) {
				t.Errorf("sweep(%q) left %q in %s, want it untouched", dir, got, sibling)
			}
		})
	}
}

func TestUninstallUnregistersTheAppAndRemovesItWithAllBesideIt(t *testing.T) {
	tests := []struct {
		name string
		// zipped is the app the build embeds, which removing it needs none of.
		zipped bool
	}{
		{name: "a build with the app", zipped: true},
		{name: "a build without it"},
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
			write(t, filepath.Join(filepath.Dir(s.path), leftover, appName, executable))
			holdLock(t, s.path)()

			if err := s.desktop.Uninstall(t.Context()); err != nil {
				t.Fatalf("Uninstall() error = %v", err)
			}

			if got, want := s.programs.ran(), [][]string{{lsregister, "-u", s.path}}; !slices.EqualFunc(got, want, slices.Equal) {
				t.Errorf("ran %q, want %q", got, want)
			}
			if got := s.beside(t); len(got) > 0 {
				t.Errorf("beside the app are %q, want the app, its sum, its lock and what a write cut short left gone", got)
			}
			if log.Has("level=WARN") {
				t.Errorf("log reads\n%s\nwant no warning", log)
			}
		})
	}
}

func TestUninstallSaysWhereTheAppCantBeUnregisteredAndRemovesItAllTheSame(t *testing.T) {
	log := logstest.Capture(t)
	s := newAppSetup(t, zipOf(t, appEntries("new")...))
	put(t, zipOf(t, appEntries("new")...), s.path)
	s.programs.fail("lsregister", exitStatus(1))

	err := s.desktop.Uninstall(t.Context())

	if !errors.Is(err, exitStatus(1)) || !strings.Contains(err.Error(), "unregister "+s.path+" from Launch Services") {
		t.Errorf("Uninstall() error = %v, want it saying the app couldn't be unregistered, wrapping %v", err, exitStatus(1))
	}
	if got := s.beside(t); len(got) > 0 {
		t.Errorf("beside the app are %q, want the app, its sum and its lock removed all the same", got)
	}
	if log.Has("level=WARN") {
		t.Errorf("log reads\n%s\nwant no warning: the error says it", log)
	}
}

func TestUninstallWithoutTheAppRemovesWhatsBesideIt(t *testing.T) {
	s := newAppSetup(t, zipOf(t, appEntries("new")...))
	write(t, sumPath(s.path))
	write(t, lockPath(s.path))
	write(t, filepath.Join(filepath.Dir(s.path), leftover, appName, executable))

	if err := s.desktop.Uninstall(t.Context()); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	if ran := s.programs.ran(); len(ran) > 0 {
		t.Errorf("ran %q, want nothing unregistered without an app", ran)
	}
	if got := s.beside(t); len(got) > 0 {
		t.Errorf("beside the app are %q, want its sum, its lock and what a write cut short left gone", got)
	}
}

func TestUninstallWithoutTheAppsDirectoryDoesNothing(t *testing.T) {
	s := newAppSetup(t, zipOf(t, appEntries("new")...))

	if err := s.desktop.Uninstall(t.Context()); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	if ran := s.programs.ran(); len(ran) > 0 {
		t.Errorf("ran %q, want nothing run", ran)
	}
	if _, err := os.Stat(filepath.Dir(s.path)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the app's directory is there (%v), want none made", err)
	}
}

func TestUninstallWaitsForAWriterHoldingTheLock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newAppSetup(t, zipOf(t, appEntries("new")...))
		put(t, zipOf(t, appEntries("new")...), s.path)
		release := holdLock(t, s.path)
		removed := make(chan error, 1)
		go func() { removed <- s.desktop.Uninstall(t.Context()) }()
		synctest.Wait()

		if got, want := s.beside(t), []string{lockName, sumName, appName}; !slices.Equal(got, want) {
			t.Errorf("beside the app are %q while a writer held the lock, want %q left", got, want)
		}
		release()
		if err := <-removed; err != nil {
			t.Fatalf("Uninstall() error = %v", err)
		}

		if got := s.beside(t); len(got) > 0 {
			t.Errorf("beside the app are %q, want all of it removed once the lock was let go", got)
		}
	})
}

func TestUninstallRemovesNothingWhereTheLockIsntHad(t *testing.T) {
	s := newAppSetup(t, zipOf(t, appEntries("new")...))
	put(t, zipOf(t, appEntries("new")...), s.path)
	s.desktop.lockWait = 2 * lockPoll
	defer holdLock(t, s.path)()

	err := s.desktop.Uninstall(t.Context())

	if err == nil || !strings.Contains(err.Error(), "another switchboard held the notifications' app's lock") {
		t.Errorf("Uninstall() error = %v, want it saying the lock wasn't had", err)
	}
	if ran := s.programs.ran(); len(ran) > 0 {
		t.Errorf("ran %q, want nothing unregistered without the lock", ran)
	}
	if got, want := s.beside(t), []string{lockName, sumName, appName}; !slices.Equal(got, want) {
		t.Errorf("beside the app are %q, want %q left", got, want)
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

// names returns the names in dir, none where it isn't there.
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	return got
}
