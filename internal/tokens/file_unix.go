//go:build unix

package tokens

import (
	"io/fs"
	"os"
	"syscall"
)

// openFlags open a token file for reading without waiting, as opening a pipe
// would until something writes to it: what's opened is checked before it's
// read, and anything but a file is refused.
const openFlags = os.O_RDONLY | syscall.O_NONBLOCK

// owner returns the id of the user who owns the file info describes.
func owner(info fs.FileInfo) (int, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}
