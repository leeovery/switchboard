//go:build !darwin

package launch

// FindProcess finds no process: only macOS's are read, so off it no launch
// is found within a session.
func FindProcess(int) (Process, bool) {
	return Process{}, false
}
