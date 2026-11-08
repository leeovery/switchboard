package shell

import (
	"os"
	"testing"
)

// SetWriteAt has RemoveLine write with write until the test ends, so a test
// can fail a write.
func SetWriteAt(t testing.TB, write func(f *os.File, b []byte, off int64) (int, error)) {
	t.Helper()
	was := writeAt
	writeAt = write
	t.Cleanup(func() { writeAt = was })
}

// SetTruncate has RemoveLine cut files with cut until the test ends, so a
// test can fail it.
func SetTruncate(t testing.TB, cut func(f *os.File, size int64) error) {
	t.Helper()
	was := truncate
	truncate = cut
	t.Cleanup(func() { truncate = was })
}
