//go:build !darwin

package launch

// Process finds no process: only macOS's are read, so off it no launch is
// found within a session.
func (ProcessTable) Process(int) (Process, bool) {
	return Process{}, false
}

// Environ reads no process's environment: only macOS's are read.
func (ProcessTable) Environ(int) ([]string, bool) {
	return nil, false
}
