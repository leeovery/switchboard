//go:build unix

package theme

import (
	"errors"
	"os"
	"syscall"
)

// lockDir takes an exclusive lock on dir, waiting while another holds it, in
// this process or another, and returns what releases it. When it can't take
// the lock, it carries on without one.
func lockDir(dir string) (unlock func()) {
	d, err := os.Open(dir)
	if err != nil {
		return func() {}
	}
	for {
		err = syscall.Flock(int(d.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		_ = d.Close()
		return func() {}
	}
	return func() { _ = d.Close() }
}
