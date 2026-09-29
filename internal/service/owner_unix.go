//go:build unix

package service

import (
	"io/fs"
	"syscall"
)

// owner returns the id of the user who owns the file info describes.
func owner(info fs.FileInfo) (int, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}
