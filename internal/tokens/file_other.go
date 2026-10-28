//go:build !unix

package tokens

import (
	"io/fs"
	"os"
)

// openFlags open a token file for reading.
const openFlags = os.O_RDONLY

// tightenFlags open a record to make it private.
const tightenFlags = openFlags

// owner reports false: only a Unix system says which user owns a file, so no
// token file counts as the user's.
func owner(fs.FileInfo) (int, bool) {
	return 0, false
}

// links reports false: only a Unix system says how many names a file has.
func links(fs.FileInfo) (uint64, bool) {
	return 0, false
}

// lockDir takes no lock where there's no flock: two writes of one account's
// token at once may lose one's date.
func lockDir(string) (unlock func()) {
	return func() {}
}
