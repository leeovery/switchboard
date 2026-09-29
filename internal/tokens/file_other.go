//go:build !unix

package tokens

import (
	"io/fs"
	"os"
)

// openFlags open a token file for reading.
const openFlags = os.O_RDONLY

// owner reports false: only a Unix system says which user owns a file, so no
// token file counts as the user's.
func owner(fs.FileInfo) (int, bool) {
	return 0, false
}
