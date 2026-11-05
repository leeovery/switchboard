//go:build !darwin

package router

// locked reports whether the file or folder at path is locked, which only
// macOS says of a file here.
func locked(string) bool {
	return false
}
