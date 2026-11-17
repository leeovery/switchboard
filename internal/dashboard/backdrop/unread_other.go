//go:build !darwin

package backdrop

// dropTyped throws away what the terminal fd holds of what was typed at it,
// unread, which only macOS does here.
func dropTyped(uintptr) error {
	return nil
}
