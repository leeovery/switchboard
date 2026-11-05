//go:build !darwin

package export

import (
	"io/fs"
	"time"
)

// changed is when the file info describes was last modified, which stands
// for when it last changed where only macOS's times are read.
func changed(info fs.FileInfo) time.Time {
	return info.ModTime()
}
