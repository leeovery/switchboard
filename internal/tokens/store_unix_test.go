//go:build unix

package tokens_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/tokens"
)

func TestReadRefusesAPipeWithoutWaitingOnIt(t *testing.T) {
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	mkdirPrivate(t, filepath.Dir(store.Path("work")))
	if err := syscall.Mkfifo(store.Path("work"), 0o600); err != nil {
		t.Fatal(err)
	}

	read := make(chan error, 1)
	go func() {
		_, err := store.Read("work")
		read <- err
	}()
	select {
	case err := <-read:
		if want := "the token file " + store.Path("work") + " isn't a file: replace it with one holding the token"; err == nil || err.Error() != want {
			t.Errorf("Read() error = %v, want %q", err, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Read() is waiting for something to write to the pipe")
	}
}

func TestWriteAndRemoveWaitForTheTokensDirectorysLock(t *testing.T) {
	token := parse(t, workToken)
	tests := []struct {
		name string
		// act is what's done while another holds the lock, and done
		// whether it's been done.
		act  func(store tokens.Store) error
		done func(store tokens.Store) bool
	}{
		{
			name: "a write, its token",
			act:  func(store tokens.Store) error { return store.Write("work", token, taken) },
			done: func(store tokens.Store) bool {
				data, err := os.ReadFile(store.Path("work"))
				return err == nil && string(data) != "test-token-stale\n"
			},
		},
		{
			name: "a removal",
			act: func(store tokens.Store) error {
				_, err := store.Remove("work")
				return err
			},
			done: func(store tokens.Store) bool {
				_, err := os.Lstat(store.Path("work"))
				return errors.Is(err, fs.ErrNotExist)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := tokens.NewStore(t.TempDir(), os.Getuid())
			writeFile(t, store.Path("work"), "test-token-stale\n", 0o600)
			dir, err := os.Open(store.Dir())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = dir.Close() }()
			if err := syscall.Flock(int(dir.Fd()), syscall.LOCK_EX); err != nil {
				t.Fatal(err)
			}

			acted := make(chan error, 1)
			go func() { acted <- tt.act(store) }()
			time.Sleep(100 * time.Millisecond)
			if tt.done(store) {
				t.Error("done while another holds the lock, want it waiting")
			}
			if err := syscall.Flock(int(dir.Fd()), syscall.LOCK_UN); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-acted:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("still waiting once the lock's free")
			}
			if !tt.done(store) {
				t.Error("not done once the lock's free")
			}
		})
	}
}
