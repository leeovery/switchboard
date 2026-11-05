//go:build darwin

package export

import (
	"io/fs"
	"syscall"
	"time"
)

// changed is when the file info describes last changed: its ctime, which
// any change to the file, setting its times included, moves on to now, and
// nothing can set back.
func changed(info fs.FileInfo) time.Time {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.ModTime()
	}
	return time.Unix(st.Ctimespec.Unix())
}
