//go:build unix

package tokens_test

import (
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
