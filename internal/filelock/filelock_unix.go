//go:build unix

package filelock

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// TakeDir takes an exclusive lock on the directory dir, waiting while
// another holds one, and returns what lets it go. Where it can't take one,
// it carries on without, its unlock letting nothing go.
func TakeDir(dir string) (unlock func()) {
	d, err := os.Open(dir)
	if err != nil {
		return func() {}
	}
	if err := Lock(d); err != nil {
		_ = d.Close()
		return func() {}
	}
	return closer(d)
}

// Lock takes an exclusive lock on f, waiting while another holds one.
func Lock(f *os.File) error {
	return flock(f, syscall.LOCK_EX)
}

// LockShared takes a shared lock on f, waiting while another holds an
// exclusive one.
func LockShared(f *os.File) error {
	return flock(f, syscall.LOCK_SH)
}

// TryLock takes an exclusive lock on f where no one else holds one,
// reporting false, without waiting, where another does.
func TryLock(f *os.File) (bool, error) {
	err := flock(f, syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

// flock takes the lock how asks for on f, as retried takes it.
func flock(f *os.File, how int) error {
	if err := retried(syscall.Flock, int(f.Fd()), how); err != nil {
		return &fs.PathError{Op: "flock", Path: f.Name(), Err: err}
	}
	return nil
}

// retried takes the lock how asks for on fd through lock, trying again
// while a signal cuts it short.
func retried(lock func(fd, how int) error, fd, how int) error {
	for {
		if err := lock(fd, how); !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}
