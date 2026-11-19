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

// tightenFlags open a record to make it private: never through a link, and
// without waiting, as openFlags does.
const tightenFlags = openFlags | syscall.O_NOFOLLOW

// owner returns the id of the user who owns the file info describes.
func owner(info fs.FileInfo) (int, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}

// links returns how many names the file info describes has.
func links(info fs.FileInfo) (uint64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Nlink), true
}
