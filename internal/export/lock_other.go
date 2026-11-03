//go:build !unix

package export

import "os"

// tryLock takes no lock where there's no flock, reporting it held: a second
// import is still refused while the router says one runs.
func tryLock(*os.File) (bool, error) {
	return true, nil
}
