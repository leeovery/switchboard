//go:build darwin

package export

import (
	"io/fs"
	"syscall"
	"time"
)

// changed is when the file info describes last changed: the later of its
// ctime and its birthtime, as setting its modified time sets one of them
// back with it, by the volume it's on. On APFS that's its birthtime, while
// its ctime moves on to now; on FAT and exFAT, macOS gives the modified time
// as the ctime, while the birthtime stays when the file was made.
func changed(info fs.FileInfo) time.Time {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.ModTime()
	}
	ctime, born := time.Unix(st.Ctimespec.Unix()), time.Unix(st.Birthtimespec.Unix())
	if born.After(ctime) {
		return born
	}
	return ctime
}
