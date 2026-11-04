//go:build unix

package routercopy

import (
	"errors"
	"os"
	"syscall"
)

// lock takes an exclusive lock on f, waiting while another holds one, and
// trying again when a signal cuts the wait short.
func lock(f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

// tryLock takes an exclusive lock on f where no one else holds one,
// reporting false, without waiting, where another does, and trying again
// when a signal cuts it short.
func tryLock(f *os.File) (bool, error) {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, syscall.EWOULDBLOCK):
			return false, nil
		case !errors.Is(err, syscall.EINTR):
			return false, err
		}
	}
}
