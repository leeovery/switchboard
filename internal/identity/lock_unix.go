//go:build unix

package identity

import (
	"errors"
	"os"
	"syscall"
)

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
