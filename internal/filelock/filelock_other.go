//go:build !unix

package filelock

import "os"

// Lock takes no lock where there's no flock.
func Lock(*os.File) error {
	return nil
}

// LockShared takes no lock where there's no flock.
func LockShared(*os.File) error {
	return nil
}

// TryLock takes no lock where there's no flock, reporting it taken.
func TryLock(*os.File) (bool, error) {
	return true, nil
}
