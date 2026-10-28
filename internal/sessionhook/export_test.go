package sessionhook

import "testing"

// SetReadFile has Install read files with read until the test ends, so a
// test can change the settings file between its reads.
func SetReadFile(t testing.TB, read func(string) ([]byte, error)) {
	t.Helper()
	was := readFile
	readFile = read
	t.Cleanup(func() { readFile = was })
}
