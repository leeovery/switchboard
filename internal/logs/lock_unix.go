//go:build unix

package logs

import (
	"errors"
	"os"
	"syscall"
)

// lockDir takes an exclusive lock on dir, waiting while another process
// holds it, and returns what releases it. When it can't take the lock, it
// carries on without one.
func lockDir(dir string) (unlock func()) {
	d, err := os.Open(dir)
	if err != nil {
		return func() {}
	}
	if err := flock(d); err != nil {
		_ = d.Close()
		return func() {}
	}
	return func() { _ = d.Close() }
}

// flock takes an exclusive lock on f, waiting while another holds it, and
// trying again when a signal cuts the wait short.
func flock(f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}
