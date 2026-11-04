//go:build !unix

package routercopy

import "os"

// lock takes no lock where there's no flock: the service it guards is
// macOS's alone.
func lock(*os.File) error {
	return nil
}

// tryLock takes no lock where there's no flock, reporting it taken.
func tryLock(*os.File) (bool, error) {
	return true, nil
}
