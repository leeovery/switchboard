//go:build !unix

package theme

// lockDir takes no lock where there's no flock: dashboards keeping their
// preferences at once may lose one's change.
func lockDir(string) (unlock func()) {
	return func() {}
}
