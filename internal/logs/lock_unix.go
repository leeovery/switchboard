//go:build unix

package logs

import (
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
	if err := syscall.Flock(int(d.Fd()), syscall.LOCK_EX); err != nil {
		_ = d.Close()
		return func() {}
	}
	return func() { _ = d.Close() }
}
