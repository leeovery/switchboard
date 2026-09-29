//go:build !unix

package service

import "io/fs"

// owner reports false: only a Unix system says which user owns a file.
func owner(fs.FileInfo) (int, bool) {
	return 0, false
}
