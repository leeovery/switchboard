//go:build !unix

package logs

// lockDir takes no lock where there's no flock: processes rolling a log over
// at once may lose a rolled-over file.
func lockDir(string) (unlock func()) {
	return func() {}
}
