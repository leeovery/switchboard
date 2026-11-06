//go:build !unix

package notify

import "os"

// tryLock takes no lock where there's no flock, reporting it taken: the app
// it guards is macOS's alone.
func tryLock(*os.File) (bool, error) {
	return true, nil
}
