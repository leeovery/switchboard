//go:build !unix

package setup

// canWrite reports false: only a Unix system says whether the user can write
// to a file before they try, and only there does the claude link go.
func canWrite(string) bool {
	return false
}
