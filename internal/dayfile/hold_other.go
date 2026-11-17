//go:build !unix

package dayfile

import "os"

// holdAcross holds nothing where there's no flock: a line another process
// appends as a day's file is rewritten may be lost.
func holdAcross(*os.File, bool) {}
